"""Every image config states its versioning profile and matches it."""
from pathlib import Path
import re
import unittest

import yaml

ROOT = Path(__file__).resolve().parents[1]


class VersioningTests(unittest.TestCase):
    def test_each_image_declares_a_consistent_profile(self):
        configs = sorted(ROOT.glob('*/config.yaml'))
        self.assertTrue(configs)
        for path in configs:
            with self.subTest(image=path.parent.name):
                config = yaml.safe_load(path.read_text())
                self.assertIn(config.get('versioning'), ('upstream', 'own'))
                if config['versioning'] == 'upstream':
                    self.assertIsInstance(config.get('revision'), int)
                    self.assertGreater(config['revision'], 0)
                else:
                    self.assertNotIn('revision', config)
                    self.assertRegex(str(config['version']), r'^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$')


if __name__ == '__main__':
    unittest.main()
