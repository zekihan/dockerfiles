from pathlib import Path
import shlex
import stat
import subprocess
import tempfile
import unittest


class DistrolessTests(unittest.TestCase):
    def test_tmp_is_writable_with_sticky_bit(self):
        dockerfile = Path(__file__).resolve().parents[1] / 'distroless' / 'Dockerfile'
        instructions = dockerfile.read_text().replace('\\\n', '').splitlines()
        setup = next(
            line.removeprefix('RUN ') for line in instructions
            if line.startswith('RUN mkdir -p ') and '/tmp/distroless/tmp' in line
        )
        with tempfile.TemporaryDirectory() as directory:
            setup = setup.replace('/tmp/distroless', shlex.quote(directory))
            subprocess.run(['sh', '-e', '-c', 'umask 022; ' + setup], check=True)
            mode = stat.S_IMODE((Path(directory) / 'tmp').stat().st_mode)
            self.assertEqual(mode, 0o1777, f'/tmp mode is {mode:o}')
