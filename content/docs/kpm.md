---
title: kpm Package Management
description: Complete guide to kpm (KryX Package Manager): binary installs, upgrades, world tracking, holds, downgrades, ABI checks, and CRUX symlink parity.
order: 2
toc: true
---

# `kpm` Package Management

**`kpm`** is KryX Linux's standalone, binary-first package manager built directly on CRUX Linux's `/var/lib/pkg/db` format, `flock(2)` locking, and `/etc/pkgadd.conf` rules.

## Daily Workflow

```sh
# Sync binary repository index (verifies index.json.sig when enabled)
sudo kpm sync

# Search across binary index, local ports, and installed packages
kpm search wlroots

# Inspect full metadata, runtime deps, make deps, and reverse dependencies
kpm info wlroots

# Install strictly binary-first with runtime dependencies
sudo kpm install --deps wlroots

# Preview and apply system upgrades
kpm update --dry-run
sudo kpm update
```

When a package has no prebuilt binary in `index.json`, `kpm install` never silently compiles behind your back—it warns you and suggests running `kpm build <pkg>`.

## World Tracking, Reverse Dependencies & Orphan Pruning

`kpm` records explicitly installed packages in `/var/lib/kpm/world` (locked alongside `/var/lib/pkg`), separate from automatically pulled dependencies:

- **Reverse-dependency protection:** `kpm remove <pkg>` checks installed dependents and aborts with exit code `1` if another installed package requires `<pkg>` (override explicitly with `--force`).
- **Orphan cleanup:** Remove dependencies no longer needed by any package in `world`:

```sh
kpm autoremove --dry-run
sudo kpm autoremove
```

## Version Pinning, Holds & Rollbacks

Pin a package so `kpm update` skips it (state is stored in `/var/lib/kpm/hold` and merged with `[repo] hold` in `/etc/kpm.ini`):

```sh
sudo kpm hold kernel
kpm hold --list
sudo kpm unhold kernel
```

Install an exact version from `index.json` or `/var/cache/kpm/bin/`, or roll back to the previous available version:

```sh
sudo kpm install zlib=1.3.1-1
sudo kpm downgrade zlib
```

## Cache Management (`kpm clean`)

Prune stale archives and partial downloads while keeping currently installed/latest cached versions by default:

```sh
kpm clean --dry-run
sudo kpm clean --keep 2
sudo kpm clean --all
```

## System & ABI Health Diagnostics

Verify dynamic library linkage across all installed ELF binaries/libraries (pure-Go `revdep` equivalent) and verify `/var/lib/pkg/db` file integrity:

```sh
kpm check --broken
kpm check --files
kpm doctor
```

## Managing Rejected Configuration Files

When `/etc/pkgadd.conf` protects a modified config file during an upgrade, the new version is placed in `/var/lib/pkg/rejected/`. Inspect or merge it via `kpm rejected` (or `rejmerge`):

```sh
kpm rejected --list
kpm rejected --diff
sudo kpm rejected --merge /etc/kpm.ini
sudo kpm rejected --keep all
```

## 100% CRUX Multi-Call Symlink Compatibility

`make install` creates symlinks in `/usr/bin` pointing to `kpm`:

- `pkgadd` (`-u`, `-f`, `-r`, `-c`, `-v`, `-h`)
- `pkgrm` (`-r`, `-v`, `-h`)
- `pkginfo` (`-i`, `-l`, `-o`, `-f`, `-r`, `-v`, `-h`)
- `pkgmk` (all 26 upstream flags, `/etc/pkgmk.conf`, `PKGMK_*` env, `.32bit`, exit codes `1..10`)
- `ports` (`-u`, `-l`, `-d`, `/etc/ports/drivers/`)
- `rejmerge` (`-r`, `-v`, `-h`, interactive permissions/diff/merge menus)

See the full [`kpm(8)` web man page](/man/kpm.8.html) for every option and exit code.
