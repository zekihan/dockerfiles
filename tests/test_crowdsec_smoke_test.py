"""Exercise the CrowdSec release smoke gate with a synthetic Docker command."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
SMOKE_TEST = ROOT / 'crowdsec-agent' / 'smoke-test.sh'
QUERY_REJECTION = (
    'crowdsec-supervisor prepare-agent: /connection/connection.yaml: '
    'unexpected key "query" (only url, headers and auth are allowed)'
)

DOCKER = r'''#!/usr/bin/env python3
import os
from pathlib import Path
import sys

args = sys.argv[1:]
entrypoint = args[args.index('--entrypoint') + 1]
if entrypoint == 'crowdsec':
    print('version: vfixture')
elif entrypoint == 'crowdsec-supervisor':
    connection = next(arg.removesuffix(':/connection:ro') for arg in args
                      if arg.endswith(':/connection:ro'))
    if 'query:' in (Path(connection) / 'connection.yaml').read_text():
        print(os.environ['PREPARE_OUTPUT'], file=sys.stderr)
        sys.exit(int(os.environ['PREPARE_STATUS']))
'''


class CrowdsecSmokeTestTests(unittest.TestCase):
    def run_smoke(self, status, output):
        with tempfile.TemporaryDirectory() as temp:
            docker = Path(temp) / 'docker'
            docker.write_text(DOCKER)
            docker.chmod(0o755)
            return subprocess.run(
                ['bash', str(SMOKE_TEST), 'fixture/candidate'],
                env={**os.environ, 'PATH': temp + os.pathsep + os.environ['PATH'],
                     'TMPDIR': temp, 'PREPARE_STATUS': str(status), 'PREPARE_OUTPUT': output},
                capture_output=True, text=True, timeout=15)

    def test_unexpected_query_rejection_passes(self):
        result = self.run_smoke(1, QUERY_REJECTION)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('crowdsec-agent smoke test passed', result.stdout)

    def test_docker_run_failure_during_rejection_check_fails(self):
        error = 'docker: Error response from daemon: fixture failed to start.'
        result = self.run_smoke(125, error)
        self.assertNotEqual(result.returncode, 0, 'Docker run failure passed smoke test')
        self.assertIn(error, result.stderr)

    def test_unrelated_supervisor_error_during_rejection_check_fails(self):
        error = 'crowdsec-supervisor prepare-agent: /tmp_config: permission denied'
        result = self.run_smoke(1, error)
        self.assertNotEqual(result.returncode, 0, 'unrelated prepare error passed smoke test')
        self.assertIn(error, result.stderr)
