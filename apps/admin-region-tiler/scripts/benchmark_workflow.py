#!/usr/bin/env python3
"""Measure real API -> worker -> published artifact in isolated loopback fixtures.

No timing thresholds, external tile requests, user databases or runtime installs.
Use --ref OLD --working-tree to verify changes before committing them.
"""
import argparse
import contextlib
import ctypes
import hashlib
import http.server
import io
import json
import os
import pathlib
import random
import shutil
import signal
import socket
import sqlite3
import struct
import subprocess
import sys
import tarfile
import tempfile
import threading
import time
import urllib.request
import zipfile
import zlib

ROOT = pathlib.Path(__file__).resolve().parents[3]
APP = pathlib.Path("apps/admin-region-tiler")


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT).decode().strip()


def png_fixture():
    rng = random.Random(20261106)
    pixels = rng.randbytes(256 * 256 * 3)
    scanlines = b"".join(b"\0" + pixels[y*768:(y+1)*768] for y in range(256))
    def chunk(kind, data):
        return struct.pack("!I", len(data)) + kind + data + struct.pack("!I", zlib.crc32(kind + data))
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack("!IIBBBBB", 256, 256, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(scanlines)) + chunk(b"IEND", b"")


class ProcessMemory:
    """Read OS counters only for the owned server and its descendants."""
    def __init__(self, pid):
        self.pid, self.peaks, self.peak_sum = pid, {}, 0
        if os.name == "nt":
            from ctypes import wintypes as w
            size = ctypes.c_size_t
            class Entry(ctypes.Structure):
                _fields_ = [("size",w.DWORD),("usage",w.DWORD),("pid",w.DWORD),("heap",size),
                            ("module",w.DWORD),("threads",w.DWORD),("parent",w.DWORD),
                            ("priority",w.LONG),("flags",w.DWORD),("name",w.WCHAR*260)]
            class Counters(ctypes.Structure):
                _fields_ = [("size",w.DWORD),("faults",w.DWORD)] + [(name,size) for name in
                    ["peak","rss","quota_peak_paged","quota_paged","quota_peak_nonpaged","quota_nonpaged","pagefile","peak_pagefile","private"]]
            self.Entry, self.Counters = Entry, Counters
            self.kernel = ctypes.WinDLL("kernel32", use_last_error=True)
            self.psapi = ctypes.WinDLL("psapi", use_last_error=True)
            self.kernel.CreateToolhelp32Snapshot.argtypes = [w.DWORD,w.DWORD]
            self.kernel.CreateToolhelp32Snapshot.restype = w.HANDLE
            self.kernel.Process32FirstW.argtypes = self.kernel.Process32NextW.argtypes = [w.HANDLE,ctypes.POINTER(Entry)]
            self.kernel.OpenProcess.argtypes = [w.DWORD,w.BOOL,w.DWORD]
            self.kernel.OpenProcess.restype = w.HANDLE
            self.kernel.CloseHandle.argtypes = [w.HANDLE]
            self.psapi.GetProcessMemoryInfo.argtypes = [w.HANDLE,ctypes.POINTER(Counters),w.DWORD]

    def sample(self):
        parents, counters = {}, {}
        if os.name == "nt":
            handle = self.kernel.CreateToolhelp32Snapshot(2, 0)
            if handle == ctypes.c_void_p(-1).value:
                raise OSError("cannot enumerate benchmark child processes")
            try:
                entry = self.Entry(); entry.size = ctypes.sizeof(entry)
                more = self.kernel.Process32FirstW(handle, ctypes.byref(entry))
                while more:
                    parents[entry.pid] = entry.parent
                    more = self.kernel.Process32NextW(handle, ctypes.byref(entry))
            finally:
                self.kernel.CloseHandle(handle)
        else:
            for path in pathlib.Path("/proc").iterdir():
                if path.name.isdigit():
                    try:
                        fields = (path/"stat").read_text().rsplit(")",1)[1].split()
                        parents[int(path.name)] = int(fields[1])
                    except (OSError,ValueError,IndexError):
                        pass
        owned = {self.pid}
        while True:
            added = {pid for pid,parent in parents.items() if parent in owned} - owned
            if not added: break
            owned.update(added)
        for pid in owned:
            if os.name == "nt":
                handle = self.kernel.OpenProcess(0x410, False, pid)
                if not handle: continue
                try:
                    value = self.Counters(); value.size = ctypes.sizeof(value)
                    if self.psapi.GetProcessMemoryInfo(handle,ctypes.byref(value),value.size):
                        counters[pid] = (int(value.rss),int(value.peak))
                finally:
                    self.kernel.CloseHandle(handle)
            else:
                try:
                    fields = dict(line.split(":",1) for line in pathlib.Path(f"/proc/{pid}/status").read_text().splitlines() if ":" in line)
                    counters[pid] = tuple(int(fields[key].split()[0])*1024 for key in ["VmRSS","VmHWM"])
                except (OSError,KeyError,ValueError):
                    continue
        for pid, (_,peak) in counters.items(): self.peaks[pid] = max(self.peaks.get(pid,0),peak)
        self.peak_sum = max(self.peak_sum, sum(value[0] for value in counters.values()))
        return counters


def snapshot(destination, ref):
    sha = git("rev-parse", "HEAD" if ref == "WORKTREE" else ref + "^{commit}")
    archive = destination.parent / "source.tar"
    subprocess.run(["git","archive","--format=tar","--output",str(archive),sha],cwd=ROOT,check=True)
    with tarfile.open(archive) as source: source.extractall(destination,filter="data")
    if ref == "WORKTREE":
        names = set(git("diff","--name-only","HEAD").splitlines())
        names.update(git("ls-files","--others","--exclude-standard").splitlines())
        for name in names:
            if not name: continue
            source, target = ROOT/name, destination/name
            if source.is_file():
                target.parent.mkdir(parents=True,exist_ok=True); shutil.copyfile(source,target)
            elif target.is_file(): target.unlink()
    return sha


def measure(binary, format_name, png):
    counts = {"requests":0,"connections":0}
    counts_lock = threading.Lock()
    class Handler(http.server.BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"
        def log_message(self,*args): pass
        def handle(self):
            try: super().handle()
            except (ConnectionResetError, BrokenPipeError):
                # Completed workers close their keep-alive sockets at exit.
                # Artifact/request assertions still detect incomplete transfers.
                self.close_connection = True
        def setup(self):
            super().setup()
            with counts_lock: counts["connections"] += 1
        def do_GET(self):
            with counts_lock: counts["requests"] += 1
            self.send_response(200); self.send_header("Content-Type","image/png")
            self.send_header("Content-Length",str(len(png))); self.end_headers(); self.wfile.write(png)
    upstream = http.server.ThreadingHTTPServer(("127.0.0.1",0),Handler)
    serving = threading.Thread(target=upstream.serve_forever,daemon=True); serving.start()
    try:
        with tempfile.TemporaryDirectory(prefix="tiler-workflow-run-") as temporary:
            root = pathlib.Path(temporary)
            with socket.socket() as port_socket:
                port_socket.bind(("127.0.0.1",0)); port = port_socket.getsockname()[1]
            config = f"[app]\nport={port}\ndatabase='tasks.db'\n[auth]\nenabled=false\n[output]\ndirectory='output'\n[task]\nmax_active=1\nmax_tiles=10000\nworkers=4\nsavepipe=2\ntimedelay=0\ntime_jitter_ms=0\nmax_retries=0\nretry_passes=0\nrequest_timeout_seconds=5\n"
            (root/"conf.toml").write_text(config,encoding="utf-8")
            environment = {key:value for key,value in os.environ.items() if not key.startswith(("APP_","AUTH_","OUTPUT_","TASK_","TILER_WORKER_"))}
            (root/"tmp").mkdir()
            environment.update({key:str(root/"tmp") for key in ["TMP","TEMP","TMPDIR","SQLITE_TMPDIR"]})
            base = f"http://127.0.0.1:{port}"
            def request(path, payload=None):
                data = None if payload is None else json.dumps(payload).encode()
                req = urllib.request.Request(base+path,data,{"Content-Type":"application/json"})
                with urllib.request.urlopen(req,timeout=10) as response: return response.read()
            with (root/"server.log").open("wb") as log:
                process = subprocess.Popen([str(binary),"-c",str(root/"conf.toml")],cwd=root,env=environment,stdout=log,stderr=log,
                    creationflags=0x08000000 if os.name=="nt" else 0,start_new_session=os.name!="nt")
                try:
                    for _ in range(300):
                        try: request("/api/tasks"); break
                        except OSError:
                            if process.poll() is not None: raise RuntimeError("fixture server exited")
                            time.sleep(.1)
                    else: raise TimeoutError("fixture server startup timed out")
                    memory = ProcessMemory(process.pid)
                    stop = threading.Event(); samples = {"disk":0,"first":None}; errors=[]
                    started = time.perf_counter()
                    def sample():
                        try:
                            while not stop.is_set():
                                memory.sample()
                                paths = list(root.rglob("*")); size=0; committed=False
                                for path in paths:
                                    try:
                                        if not path.is_file(): continue
                                        length=path.stat().st_size; size+=length
                                        if format_name=="zip" and path.suffix==".png" and length==len(png): committed=True
                                    except (FileNotFoundError,PermissionError): pass
                                samples["disk"]=max(samples["disk"],size)
                                if committed and samples["first"] is None: samples["first"]=(time.perf_counter()-started)*1000
                                stop.wait(.02)
                        except BaseException as error: errors.append(str(error))
                    sampler=threading.Thread(target=sample,daemon=True); sampler.start()
                    try:
                        payload={"name":"whole workflow fixture","mode":"bbox","area":{"bbox":{"minLon":-180,"minLat":-85,"maxLon":179.99,"maxLat":85}},
                            "zoom":{"min":4,"max":4},"output":{"format":format_name},"workers":4,"savePipe":2,"timeDelay":0,"scheduleMode":"immediate",
                            "sources":[{"name":"loopback","url":f"http://127.0.0.1:{upstream.server_port}/{{z}}/{{x}}/{{y}}","format":"png","schema":"xyz"}]}
                        task=json.loads(request("/api/tasks",payload)); task_id=task["id"]
                        for _ in range(6000):
                            task=json.loads(request("/api/tasks/"+task_id))
                            if task.get("children"): task=task["children"][0]
                            if format_name=="mbtiles" and samples["first"] is None and task.get("successCount",0)>0:
                                samples["first"]=(time.perf_counter()-started)*1000
                            if task.get("downloadUrl"): break
                            if task.get("status") in ["failed","cancelled"]: raise RuntimeError(json.dumps(task))
                            time.sleep(.02)
                        else: raise TimeoutError("fixture did not publish")
                        elapsed=(time.perf_counter()-started)*1000
                        stop.set(); sampler.join()
                        # Publication is the timing endpoint; downloading verification bytes is excluded.
                        artifact=request(task["downloadUrl"])
                        if format_name=="zip":
                            with zipfile.ZipFile(io.BytesIO(artifact)) as archive:
                                assert len(archive.namelist())==256
                                for name in archive.namelist(): assert archive.read(name)==png
                        else:
                            check=root/"verify.mbtiles"; check.write_bytes(artifact)
                            with contextlib.closing(sqlite3.connect(check)) as database:
                                rows=database.execute("SELECT tile_data FROM tiles").fetchall()
                                assert len(rows)==256 and all(row[0]==png for row in rows)
                            check.unlink()
                        assert counts["requests"]==256,counts
                    finally:
                        stop.set(); sampler.join()
                    if errors: raise RuntimeError("resource sampler: "+"; ".join(errors))
                    if len(memory.peaks)<2: raise RuntimeError("sampler did not observe both server and worker")
                    if samples["first"] is None: raise RuntimeError("sampler did not observe a committed tile")
                    return {"format":format_name,"tiles":256,**counts,"elapsed_to_publication_ms":round(elapsed,3),
                        "first_observed_committed_tile_ms":round(samples["first"],3),
                        "sampled_peak_process_tree_rss_bytes":memory.peak_sum,
                        "observed_os_peak_rss_by_process_bytes":sorted(memory.peaks.values()),
                        "sampled_peak_runtime_directory_bytes":samples["disk"],"artifact_bytes":len(artifact)}
                except BaseException:
                    log.flush(); print((root/"server.log").read_text(errors="replace")[-4000:],flush=True); raise
                finally:
                    if os.name=="nt": subprocess.run(["taskkill","/pid",str(process.pid),"/T","/F"],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
                    else:
                        try: os.killpg(process.pid,signal.SIGTERM)
                        except ProcessLookupError: pass
                    try: process.wait(timeout=10)
                    except subprocess.TimeoutExpired: process.kill(); process.wait()
    finally:
        upstream.shutdown(); upstream.server_close(); serving.join()


def revision(ref, png):
    with tempfile.TemporaryDirectory(prefix="tiler-workflow-source-") as directory:
        temporary=pathlib.Path(directory); source=temporary/"source"
        sha=snapshot(source,ref); app=source/APP
        toolchain=subprocess.check_output(["go","version"],cwd=app,text=True).strip()
        digest=hashlib.sha256()
        for path in sorted(app.rglob("*")):
            if path.is_file() and ((path.suffix==".go" and not path.name.endswith("_test.go")) or path.name in {"go.mod","go.sum","conf.toml"}):
                digest.update(path.relative_to(app).as_posix().encode()+b"\0"+path.read_bytes()+b"\0")
        binary=temporary/("tiler.exe" if os.name=="nt" else "tiler")
        subprocess.run(["go","build","-buildvcs=false","-o",str(binary),"."],cwd=app,check=True)
        result={"ref":ref,"base_commit":sha,"toolchain":toolchain,"working_tree":ref=="WORKTREE","source_code_sha256":digest.hexdigest(),
                "measurements":[measure(binary,kind,png) for kind in ["zip","mbtiles"]]}
        print(json.dumps(result,ensure_ascii=False),flush=True); return result


if __name__=="__main__":
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ref",action="append",default=[])
    parser.add_argument("--working-tree",action="store_true")
    parser.add_argument("--output",type=pathlib.Path,required=True)
    args=parser.parse_args(); refs=args.ref+(["WORKTREE"] if args.working_tree else [])
    if not refs: parser.error("provide --ref or --working-tree")
    png=png_fixture()
    report={"fixture":"256 deterministic 256x256 RGB PNGs, bbox z4, four workers, zero delay, loopback HTTP; real API, scheduler, worker and publication",
        "python":sys.version.split()[0],"png_bytes":len(png),"png_sha256":hashlib.sha256(png).hexdigest(),"sample_interval_ms":20,
        "limits":"Publication timing excludes verification download. First committed tile is externally observed from a complete file or the MBTiles success counter, so includes polling/progress persistence latency; no external reader is attached to the live MBTiles writer. RSS is sampled simultaneous process-tree RSS; OS per-process peak counters include startup and are not additive simultaneous peaks. All child temp-directory variables point inside the isolated runtime directory, including SQLite/VACUUM temps. Directory sizes include control DB, raw, catalogues, journals and packaging; exclude source/binary, and are logical file sizes sampled at 20ms, so short-lived peaks may be missed. Polling adds equal observation overhead to both revisions. Single paired run; no timing pass threshold.",
        "revisions":[revision(ref,png) for ref in refs]}
    args.output.parent.mkdir(parents=True,exist_ok=True)
    args.output.write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
