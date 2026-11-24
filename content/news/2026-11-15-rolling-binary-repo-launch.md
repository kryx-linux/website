---
title: Rolling x86_64 Binary Repository & Pure-Go gports Launch
date: 2026-11-15
description: Automated matrix builds for rolling-x86_64 binary packages, backward-compatible Make Depends, and pure-Go gports with native rsync/git/httpup.
---

# Rolling `x86_64` Binary Repository & Pure-Go `gports` Launch

KryX Linux's `rolling-x86_64` binary package repository and pure-Go `gports` utility are now live:

- **Pure-Go `gports`:** Native `git`, `rsync`, `httpup`, and `tarball` collection drivers compiled into a single static binary with zero external runtime dependencies.
- **Backward-Compatible `# Make depends on:`:** `Pkgfile` recipes can now separate build-time toolchains from runtime dependencies so source builds install compilers from prebuilt binaries without bloating runtime installs.
- **Topological CI Matrix Builds:** `kryx-src plan` partitions port updates into bounded GitHub Actions build jobs and stages verified `.pkg.tar.zst` archives into `index.json`.

Browse available packages in the [Packages Browser](/packages/) or explore recipes in the [Ports Tree](/ports/).
