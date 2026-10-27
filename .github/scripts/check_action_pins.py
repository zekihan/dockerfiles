#!/usr/bin/env python3
"""Parse workflows and reject mutable external Actions and broad secret scopes."""
import re
import sys
from pathlib import Path
import yaml

PIN = re.compile(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_./-]+@[0-9a-f]{40}\Z")
LOCAL = re.compile(r"\./[A-Za-z0-9_./-]+\Z")


def violations(text):
    try:
        workflow = yaml.safe_load(text)
    except yaml.YAMLError as exc:
        return [f"invalid workflow YAML: {exc}"]
    errors = []
    visited = set()

    def walk(value):
        if not isinstance(value, (dict, list)) or id(value) in visited:
            return
        visited.add(id(value))
        if isinstance(value, list):
            for item in value:
                walk(item)
            return
        if 'uses' in value:
            action = value['uses']
            if not isinstance(action, str) or not (PIN.fullmatch(action) or LOCAL.fullmatch(action)):
                errors.append(f'external uses must have a full commit SHA: {action}')
            if isinstance(action, str) and action.startswith('actions/checkout@'):
                if value.get('with', {}).get('persist-credentials') is not False:
                    errors.append('checkout must disable persisted credentials')
                if 'secrets.' in str(value):
                    errors.append('checkout must not receive publication secrets')
        for item in value.values():
            walk(item)

    walk(workflow)
    if isinstance(workflow, dict):
        for scope in [workflow, *workflow.get('jobs', {}).values()]:
            if 'secrets.' in str(scope.get('env', {})):
                errors.append('publication secrets must be step scoped')
        if any(value == 'write' for value in workflow.get('permissions', {}).values()):
            errors.append('write permissions must be job scoped')
    return errors


def main():
    errors = []
    root = Path(__file__).resolve().parents[2]
    for path in sorted((root / '.github/workflows').glob('*')):
        if path.suffix in ('.yml', '.yaml'):
            errors.extend(f'{path.name}: {error}' for error in violations(path.read_text()))
    print('\n'.join(errors) if errors else 'Immutable Action and credential scope policy passed.')
    return bool(errors)


if __name__ == '__main__':
    sys.exit(main())
