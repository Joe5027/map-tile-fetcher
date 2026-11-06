#!/usr/bin/env python3
"""Compare isolated Git revisions with the same loopback download fixture."""
import argparse
import json
import os
import pathlib
import shutil
import subprocess
import tarfile
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[3]
APP = pathlib.Path("apps/admin-region-tiler")


def measure(ref):
    sha = subprocess.check_output(["git", "rev-parse", ref + "^{commit}"], cwd=ROOT, text=True).strip()
    with tempfile.TemporaryDirectory(prefix="tiler-performance-") as directory:
        temporary = pathlib.Path(directory)
        archive = temporary / "source.tar"
        subprocess.run(["git", "archive", "--format=tar", "--output", str(archive), sha], cwd=ROOT, check=True)
        with tarfile.open(archive) as source:
            source.extractall(temporary / "source", filter="data")
        app = temporary / "source" / APP
        shutil.copyfile(ROOT / APP / "performance_fixture_test.go", app / "performance_fixture_test.go")
        environment = {**os.environ, "TILER_PERFORMANCE_FIXTURE": "1"}
        toolchain = subprocess.check_output(["go", "version"], cwd=app, env=environment, text=True).strip()
        run = subprocess.run(["go", "test", "-run", "^TestDownloadPerformanceFixture$", "-count=1", "-v", "."],
                             cwd=app, env=environment, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        print(run.stdout)
        run.check_returncode()
        results = [json.loads(line.split("PERFORMANCE ", 1)[1]) for line in run.stdout.splitlines() if "PERFORMANCE " in line]
        if len(results) != 2:
            raise ValueError("expected ZIP-file-tree and MBTiles measurements")
        return {"ref": ref, "commit": sha, "toolchain": toolchain, "results": results}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ref", action="append", required=True, help="repeat for before/after commits")
    parser.add_argument("--output", type=pathlib.Path, help="optional local JSON evidence file")
    args = parser.parse_args()
    report = {"fixture": "256 one-pixel PNGs, four workers, zero delay, loopback HTTP, 10 ms sampling",
              "limits": "Download engine only; ZIP packaging excluded. Heap/output peaks are sampled, not OS RSS. No timing pass threshold.",
              "measurements": [measure(ref) for ref in args.ref]}
    text = json.dumps(report, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(text, encoding="utf-8")
    print(text)
