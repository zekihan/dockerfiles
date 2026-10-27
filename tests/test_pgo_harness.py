"""Offline safety and regression tests; never contacts Kubernetes or Docker."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

PATH = Path(__file__).resolve().parents[1] / 'crunchy-postgres-vectorchord/pgo-test/run.py'
spec = importlib.util.spec_from_file_location('pgo', PATH)
pgo = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pgo)
IMAGE = 'example/candidate@sha256:' + 'a' * 64


class HarnessTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        operator = self.root / 'operator.yaml'
        operator.write_text('kind: List\nitems: []\n')
        self.argv = ['--allow-disposable-cluster', '--image', IMAGE,
                     '--backrest-image', IMAGE, '--node-image', IMAGE,
                     '--operator-manifest', str(operator), '--output', str(self.root / 'out.json')]
        self.args = pgo.arguments(self.argv)
        self.h = pgo.Harness(self.args, self.root)

    def test_opt_in_and_immutable_images_required(self):
        for args in (self.argv[1:], [a.replace(IMAGE, 'latest') for a in self.argv]):
            with self.assertRaises(SystemExit):
                pgo.arguments(args)

    def test_private_context_on_every_kubectl_command(self):
        with patch.object(pgo, 'run', return_value='') as run:
            self.h.k('get', 'pods')
        command = run.call_args.args[0]
        self.assertEqual(command[1:5], ['--kubeconfig', self.h.config, '--context', 'kind-' + self.h.name])

    def test_restore_pins_backup_and_exact_image(self):
        spec = pgo.manifest('recovered', IMAGE, IMAGE, '20261027-010101F')['spec']
        self.assertEqual(spec['image'], IMAGE)
        self.assertEqual(spec['instances'][0]['replicas'], 2)
        self.assertEqual(spec['dataSource']['postgresCluster']['options'],
                         ['--type=immediate', '--set=20261027-010101F'])
        self.assertNotIn('s3', json.dumps(spec))

    def test_diagnostics_do_not_leak_secrets_or_messages(self):
        pods = [{'metadata': {'name': 'test', 'annotations': {'password': 'SECRET'}},
                 'spec': {'env': 'SECRET'}, 'status': {'phase': 'Pending', 'message': 'SECRET',
                 'containerStatuses': [{'name': 'database', 'ready': False,
                                        'state': {'waiting': {'message': 'SECRET'}}}]}}]
        self.assertNotIn('SECRET', json.dumps(pgo.sanitized_pods(pods)))

    def test_cleanup_only_owned_kind_cluster_and_failure_recorded(self):
        self.h.created = True
        self.h.evidence['result'] = 'failed'
        with patch.object(pgo, 'run', side_effect=RuntimeError('secret error')) as run:
            self.h.finish()
        self.assertEqual(run.call_args.args[0], ['kind', 'delete', 'cluster', '--name', self.h.name])
        evidence = json.loads(self.args.output.read_text())
        self.assertEqual(evidence['cleanup'], 'failed')
        self.assertEqual(evidence['remainingKindCluster'], self.h.name)
        self.assertNotIn('secret', self.args.output.read_text())

    def test_existing_cluster_is_never_deleted(self):
        with patch.object(pgo, 'run', return_value=self.h.name):
            with self.assertRaises(RuntimeError):
                self.h.execute()
        self.assertFalse(self.h.created)

    def test_recovery_sequence_and_non_destructive_verifier(self):
        calls = []
        self.h.members = lambda cluster: ('source-old', 'source-new') if self.h.stage != 'replica promotion' else ('source-new', 'source-old')
        self.h.wait = lambda predicate: predicate()
        self.h.apply = lambda obj: calls.append(('apply', obj))
        self.h.check = lambda pod: calls.append(('check', pod))
        def sql(pod, text):
            calls.append(('sql', text))
            return '1000' if 'count(*)' in text else ('t' if self.h.stage == 'source reconciliation' else 'f')
        self.h.sql = sql
        def kubectl(*args, **kwargs):
            calls.append(('k', args))
            if 'info' in args:
                return json.dumps([{'backup': [{'type': 'full', 'timestamp': {'stop': 1},
                                               'label': '20261027-010101F'}]}])
            return ''
        self.h.k = kubectl
        with patch.object(pgo, 'run', return_value=''):
            self.h.execute()
        self.assertEqual(len(self.h.evidence['stages']), 4)
        backup = next(i for i, c in enumerate(calls) if c[0] == 'k' and 'backup' in c[1])
        drop = calls.index(('sql', 'DROP TABLE embeddings;'))
        restore = next(i for i, c in enumerate(calls) if c[0] == 'apply' and c[1]['metadata']['name'] == 'recovered')
        self.assertLess(backup, drop)
        self.assertLess(drop, restore)
        verify = (PATH.parent / 'verify.sql').read_text()
        self.assertNotIn('CREATE', verify)
        self.assertNotIn('DROP', verify)
        self.assertIn('embeddings_idx', verify)
        self.assertIn('IS DISTINCT FROM 500', verify)


if __name__ == '__main__':
    unittest.main()
