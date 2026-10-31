#!/usr/bin/env python3
"""Unit tests for ops/manifest.py."""

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent / "manifest.py"


class ManifestToolTests(unittest.TestCase):
    def test_create_and_verify_acr_and_bundle_modes(self) -> None:
        sha = "a" * 40
        digest_a = "sha256:" + ("1" * 64)
        digest_c = "sha256:" + ("2" * 64)

        with tempfile.TemporaryDirectory() as tmp:
            d = Path(tmp)
            (d / "avalon.json").write_text(
                json.dumps({"service": "avalon", "image": f"ghcr.io/zzqdeco/teamie-avalon@{digest_a}"}),
                encoding="utf-8",
            )
            (d / "caddy.json").write_text(
                json.dumps({"service": "caddy", "image": f"caddy:2.11.7-alpine@{digest_c}"}),
                encoding="utf-8",
            )

            out = subprocess.check_output(
                [
                    sys.executable,
                    str(SCRIPT),
                    "create",
                    "--repository",
                    "zzqDeco/teamie-avalon",
                    "--version",
                    "v0.1.0",
                    "--sha",
                    sha,
                    "--main-sha",
                    sha,
                    "--digests",
                    str(d),
                ],
                text=True,
            )
            manifest_path = d / "release-manifest.json"
            manifest_path.write_text(out, encoding="utf-8")

            env_out = subprocess.check_output(
                [
                    sys.executable,
                    str(SCRIPT),
                    "verify",
                    "--manifest",
                    str(manifest_path),
                    "--print-env",
                ],
                text=True,
            )
            self.assertIn("RELEASE_VERSION=v0.1.0", env_out)
            self.assertIn(f"AVALON_IMAGE=ghcr.io/zzqdeco/teamie-avalon@{digest_a}", env_out)


if __name__ == "__main__":
    unittest.main()
