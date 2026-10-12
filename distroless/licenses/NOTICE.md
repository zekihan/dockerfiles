# distroless third-party notices

This image contains Mozilla CA certificates from Alpine's ca-certificates
package and IANA timezone data from Alpine's tzdata package. It does not contain
the Alpine runtime, certificate-update tools, or timezone utilities.

Mozilla certificate data is distributed under MPL-2.0; see `MPL-2.0.txt`.
Package source and notices:
https://gitlab.alpinelinux.org/alpine/ca-certificates.
`alpine-packages.txt` records the installed package versions. The corresponding
certificate-package source is available at
`https://gitlab.alpinelinux.org/alpine/ca-certificates/-/archive/<version>/ca-certificates-<version>.tar.bz2`.

IANA timezone data is public domain; see `tzdata-LICENSE`. The SPDX label uses
`LicenseRef-IANA-TZ-Public-Domain` for that declaration. Corresponding releases:
https://data.iana.org/time-zones/releases/.

The repository's AGPL-3.0-only license applies to its original build recipe and
documentation, not to the upstream certificate or timezone data.
