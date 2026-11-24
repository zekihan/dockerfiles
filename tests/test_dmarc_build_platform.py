"""Exercise the DMARC Dockerfile's build command without fetching or compiling."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

IMAGE = Path(__file__).resolve().parents[1] / 'dmarc-report-converter'


class DmarcBuildPlatformTests(unittest.TestCase):
    def test_build_uses_target_architecture_and_arm_variant(self):
        instructions = (IMAGE / 'Dockerfile').read_text().replace('\\\n', '').splitlines()
        build = next(line.removeprefix('RUN ') for line in instructions
                     if line.startswith('RUN VERSION='))
        build = build.replace('/tmp/repo', './repo')
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            shutil.copyfile(IMAGE / 'config.yaml', root / 'config.yaml')
            (root / 'repo').mkdir()
            go = root / 'go'
            go.write_text('#!/bin/sh\nprintf "%s/%s:%s\n" "$GOOS" "$GOARCH" "$GOARM"\n')
            go.chmod(0o755)
            for arch, variant, goarm in (('amd64', '', ''), ('arm64', '', ''),
                                         ('arm64', 'v9', ''), ('arm', 'v7', '7'),
                                         ('arm', 'v6', '6')):
                platform = '/'.join(filter(None, ('linux', arch, variant)))
                with self.subTest(platform=platform):
                    result = subprocess.run(
                        ['sh', '-c', build], cwd=root,
                        env={'PATH': directory + os.pathsep + os.environ['PATH'],
                             'TARGETPLATFORM': platform, 'TARGETOS': 'linux',
                             'TARGETARCH': arch, 'TARGETVARIANT': variant},
                        capture_output=True, text=True, timeout=10)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(result.stdout.strip(), f'linux/{arch}:{goarm}')
