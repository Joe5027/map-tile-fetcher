"""Build the Docker image with the same metadata and verify its version command."""
import json
import argparse
import pathlib
import subprocess
import sys

sys.dont_write_bytecode=True
from build_release import ROOT, APP, metadata

parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument("--allow-dirty",action="store_true")
args=parser.parse_args()
info,_=metadata(ROOT,"HEAD",args.allow_dirty)
image="tiler-build-verification:"+info["commit"][:12]
command=["docker","build","-t",image]
for key,value in {"VERSION":info["version"],"COMMIT":info["commit"],"BUILT_AT":info["builtAt"],"DIRTY":str(info["dirty"]).lower()}.items():
    command += ["--build-arg",key+"="+value]
command += [str(ROOT/APP)]
subprocess.run(command,check=True)
raw=subprocess.check_output(["docker","run","--rm",image,"./tiler","--version"])
assert json.loads(raw)==info,"Docker metadata does not match source metadata"
print("Docker metadata verification passed")
