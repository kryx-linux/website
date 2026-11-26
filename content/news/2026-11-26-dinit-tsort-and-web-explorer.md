---
title: Opt-in dinit Init, Unified Dependency Graph, and Void-Style Zero-JS Web Explorer
date: 2026-11-26
description: KryX Linux introduces opt-in dinit 0.23.0 + turnstile dual-init support, a Tarjan/Kahn dependency graph engine in kpm, and zero-JS web search, filtering, and dependency exploration.
---

# Opt-in `dinit` Init, Unified Dependency Graph, and Zero-JS Web Explorer

Today's update brings three major upgrades across `kpm`, `ports`, `iso`, and `kryx.anrix.org`:

## 1. Opt-In `dinit` (`0.23.0`) + `turnstile` Dual-Init Stack

CRUX's classic SysVinit + `rc` remains the default, while users who want a modern, non-systemd supervising DAG init can opt into `dinit` cleanly:

- New `system` ports collection shipping `dinit` (`0.23.0`, built with `--shutdown-prefix=dinit-` for zero conflict with `core/sysvinit`), `turnstile` (`0.1.11` session & `$XDG_RUNTIME_DIR` manager), and `dinit-kryx` (boot DAG, FD-ready `dbus` → `iwd`, `rc-compat` bridge for `/etc/rc.d/*`, and `kryx-init`).
- First-class `--init rc|dinit` selection in both `setup` and `setup-kryx`, plus post-install switching via `kryx-init use dinit|rc`.
- See the new [Init Systems (`rc` & `dinit`) Handbook Guide](/docs/init-systems.html).

## 2. Unified Dependency Graph Engine in `kpm`

`kpm` now shares a single iterative Tarjan SCC + Kahn topological sort engine across `install`, `update`, `remove`, `autoremove`, `deps`, `why`, and `kryx-src`:

- Added `kpm deps [--tree|--dot|--make]` and `kpm why <pkg>`.
- Added `kpm remove --cascade --dry-run` and reverse-topological `kpm autoremove`.
- Added 4-worker concurrent binary prefetching during `kpm install`.
- See the new [Dependency Graph & `kpm deps`/`why` Guide](/docs/depgraph.html).

## 3. Void-Style Zero-JS Website Upgrade

`kryx.anrix.org` has been refreshed with a lightweight, calm Void-Linux-inspired layout and new server-side Cloudflare Pages Functions — keeping **0 bytes of client-side JavaScript** and full `lynx`/`w3m`/`curl` compatibility:

- Individual detail pages for all 2,200+ ports (`/ports/<collection>/<port>.html`) with clickable dependencies and reverse dependencies.
- Server-side zero-JS [Search](/search/), [Filterable Browser](/browse), and [Dependency Explorer](/deps/) (including `?format=txt` and `?format=dot`).
- Void-style Handbook chapter sidebar, Prev/Next navigation, OpenSearch address-bar integration, and zero-JS edge Light/Dark/Auto theme switching.
