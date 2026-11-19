"""Exercise Bridge certificate exports with synthetic state and CLI commands."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ENTRYPOINT = Path(__file__).resolve().parents[1] / 'protonmailbridge/entrypoint.sh'


class ProtonMailBridgeEntrypointTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        (self.root / 'state/.password-store').mkdir(parents=True)
        (self.root / 'state/.password-store/.gpg-id').write_text('fixture-key\n')
        (self.root / 'state/tls-export').mkdir()
        (self.root / 'state/tls-export/cert.pem').write_text('stale export\n')
        (self.root / 'trust').mkdir()
        self.trusted_cert = self.root / 'trust/cert.pem'
        self.trusted_cert.write_text('current trust\n')
        (self.root / 'bin').mkdir()
        self.command('id', 'echo 65532')
        self.command('gpg', 'exit 2')
        self.command('pass', '''case "$1" in
  insert) cat >/dev/null ;;
  show) echo mailvault-keyring-check ;;
  rm) ;;
  *) exit 2 ;;
esac''')
        entrypoint = (ENTRYPOINT.read_text().replace('/root', 'state')
                     .replace('/bridge-certs', 'trust')
                     .replace('/protonmail/', './bin/'))
        (self.root / 'entrypoint.sh').write_text(entrypoint)

    def command(self, name, script):
        command = self.root / 'bin' / name
        command.write_text(f'#!/bin/sh\nset -eu\n{script}\n')
        command.chmod(0o755)

    def run_init(self, cli):
        self.command('proton-bridge', 'test "$1" = --cli\n' + cli)
        return subprocess.run(['bash', 'entrypoint.sh', 'init'], cwd=self.root,
                              env={'HOME': str(self.root / 'state'),
                                   'PATH': f'{self.root / "bin"}{os.pathsep}{os.defpath}'},
                              capture_output=True, text=True, timeout=10)

    def test_skipped_export_does_not_replace_trust_certificate(self):
        result = self.run_init(':')
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn('Certificate not exported', result.stderr)
        self.assertEqual(self.trusted_cert.read_text(), 'current trust\n')

    def test_new_export_replaces_trust_certificate(self):
        result = self.run_init('echo "fresh export" >state/tls-export/cert.pem')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.trusted_cert.read_text(), 'fresh export\n')
        self.assertEqual(self.trusted_cert.stat().st_mode & 0o777, 0o644)
