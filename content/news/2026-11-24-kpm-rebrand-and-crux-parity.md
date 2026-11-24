---
title: kpm Rebrand, 100% CRUX Parity, and Advanced Package Features
date: 2026-11-24
description: Finalizing kpm (KryX Package Manager) with multi-call CRUX symlink parity, signify index verification, ELF revdep checks, and Docker-safe kryx-src builds.
---

# `kpm` Rebrand, 100% CRUX Parity, and Advanced Package Features

The KryX Linux package manager is now officially **`kpm`** (with repository indexer **`kpm-index`**, source machine **`kryx-src`**, and integrated pure-Go **`ports`** merged from former `gports`), replacing legacy `pe` naming across binaries, configs (`/etc/kpm.ini`, `/etc/ports/sources.list`), caches (`/var/cache/kpm`), state files (`/var/lib/kpm`), man pages, and container/ISO images.

## Highlights

- **100% Upstream CRUX `pkgutils` & `ports` Parity:** Installing `kpm` creates multi-call symlinks (`pkgadd`, `pkgrm`, `pkginfo`, `pkgmk`, `ports`, `rejmerge` → `kpm`) with full flag, config, output, and exit-code parity (including all 26 `pkgmk` flags and exit codes `1..10`) plus the pure-Go native `ports` syncer (`git`, `rsync`, `httpup`, `tarball`).
- **Unified `kryx-linux/iso` Builder:** Combines our OCI CI base image (`ghcr.io/kryx-linux/iso:latest`) and hybrid **GRUB (BIOS + UEFI)** Live ISO (`kryx-linux-x86_64.iso`) running CRUX's `linux-6.18.63` ISO kernel with `setup` / `setup-chroot` powered exclusively by `kpm`.
- **Dedicated CRUX 3.8-Derived Collection Repos:** `kryx-linux/{core,opt,xorg,contrib}` now ship standalone repositories with `main`, `dev`, `crux`, and `lts` branches derived from CRUX `3.8`, alongside `kryx-linux/ports` (`pkg-tools/` & `kernel/`).
- **Cryptographic `signify` Index Verification:** `kpm-index --sign-key` emits OpenBSD `signify`-compatible `index.json.sig` detached signatures, and `kpm sync` verifies signatures before atomically replacing cached `index.json`.
- **World Tracking, Holds, Rollbacks & ABI Health:** Added `/var/lib/kpm/world`, `kpm info` (with reverse dependencies), `kpm autoremove`, `kpm hold`/`unhold`, `kpm install <pkg>=<ver>`, `kpm downgrade`, `kpm clean`, `kpm rejected`, and pure-Go ELF ABI checking (`kpm check --broken` and `kpm check --files`).
- **Docker-Safe `kryx-src` Ports Resolution:** `kryx-src` resolves ports via `--ports-dir`/`-P`, `$KRYX_PORTS_DIR`, `$KPM_PORTS_DIR`, `kpm.ini`, or workspace auto-detection instead of hardcoding `/usr/ports`.

Read the updated [`kpm` Package Management Guide](/docs/kpm.html) and [`kpm(8)` manual](/man/kpm.8.html).
