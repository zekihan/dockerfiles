"""Offline release policy and mutable-tag regression tests; no registry access."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
IMAGES = ('distroless', 'dmarc-report-converter', 'go-mmproxy', 'protonmailbridge')


def yaml(path):
    return json.loads(subprocess.check_output(['yq', '-o=json', '.', str(path)]))


class PublishPolicyTests(unittest.TestCase):
    def test_publication_is_restricted_to_main_and_allowed_events(self):
        for image in IMAGES:
            workflow = yaml(ROOT / '.github/workflows' / f'{image}_docker_build.yaml')
            self.assertNotIn('packages', workflow['permissions'])
            self.assertNotIn('DOCKER_PASSWORD', workflow['env'])
            # Discover every credentialed job, so moving a login or push cannot
            # silently escape the tested boundary.
            publication_steps = 0
            for job in workflow['jobs'].values():
                steps = job['steps']
                credentialed = [s for s in steps if 'login-action' in s.get('uses', '')
                                or s.get('with', {}).get('push') is True]
                publication_steps += len(credentialed)
                if not credentialed:
                    continue
                guard = job['if'].replace('&&', ' and ').replace('||', ' or ')
                for ref in ('refs/heads/main', 'refs/heads/feature', 'refs/tags/main'):
                    for event in ('push', 'workflow_dispatch', 'pull_request',
                                  'pull_request_target', 'schedule', 'workflow_run'):
                        expression = guard.replace('github.ref', repr(ref)).replace(
                            'github.event_name', repr(event))
                        allowed = eval(expression, {'__builtins__': {}}, {})
                        with self.subTest(image=image, ref=ref, event=event):
                            self.assertEqual(allowed, ref == 'refs/heads/main' and
                                             event in ('push', 'workflow_dispatch'))
                checkout = next(s for s in steps if 'actions/checkout@' in s.get('uses', ''))
                self.assertEqual(checkout['with']['ref'], '${{ github.sha }}')
                self.assertFalse(checkout['with']['persist-credentials'])
                self.assertTrue(any('GITHUB_STEP_SUMMARY' in s.get('run', '') and
                                    'GITHUB_SHA' in s['run'] for s in steps))
                login = next(s for s in steps if s['name'] == 'Login to Docker Hub')
                self.assertEqual(login['with']['password'], '${{ secrets.DOCKER_PASSWORD }}')
            self.assertEqual(publication_steps, 3)


class SourcePinTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.upstream = self.base / 'upstream'
        self.upstream.mkdir()
        self.git('init', '--quiet')
        self.git('config', 'commit.gpgsign', 'false')
        self.git('config', 'tag.gpgsign', 'false')
        self.git('config', 'user.name', 'Source fixture')
        self.git('config', 'user.email', 'fixture@example.invalid')
        self.commit('reviewed')
        self.expected = self.git('rev-parse', 'HEAD')
        self.git('tag', 'v0.8.1')

    def git(self, *args):
        return subprocess.check_output(['git', '-C', str(self.upstream), *args],
                                       text=True).strip()

    def commit(self, content):
        (self.upstream / 'source').write_text(content)
        self.git('add', 'source')
        self.git('commit', '--quiet', '-m', content)

    def verify(self, destination, expected=None):
        return subprocess.run(['sh', str(ROOT / 'dmarc-report-converter/verify-source.sh'),
                               self.upstream.as_uri(), '0.8.1',
                               self.expected if expected is None else expected,
                               str(destination)], capture_output=True, text=True,
                              env={**os.environ, 'GIT_CONFIG_NOSYSTEM': '1'})

    def test_repeat_fetches_have_identical_verified_identity(self):
        for name in ('first', 'second'):
            destination = self.base / name
            result = self.verify(destination)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual((destination / 'source-revision').read_text(), self.expected + '\n')
            self.assertEqual((destination / 'source').read_text(), 'reviewed')

    def test_annotated_tag_is_peeled_to_commit(self):
        self.git('tag', '--force', '--annotate', 'v0.8.1', '-m', 'release')
        result = self.verify(self.base / 'annotated')
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_retargeted_tag_fails_before_source_checkout(self):
        self.commit('unreviewed')
        self.git('tag', '--force', 'v0.8.1')
        destination = self.base / 'moved'
        result = self.verify(destination)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Source commit mismatch', result.stderr)
        self.assertFalse((destination / 'source-revision').exists())
        self.assertFalse((destination / 'source').exists())

    def test_missing_or_abbreviated_pin_fails_closed(self):
        for pin in ('', 'null', 'abc123', 'g' * 40):
            with self.subTest(pin=pin):
                self.assertNotEqual(self.verify(self.base / 'invalid', pin).returncode, 0)

    def test_docker_build_uses_verifier_before_compilation(self):
        dockerfile = (ROOT / 'dmarc-report-converter/Dockerfile').read_text()
        self.assertLess(dockerfile.index('sh /usr/local/bin/verify-source.sh'),
                        dockerfile.index('go build'))
        self.assertIn("yq '.sourceCommit' config.yaml", dockerfile)
        self.assertIn('/usr/share/dmarc-report-converter/source-revision', dockerfile)
        self.assertRegex(yaml(ROOT / 'dmarc-report-converter/config.yaml')['sourceCommit'],
                         r'^[0-9a-f]{40}$')


if __name__ == '__main__':
    unittest.main()
