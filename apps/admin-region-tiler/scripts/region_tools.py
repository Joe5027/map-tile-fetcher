"""Read-only region inputs and isolated, verifiable output bundles."""
import concurrent.futures
import errno
import hashlib
import json
import math
import os
import pathlib
import ssl
import socket
import signal
import threading
import time
import urllib.error
import urllib.parse
import urllib.request

from shapely.geometry import mapping, shape
from shapely.ops import unary_union

MAX_RESPONSE = 32 * 1024 * 1024
BASE_URL = "https://geo.datav.aliyun.com/areas_v3/bound/geojson?code={code}"
RETRY_STATUSES = {429, 500, 502, 503, 504}
CANCELLED = threading.Event()


class RegionError(ValueError):
    pass


def digest(data):
    return hashlib.sha256(data).hexdigest()


def encode(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"), allow_nan=False).encode("utf-8")


def code(value):
    return "100000" if value == "china" else "" if value in (None, "world") else str(value)


def reject_links(path):
    for entry in (path, *path.parents):
        if entry.is_symlink() or (hasattr(entry, "is_junction") and entry.is_junction()):
            raise RegionError("symlink_or_junction_not_allowed")


def input_root(value):
    path = pathlib.Path(value).absolute()
    reject_links(path)
    path = path.resolve(strict=True)
    if not path.is_dir():
        raise RegionError("input_must_be_directory")
    return path


def relative_region_path(value):
    value = str(value)
    if "\\" in value or ":" in value:
        raise RegionError("invalid_region_path")
    path = pathlib.PurePosixPath(value)
    if path.is_absolute() or ".." in path.parts or len(path.parts) < 2:
        raise RegionError("invalid_region_path")
    if path.parts[0] != "geojson" or path.suffix != ".geojson":
        raise RegionError("invalid_region_path")
    return path


def source_path(root, item):
    path = root.joinpath(*relative_region_path(item["geojson"]).parts)
    reject_links(path)
    if not path.resolve().is_relative_to(root):
        raise RegionError("source_outside_root")
    return path


def load_catalog(root):
    path = root / "geojson/regions.json"
    reject_links(path)
    records = json.loads(path.read_bytes())
    if not isinstance(records, list):
        raise RegionError("invalid_catalog")
    ids, paths = set(), set()
    for item in records:
        if not isinstance(item, dict) or not {"id", "name", "geojson", "level"} <= item.keys():
            raise RegionError("invalid_catalog_entry")
        identity = code(item["id"])
        if item["level"] != "world" and (len(identity) != 6 or not identity.isdigit()):
            raise RegionError("invalid_administrative_code")
        target = source_path(root, item)
        if identity in ids or str(target).casefold() in paths:
            raise RegionError("duplicate_catalog_entry")
        ids.add(identity)
        paths.add(str(target).casefold())
    return records


def output_path(value, inputs):
    path = pathlib.Path(value).absolute()
    reject_links(path)
    path = path.resolve()
    if path.exists():
        raise RegionError("output_directory_already_exists")
    if any(path.is_relative_to(root) or root.is_relative_to(path) for root in inputs):
        raise RegionError("input_output_overlap")
    return path


def bounded_read(path):
    reject_links(path)
    with path.open("rb") as stream:
        data = stream.read(MAX_RESPONSE + 1)
    if len(data) > MAX_RESPONSE:
        raise RegionError("response_too_large")
    return data


def validate_geometry(geometry):
    if not isinstance(geometry, dict) or geometry.get("type") not in {"Polygon", "MultiPolygon"}:
        raise RegionError("polygon_required")
    polygons = [geometry.get("coordinates")] if geometry["type"] == "Polygon" else geometry.get("coordinates")
    if not isinstance(polygons, list) or not polygons:
        raise RegionError("empty_geometry")
    for polygon in polygons:
        if not isinstance(polygon, list) or not polygon:
            raise RegionError("empty_polygon")
        for ring in polygon:
            if not isinstance(ring, list) or len(ring) < 4 or ring[0] != ring[-1]:
                raise RegionError("unclosed_ring")
            for point in ring:
                if not isinstance(point, (list, tuple)) or len(point) != 2:
                    raise RegionError("longitude_latitude_required")
                if any(isinstance(v, bool) or not isinstance(v, (float, int)) or not math.isfinite(v) for v in point):
                    raise RegionError("nonfinite_coordinate")
                if not -180 <= point[0] <= 180 or not -90 <= point[1] <= 90:
                    raise RegionError("coordinate_out_of_range")
    result = shape(geometry)
    if result.is_empty or not result.is_valid:
        raise RegionError("invalid_topology")
    return result


def select_features(payload, item):
    if not isinstance(payload, dict) or payload.get("crs"):
        raise RegionError("unsupported_coordinate_reference")
    features = [payload] if payload.get("type") == "Feature" else payload.get("features")
    if not isinstance(features, list):
        raise RegionError("feature_collection_required")
    selected = []
    for feature in features:
        if not isinstance(feature, dict):
            raise RegionError("invalid_feature")
        props = feature.get("properties") or {}
        if not isinstance(props, dict) or not isinstance(props.get("parent") or {}, dict):
            raise RegionError("invalid_properties")
        if code(props.get("adcode")) != code(item["id"]):
            continue
        if feature.get("type") != "Feature":
            raise RegionError("invalid_feature")
        parent = code((props.get("parent") or {}).get("adcode"))
        if parent != code(item.get("parentId")):
            raise RegionError("parent_code_mismatch")
        if props.get("name") != item["name"]:
            raise RegionError("region_name_mismatch")
        validate_geometry(feature.get("geometry"))
        selected.append(feature)
    if not selected:
        raise RegionError("target_code_missing")
    return {"type": "FeatureCollection", "features": selected}


def merge_children(item, children, collections):
    if not children or len(children) != len(collections):
        raise RegionError("incomplete_child_collection")
    geometries = []
    for child, collection in zip(children, collections):
        if code(child.get("parentId")) != code(item["id"]):
            raise RegionError("child_parent_mismatch")
        verified = select_features(collection, child)
        geometries.extend(validate_geometry(f["geometry"]) for f in verified["features"])
    geometry = json.loads(encode(mapping(unary_union(geometries))))
    result = {"type": "FeatureCollection", "features": [{"type": "Feature", "properties": {
        "adcode": int(code(item["id"])), "name": item["name"],
        "parent": {"adcode": int(code(item["parentId"])) if code(item.get("parentId")) else None},
        "level": item["level"], "childrenNum": len(children)}, "geometry": geometry}]}
    return select_features(result, item)


def fetch_bytes(url, timeout, retries):
    for attempt in range(retries):
        if CANCELLED.is_set():
            raise RegionError("cancelled")
        try:
            request = urllib.request.Request(url, headers={"User-Agent": "Map-Tile-Fetcher/region-maintenance"})
            deadline = time.monotonic() + timeout
            with urllib.request.urlopen(request, timeout=timeout) as response:
                chunks, size = [], 0
                while size <= MAX_RESPONSE:
                    if CANCELLED.is_set():
                        raise RegionError("cancelled")
                    if time.monotonic() > deadline:
                        raise TimeoutError()
                    chunk = response.read1(min(65536, MAX_RESPONSE + 1 - size))
                    if not chunk:
                        break
                    chunks.append(chunk)
                    size += len(chunk)
                data = b"".join(chunks)
            if len(data) > MAX_RESPONSE:
                raise RegionError("response_too_large")
            return data
        except urllib.error.HTTPError as error:
            if error.code not in RETRY_STATUSES or attempt + 1 == retries:
                raise RegionError(f"http_{error.code}") from None
        except (urllib.error.URLError, TimeoutError, ConnectionError) as error:
            reason = getattr(error, "reason", error)
            if isinstance(reason, (ssl.SSLCertVerificationError, ssl.CertificateError)):
                raise RegionError("tls_verification_failed") from None
            temporary = isinstance(reason, (TimeoutError, ConnectionError)) or (
                isinstance(reason, OSError) and reason.errno in {
                    errno.ECONNRESET, errno.ECONNREFUSED, errno.ETIMEDOUT, errno.EHOSTUNREACH,
                    errno.ENETUNREACH, socket.EAI_AGAIN})
            if not temporary:
                raise RegionError("network_error_not_retryable") from None
            if attempt + 1 == retries:
                raise RegionError("network_unavailable") from None
        CANCELLED.wait(min(0.5 * (2 ** attempt), 4))
    raise RegionError("network_unavailable")


def safe_url(url):
    parts = urllib.parse.urlsplit(url)
    return urllib.parse.urlunsplit((parts.scheme, parts.hostname or "", parts.path, "", ""))


def load_download(root, item, index, timeout, retries):
    local = source_path(root, item)
    if local.exists():
        raw = bounded_read(local)
        return select_features(json.loads(raw), item), [{"source": str(relative_region_path(item["geojson"])),
                "sha256": digest(raw)}], "validated_local"
    url = BASE_URL.format(code=code(item["id"]))
    try:
        raw = fetch_bytes(url, timeout, retries)
    except RegionError as error:
        if str(error) != "http_404" or not item.get("parentId"):
            raise
        parent = index.get(code(item["parentId"]))
        if not parent:
            raise
        parent_path = source_path(root, parent)
        parent_url = BASE_URL.format(code=code(parent["id"]))
        raw = bounded_read(parent_path) if parent_path.exists() else fetch_bytes(parent_url, timeout, retries)
        payload = select_features(json.loads(raw), item)
        origin = str(relative_region_path(parent["geojson"])) if parent_path.exists() else safe_url(parent_url)
        return payload, [{"source": origin, "sha256": digest(raw)}], "extracted_target_feature"
    return select_features(json.loads(raw), item), [{"source": safe_url(url), "sha256": digest(raw)}], "extracted_target_feature"


class Mirror:
    def __init__(self, root, catalog):
        self.root, self.catalog = root, catalog
        self.sources, self.memo, self.visiting = {}, {}, set()
        paths = [root / "china.json"]
        for folder in ("province", "citys", "county"):
            reject_links(root / folder)
            paths.extend(sorted((root / folder).glob("*.json")))
        for path in paths:
            if not path.exists():
                continue
            raw = bounded_read(path)
            payload = json.loads(raw)
            if not isinstance(payload, dict) or payload.get("crs"):
                raise RegionError("unsupported_coordinate_reference")
            for feature in payload.get("features", []):
                if not isinstance(feature, dict) or not isinstance(feature.get("properties"), dict):
                    raise RegionError("invalid_feature")
                identity = code((feature.get("properties") or {}).get("adcode"))
                if identity:
                    self.sources.setdefault(identity, []).append((feature, {
                        "source": path.relative_to(root).as_posix(), "sha256": digest(raw)}))

    def load(self, item):
        identity = code(item["id"])
        if identity in self.memo:
            return self.memo[identity]
        if identity in self.visiting:
            raise RegionError("cyclic_catalog")
        self.visiting.add(identity)
        try:
            records = self.sources.get(identity, [])
            if records:
                unique = {digest(encode(feature)): feature for feature, _ in records}
                payload = select_features({"type": "FeatureCollection", "features": list(unique.values())}, item)
                origins = list({(s["source"], s["sha256"]): s for _, s in records}.values())
                result = payload, origins, "extracted_target_feature"
            else:
                children = [r for r in self.catalog if code(r.get("parentId")) == identity]
                if not children:
                    raise RegionError("target_code_missing")
                loaded = [self.load(child) for child in children]
                result = merge_children(item, children, [v[0] for v in loaded]), [s for v in loaded for s in v[1]], "merged_complete_children"
            self.memo[identity] = result
            return result
        finally:
            self.visiting.remove(identity)


def atomic_write(path, data):
    reject_links(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_name(path.name + ".partial")
    with temporary.open("xb") as stream:
        stream.write(data)
        stream.flush()
        os.fsync(stream.fileno())
    if digest(temporary.read_bytes()) != digest(data):
        raise RegionError("output_checksum_mismatch")
    os.replace(temporary, path)


def build_bundle(output, root, catalog, loader, workers):
    output.mkdir(parents=True, exist_ok=False)
    marker = output / "INCOMPLETE"
    marker.write_text("Bundle has not completed validation.\n", encoding="ascii")
    results = []

    def process(item):
        try:
            if CANCELLED.is_set():
                raise RegionError("cancelled")
            payload, origins, method = loader(item)
            data = encode(select_features(payload, item))
            relative = relative_region_path(item["geojson"])
            atomic_write(output.joinpath(*relative.parts), data)
            original = source_path(root, item)
            previous = digest(bounded_read(original)) if original.exists() else None
            return {"id": item["id"], "path": relative.as_posix(), "status": "ready", "sha256": digest(data),
                    "previousSha256": previous, "change": "unchanged" if previous == digest(data) else "changed" if previous else "added",
                    "sources": origins, "method": method}
        except RegionError as error:
            return {"id": item["id"], "status": "missing", "reasonCode": str(error)}
        except (ValueError, KeyError, TypeError):
            return {"id": item["id"], "status": "failed", "reasonCode": "invalid_region_data"}

    # INCOMPLETE is created first and survives failed writes and cancellation.
    executor = concurrent.futures.ThreadPoolExecutor(max_workers=workers)
    try:
        for result in executor.map(process, catalog):
            results.append(result)
    except BaseException:
        CANCELLED.set()
        raise
    finally:
        executor.shutdown(wait=True, cancel_futures=True)
    complete = all(r["status"] == "ready" for r in results)
    report = {"mode": "apply", "status": "complete" if complete else "incomplete", "results": results}
    atomic_write(output / "geojson/regions.json", encode(catalog))
    atomic_write(output / "manifest.json", encode(report))
    atomic_write(output / "report.json", encode({"missing": [r for r in results if r["status"] != "ready"],
                                                 "changes": [{k: r[k] for k in ("id", "change", "sha256")} for r in results if r["status"] == "ready"]}))
    if complete:
        marker.unlink()
    return report, 0 if complete else 1


def common_arguments(parser, root_name="--root"):
    parser.add_argument(root_name, default=".", help="Read-only input root containing geojson/regions.json")
    parser.add_argument("--output-dir", help="New isolated bundle directory (required for --apply)")
    parser.add_argument("--apply", action="store_true", help="Generate a new bundle; default is offline check only")
    parser.add_argument("--limit", type=int, default=0, help="Maximum catalog entries to process, 0 for all")
    parser.add_argument("--deploy-root", help="Removed: direct deployment is not supported")
    parser.add_argument("--overwrite", action="store_true", help="Removed: source files are always read-only")


def prepare(parser, args, mirror=False):
    if args.deploy_root is not None or args.overwrite:
        parser.error("direct deployment and overwrite were removed; use --output-dir with --apply to create a new bundle")
    if args.limit < 0:
        parser.error("--limit must be nonnegative")
    if args.apply and not args.output_dir:
        parser.error("--apply requires --output-dir")
    try:
        root = input_root(args.project_root if mirror else args.root)
        catalog = load_catalog(root)
        items = [r for r in catalog if r["level"] != "world"]
        if args.limit:
            items = items[:args.limit]
        mirror_root = input_root(args.mirror_root) if mirror else None
        inputs = [root] + ([mirror_root] if mirror else [])
        output = output_path(args.output_dir, inputs) if args.output_dir else None
        return root, catalog, items, mirror_root, output
    except (RegionError, OSError, ValueError, KeyError):
        parser.error("invalid input/catalog or unsafe output path; inputs must exist and output must be new and disjoint")


def check_plan(root, items, mirror=None):
    results = []
    for item in items:
        try:
            if mirror:
                mirror.load(item)
            else:
                path = source_path(root, item)
                if not path.exists():
                    results.append({"id": item["id"], "status": "needs_source"})
                    continue
                select_features(json.loads(bounded_read(path)), item)
            results.append({"id": item["id"], "status": "validated"})
        except (ValueError, OSError, KeyError, TypeError):
            results.append({"id": item["id"], "status": "invalid_or_missing"})
    return {"mode": "check", "network": False, "writes": False, "results": results}


def run_cli(action):
    CANCELLED.clear()
    def interrupted(signum, frame):
        CANCELLED.set()
        raise KeyboardInterrupt()
    signal.signal(signal.SIGTERM, interrupted)
    try:
        report, exit_code = action()
    except KeyboardInterrupt:
        report, exit_code = {"status": "incomplete", "reasonCode": "cancelled"}, 1
    except (OSError, ValueError, KeyError, TypeError):
        report, exit_code = {"status": "incomplete", "reasonCode": "operation_failed"}, 1
    print(json.dumps(report, ensure_ascii=True, sort_keys=True))
    return exit_code
