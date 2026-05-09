#!/usr/bin/env python3
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[1]
SCRIPT = REPO_ROOT / "scripts" / "generate-manifest.py"


class GenerateManifestTests(unittest.TestCase):
    def run_manifest(self, version, filenames, *extra_args):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            artifacts_dir = tmp_path / "artifacts"
            artifacts_dir.mkdir()
            checksums_path = tmp_path / "checksums.txt"
            output_path = tmp_path / "manifest.json"

            checksums = []
            for index, filename in enumerate(filenames, start=1):
                artifact_path = artifacts_dir / filename
                artifact_path.write_bytes(f"artifact-{index}".encode())
                checksums.append(f"{index:064x}  {filename}\n")
            checksums_path.write_text("".join(checksums))

            cmd = [
                sys.executable,
                str(SCRIPT),
                "--version",
                version,
                "--checksums",
                str(checksums_path),
                "--artifacts-dir",
                str(artifacts_dir),
                "--base-url",
                "https://dl.lynxdb.org",
                "--output",
                str(output_path),
                *extra_args,
            ]
            result = subprocess.run(cmd, text=True, capture_output=True, check=False)
            manifest = None
            if output_path.exists():
                manifest = json.loads(output_path.read_text())
            return result, manifest

    def test_stable_artifact_parses(self):
        result, manifest = self.run_manifest("v0.7.0", ["lynxdb-v0.7.0-linux-amd64.tar.gz"])

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(manifest["channel"], "stable")
        self.assertIn("linux-amd64", manifest["artifacts"])
        self.assertEqual(
            manifest["artifacts"]["linux-amd64"]["url"],
            "https://dl.lynxdb.org/v0.7.0/lynxdb-v0.7.0-linux-amd64.tar.gz",
        )

    def test_rc_artifact_parses_with_override(self):
        result, manifest = self.run_manifest(
            "v0.7.0-rc.1",
            ["lynxdb-v0.7.0-rc.1-linux-amd64.tar.gz"],
            "--allow-stable-prerelease",
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(manifest["version"], "v0.7.0-rc.1")
        self.assertIn("linux-amd64", manifest["artifacts"])

    def test_nightly_artifact_parses(self):
        version = "v0.7.0-nightly.20260509.g1a2b3c4"
        result, manifest = self.run_manifest(
            version,
            ["lynxdb-v0.7.0-nightly.20260509.g1a2b3c4-linux-amd64.tar.gz"],
            "--channel",
            "nightly",
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(manifest["channel"], "nightly")
        self.assertEqual(manifest["source_commit"], "1a2b3c4")
        self.assertIn("linux-amd64", manifest["artifacts"])

    def test_stable_channel_rejects_nightly_without_override(self):
        result, _ = self.run_manifest(
            "v0.7.0-nightly.20260509.g1a2b3c4",
            ["lynxdb-v0.7.0-nightly.20260509.g1a2b3c4-linux-amd64.tar.gz"],
        )

        self.assertEqual(result.returncode, 2)
        self.assertIn("stable channel cannot be used", result.stderr)

    def test_missing_required_platforms_warns(self):
        result, _ = self.run_manifest("v0.7.0", ["lynxdb-v0.7.0-linux-amd64.tar.gz"])

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Missing expected platform: linux-arm64", result.stderr)
        self.assertIn("Missing expected platform: darwin-amd64", result.stderr)
        self.assertIn("Missing expected platform: darwin-arm64", result.stderr)

    def test_zero_matching_artifacts_exits_nonzero(self):
        result, manifest = self.run_manifest("v0.7.0", ["lynxdb-v0.8.0-linux-amd64.tar.gz"])

        self.assertEqual(result.returncode, 1)
        self.assertIsNone(manifest)
        self.assertIn("no release artifacts matched", result.stderr)


if __name__ == "__main__":
    unittest.main()
