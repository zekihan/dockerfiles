# crowdsec-agent third-party notices

This image extends the unmodified upstream CrowdSec image
`docker.io/crowdsecurity/crowdsec`, pinned by digest in the Dockerfile. CrowdSec
is copyright Crowdsec and distributed under the MIT license, included as
`crowdsec-LICENSE`. Source: https://github.com/crowdsecurity/crowdsec. Alpine
packages in the upstream base image retain their own licenses.

`/usr/local/bin/crowdsec-supervisor` and
`/usr/local/lib/crowdsec-supervisor/apply-whitelists.sh` are original to the
dockerfiles source repository and licensed under AGPL-3.0-only
(`AGPL-3.0-only.txt`). The supervisor executable also contains:

- the Go runtime and standard library, BSD-3-Clause (`go-LICENSE`);
  source: https://go.dev/src/;
- go.yaml.in/yaml/v3, MIT and Apache-2.0 (`go-yaml-LICENSE`,
  `go-yaml-NOTICE`); source: https://github.com/yaml/go-yaml.

The image is independently packaged and is not an official CrowdSec image.
