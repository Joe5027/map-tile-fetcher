import json
import pathlib
import tempfile
import unittest
from check_region_catalog import validate_catalog


class CatalogGapTests(unittest.TestCase):
    def test_repository_gaps_are_documented(self):
        result = validate_catalog(pathlib.Path(__file__).resolve().parent.parent)
        self.assertEqual(result["catalogCount"], 3227)

    def test_new_gap_and_stale_exception_fail(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            (root / "geojson").mkdir()
            item = {"id":"110000", "name":"test", "parentId":"china", "level":"province", "geojson":"geojson/test.geojson"}
            (root / "geojson/regions.json").write_text(json.dumps([item]))
            (root / "geojson/region-gaps.json").write_text("{}")
            with self.assertRaisesRegex(ValueError, "unexpected gaps"):
                validate_catalog(root)
            (root / "geojson/test.geojson").write_text("{}")
            (root / "geojson/region-gaps.json").write_text('{"110000":{}}')
            with self.assertRaisesRegex(ValueError, "stale exceptions"):
                validate_catalog(root)
