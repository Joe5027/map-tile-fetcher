#!/usr/bin/env python3
import argparse
import json
import pathlib
import shutil
from collections import defaultdict

from shapely import make_valid
from shapely.geometry import shape, mapping
from shapely.ops import unary_union

NAME_ALIASES = {
    ("540400", "米林市"): "米林县",
    ("540500", "错那市"): "错那县",
    ("460300", "西沙群岛"): "西沙区",
    ("460300", "南沙群岛"): "南沙区",
}


def read_json(path: pathlib.Path):
    for encoding in ("utf-8", "utf-8-sig"):
        try:
            return json.loads(path.read_text(encoding=encoding))
        except UnicodeDecodeError:
            continue
    return json.loads(path.read_text())


def feature_collection(feature):
    return {"type": "FeatureCollection", "features": [feature]}


def normalize_parent(value):
    if value in (None, "", "None"):
        return ""
    return str(value)


def build_source_map(mirror_root: pathlib.Path):
    source_map = {}
    name_parent_map = {}

    china = read_json(mirror_root / "china.json")
    china_feature = china.get("features", [])[0]
    source_map["china"] = china_feature
    name_parent_map[("100000", "中华人民共和国")] = china_feature

    for folder in ("province", "citys", "county"):
        for path in (mirror_root / folder).glob("*.json"):
            payload = read_json(path)
            for feature in payload.get("features", []):
                props = feature.get("properties") or {}
                code = str(props.get("adcode", ""))
                parent = normalize_parent((props.get("parent") or {}).get("adcode"))
                name = props.get("name", "")
                if code:
                    source_map.setdefault(code, feature)
                if parent and name:
                    name_parent_map.setdefault((parent, name), feature)
    return source_map, name_parent_map


def build_region_maps(regions):
    region_index = {item["id"]: item for item in regions}
    children = defaultdict(list)
    for item in regions:
        children[normalize_parent(item.get("parentId"))].append(item["id"])
    return region_index, children


def build_feature(code, region_index, children, source_map, name_parent_map, mirror_root, memo):
    if code in memo:
        return memo[code]

    item = region_index[code]
    source_feature = source_map.get(code)
    if source_feature:
        memo[code] = source_feature
        return source_feature

    lookup_parent = normalize_parent(item.get("parentId"))
    if lookup_parent == "china":
        lookup_parent = "100000"
    source_feature = name_parent_map.get((lookup_parent, item["name"]))
    if source_feature:
        memo[code] = source_feature
        return source_feature

    alias_name = NAME_ALIASES.get((lookup_parent, item["name"]))
    if alias_name:
        source_feature = name_parent_map.get((lookup_parent, alias_name))
        if source_feature:
            memo[code] = source_feature
            return source_feature

    if item["level"] == "province":
        province_bundle = mirror_root / "province" / f"{item['name']}.json"
        if province_bundle.exists():
            payload = read_json(province_bundle)
            bundle_features = payload.get("features", [])
            if bundle_features:
                geometries = []
                for feature in bundle_features:
                    geometry = make_valid(shape(feature["geometry"]))
                    if not geometry.is_valid:
                        geometry = geometry.buffer(0)
                    geometries.append(geometry)
                merged_geometry = unary_union(geometries)
                if not merged_geometry.is_valid:
                    merged_geometry = make_valid(merged_geometry)
                feature = {
                    "type": "Feature",
                    "properties": {
                        "adcode": int(code),
                        "name": item["name"],
                        "level": item["level"],
                        "parent": {"adcode": 100000},
                        "childrenNum": len(bundle_features),
                    },
                    "geometry": mapping(merged_geometry),
                }
                memo[code] = feature
                return feature

    child_codes = children.get(code, [])
    child_features = []
    for child_code in child_codes:
        child_feature = build_feature(child_code, region_index, children, source_map, name_parent_map, mirror_root, memo)
        if child_feature:
            child_features.append(child_feature)

    if not child_features:
        memo[code] = None
        return None

    geometries = []
    for child in child_features:
        geometry = make_valid(shape(child["geometry"]))
        if not geometry.is_valid:
            geometry = geometry.buffer(0)
        geometries.append(geometry)
    merged_geometry = unary_union(geometries)
    if not merged_geometry.is_valid:
        merged_geometry = make_valid(merged_geometry)
    feature = {
        "type": "Feature",
        "properties": {
            "adcode": 100000 if code == "china" else int(code),
            "name": item["name"],
            "level": item["level"],
            "parent": {
                "adcode": None
                if code == "china"
                else (
                    100000
                    if normalize_parent(item.get("parentId")) == "china"
                    else (
                        None
                        if normalize_parent(item.get("parentId")) == ""
                        else int(normalize_parent(item.get("parentId")))
                    )
                )
            },
            "childrenNum": len(child_codes),
        },
        "geometry": mapping(merged_geometry),
    }
    memo[code] = feature
    return feature


def write_regions(project_root: pathlib.Path, mirror_root: pathlib.Path, regions, region_index, children, source_map, name_parent_map):
    geo_root = project_root / "geojson"
    memo = {}
    summary = {"written": 0, "missing": []}

    for item in regions:
        if item["level"] == "world":
            continue

        code = item["id"]
        feature = build_feature(code, region_index, children, source_map, name_parent_map, mirror_root, memo)
        if not feature:
            summary["missing"].append({"id": code, "name": item["name"], "level": item["level"]})
            continue

        geojson_rel = item["geojson"].replace("./", "", 1)
        target = project_root / geojson_rel
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(json.dumps(feature_collection(feature), ensure_ascii=False), encoding="utf-8")
        summary["written"] += 1

    report_path = geo_root / "materialize-report.json"
    report_path.write_text(json.dumps(summary, ensure_ascii=False, indent=2), encoding="utf-8")
    return summary


def sync_to_deploy(project_root: pathlib.Path, deploy_root: pathlib.Path):
    src = project_root / "geojson"
    dst = deploy_root / "geojson"
    if dst.exists():
        shutil.rmtree(dst)
    shutil.copytree(src, dst)


def main():
    parser = argparse.ArgumentParser(description="Build per-region GeoJSON files from a DataV mirror.")
    parser.add_argument("--project-root", default=".", help="Project root containing geojson/regions.json")
    parser.add_argument("--mirror-root", required=True, help="Mirror root, e.g. .tmp-china-geojson-mirror")
    parser.add_argument("--deploy-root", default="", help="Optional deploy root to sync geojson directory")
    args = parser.parse_args()

    project_root = pathlib.Path(args.project_root).resolve()
    mirror_root = pathlib.Path(args.mirror_root).resolve()
    regions = read_json(project_root / "geojson" / "regions.json")
    region_index, children = build_region_maps(regions)
    source_map, name_parent_map = build_source_map(mirror_root)
    summary = write_regions(project_root, mirror_root, regions, region_index, children, source_map, name_parent_map)

    if args.deploy_root:
        sync_to_deploy(project_root, pathlib.Path(args.deploy_root).resolve())

    print(json.dumps(summary, ensure_ascii=False))


if __name__ == "__main__":
    main()
