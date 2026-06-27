#!/usr/bin/env python3
import argparse
import concurrent.futures
import json
import pathlib
import threading
import time
import urllib.error
import urllib.parse
import urllib.request


BASE_URL = "https://geo.datav.aliyun.com/areas_v3/bound/geojson?code={code}"
USER_AGENT = "Mozilla/5.0 (compatible; tiler-master/1.0; +https://datav.aliyun.com/)"


def load_regions(path: pathlib.Path):
    return json.loads(path.read_text(encoding="utf-8"))


def fetch_json(url: str, timeout: int):
    request = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(request, timeout=timeout) as response:
        return json.loads(response.read().decode("utf-8"))


def ensure_feature_name(payload: dict, expected_name: str):
    features = payload.get("features") or []
    if not features:
        raise ValueError("empty feature collection")
    props = features[0].get("properties") or {}
    if not props.get("name"):
        props["name"] = expected_name
    return payload


def write_geojson(path: pathlib.Path, payload: dict):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        json.dumps(payload, ensure_ascii=False, separators=(",", ":")),
        encoding="utf-8",
    )


def build_index(regions):
    return {item["id"]: item for item in regions}


def resolve_target(root: pathlib.Path, item: dict):
    geojson_path = item.get("geojson", "").replace("./", "", 1)
    return root / geojson_path


def download_item(root: pathlib.Path, item: dict, index: dict, timeout: int, retries: int, overwrite: bool):
    if item["level"] == "world":
        return {"id": item["id"], "status": "skipped", "reason": "world"}

    target = resolve_target(root, item)
    if target.exists() and not overwrite:
        return {"id": item["id"], "status": "exists", "path": str(target)}

    code = "100000" if item["level"] == "country" else item["id"]
    url = BASE_URL.format(code=urllib.parse.quote(code))
    last_error = None
    payload = None

    for attempt in range(retries):
        try:
            payload = fetch_json(url, timeout=timeout)
            break
        except urllib.error.HTTPError as exc:
            if exc.code == 404 and item["level"] == "city":
                parent = index.get(item.get("parentId", ""))
                if parent:
                    parent_target = resolve_target(root, parent)
                    if parent_target.exists():
                        target.write_bytes(parent_target.read_bytes())
                        return {
                            "id": item["id"],
                            "status": "fallback",
                            "source": parent["id"],
                            "path": str(target),
                        }
                    fallback_url = BASE_URL.format(code=urllib.parse.quote(parent["id"]))
                    try:
                        payload = fetch_json(fallback_url, timeout=timeout)
                        break
                    except Exception as fallback_exc:
                        last_error = fallback_exc
                last_error = exc
                break
            last_error = exc
            if exc.code >= 500 and attempt + 1 < retries:
                time.sleep(0.5 * (attempt + 1))
                continue
            break
        except Exception as exc:
            last_error = exc
            if attempt + 1 < retries:
                time.sleep(0.5 * (attempt + 1))
                continue
            break

    if payload is None:
        return {
            "id": item["id"],
            "status": "error",
            "path": str(target),
            "error": repr(last_error),
        }

    payload = ensure_feature_name(payload, item["name"])
    write_geojson(target, payload)
    return {"id": item["id"], "status": "downloaded", "path": str(target)}


def main():
    parser = argparse.ArgumentParser(description="Download Aliyun administrative GeoJSON files.")
    parser.add_argument("--root", default=".", help="Project root containing geojson/regions.json")
    parser.add_argument("--workers", type=int, default=8, help="Concurrent download workers")
    parser.add_argument("--timeout", type=int, default=30, help="HTTP timeout in seconds")
    parser.add_argument("--retries", type=int, default=3, help="Retry count per file")
    parser.add_argument("--overwrite", action="store_true", help="Overwrite existing files")
    parser.add_argument("--limit", type=int, default=0, help="Optional limit for dry runs")
    args = parser.parse_args()

    root = pathlib.Path(args.root).resolve()
    regions_path = root / "geojson" / "regions.json"
    regions = load_regions(regions_path)
    index = build_index(regions)

    items = [item for item in regions if item["level"] != "world"]
    if args.limit > 0:
        items = items[: args.limit]

    results = []
    lock = threading.Lock()
    total = len(items)
    progress = {"done": 0}

    def run(item):
        result = download_item(root, item, index, args.timeout, args.retries, args.overwrite)
        with lock:
            progress["done"] += 1
            if progress["done"] % 100 == 0 or progress["done"] == total:
                print(f"[{progress['done']}/{total}] {result['status']} {item['name']}")
        return result

    with concurrent.futures.ThreadPoolExecutor(max_workers=args.workers) as executor:
        for result in executor.map(run, items):
            results.append(result)

    summary = {}
    for result in results:
        summary[result["status"]] = summary.get(result["status"], 0) + 1

    report = {
        "generatedAt": time.strftime("%Y-%m-%d %H:%M:%S"),
        "summary": summary,
        "errors": [item for item in results if item["status"] == "error"][:1000],
    }
    report_path = root / "geojson" / "download-report.json"
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(report["summary"], ensure_ascii=False))
    if report["errors"]:
        print(f"errors: {len(report['errors'])}, report: {report_path}")


if __name__ == "__main__":
    main()
