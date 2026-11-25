"""Offline release policy and mutable-tag regression tests; no registry access."""
import json
from pathlib import Path
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[1]
IMAGES = ('distroless', 'go-mmproxy', 'protonmailbridge', 'crowdsec-agent')


def yaml(path):
    return json.loads(subprocess.check_output(['yq', '-o=json', '.', str(path)]))


class PublishPolicyTests(unittest.TestCase):
    def test_publication_is_restricted_to_scheduled_or_manual_main(self):
        workflow = yaml(ROOT / '.github/workflows/publish.yaml')
        self.assertEqual(set(workflow['on']), {'schedule', 'workflow_dispatch'})
        self.assertEqual(workflow['permissions'], {'contents': 'read'})
        self.assertFalse(workflow['concurrency']['cancel-in-progress'])
        for image in (*IMAGES, 'crunchy-postgres-vectorchord'):
            key = image.replace('-', '_')
            decide = workflow['jobs'][key + '_decide']
            self.assertEqual(decide['with']['mode'], 'declarative')
            self.assertEqual(decide['with']['version-file'], f'{image}/config.yaml')
            self.assertEqual(decide['secrets'], {name: '${{ secrets.' + name + ' }}'
                                               for name in ['DOCKER_USERNAME', 'DOCKER_PASSWORD']})
            self.assertRegex(decide['uses'], r'decide.yaml@[0-9a-f]{40}$')


if __name__ == '__main__':
    unittest.main()
