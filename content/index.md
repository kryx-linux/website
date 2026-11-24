---
title: KryX Linux
description: CRUX-inspired, binary-first, minimal Linux distribution focused on clarity, Wayland/CLI speed, and pure-Go tooling.
---

# KryX Linux

**KryX Linux** is a lightweight, CRUX-inspired rolling Linux distribution for `x86_64` that combines the simplicity and transparency of BSD-style ports (`Pkgfile`) with a strictly **binary-first** package workflow, pure-Go static tooling, and a clean Wayland/CLI-first environment.

## Core Philosophy

- **Keep it simple and inspectable:** Plain-text configuration (`/etc/kpm.ini`, `/etc/gports/sources.list`, `/var/lib/pkg/db`), no hidden daemons, and zero runtime bloat.
- **Binary-first by default, source-ready on demand:** `kpm install` downloads SHA-256 and `signify`-verified `.pkg.tar.zst` archives immediately. When you want custom flags or a port without a binary, `kpm build` compiles cleanly from local `Pkgfile` recipes and satisfies `# Make depends on:` from prebuilt binaries.
- **100% CRUX `pkgutils` & `ports` parity:** Every stock CRUX command (`pkgadd`, `pkgrm`, `pkginfo`, `pkgmk`, `ports`, `rejmerge`) works identically via multi-call symlinks to `/usr/bin/kpm`.
- **Static, self-contained toolchain:** Core distribution utilities (`kpm`, `kpm-index`, `kryx-src`, `gports`) compile statically (`CGO_ENABLED=0`) with zero external runtime dependencies—allowing you to bootstrap KryX from any x86_64 live Linux ISO.

## Architecture at a Glance

| Tool | Role | Reference |
| :--- | :--- | :--- |
| **`kpm`** | Binary-first package manager (`sync`, `install`, `update`, `build`, `info`, `remove`, `autoremove`, `check`, `hold`, `downgrade`, `clean`, `rejected`) + CRUX symlinks | [kpm(8)](/man/kpm.8.html) |
| **`kpm-index`** | Generates and `signify`-signs `index.json` repository metadata from `.pkg.tar.<ext>` archives | [kpm-index(8)](/man/kpm-index.8.html) |
| **`ports`** | Pure-Go, zero-`os/exec` ports tree synchronizer (`git`, `rsync`, `httpup`, `tarball`), searcher, and DB differ (integrated into `kpm`) | [ports(8)](/man/ports.8.html) |
| **`kryx-src`** | `xbps-src`-style source build, linting, topological matrix planning, and release staging engine | [kryx-src(8)](/man/kryx-src.8.html) |

## Quick Start

Sync the binary repository index, inspect a package, and install it with runtime dependencies:

```sh
kpm sync
kpm search wlroots
kpm info wlroots
kpm install --deps wlroots
```

Prefer compiling from ports with binary build-dependencies (`# Make depends on:`)?

```sh
gports -u
kpm build --deps --sync --install zig
```

Booting a live ISO without a graphical browser? Read the plain-text install guide directly in your terminal:

```sh
curl -s https://kryx.anrix.org/install.txt | less
```

## Where to Go Next

- [Download KryX Linux](/download/) — RootFS tarballs, Live ISOs, `SHA256SUMS`, and `signify` key verification
- [Handbook & Documentation](/docs/) — Installation, `kpm` guide, building ports, configuration, and [web man pages](/man/)
- [Packages Browser](/packages/) & [Ports Tree](/ports/) — Browse binary packages, dependencies, and `Pkgfile` recipes
- [News & Releases](/news/) ([RSS Feed](/feed.xml)) — Latest distribution updates
