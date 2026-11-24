"""The DMARC IMAP retry patch is applied and tested before every build."""
from pathlib import Path
import re
import unittest

IMAGE = Path(__file__).resolve().parents[1] / 'dmarc-report-converter'
PATCH = IMAGE / 'patches' / 'imap-connect-retry.patch'


class DmarcPatchTests(unittest.TestCase):
    def setUp(self):
        self.dockerfile = (IMAGE / 'Dockerfile').read_text().replace('\\\n', '')
        self.patch = PATCH.read_text()

    def test_patch_touches_only_imap_sources(self):
        self.assertEqual(re.findall(r'^diff --git a/(\S+) b/\1$', self.patch, re.M),
                         ['cmd/dmarc-report-converter/imap.go',
                          'cmd/dmarc-report-converter/imap_connect_test.go'])

    def test_patch_retries_with_backoff_and_never_logs_out_nil_client(self):
        added = '\n'.join(line[1:] for line in self.patch.splitlines()
                          if line.startswith('+') and not line.startswith('+++'))
        self.assertIn('1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second', added)
        self.assertIn('connect attempt %d/%d to %v failed: %v; retrying in %v', added)
        removed = [line for line in self.patch.splitlines() if line.startswith('-\t')]
        self.assertIn('-\t\t\tc.Logout()', removed)

    def test_patch_is_applied_and_tested_before_build(self):
        run = next(line for line in self.dockerfile.splitlines()
                   if 'verify-source.sh https://' in line)
        self.assertIn('COPY patches/imap-connect-retry.patch /build/patches/imap-connect-retry.patch',
                      self.dockerfile)
        steps = ['verify-source.sh', 'git apply /build/patches/imap-connect-retry.patch',
                 'go vet -mod=vendor', 'go test -mod=vendor']
        positions = [run.index(step) for step in steps]
        self.assertEqual(positions, sorted(positions))
        self.assertNotIn('--reject', run)
        self.assertLess(self.dockerfile.index(run), self.dockerfile.index('go build -mod=vendor'))
