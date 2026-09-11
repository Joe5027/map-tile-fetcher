#!/usr/bin/env python3
import argparse
import sys

sys.dont_write_bytecode = True

from region_tools import build_bundle, check_plan, code, common_arguments, load_download, prepare, run_cli


def main():
    parser = argparse.ArgumentParser(description="Check region inputs offline or build an isolated DataV region bundle.")
    common_arguments(parser)
    parser.add_argument("--workers", type=int, default=8)
    parser.add_argument("--timeout", type=int, default=30)
    parser.add_argument("--retries", type=int, default=3)
    args = parser.parse_args()
    if not 1 <= args.workers <= 16 or not 1 <= args.timeout <= 120 or not 1 <= args.retries <= 5:
        parser.error("workers: 1-16; timeout: 1-120 seconds; retries: 1-5 attempts")
    root, catalog, items, _, output = prepare(parser, args)
    if not args.apply:
        report = check_plan(root, items)
        return report, int(any(r["status"] != "validated" for r in report["results"]))
    index = {code(r["id"]): r for r in catalog}
    return build_bundle(output, root, items,
                        lambda item: load_download(root, item, index, args.timeout, args.retries), args.workers)


if __name__ == "__main__":
    sys.exit(run_cli(main))
