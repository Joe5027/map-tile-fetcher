import pathlib
import subprocess
import tempfile
import unittest
import zipfile
import tarfile
from build_release import metadata, archive_package, allowed_source


class BuildContracts(unittest.TestCase):
    def test_versions_clean_tag_dirty_and_dev(self):
        with tempfile.TemporaryDirectory() as directory:
            root=pathlib.Path(directory)
            def git(*args):
                return subprocess.check_output(["git",*args],cwd=root,stderr=subprocess.PIPE)
            git("init")
            git("config","user.email","fixture@example.invalid")
            git("config","user.name","Fixture")
            (root/"source.txt").write_text("source")
            git("add","source.txt")
            git("-c","core.hooksPath=/dev/null","commit","-m","fixture")
            info,_=metadata(root,"HEAD")
            self.assertTrue(info["version"].startswith("dev+"))
            git("tag","v9.8.7")
            self.assertEqual(metadata(root,"HEAD")[0]["version"],"v9.8.7")
            (root/"source.txt").write_text("dirty")
            with self.assertRaisesRegex(ValueError,"dirty"):
                metadata(root,"HEAD")
            self.assertTrue(metadata(root,"HEAD",True)[0]["version"].endswith(".dirty"))

    def test_archives_repeat_with_permissions(self):
        files={"tiler":(b"binary",0o755),"geojson/example.geojson":(b"{}",0o644)}
        with tempfile.TemporaryDirectory() as directory:
            root=pathlib.Path(directory)
            for suffix in [".zip",".tar.gz"]:
                first=archive_package(files,root/("a"+suffix),1700000000)
                second=archive_package(dict(reversed(list(files.items()))),root/("b"+suffix),1700000000)
                self.assertEqual(first,second)
            with tarfile.open(root/"a.tar.gz") as archive:
                self.assertEqual(archive.getmember("tiler").mode,0o755)
            with zipfile.ZipFile(root/"a.zip") as archive:
                self.assertEqual(archive.getinfo("tiler").external_attr>>16 & 0o777,0o755)

    def test_source_allowlist_excludes_user_data_and_tests(self):
        prefix="apps/admin-region-tiler/"
        for name in [".env","data/tiler.db","tmp/report.json","output/tiles.zip","foo_test.go","scripts/test.html","geojson/materialize-report.json"]:
            self.assertFalse(allowed_source(prefix+name),name)
        for name in ["main.go","internal/area/area.go","static/index.html","geojson/regions.json","conf.toml"]:
            self.assertTrue(allowed_source(prefix+name),name)
