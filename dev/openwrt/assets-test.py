#!/usr/bin/env python3
"""Check lazy-loaded assets survive one update without growing indefinitely."""
import pathlib
import subprocess
import tempfile
import unittest

INSTALLER = pathlib.Path(__file__).with_name("install-assets.sh")


class AssetRetentionTest(unittest.TestCase):
    def test_updates_keep_previous_lazy_chunks_and_remove_older_files(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            public, private = root / "public", root / "private"
            for generation in ("first", "second", "third"):
                source = root / generation
                (source / "assets").mkdir(parents=True)
                (source / "index.html").write_text(generation)
                (source / "assets" / f"lazy-{generation}.js").write_text(generation)
                (source / "assets/shared.webp").write_bytes(b"shared")
                subprocess.run(["sh", str(INSTALLER), str(source), str(public), str(private)], check=True)
                self.assertEqual((private / "index.html").read_text(), generation)
            self.assertFalse((public / "index.html").exists())
            self.assertEqual({p.name for p in (public / "assets").iterdir()},
                             {"lazy-second.js", "lazy-third.js", "shared.webp"})

    def test_legacy_files_expire_after_adoption_and_next_deployment(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            source, public, private = root / "source", root / "public", root / "private"
            (source / "assets").mkdir(parents=True)
            (source / "index.html").write_text("new")
            (source / "assets/current.js").write_text("new")
            (public / "assets").mkdir(parents=True)
            old = public / "assets/legacy-lazy.js"
            old.write_text("old")
            command = ["sh", str(INSTALLER), str(source), str(public), str(private)]
            subprocess.run(command, check=True)
            self.assertTrue(old.exists())
            subprocess.run(command, check=True)
            self.assertFalse(old.exists())
            self.assertTrue((public / "assets/current.js").exists())


if __name__ == "__main__":
    unittest.main()
