#!/usr/bin/env python3
"""Validate image platforms and pass the selected list to the publisher."""
import os
import re
import sys
from pathlib import Path

import yaml

PLATFORM = re.compile(r'linux/[a-z0-9]+(?:/[a-z0-9][a-z0-9._-]*)?')
REQUIRED_PLATFORMS = {
    'crunchy-postgres-vectorchord': ('linux/amd64',),
    'protonmailbridge': ('linux/amd64', 'linux/arm64'),
}


def read_platforms(path):
    config = yaml.safe_load(path.read_text())
    arch = config.get('arch') if isinstance(config, dict) else None
    platforms = [item.strip(' \t') for item in arch.split(',')] if isinstance(arch, str) else []
    if (not platforms or len(platforms) != len(set(platforms))
            or not all(PLATFORM.fullmatch(platform) for platform in platforms)):
        raise ValueError('arch must list distinct Linux platforms separated by commas')

    required = REQUIRED_PLATFORMS.get(path.parent.name)
    if required and set(platforms) != set(required):
        raise ValueError(f'arch must build {",".join(required)}')
    return ','.join(required or platforms)


def main():
    image = os.environ.get('PUBLISH_IMAGE', '')
    paths = sorted(dockerfile.with_name('config.yaml') for dockerfile in Path('.').glob('*/Dockerfile')
                   if not image or dockerfile.parent.name == image)
    if not paths:
        sys.exit(f'Image config not found: {image or "*/config.yaml"}')

    for path in paths:
        try:
            platforms = read_platforms(path)
        except (OSError, ValueError, yaml.YAMLError) as exc:
            sys.exit(f'{path}: {exc}')

    if image:
        with open(os.environ['GITHUB_OUTPUT'], 'a') as output:
            print(f'platforms={platforms}', file=output)


if __name__ == '__main__':
    main()
