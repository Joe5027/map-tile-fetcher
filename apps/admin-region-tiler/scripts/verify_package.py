#!/usr/bin/env python3
"""Extract a native package in a fresh directory and exercise real API/worker output."""
import argparse
import base64
import hashlib
import http.cookiejar
import http.server
import io
import json
import os
import pathlib
import platform
import socket
import subprocess
import tarfile
import tempfile
import threading
import time
import urllib.request
import zipfile


def verify(archive_path):
    windows=platform.system()=="Windows"
    suffix=".zip" if windows else ".tar.gz"
    if not archive_path.name.endswith(suffix):
        raise ValueError("package must match the native operating system")
    with tempfile.TemporaryDirectory(prefix="tiler-package-") as directory:
        root=pathlib.Path(directory)
        if windows:
            with zipfile.ZipFile(archive_path) as archive:
                for name in archive.namelist():
                    if pathlib.PurePosixPath(name).is_absolute() or ".." in pathlib.PurePosixPath(name).parts:
                        raise ValueError("unsafe archive path")
                archive.extractall(root)
        else:
            with tarfile.open(archive_path) as archive:
                archive.extractall(root,filter="data")
        manifest=json.loads((root/"manifest.json").read_bytes())
        for entry in manifest:
            if hashlib.sha256((root/entry["path"]).read_bytes()).hexdigest()!=entry["sha256"]:
                raise AssertionError("manifest hash mismatch: "+entry["path"])
        actual={p.relative_to(root).as_posix() for p in root.rglob("*") if p.is_file()}
        assert actual=={r["path"] for r in manifest}|{"manifest.json"}, "unmanifested package entries"
        assert not any(p.endswith((".db",".log","_test.go")) or p==".env" for p in actual)
        info=json.loads((root/"build-info.json").read_bytes())
        binary=root/("tiler.exe" if windows else "tiler")
        if not windows:
            assert os.access(binary,os.X_OK),"Linux binary not executable"
        empty=root/"version-only"
        empty.mkdir()
        result=subprocess.run([str(binary),"--version"],cwd=empty,capture_output=True,text=True,check=True)
        assert json.loads(result.stdout)==info,"CLI metadata differs from package"
        assert not list(empty.iterdir()),"version command created runtime data"
        assert "ExecStart=/opt/tiler/tiler " in (root/"deploy/systemd/tiler.service").read_text()
        png=base64.b64decode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9ZlS8AAAAASUVORK5CYII=")
        hits=[]
        class Tiles(http.server.BaseHTTPRequestHandler):
            def log_message(self,*args): pass
            def do_GET(self):
                hits.append(self.path)
                self.send_response(200)
                self.send_header("Content-Type","image/png")
                self.end_headers()
                self.wfile.write(png)
        tiles=http.server.ThreadingHTTPServer(("127.0.0.1",0),Tiles)
        thread=threading.Thread(target=tiles.serve_forever,daemon=True)
        thread.start()
        with socket.socket() as sock:
            sock.bind(("127.0.0.1",0));port=sock.getsockname()[1]
        env={k:v for k,v in os.environ.items() if not k.startswith(("APP_","AUTH_","TASK_","OUTPUT_"))}
        env.update({"APP_PORT":str(port),"AUTH_DEFAULT_USERNAME":"fixture","AUTH_DEFAULT_PASSWORD":"package-fixture-password",
                    "AUTH_ENABLED":"true","TASK_TIMEDELAY":"0","TASK_TIME_JITTER_MS":"0","TASK_RETRY_PASSES":"0"})
        opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        def request(path,payload=None):
            data=None if payload is None else json.dumps(payload).encode()
            req=urllib.request.Request(f"http://127.0.0.1:{port}"+path,data,headers={"Content-Type":"application/json"})
            with opener.open(req,timeout=10) as response:
                return response.read()
        with (root/"fixture.log").open("wb") as log:
            process=subprocess.Popen([str(binary)],cwd=root,env=env,stdout=log,stderr=log)
            try:
                for _ in range(200):
                    try:
                        version=json.loads(request("/api/version"));break
                    except OSError:
                        if process.poll() is not None: raise AssertionError("package server exited")
                        time.sleep(.1)
                else: raise AssertionError("package server did not start")
                assert version==info,"API metadata differs from CLI"
                assert b"loginForm" in request("/")
                assert len(request("/static/vendor/leaflet/leaflet.js"))>1000
                request("/api/auth/login",{"username":"fixture","password":"package-fixture-password"})
                payload={"name":"package fixture","mode":"bbox","area":{"bbox":{"minLon":1,"minLat":1,"maxLon":2,"maxLat":2}},
                    "zoom":{"min":1,"max":1},"output":{"format":"zip"},"workers":1,"timeDelay":0,"scheduleMode":"immediate",
                    "sources":[{"name":"fixture","url":f"http://127.0.0.1:{tiles.server_port}/{{z}}/{{x}}/{{y}}","format":"png","schema":"xyz"}]}
                task=json.loads(request("/api/tasks",payload))
                task_id=task["id"]
                for _ in range(300):
                    task=json.loads(request("/api/tasks/"+task_id))
                    if task.get("children"): task=task["children"][0]
                    if task.get("downloadUrl"):break
                    if task.get("status") in {"failed","cancelled"}:raise AssertionError(task)
                    time.sleep(.1)
                else: raise AssertionError("worker did not publish output")
                with zipfile.ZipFile(io.BytesIO(request(task["downloadUrl"]))) as result:
                    names=result.namelist()
                    assert len(names)==1 and names[0].endswith("1/1/0.png"),names
                assert hits==["/1/1/0"],hits
            except BaseException:
                log.flush()
                print((root/"fixture.log").read_text(errors="replace")[-12000:])
                raise
            finally:
                process.terminate()
                try:process.wait(timeout=10)
                except subprocess.TimeoutExpired:process.kill();process.wait()
                tiles.shutdown();tiles.server_close();thread.join()
        return {"native":platform.system(),"build":info,"manifestFiles":len(manifest),"tiles":len(hits),"status":"passed"}


if __name__=="__main__":
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive",type=pathlib.Path)
    args=parser.parse_args()
    print(json.dumps(verify(args.archive),sort_keys=True))
