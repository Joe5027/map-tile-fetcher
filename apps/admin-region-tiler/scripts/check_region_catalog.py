"""Offline catalog references and documented gap gate; no geometry rewrites."""
import json
import pathlib
import sys

sys.dont_write_bytecode = True
from region_tools import input_root, load_catalog, source_path


def validate_catalog(root):
    root = input_root(root)
    catalog = load_catalog(root)
    gaps = json.loads((root / "geojson/region-gaps.json").read_bytes())
    missing = {str(item["id"]) for item in catalog if not source_path(root, item).is_file()}
    if missing != set(gaps):
        raise ValueError(f"unexpected gaps: {sorted(missing-set(gaps))}; stale exceptions: {sorted(set(gaps)-missing)}")
    by_id = {str(item["id"]): item for item in catalog}
    for identity, gap in gaps.items():
        if gap.get("name") != by_id[identity]["name"] or not all(gap.get(k) for k in (
            "reasonCode", "reason", "checkedAt", "sourceDate", "boundaryDate", "crs", "usageTerms", "source", "provider")):
            raise ValueError("missing gap investigation fields")
        if not gap["source"].startswith("https://"):
            raise ValueError("documented source URL required")
    return {"catalogCount": len(catalog), "availableCount": len(catalog)-len(missing), "documentedGaps": sorted(missing)}


if __name__ == "__main__":
    try:
        print(json.dumps(validate_catalog(pathlib.Path(__file__).resolve().parent.parent)))
    except (ValueError, OSError, KeyError) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
