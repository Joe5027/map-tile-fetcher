import hashlib
import copy
import errno
import http.server
import json
import pathlib
import subprocess
import sys
import tempfile
import threading
import unittest
from unittest import mock

sys.dont_write_bytecode = True
import region_tools as tools


SCRIPTS = pathlib.Path(__file__).resolve().parent


def region(code="110100", parent="110000"):
    return {"id": code, "name": "Test", "level": "city", "parentId": parent,
            "geojson": f"./geojson/{code}.geojson"}


def feature(code="110100", parent="110000", offset=0):
    return {"type": "Feature", "properties": {"adcode": int(code), "name": "Test",
            "parent": {"adcode": int(parent)}}, "geometry": {"type": "Polygon",
            "coordinates": [[[offset, 0], [offset + 1, 0], [offset + 1, 1], [offset, 0]]]}}


class RegionCLIContracts(unittest.TestCase):
    def test_default_plan_never_writes_or_downloads(self):
        with tempfile.TemporaryDirectory() as directory:
            base = pathlib.Path(directory)
            root = base / "input"
            (root / "geojson").mkdir(parents=True)
            catalog = root / "geojson/regions.json"
            catalog.write_text(json.dumps([region()]), encoding="utf-8")
            (root / "geojson/110100.geojson").write_text(json.dumps(feature()), encoding="utf-8")
            before = hashlib.sha256(catalog.read_bytes()).hexdigest()
            result = subprocess.run([sys.executable, str(SCRIPTS / "download_aliyun_geojson.py"),
                                     "--root", str(root), "--output-dir", str(base / "bundle")],
                                    capture_output=True, text=True, timeout=15)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(json.loads(result.stdout)["mode"], "check")
            self.assertFalse((base / "bundle").exists())
            self.assertEqual(len(list((root / "geojson").iterdir())), 2)
            self.assertEqual(hashlib.sha256(catalog.read_bytes()).hexdigest(), before)

    def test_legacy_and_invalid_flags_fail_before_input_access(self):
        for script, extra in [("download_aliyun_geojson.py", []),
                              ("materialize_geojson_from_mirror.py", ["--mirror-root", "absent"])]:
            for flag in [["--deploy-root", "absent"], ["--overwrite"]]:
                result = subprocess.run([sys.executable, str(SCRIPTS / script), *extra, *flag],
                                        capture_output=True, text=True)
                self.assertEqual(result.returncode, 2)
                self.assertIn("removed", result.stderr)
        for option, value in [("workers", "17"), ("timeout", "121"), ("retries", "6"), ("limit", "-1")]:
            result = subprocess.run([sys.executable, str(SCRIPTS / "download_aliyun_geojson.py"),
                                    "--" + option, value], capture_output=True, text=True)
            self.assertEqual(result.returncode, 2)


class RegionContracts(unittest.TestCase):
    def setUp(self):
        tools.CANCELLED.clear()
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.base = pathlib.Path(self.temporary.name)
        self.root = self.base / "input"
        (self.root / "geojson").mkdir(parents=True)
        self.item = region()
        self.catalog = self.root / "geojson/regions.json"
        self.catalog.write_bytes(tools.encode([self.item]))

    def test_path_constraints(self):
        for path in [self.root, self.root / "new", self.base]:
            with self.assertRaises(tools.RegionError):
                tools.output_path(path, [self.root])
        for path in ["../outside.geojson", "geojson/../../outside.geojson", "C:/geojson/a.geojson"]:
            with self.assertRaises(tools.RegionError):
                tools.source_path(self.root, dict(self.item, geojson=path))
        self.assertEqual(tools.output_path(self.base / "new", [self.root]), self.base / "new")

    def test_symbolic_links_rejected(self):
        link = self.base / "link"
        try:
            link.symlink_to(self.root, target_is_directory=True)
        except OSError:
            self.skipTest("OS does not grant symbolic-link creation")
        with self.assertRaises(tools.RegionError):
            tools.output_path(link / "new", [self.root])
        with self.assertRaises(tools.RegionError):
            tools.input_root(link)

    def test_check_is_offline_and_reports_missing_and_invalid(self):
        with mock.patch.object(tools.urllib.request, "urlopen", side_effect=AssertionError("network")):
            report = tools.check_plan(self.root, [self.item])
            self.assertEqual(report["results"][0]["status"], "needs_source")
            (self.root / "geojson/110100.geojson").write_bytes(b"{}")
            self.assertEqual(tools.check_plan(self.root, [self.item])["results"][0]["status"], "invalid_or_missing")

    def test_exact_multiple_features_and_parent(self):
        payload = {"type": "FeatureCollection", "features": [feature(), feature(offset=3), feature("110200")]}
        self.assertEqual(len(tools.select_features(payload, self.item)["features"]), 2)
        for changed in [feature(parent="120000"), feature("110200")]:
            with self.assertRaises(tools.RegionError):
                tools.select_features(changed, self.item)
        changed = feature()
        changed["properties"]["name"] = "Wrong"
        with self.assertRaises(tools.RegionError):
            tools.select_features(changed, self.item)

    def test_invalid_geometry_is_never_repaired(self):
        invalid = [[], [[[0, 0], [1, 1], [0, 1], [1, 0], [0, 0]]],
                   [[[0, 0], [1, 0], [1, 1], [0, 1]]],
                   [[[200, 0], [201, 0], [201, 1], [200, 0]]]]
        for coordinates in invalid:
            with self.assertRaises(tools.RegionError):
                tools.validate_geometry({"type": "Polygon", "coordinates": coordinates})

    def test_404_does_not_copy_parent(self):
        parent = region("110000", "china")
        (self.root / "geojson/110000.geojson").write_bytes(tools.encode(feature("110000", "100000")))
        with mock.patch.object(tools, "fetch_bytes", side_effect=tools.RegionError("http_404")):
            with self.assertRaisesRegex(tools.RegionError, "target_code_missing"):
                tools.load_download(self.root, self.item, {"110000": parent}, 1, 1)
            (self.root / "geojson/110000.geojson").write_bytes(tools.encode({"features": [feature(), feature(offset=3)]}))
            result, sources, method = tools.load_download(self.root, self.item, {"110000": parent}, 1, 1)
            self.assertEqual(len(result["features"]), 2)
            self.assertEqual(method, "extracted_target_feature")
            self.assertEqual(len(sources[0]["sha256"]), 64)

    def test_complete_child_merge_only(self):
        parent = region("110000", "china")
        children = [region(), region("110200")]
        with self.assertRaises(tools.RegionError):
            tools.merge_children(parent, children, [feature()])
        merged = tools.merge_children(parent, children, [feature(), feature("110200", offset=3)])
        self.assertEqual(merged["features"][0]["geometry"]["type"], "MultiPolygon")

    def test_bundle_hashes_preserve_inputs_and_failure_marker(self):
        path = self.root / "geojson/110100.geojson"
        path.write_bytes(tools.encode(feature()))
        before = path.read_bytes()
        loader = lambda item: tools.load_download(self.root, item, {}, 1, 1)
        output = self.base / "bundle"
        report, status = tools.build_bundle(output, self.root, [self.item], loader, 1)
        self.assertEqual(status, 0)
        self.assertFalse((output / "INCOMPLETE").exists())
        self.assertEqual(path.read_bytes(), before)
        self.assertEqual(report["results"][0]["sha256"], tools.digest((output / "geojson/110100.geojson").read_bytes()))
        failed = self.base / "failed"
        with mock.patch.object(tools, "atomic_write", side_effect=OSError(errno.ENOSPC, "disk full")):
            with self.assertRaises(OSError):
                tools.build_bundle(failed, self.root, [self.item], loader, 1)
        self.assertTrue((failed / "INCOMPLETE").exists())
        cancelled = self.base / "cancelled"
        tools.CANCELLED.set()
        _, status = tools.build_bundle(cancelled, self.root, [self.item], loader, 1)
        self.assertEqual(status, 1)
        self.assertTrue((cancelled / "INCOMPLETE").exists())

    def test_mirror_missing_children_and_all_features(self):
        mirror = self.base / "mirror"
        mirror.mkdir()
        (mirror / "china.json").write_bytes(tools.encode({"features": [feature(), feature(offset=3)]}))
        source = tools.Mirror(mirror, [region("110000", "china"), self.item, region("110200")])
        self.assertEqual(len(source.load(self.item)[0]["features"]), 2)
        with self.assertRaisesRegex(tools.RegionError, "target_code_missing"):
            source.load(region("110000", "china"))


class HTTPContracts(unittest.TestCase):
    def test_retry_statuses_and_size_bound(self):
        requests = []
        class Handler(http.server.BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass
            def do_GET(self):
                requests.append(self.path)
                status = int(self.path.strip("/"))
                self.send_response(status)
                self.end_headers()
                self.wfile.write(b"x" * 1025)
        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            for status, count in [(404, 1), (403, 1), (501, 1), (429, 3), (503, 3)]:
                requests.clear()
                with self.assertRaises(tools.RegionError), mock.patch.object(tools.CANCELLED, "wait"):
                    tools.fetch_bytes(f"http://127.0.0.1:{server.server_port}/{status}", 1, 3)
                self.assertEqual(len(requests), count)
            with mock.patch.object(tools, "MAX_RESPONSE", 1024), self.assertRaisesRegex(tools.RegionError, "response_too_large"):
                tools.fetch_bytes(f"http://127.0.0.1:{server.server_port}/200", 1, 1)
        finally:
            server.shutdown()
            server.server_close()
            thread.join()

    def test_permanent_network_errors_are_not_retried(self):
        import ssl
        import urllib.error
        for reason in [ssl.SSLCertVerificationError(), ValueError("invalid URL")]:
            with mock.patch.object(tools.urllib.request, "urlopen", side_effect=urllib.error.URLError(reason)) as fetch:
                with self.assertRaises(tools.RegionError):
                    tools.fetch_bytes("https://invalid.example", 1, 3)
                self.assertEqual(fetch.call_count, 1)


if __name__ == "__main__":
    unittest.main()
