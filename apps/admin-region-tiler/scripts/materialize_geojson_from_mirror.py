#!/usr/bin/env python3
import argparse
import sys

sys.dont_write_bytecode = True

from region_tools import Mirror, build_bundle, check_plan, common_arguments, prepare, run_cli


def main():
    parser = argparse.ArgumentParser(description="Validate a DataV mirror and optionally create an isolated region bundle.")
    common_arguments(parser, "--project-root")
    parser.add_argument("--mirror-root", required=True)
    args = parser.parse_args()
    root, catalog, items, mirror_root, output = prepare(parser, args, mirror=True)
    mirror = Mirror(mirror_root, catalog)
    if not args.apply:
        report = check_plan(root, items, mirror)
        return report, int(any(r["status"] == "invalid_or_missing" for r in report["results"]))
    return build_bundle(output, root, items, mirror.load, 1)


if __name__ == "__main__":
    sys.exit(run_cli(main))
