"""Run the smoke-test shell with fake Docker/Postgres commands, never a daemon."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
IMAGE = ROOT / 'crunchy-postgres-vectorchord'

# The fake database can retain its rows yet return a wrong neighbor after restore
# or restart. SQL succeeds unless the caller actually asserts the result.
COMMAND = r'''#!/usr/bin/env python3
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

root = Path(os.environ['MOCK_ROOT'])
name = Path(sys.argv[0]).name
args = sys.argv[1:]
with (root / 'commands').open('a') as log:
    log.write(json.dumps([name, *args]) + '\n')
if name == 'docker':
    if args[0] == 'create':
        version = next(a.split('=', 1)[1] for a in args if a.startswith('EXPECTED_VERSION='))
        (root / 'version').write_text(version)
        print('fixture-container')
    elif args[0] == 'cp':
        shutil.copyfile(args[1], root / args[2].split(':/')[1])
    elif args[0] == 'start':
        env = dict(os.environ, EXPECTED_VERSION=(root / 'version').read_text())
        sys.exit(subprocess.run(['bash', '-s'], input=sys.stdin.read(), text=True, env=env).returncode)
    elif args[0] == 'inspect':
        print('0')
elif name in ('pg_ctl', 'pg_restore'):
    phase = 'restore' if name == 'pg_restore' else ('restart' if 'restart' in args else '')
    if phase == os.environ['FAULT']:
        (root / 'corrupt').touch()
elif name == 'pg_config':
    print('PostgreSQL ' + (root / 'version').read_text().split('-vectorchord')[0])
elif name == 'psql':
    if '-f' in args:
        sql = (root / Path(args[args.index('-f') + 1]).name).read_text()
    else:
        sql = args[args.index('-c') + 1]
    if 'DROP TABLE' in sql:
        (root / 'corrupt').unlink(missing_ok=True)
    if 'RAISE EXCEPTION' in sql and (root / 'corrupt').exists():
        print('Unexpected nearest neighbor: 499', file=sys.stderr)
        sys.exit(1)
    if 'SELECT extversion' in sql:
        print((root / 'version').read_text().split('-vectorchord')[1])
    elif 'SELECT count(*)' in sql:
        print('1000')
    elif 'SELECT id' in sql:
        print('499' if (root / 'corrupt').exists() else '500')
# initdb, pg_dump, createdb, cat (fixture logs only), and docker rm are no-ops.
'''


class SmokeTestTests(unittest.TestCase):
    def run_smoke(self, fault):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            for name in ('docker', 'initdb', 'pg_ctl', 'psql', 'pg_config',
                         'pg_dump', 'createdb', 'pg_restore', 'cat'):
                command = root / name
                command.write_text(COMMAND)
                command.chmod(0o755)
            result = subprocess.run(
                ['bash', str(IMAGE / 'smoke-test.sh'), 'fixture/candidate'],
                env={**os.environ, 'PATH': temp + os.pathsep + os.environ['PATH'],
                     'MOCK_ROOT': temp, 'FAULT': fault},
                capture_output=True, text=True, timeout=15)
            return result, (root / 'commands').read_text()

    def test_healthy_image_passes_and_container_is_removed(self):
        result, commands = self.run_smoke('none')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('["docker", "rm", "-f", "fixture-container"]', commands)

    def test_restart_corruption_fails_without_reseeding(self):
        result, commands = self.run_smoke('restart')
        self.assertNotEqual(result.returncode, 0, 'restart corruption passed smoke test')
        self.assertIn('Unexpected nearest neighbor', result.stderr)
        self.assertIn('["docker", "rm", "-f", "fixture-container"]', commands)

    def test_restored_wrong_neighbor_fails_despite_intact_row_count(self):
        result, commands = self.run_smoke('restore')
        self.assertNotEqual(result.returncode, 0, 'incorrect restored query result passed smoke test')
        self.assertIn('Unexpected nearest neighbor', result.stderr)
        self.assertNotIn('"restart"', commands)
        self.assertIn('["docker", "rm", "-f", "fixture-container"]', commands)
