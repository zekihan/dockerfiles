"""Caller gates remain local; archive verification is tested in release-workflows."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


def workflow(name):
    path = ROOT / '.github/workflows' / f'{name}.yaml'
    return json.loads(subprocess.check_output(['yq', '-o=json', '.', str(path)]))


class PublicationTests(unittest.TestCase):
    def test_every_publisher_depends_on_checks_and_the_pinned_decision(self):
        doc = workflow('publish')
        publishers = {name: job for name, job in doc['jobs'].items() if name.endswith('_publish')}
        self.assertEqual(len(publishers), 5)
        for name, job in publishers.items():
            key = name.removesuffix('_publish')
            self.assertEqual(job['needs'], [key + '_decide', key + '_checks'])
            self.assertRegex(job['uses'], r'image.yaml@[0-9a-f]{40}$')
            self.assertEqual(job['uses'].split('@')[1], doc['jobs'][key + '_decide']['uses'].split('@')[1])
            self.assertEqual(job['permissions'], {'contents': 'read', 'packages': 'write'})
            check = doc['jobs'][key + '_checks']
            self.assertEqual(check['permissions'], {'contents': 'read'})
            self.assertNotIn('secrets', check)
            self.assertEqual(check['needs'], key + '_decide')
            self.assertEqual(check['uses'], './.github/workflows/checks.yaml')
            self.assertEqual(check['with']['image'], job['with']['image'])
            self.assertEqual(check['with']['image'], job['with']['context'])
            self.assertEqual(job['with']['platforms'], '${{ needs.' + key + '_checks.outputs.platforms }}')
        bridge = publishers['protonmailbridge_publish']['with']
        self.assertTrue(bridge['native-arm'])
        postgres = publishers['crunchy_postgres_vectorchord_publish']['with']
        self.assertFalse(postgres['native-arm'])
        self.assertIn('smoke-test.sh', postgres['smoke-command'])
        crowdsec = publishers['crowdsec_agent_publish']['with']
        self.assertFalse(crowdsec['native-arm'])
        self.assertIn('crowdsec-agent/smoke-test.sh', crowdsec['smoke-command'])

    def test_native_checks_do_not_build_images(self):
        text = (ROOT / '.github/workflows/checks.yaml').read_text()
        self.assertNotIn('build-push-action', text)
        self.assertNotIn('setup-qemu', text)
        self.assertNotIn('docker build', text)


class BuildPlatformTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory(prefix='dockerfiles-platforms-')
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        self.output = self.root / 'github-output'
        (self.root / '.github').symlink_to(ROOT / '.github', target_is_directory=True)

    def config(self, image, content=None):
        directory = self.root / image
        directory.mkdir(exist_ok=True)
        (directory / 'Dockerfile').write_text('FROM scratch\n')
        if content is not None:
            (directory / 'config.yaml').write_text(content)

    def check_platforms(self, image=''):
        checks = workflow('checks')
        self.assertEqual(checks['on']['workflow_call']['outputs']['platforms']['value'],
                         '${{ jobs.check.outputs.platforms }}')
        job = checks['jobs']['check']
        self.assertEqual(job['outputs']['platforms'], '${{ steps.platforms.outputs.platforms }}')
        step = next(step for step in job['steps'] if step.get('id') == 'platforms')
        self.assertEqual(step['env']['PUBLISH_IMAGE'], '${{ inputs.image }}')
        return subprocess.run(['bash', '-e', '-c', step['run']], cwd=self.root,
                              env={**os.environ, 'PUBLISH_IMAGE': image, 'GITHUB_OUTPUT': str(self.output)},
                              capture_output=True, text=True, timeout=15)

    def test_publisher_uses_platforms_from_the_image_config(self):
        self.config('distroless', 'arch: linux/amd64\n')
        self.config('fixture-other', 'arch: linux/arm64\n')
        publisher = workflow('publish')['jobs']['distroless_publish']
        self.assertEqual(publisher['with']['platforms'], '${{ needs.distroless_checks.outputs.platforms }}')
        result = self.check_platforms('distroless')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.output.read_text(), 'platforms=linux/amd64\n')

    def test_pr_checks_reject_invalid_platform_configs(self):
        self.config('distroless', 'arch: linux/amd64,linux/arm64\n')
        for arch in ('', '[linux/arm64]', 'linux/amd64,linux/amd64',
                     'linux/amd64,', 'windows/amd64', r'"linux/amd64\nlinux/arm64"'):
            with self.subTest(arch=arch):
                self.config('fixture-other', f'arch: {arch}\n')
                result = self.check_platforms()
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('fixture-other/config.yaml: arch must list distinct Linux', result.stderr)
                self.assertFalse(self.output.exists())

    def test_publisher_rejects_missing_config(self):
        self.config('distroless')
        result = self.check_platforms('distroless')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('distroless/config.yaml', result.stderr)
        self.assertFalse(self.output.exists())

    def test_native_builds_require_their_platforms(self):
        for image, invalid, required in (
            ('crunchy-postgres-vectorchord', 'linux/amd64,linux/arm64', 'linux/amd64'),
            ('protonmailbridge', 'linux/arm64', 'linux/amd64,linux/arm64'),
        ):
            with self.subTest(image=image):
                self.config(image, f'arch: {invalid}\n')
                result = self.check_platforms(image)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(f'{image}/config.yaml: arch must build {required}', result.stderr)
                self.assertFalse(self.output.exists())
