"""go-mmproxy keeps its scratch runtime: the entrypoint helper replaces any shell."""
import json
from pathlib import Path
import re
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[1]
IMAGE = ROOT / 'go-mmproxy'


class EntrypointTests(unittest.TestCase):
    def setUp(self):
        self.dockerfile = (IMAGE / 'Dockerfile').read_text()

    def test_final_image_is_scratch_with_the_helper_as_entrypoint(self):
        final = self.dockerfile.split('FROM scratch AS final')[1]
        self.assertIn('ENTRYPOINT ["/usr/local/bin/mmproxy-entrypoint"]', final)
        self.assertIn('/usr/local/bin/go-mmproxy', final)
        self.assertNotRegex(final, r'(?m)^(RUN|ADD)\b')
        for pkg in ('busybox', 'iproute', 'apk add', 'apt-get'):
            self.assertNotIn(pkg, final)

    def test_helper_is_tested_before_it_is_built(self):
        self.assertIn('COPY entrypoint/ /build/entrypoint/', self.dockerfile)
        self.assertLess(self.dockerfile.index('go test -count=1 ./...'),
                        self.dockerfile.index('-o /mmproxy-entrypoint'))

    def test_helper_has_no_third_party_dependencies(self):
        gomod = (IMAGE / 'entrypoint/go.mod').read_text()
        self.assertNotRegex(gomod, r'(?m)^\s*require\b')
        self.assertFalse((IMAGE / 'entrypoint/go.sum').exists())

    def test_routing_is_opt_in_and_idempotent(self):
        main = (IMAGE / 'entrypoint/main.go').read_text()
        self.assertIn('MMPROXY_SETUP_ROUTING', main)
        self.assertNotRegex(main, r'enabled:\s*true')
        netlink = (IMAGE / 'entrypoint/netlink_linux.go').read_text()
        self.assertRegex(netlink, r'nlmFCreate\|nlmFReplace')
        self.assertRegex(netlink, r'nlmFCreate\|nlmFExcl')
        self.assertIn('syscall.EEXIST', netlink)

    def test_go_unit_tests_pass(self):
        result = subprocess.run(['go', 'test', '-count=1', './...'], cwd=IMAGE / 'entrypoint',
                                capture_output=True, text=True)
        if result.returncode and 'executable file not found' in result.stderr:
            self.skipTest('go is not installed')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_revision_is_a_positive_integer(self):
        doc = json.loads(subprocess.check_output(['yq', '-o=json', '.', str(IMAGE / 'config.yaml')]))
        self.assertIsInstance(doc['revision'], int)
        self.assertGreater(doc['revision'], 0)


if __name__ == '__main__':
    unittest.main()
