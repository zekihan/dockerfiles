"""Run the dependency installer with isolated commands, without sudo or network."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
INSTALLER = ROOT / '.github/scripts/install_build_deps.sh'


class InstallBuildDepsTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.bin = Path(self.temp.name)
        self.log = self.bin / 'commands.log'
        self.shells = [shutil.which('sh'), shutil.which('bash')]
        # An exclusive PATH prevents accidental package installation or downloads.
        (self.bin / 'dirname').symlink_to(shutil.which('dirname'))
        self.command('uname', 'echo x86_64')
        self.command('yq', 'echo yq-fixture')
        self.command('jq', 'exit 0')
        self.command('sudo', '''
printf '%s\\n' "$*" >> "$COMMAND_LOG"
case "$*" in
    'apt-get update') exit "${UPDATE_STATUS:-0}" ;;
    'apt-get install jq -y') exit "${INSTALL_STATUS:-0}" ;;
    *) echo 'Unexpected privileged command' >&2; exit 99 ;;
esac
''')

    def command(self, name, body):
        path = self.bin / name
        path.write_text('#!/bin/sh\n' + body + '\n')
        path.chmod(0o755)

    def run_installer(self, shell, **env):
        return subprocess.run(
            [shell, str(INSTALLER)], capture_output=True, text=True,
            env={**os.environ, 'PATH': str(self.bin),
                 'COMMAND_LOG': str(self.log), **env})

    def test_broken_existing_yq_fails(self):
        self.command('yq', 'echo "yq fixture failed" >&2; exit 19')
        for shell in self.shells:
            with self.subTest(shell=shell):
                result = self.run_installer(shell)
                self.assertEqual(result.returncode, 19, result.stderr)
                self.assertNotIn('Installed yq version:', result.stdout)
                self.assertFalse(self.log.exists())

    def test_failed_apt_update_stops_before_install(self):
        (self.bin / 'jq').unlink()
        for shell in self.shells:
            with self.subTest(shell=shell):
                self.log.unlink(missing_ok=True)
                result = self.run_installer(shell, UPDATE_STATUS='23')
                self.assertEqual(result.returncode, 23, result.stderr)
                self.assertEqual(self.log.read_text().splitlines(), ['apt-get update'])
                self.assertNotIn('Installed yq version:', result.stdout)

    def test_apt_install_status_is_preserved(self):
        (self.bin / 'jq').unlink()
        for shell in self.shells:
            for status in ('0', '24'):
                with self.subTest(shell=shell, status=status):
                    self.log.unlink(missing_ok=True)
                    result = self.run_installer(shell, INSTALL_STATUS=status)
                    self.assertEqual(result.returncode, int(status), result.stderr)
                    self.assertEqual(self.log.read_text().splitlines(),
                                     ['apt-get update', 'apt-get install jq -y'])
                    self.assertEqual('Installed yq version: yq-fixture' in result.stdout,
                                     status == '0')

    def test_existing_tools_need_no_install(self):
        for shell in self.shells:
            with self.subTest(shell=shell):
                result = self.run_installer(shell)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn('Installed yq version: yq-fixture', result.stdout)
                self.assertFalse(self.log.exists())
