---
title: Handbook & Documentation
description: Complete technical handbook for installing, administering, and building packages on KryX Linux.
order: 0
toc: false
---

# KryX Linux Handbook

Concise, practical documentation for installing, running, and packaging software on KryX Linux. Every page prints cleanly to PDF via your browser's print dialog and renders natively in `lynx`, `w3m`, and `links`.

## Core Guides

1. [Installation Guide](/docs/installation.html) — Partitioning, bootstrapping from RootFS or any x86_64 live ISO, `setup-repo`, `setup-chroot`, kernel, and Limine bootloader setup.
2. [`kpm` Package Management](/docs/kpm.html) — Syncing, binary installs, version pinning, holds, downgrades, orphan pruning, ABI checks (`kpm check --broken`), rejected config merges, and 100% CRUX symlink parity.
3. [Building Ports (`Pkgfile` & `kryx-src`)](/docs/building-ports.html) — Writing `Pkgfile` recipes, `# Make depends on:`, local/git Nix-like builds, overlays with `gports`, and `kryx-src` CI staging.
4. [System Configuration](/docs/configuration.html) — `/etc/kpm.ini`, `/etc/gports/sources.list`, `/etc/pkgadd.conf`, `signify` key rotation, and Wayland/CLI base setup.

## Web Manual Pages

All KryX CLI man pages are converted directly from roff source into readable, hyperlinked HTML:

- [Browse all Web Man Pages (`/man/`)](/man/)
- [`kpm(8)`](/man/kpm.8.html) — KryX Package Manager and CRUX multi-call compatibility reference
- [`kpm-index(8)`](/man/kpm-index.8.html) — Binary repository index generator and `signify` signer
- [`kryx-src(8)`](/man/kryx-src.8.html) — Unified source build, linting, matrix planning, and staging machine
- [`ports(8)`](/man/ports.8.html) — Integrated pure-Go CRUX/KryX ports tree synchronizer (`git`, `rsync`, `httpup`, `tarball`), built into `kpm`

## Plain-Text Console Endpoints

For live-ISO installation or SSH terminals without a browser:

- [/install.txt](/install.txt) — Compact live-ISO installation walkthrough
- [/SHA256SUMS.txt](/SHA256SUMS.txt) — Release archive SHA-256 checksums
- [/kryx-release.pub](/kryx-release.pub) — Public `signify` signing key
- [/packages.txt](/packages.txt) — Plain-text binary package index
