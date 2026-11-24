---
title: System Configuration
description: Reference for /etc/kpm.ini, /etc/ports/sources.list, /etc/pkgadd.conf, signify signing keys, and multi-repository overlays.
order: 4
toc: true
---

# System Configuration

KryX Linux favors explicit, readable plain-text files under `/etc/`.

## `/etc/kpm.ini`

Override the active configuration file with `kpm --config <file>` or `KPM_CONFIG=<file>`.

```ini
[paths]
root = /
ports_dir = /usr/ports
collections = pkg-tools, kernel, core, opt, xorg, contrib
cache_dir = /var/cache/kpm
source_cache = /var/cache/kpm/sources
package_dir = /var/cache/kpm/packages
work_dir = /var/cache/kpm/work
pkgadd_conf = /etc/pkgadd.conf
keys_dir = /etc/kpm/keys

[repo]
repository = kryx-linux/ports
tag = rolling-x86_64
arch = x86_64
index_url = https://github.com/kryx-linux/ports/releases/download/rolling-x86_64/index.json
verify_index_signature = true
pubkey = /etc/kpm/keys/kryx-release.pub
hold =
timeout_seconds = 120

[build]
jobs = 4
cflags = -O2 -march=x86-64 -pipe
cxxflags = -O2 -march=x86-64 -pipe
compression = zst
strip = true
compress_manpages = true

[ui]
color = auto
```

### Multi-Repository Priority

Define `[repo.<name>]` sections in priority order (first match wins):

```ini
[repo.rolling]
repository = kryx-linux/ports
tag = rolling-x86_64
verify_index_signature = true

[repo.testing]
repository = kryx-linux/ports
tag = testing-x86_64
verify_index_signature = true
```

## `/etc/ports/sources.list`

Drop additional overlays in `/etc/ports/sources.list.d/*.list` (legacy `/etc/gports/sources.list` is still read automatically as a fallback if `/etc/ports/sources.list` does not exist):

```text
git rolling https://github.com/kryx-linux/ports.git /usr/ports/rolling-repo
git core    https://github.com/kryx-linux/core.git
git opt     https://github.com/kryx-linux/opt.git
git xorg    https://github.com/kryx-linux/xorg.git
git contrib https://github.com/kryx-linux/contrib.git
```

## `/etc/pkgadd.conf`

Protect local configuration files from being silently overwritten during upgrades:

```text
UPGRADE		^etc/.*$		NO
UPGRADE		^var/log/.*$		NO
UPGRADE		^etc/kpm\.ini$		NO
UPGRADE		^etc/ports/sources\.list$	NO
```

When a protected file changes in an upgraded archive, `kpm` places the new file under `/var/lib/pkg/rejected/` for review via `kpm rejected` or `rejmerge`.

## Key Management and Rotation (`signify` / Ed25519)

`kpm` verifies OpenBSD `signify`, `kryx-ed25519`, and OpenSSH `ssh-ed25519` public keys natively in Go:

```sh
# Generate a new signify-compatible keypair
kpm key gen --signify --out kryx-2027 --comment "kryx-release-2027"

# Trust a new public key alongside an existing key during rotation
sudo kpm key trust https://kryx.anrix.org/kryx-release.pub --name kryx-official
kpm key list

# Revoke a retired key
sudo kpm key revoke old-key-name
```
