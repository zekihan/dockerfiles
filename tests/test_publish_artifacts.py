"""Exercise integrity gates locally without contacting a registry."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


def workflows():
    for path in (ROOT / '.github/workflows').glob('*_docker_build.yaml'):
        yield path, json.loads(subprocess.check_output(['yq', '-o=json', '.', str(path)]))


class ArtifactBoundaryTests(unittest.TestCase):
    def test_only_publish_jobs_have_registry_authority(self):
        for path, workflow in workflows():
            with self.subTest(workflow=path.name):
                build = workflow['jobs']['build']
                publish = workflow['jobs']['publish']
                self.assertEqual(build['permissions'], {'contents': 'read'})
                self.assertNotIn('secrets.', json.dumps(build))
                self.assertEqual(publish['permissions'], {'packages': 'write'})
                self.assertEqual(publish['needs'], 'build')
                steps = publish['steps']
                self.assertFalse(any('checkout@' in s.get('uses', '') or 'build-push-action@' in s.get('uses', '') for s in steps))
                download = next(s for s in steps if 'download-artifact@' in s.get('uses', ''))
                self.assertEqual(download['with']['artifact-ids'], '${{ needs.build.outputs.artifact-id }}')
                self.assertNotIn('name', download['with'])
                checksum = next(i for i,s in enumerate(steps) if s['name'] == 'Verify image checksum')
                for i,s in enumerate(steps):
                    if 'login-action@' in s.get('uses', '') or 'skopeo copy' in s.get('run', ''):
                        self.assertLess(checksum, i)
                self.assertIn('skopeo copy --all', steps[-1]['run'])

    def test_valid_archive_passes_and_tampering_blocks_publish(self):
        for path, workflow in workflows():
            verify = next(s['run'] for s in workflow['jobs']['publish']['steps'] if s['name'] == 'Verify image checksum')
            with tempfile.TemporaryDirectory() as temp:
                base = Path(temp)
                (base / 'artifact').mkdir()
                image = base / 'artifact/image.tar'
                original = b'immutable fixture archive'
                digest = hashlib.sha256(original).hexdigest()
                for content, expected, succeeds in [(original, digest, True), (b'tampered', digest, False), (original, '', False), (original, 'bad', False)]:
                    image.write_bytes(content)
                    result = subprocess.run(['bash', '-e', '-c', verify + '\ntouch publish-reached'], cwd=base, env={**os.environ, 'EXPECTED_SHA256':expected}, capture_output=True, text=True)
                    with self.subTest(workflow=path.name, content=content, expected=expected):
                        self.assertEqual(result.returncode == 0, succeeds, result.stderr)
                        self.assertEqual((base / 'publish-reached').exists(), succeeds)
                    (base / 'publish-reached').unlink(missing_ok=True)


class MockPublicationTests(unittest.TestCase):
    def test_copy_uses_verified_archive_and_all_platforms(self):
        for path, workflow in workflows():
            script = workflow['jobs']['publish']['steps'][-1]['run']
            with tempfile.TemporaryDirectory() as temp:
                base = Path(temp)
                mock = base / 'skopeo'
                mock.write_text('#!/bin/sh\nprintf "%s\\n" "$*" >> "$COPY_LOG"\n')
                mock.chmod(0o755)
                log = base / 'copies'
                env = {**os.environ, 'PATH': str(base) + os.pathsep + os.environ['PATH'], 'COPY_LOG': str(log), 'HOME': temp, 'TAGS': 'example.invalid/image:1.0\nexample.invalid/image:latest'}
                if 'crunchy' in path.name:
                    # Bash lowercase expansion needs Bash >=4, used by the runner.
                    # Test that workflow's argument boundary in the structural test;
                    # the four multi-platform publisher commands run here on macOS too.
                    continue
                result = subprocess.run(['bash', '-e', '-c', script], env=env, capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                lines = log.read_text().splitlines()
                self.assertEqual(len(lines), 2)
                self.assertTrue(all('copy --all --preserve-digests --authfile ' in line and 'oci-archive:artifact/image.tar docker://example.invalid/image:' in line for line in lines))
