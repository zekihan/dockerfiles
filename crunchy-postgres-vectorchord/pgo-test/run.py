#!/usr/bin/env python3
"""Opt-in kind-only PGO integration test. Never uses the ambient kubeconfig."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import time
import uuid

PREFIX = 'postgres-operator.crunchydata.com/'
HERE = Path(__file__).resolve().parent


def run(argv, data=None, timeout=180):
    # Do not expose arbitrary process output (may contain credentials).
    result = subprocess.run(argv, input=data, text=True, capture_output=True,
                            timeout=timeout, check=False)
    if result.returncode:
        raise RuntimeError(f'{argv[0]} failed (exit {result.returncode}); raw output suppressed')
    return result.stdout.strip()


def manifest(name, image, backrest, backup=None):
    pvc = {'accessModes': ['ReadWriteOnce'],
           'resources': {'requests': {'storage': '2Gi'}}}
    spec = {
        'postgresVersion': 18, 'image': image,
        'patroni': {'dynamicConfiguration': {'postgresql': {'parameters': {
            'shared_preload_libraries': 'pg_stat_statements,vchord.so'}}}},
        'instances': [{'name': 'instance', 'replicas': 2,
                       'dataVolumeClaimSpec': pvc}],
        'backups': {'pgbackrest': {'image': backrest, 'repos': [
            {'name': 'repo1', 'volume': {'volumeClaimSpec': pvc}}]}}}
    if backup:
        spec['dataSource'] = {'postgresCluster': {
            'clusterName': 'source', 'repoName': 'repo1',
            'options': ['--type=immediate', f'--set={backup}']}}
    return {'apiVersion': PREFIX[:-1] + '/v1beta1', 'kind': 'PostgresCluster',
            'metadata': {'name': name, 'namespace': 'pgo-test'}, 'spec': spec}


def sanitized_pods(items):
    # Deliberately omit logs, events, messages, env, annotations, specs and Secrets.
    return [{'name': p['metadata']['name'], 'phase': p.get('status', {}).get('phase'),
             'containers': [{'name': c['name'], 'ready': c.get('ready', False),
                             'restartCount': c.get('restartCount', 0),
                             'imageID': c.get('imageID', '')}
                            for c in p.get('status', {}).get('containerStatuses', [])]}
            for p in items]


class Harness:
    def __init__(self, args, directory):
        self.args = args
        self.name = 'ops26-' + uuid.uuid4().hex[:16]
        self.config = str(Path(directory) / 'kubeconfig')
        self.created = False
        self.stage = 'preflight'
        self.evidence = {'candidate': args.image, 'stages': []}

    def k(self, *args, data=None):
        return run(['kubectl', '--kubeconfig', self.config, '--context', 'kind-' + self.name,
                    '--request-timeout=30s', '-n', 'pgo-test', *args], data)

    def apply(self, obj):
        self.k('apply', '-f', '-', data=json.dumps(obj))

    def pods(self, cluster):
        return json.loads(self.k('get', 'pods', '-l', PREFIX + 'cluster=' + cluster,
                                 '-o', 'json'))['items']

    def wait(self, predicate):
        deadline = time.monotonic() + self.args.timeout
        while time.monotonic() < deadline:
            try:
                result = predicate()
                if result:
                    return result
            except (RuntimeError, KeyError, ValueError):
                pass
            time.sleep(3)
        raise RuntimeError('timed out during ' + self.stage)

    def members(self, cluster):
        pods = [p for p in self.pods(cluster)
                if PREFIX + 'instance' in p['metadata'].get('labels', {})
                and not p['metadata'].get('deletionTimestamp')]
        if len(pods) != 2:
            return None
        for p in pods:
            status = next((c for c in p.get('status', {}).get('containerStatuses', [])
                           if c['name'] == 'database'), {})
            container = next(c for c in p['spec']['containers'] if c['name'] == 'database')
            if not status.get('ready') or container['image'] != self.args.image:
                return None
        leaders = [p for p in pods if p['metadata']['labels'].get(PREFIX + 'role') == 'master']
        if len(leaders) != 1:
            return None
        leader = leaders[0]['metadata']['name']
        follower = next(p['metadata']['name'] for p in pods if p['metadata']['name'] != leader)
        return leader, follower

    def sql(self, pod, sql):
        return self.k('exec', '-i', pod, '-c', 'database', '--', 'psql', '-XAt',
                      '-v', 'ON_ERROR_STOP=1', '-d', 'postgres', data=sql)

    def check(self, pod):
        self.sql(pod, (HERE / 'verify.sql').read_text())

    def checkpoint(self, stage):
        self.evidence['stages'].append(stage)

    def execute(self):
        self.stage = 'kind creation'
        # A random name plus pre-existence check prevents deleting another run.
        if self.name in run(['kind', 'get', 'clusters']).splitlines():
            raise RuntimeError('generated cluster already exists')
        self.created = True  # also clean partially-created kind nodes on failure
        run(['kind', 'create', 'cluster', '--name', self.name, '--kubeconfig', self.config,
             '--image', self.args.node_image, '--wait', '180s'], timeout=300)
        self.stage = 'operator installation'
        self.k('apply', '-f', str(self.args.operator_manifest))
        self.k('wait', '--for=condition=Established',
               'crd/postgresclusters.' + PREFIX[:-1], '--timeout=120s')
        self.apply({'apiVersion': 'v1', 'kind': 'Namespace', 'metadata': {'name': 'pgo-test'}})
        self.stage = 'source reconciliation'
        self.apply(manifest('source', self.args.image, self.args.backrest_image))
        leader, follower = self.wait(lambda: self.members('source'))
        self.sql(leader, (HERE.parent / 'smoke-test.sql').read_text())
        self.wait(lambda: self.sql(follower, 'SELECT count(*) FROM embeddings;') == '1000')
        if self.sql(follower, 'SELECT pg_is_in_recovery();') != 't':
            raise RuntimeError('promotion target is not a replica')
        self.checkpoint(self.stage)
        self.stage = 'replica promotion'
        self.k('exec', leader, '-c', 'database', '--', 'patronictl', 'switchover',
               '--leader', leader, '--candidate', follower, '--force')
        def promoted():
            members = self.members('source')
            return members and members[0] == follower and members
        leader, _ = self.wait(promoted)
        self.check(leader)
        if self.sql(leader, 'SELECT pg_is_in_recovery();') != 'f':
            raise RuntimeError('promoted database is not writable primary')
        self.checkpoint(self.stage)
        self.stage = 'full pgBackRest backup'
        self.k('exec', leader, '-c', 'database', '--', 'pgbackrest', '--stanza=db',
               '--repo=1', '--type=full', 'backup')
        info = json.loads(self.k('exec', leader, '-c', 'database', '--',
                                'pgbackrest', '--stanza=db', '--repo=1', '--output=json', 'info'))
        backups = [b for b in info[0]['backup'] if b['type'] == 'full']
        backup = max(backups, key=lambda b: b['timestamp']['stop'])['label']
        if not re.fullmatch(r'\d{8}-\d{6}F', backup):
            raise RuntimeError('unexpected backup label')
        self.evidence['backup'] = backup
        self.checkpoint(self.stage)
        self.stage = 'pgBackRest recovery'
        # Removing the live table proves the clone is recovered from the selected backup.
        self.sql(leader, 'DROP TABLE embeddings;')
        self.apply(manifest('recovered', self.args.image, self.args.backrest_image, backup))
        restored, _ = self.wait(lambda: self.members('recovered'))
        self.check(restored)
        self.checkpoint(self.stage)

    def finish(self):
        try:
            self.evidence['pods'] = sanitized_pods(json.loads(
                self.k('get', 'pods', '-o', 'json'))['items']) if Path(self.config).exists() else []
        except Exception:
            self.evidence['diagnosticsUnavailable'] = True
        if self.created:
            try:
                run(['kind', 'delete', 'cluster', '--name', self.name], timeout=180)
                self.evidence['cleanup'] = 'passed'
            except Exception:
                self.evidence['cleanup'] = 'failed'
                self.evidence['remainingKindCluster'] = self.name
        self.args.output.write_text(json.dumps(self.evidence, indent=2) + '\n')


def arguments(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--allow-disposable-cluster', action='store_true', required=True)
    for name in ('image', 'backrest-image', 'node-image'):
        parser.add_argument('--' + name, required=True)
    parser.add_argument('--operator-manifest', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--timeout', type=int, default=900)
    args = parser.parse_args(argv)
    for value in (args.image, args.backrest_image, args.node_image):
        if not re.fullmatch(r'[^\s]+@sha256:[0-9a-f]{64}', value):
            parser.error('all images must use immutable sha256 digests')
    if args.timeout <= 0:
        parser.error('timeout must be positive')
    args.operator_manifest = args.operator_manifest.resolve(strict=True)
    if args.output.exists():
        parser.error('output already exists; choose a fresh evidence path')
    return args


def main():
    args = arguments()
    os.umask(0o077)
    with tempfile.TemporaryDirectory(prefix='ops26-') as directory:
        harness = Harness(args, directory)
        harness.evidence['operatorManifestSHA256'] = hashlib.sha256(args.operator_manifest.read_bytes()).hexdigest()
        try:
            harness.execute()
            harness.evidence['result'] = 'passed'
        except (Exception, KeyboardInterrupt):
            harness.evidence.update(result='failed', failedStage=harness.stage)
        finally:
            harness.finish()
        return 0 if harness.evidence.get('result') == 'passed' and harness.evidence.get('cleanup') == 'passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())
