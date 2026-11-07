"""crowdsec-agent extends upstream CrowdSec without changing it: only files are added."""
import json
from pathlib import Path
import re
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[1]
IMAGE = ROOT / 'crowdsec-agent'
PINNED = r'[a-z0-9./_-]+:[A-Za-z0-9._-]+@sha256:[0-9a-f]{64}'


class CrowdsecAgentTests(unittest.TestCase):
    def setUp(self):
        self.dockerfile = (IMAGE / 'Dockerfile').read_text()
        self.final = self.dockerfile.split(' AS final')[1]

    def test_bases_are_pinned_and_final_matches_declared_version(self):
        bases = re.findall(r'(?m)^FROM (?:--platform=\S+ )?(\S+)', self.dockerfile)
        self.assertEqual(len(bases), 2)
        for base in bases:
            self.assertRegex(base, '^' + PINNED + '$')
        config = json.loads(subprocess.check_output(['yq', '-o=json', '.', str(IMAGE / 'config.yaml')]))
        self.assertTrue(bases[1].startswith(f'docker.io/crowdsecurity/crowdsec:{config["version"]}@'))
        self.assertIsInstance(config['revision'], int)
        self.assertGreater(config['revision'], 0)
        self.assertEqual(config['arch'], 'linux/amd64,linux/arm64')
        self.assertIn('# renovate: depName=crowdsecurity/crowdsec datasource=docker',
                      (IMAGE / 'config.yaml').read_text())

    def test_final_stage_only_adds_files(self):
        # No RUN keeps arm64 cross-builds free of emulation; no ENTRYPOINT/CMD/USER/ENV
        # keeps upstream docker_start.sh and its PATH in charge.
        self.assertNotRegex(self.final, r'(?m)^(RUN|ADD|ENTRYPOINT|CMD|USER|ENV)\b')
        self.assertIn('COPY --from=builder /out/ /', self.final)
        self.assertIn('ln -s /usr/local/bin/crowdsec-supervisor /out/usr/local/sbin/crowdsec',
                      self.dockerfile)
        self.assertIn('/out/usr/local/lib/crowdsec-supervisor/apply-whitelists.sh', self.dockerfile)

    def test_supervisor_is_static_and_tested_before_it_is_built(self):
        self.assertIn('CGO_ENABLED=0', self.dockerfile)
        self.assertLess(self.dockerfile.index('go test -count=1 ./...'),
                        self.dockerfile.index('-o /out/usr/local/bin/crowdsec-supervisor'))

    def test_supervisor_depends_only_on_a_yaml_library(self):
        gomod = (IMAGE / 'supervisor/go.mod').read_text()
        requires = re.findall(r'(?m)^require (\S+) ', gomod)
        self.assertEqual(requires, ['go.yaml.in/yaml/v3'])

    def test_labels_and_notices(self):
        for label in ('authors', 'vendor', 'licenses', 'source', 'title', 'description'):
            self.assertIn(f'org.opencontainers.image.{label}=', self.final)
        notices = {p.name for p in (IMAGE / 'licenses').iterdir()}
        self.assertTrue({'NOTICE.md', 'crowdsec-LICENSE', 'AGPL-3.0-only.txt'} <= notices)
        self.assertIn('COPY licenses/ /usr/share/licenses/dockerfiles/', self.final)

    def test_go_unit_tests_pass(self):
        try:
            result = subprocess.run(['go', 'test', '-count=1', './...'], cwd=IMAGE / 'supervisor',
                                    capture_output=True, text=True)
        except FileNotFoundError:
            self.skipTest('go is not installed')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)


if __name__ == '__main__':
    unittest.main()
