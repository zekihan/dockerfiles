# Proton Mail Bridge

Unofficial source-built Proton Mail Bridge for linux/amd64 and linux/arm64.
Published by the image workflow to `ghcr.io/zekihan/protonmailbridge` and
`docker.io/<DOCKER_USERNAME>/protonmailbridge`. Requires a paid Proton Mail account.

Initialize interactively (use the same image tag for all commands):

```sh
docker run --rm -it --name proton-bridge-init \
  -v proton-bridge-state:/root -v proton-bridge-certs:/bridge-certs \
  ghcr.io/zekihan/protonmailbridge:3.27.1 init
```

Run `login`, `info`, then `cert export`; export to `/root/tls-export`, then `exit`.
Use the username and generated Bridge password from `info` in the mail client.
Only the public certificate is copied into `/bridge-certs/cert.pem`.

```sh
docker run -d --name proton-bridge --restart unless-stopped \
  -v proton-bridge-state:/root -v proton-bridge-certs:/bridge-certs \
  ghcr.io/zekihan/protonmailbridge:3.27.1
```

Bridge listens on loopback. Run the client with `--network container:proton-bridge`
(or Compose `network_mode: service:bridge`), mount proton-bridge-certs read-only,
and trust its cert.pem. Use IMAP `127.0.0.1:1143` with STARTTLS and normal
certificate/hostname verification. Do not disable TLS verification. No ports
are published by these commands.

Persist both volumes. `/root` contains credentials, cache, and the GPG key used
by pass. The unattended key has no passphrase: possession of the volume permits
credential decryption. Protect the volume and its backups. Never run two Bridge
instances against the same state; the entrypoint enforces an exclusive lock.
For re-login or certificate rotation, stop Bridge, repeat init, and restart it.
Do not delete the volumes when replacing the container.

To build locally: `docker build -t local/protonmailbridge protonmailbridge`.
Update config.yaml version and sourceRevision together for upstream upgrades.
Base images are digest pinned. The workflow follows the repository's version,
latest, and run-number tag conventions. See [licenses](licenses/NOTICE.md).
