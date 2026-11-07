"""Caller gates remain local; archive verification is tested in release-workflows."""
import unittest
from pathlib import Path
import json
import subprocess

ROOT = Path(__file__).resolve().parents[1]

class PublicationTests(unittest.TestCase):
    def test_every_publisher_depends_on_checks_and_the_pinned_decision(self):
        doc = json.loads(subprocess.check_output(['yq', '-o=json', '.', str(ROOT / '.github/workflows/publish.yaml')]))
        publishers = {name: job for name, job in doc['jobs'].items() if name.endswith('_publish')}
        self.assertEqual(len(publishers), 6)
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
        bridge = publishers['protonmailbridge_publish']['with']
        self.assertTrue(bridge['native-arm'])
        postgres = publishers['crunchy_postgres_vectorchord_publish']['with']
        self.assertEqual(postgres['platforms'], 'linux/amd64')
        self.assertIn('smoke-test.sh', postgres['smoke-command'])
        crowdsec = publishers['crowdsec_agent_publish']['with']
        self.assertEqual(crowdsec['platforms'], 'linux/amd64,linux/arm64')
        self.assertFalse(crowdsec['native-arm'])
        self.assertIn('crowdsec-agent/smoke-test.sh', crowdsec['smoke-command'])

    def test_native_checks_do_not_build_images(self):
        text = (ROOT / '.github/workflows/checks.yaml').read_text()
        self.assertNotIn('build-push-action', text)
        self.assertNotIn('setup-qemu', text)
        self.assertNotIn('docker build', text)
