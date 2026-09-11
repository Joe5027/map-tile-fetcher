#!/usr/bin/env python3
"""Build deterministic development/release packages from an explicit Git revision."""
import argparse
import datetime
import gzip
import hashlib
import io
import json
import os
import pathlib
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import zipfile

sys.dont_write_bytecode = True
APP = "apps/admin-region-tiler"
ROOT = pathlib.Path(__file__).resolve().parents[3]
RUNTIME_FILES = {"conf.toml", ".env.example", "README.md", "Dockerfile", "docker-compose.yml", ".dockerignore"}
DOCS = {"docs/user-manual.md", "docs/user-manual-zh.md", "docs/password-recovery.md", "docs/region-gap-investigation-2026-09-11.md", "docs/build-and-install.md"}


def git(root, *args):
    return subprocess.check_output(["git", "-c", "core.autocrlf=false", *args], cwd=root, stderr=subprocess.PIPE)


def metadata(root, ref, allow_dirty=False):
    sha = git(root, "rev-parse", "--verify", ref + "^{commit}").decode().strip()
    dirty = bool(git(root, "status", "--porcelain", "--untracked-files=all").strip())
    if dirty and not allow_dirty:
        raise ValueError("working tree is dirty; commit changes first (or --allow-dirty for development verification only)")
    if allow_dirty and sha != git(root, "rev-parse", "HEAD").decode().strip():
        raise ValueError("--allow-dirty only supports HEAD")
    epoch = int(git(root, "show", "-s", "--format=%ct", sha).decode().strip())
    tags = sorted(t for t in git(root, "tag", "--points-at", sha).decode().splitlines() if re.fullmatch(r"v\d+\.\d+\.\d+(?:-[A-Za-z0-9.]+)?", t))
    version = tags[0] if tags and not dirty else "dev+" + sha[:12] + (".dirty" if dirty else "")
    stamp = datetime.datetime.fromtimestamp(epoch, datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    return {"version": version, "commit": sha, "builtAt": stamp, "dirty": dirty}, epoch


def allowed_source(name):
    if name == "LICENSE" or name in DOCS:
        return True
    if not name.startswith(APP + "/"):
        return False
    relative = name[len(APP)+1:]
    path = pathlib.PurePosixPath(relative)
    if any(part in {"data", "output", "tiles", "tmp", "build", "dist", "__pycache__"} or part.startswith(".venv") for part in path.parts):
        return False
    if path.name.endswith("_test.go") or path.name.startswith("test_"):
        return False
    return relative in RUNTIME_FILES or relative in {"go.mod", "go.sum"} or (
        path.suffix == ".go" and (len(path.parts) == 1 or path.parts[0] == "internal")) or (
        path.parts[0] in {"static", "geojson", "deploy"} and path.suffix.lower() in {
            ".html", ".css", ".js", ".png", ".svg", ".jpg", ".ico", ".woff2", ".geojson", ".json", ".conf", ".service", ".md", ".sh"}
        and not path.name.endswith("-report.json"))


def snapshot(root, ref, destination, dirty):
    if dirty:
        names = git(root, "ls-files", "-z").decode().split("\0")
        for name in names:
            if not name or not allowed_source(name):
                continue
            source = root / name
            if source.is_symlink() or not source.is_file():
                raise ValueError("tracked input is missing or a symbolic link: " + name)
            target = destination / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(source, target)
        return
    process = subprocess.Popen(["git", "archive", "--format=tar", ref], cwd=root, stdout=subprocess.PIPE)
    try:
        with tarfile.open(fileobj=process.stdout, mode="r|") as archive:
            for member in archive:
                if not allowed_source(member.name):
                    continue
                if not member.isfile() or ".." in pathlib.PurePosixPath(member.name).parts or member.name.startswith("/"):
                    raise ValueError("unsafe source archive member")
                target = destination / member.name
                target.parent.mkdir(parents=True, exist_ok=True)
                with archive.extractfile(member) as source, target.open("wb") as output:
                    shutil.copyfileobj(source, output)
        if process.wait() != 0:
            raise ValueError("git archive failed")
    finally:
        if process.poll() is None:
            process.kill()
            process.wait()


def json_bytes(value):
    return (json.dumps(value, ensure_ascii=False, sort_keys=True, indent=2) + "\n").encode()


def archive_package(files, destination, epoch):
    temporary = destination.with_name(destination.name + ".partial")
    if destination.suffix == ".zip":
        stamp = datetime.datetime.fromtimestamp(max(315532800, epoch), datetime.timezone.utc)
        with zipfile.ZipFile(temporary, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
            for name, (data, mode) in sorted(files.items()):
                info = zipfile.ZipInfo(name, stamp.timetuple()[:6])
                info.create_system = 3
                info.external_attr = (0o100000 | mode) << 16
                info.compress_type = zipfile.ZIP_DEFLATED
                archive.writestr(info, data, compresslevel=9)
    else:
        with temporary.open("wb") as raw, gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=epoch, compresslevel=9) as gz:
            with tarfile.open(fileobj=gz, mode="w", format=tarfile.PAX_FORMAT) as archive:
                for name, (data, mode) in sorted(files.items()):
                    info = tarfile.TarInfo(name)
                    info.size, info.mode, info.mtime = len(data), mode, epoch
                    info.uid = info.gid = 0
                    info.uname = info.gname = ""
                    archive.addfile(info, io.BytesIO(data))
    os.replace(temporary, destination)
    return hashlib.sha256(destination.read_bytes()).hexdigest()


def runtime_files(snapshot_root, binary, target, info):
    app = snapshot_root / APP
    files = {}
    for path in sorted(app.rglob("*")):
        if not path.is_file():
            continue
        relative = path.relative_to(app).as_posix()
        if relative in RUNTIME_FILES or relative.split("/")[0] in {"static", "geojson", "deploy"}:
            files[relative] = (path.read_bytes(), 0o644)
    files["LICENSE"] = ((snapshot_root / "LICENSE").read_bytes(), 0o644)
    for name in DOCS:
        path = snapshot_root / name
        if path.exists():
            files[name] = (path.read_bytes(), 0o644)
    files["tiler.exe" if target == "windows" else "tiler"] = (binary.read_bytes(), 0o755)
    files["build-info.json"] = (json_bytes(info), 0o644)
    files["README.md"] = (("# Map Tile Fetcher\n\nBuild: " + info["version"] + "\n\n"
        "Run tiler.exe (Windows) or ./tiler (Linux) from this directory.\n"
        "Keep conf.toml, static/ and geojson/ next to the executable.\n"
        "Before first start, set AUTH_DEFAULT_USERNAME and AUTH_DEFAULT_PASSWORD in the process environment.\n"
        "Binary runs do not load .env automatically.\n\n"
        "[Chinese manual](docs/user-manual-zh.md) | [English manual](docs/user-manual.md) | "
        "[Password recovery](docs/password-recovery.md) | [Build and install](docs/build-and-install.md)\n").encode(),0o644)
    manifest = [{"path": name, "size": len(data), "mode": oct(mode), "sha256": hashlib.sha256(data).hexdigest()} for name, (data, mode) in sorted(files.items())]
    files["manifest.json"] = (json_bytes(manifest), 0o644)
    return files


def build(root, ref, output, allow_dirty=False, targets=("windows", "linux")):
    info, epoch = metadata(root, ref, allow_dirty)
    output = output.absolute()
    if output.exists() or any(p.is_symlink() or (hasattr(p,"is_junction") and p.is_junction()) for p in (output,*output.parents)):
        raise ValueError("output directory must be new and must not use symbolic links")
    output.mkdir(parents=True)
    marker = output / "INCOMPLETE"
    marker.write_text("Build has not completed validation.\n")
    checksums = {}
    with tempfile.TemporaryDirectory(prefix="tiler-build-") as temporary:
        source = pathlib.Path(temporary) / "source"
        source.mkdir()
        snapshot(root, info["commit"], source, info["dirty"])
        environment = {k:v for k,v in os.environ.items() if k not in {"GOFLAGS", "GOOS", "GOARCH", "CGO_ENABLED"}}
        environment.update({"CGO_ENABLED":"0", "GOARCH":"amd64", "GOAMD64":"v1", "TZ":"UTC"})
        toolchain = subprocess.check_output(["go","version"], env=environment).decode().strip()
        flags = " ".join(["-buildid=", "-X main.buildVersion="+info["version"], "-X main.buildCommit="+info["commit"],
                          "-X main.buildTime="+info["builtAt"], "-X main.buildDirty="+str(info["dirty"]).lower()])
        for target in targets:
            print("Building " + target + "/amd64 " + info["version"], file=sys.stderr)
            environment["GOOS"] = target
            binary = pathlib.Path(temporary) / ("tiler.exe" if target=="windows" else "tiler")
            subprocess.run(["go","build","-trimpath","-buildvcs=false","-ldflags",flags,"-o",str(binary),"."],
                           cwd=source/APP, env=environment, check=True)
            suffix = ".zip" if target=="windows" else ".tar.gz"
            name = "map-tile-fetcher-"+info["version"]+"-"+target+"-amd64"+suffix
            files = runtime_files(source,binary,target,info)
            checksums[name] = archive_package(files,output/name,epoch)
        (output/"build-info.json").write_bytes(json_bytes({**info,"toolchain":toolchain,"sourceDateEpoch":epoch}))
        (output/"SHA256SUMS").write_text("".join(f"{value}  {name}\n" for name,value in sorted(checksums.items())),encoding="ascii",newline="\n")
    marker.unlink()
    return {"build":info,"checksums":checksums}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ref", required=True, help="Explicit source commit or ref")
    parser.add_argument("--output-dir", type=pathlib.Path)
    parser.add_argument("--allow-dirty", action="store_true", help="Development verification only; never emits a tag version")
    parser.add_argument("--metadata-json", action="store_true", help="Print metadata only; no files or builds")
    parser.add_argument("--target", choices=["windows","linux","all"],default="all")
    args = parser.parse_args()
    if not args.metadata_json and not args.output_dir:
        parser.error("--output-dir required for packaging")
    try:
        if args.metadata_json:
            result = metadata(ROOT,args.ref,args.allow_dirty)[0]
        else:
            result = build(ROOT,args.ref,args.output_dir,args.allow_dirty,("windows","linux") if args.target=="all" else (args.target,))
        print(json.dumps(result,sort_keys=True))
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        print("Build failed: " + str(error),file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
