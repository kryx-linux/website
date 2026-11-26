---
title: Dependency Graph & kpm deps/why
description: Understanding kpm's Tarjan SCC and Kahn topological dependency engine, kpm deps, kpm why, cascade removal, orphan pruning, and the web Dependency Explorer.
order: 6
toc: true
---

# Dependency Graph & `kpm deps` / `kpm why`

KryX Linux uses a unified, iterative **Tarjan Strongly Connected Components (SCC) + Kahn priority topological sort** engine (`internal/tsort` and `internal/depgraph`) shared across `kpm`, `ports`, and `kryx-src`.

Unlike naive recursive resolvers, the engine:

- Never overflows the call stack on deep graphs (strictly iterative Tarjan + Kahn).
- Detects dependency cycles deterministically and prints exact cycle witness paths (`a -> b -> c -> a`).
- Produces reproducible, tie-broken installation and build orders across all 2,200+ CRUX/KryX ports.
- Groups independent build targets into parallel topological **waves** for `kryx-src` CI matrices.

## Inspecting Dependencies (`kpm deps`)

Display transitive runtime dependencies for any installed, binary-indexed, or local port package:

```sh
# Flat topologically ordered dependency list
kpm deps wlroots

# Indented ASCII dependency tree
kpm deps --tree wlroots

# Include build-time dependencies (# Make depends on:)
kpm deps --tree --make wlroots

# Export Graphviz DOT graph
kpm deps --dot grub2 | dot -Tsvg > grub2.svg
```

You can also inspect any package's dependency graph directly on the web with zero JavaScript via the [Dependency Explorer](/deps/) (including plain-text `?format=txt` and Graphviz `?format=dot` endpoints).

## Reverse Dependencies (`kpm why`)

Find out why a package is installed (every path from explicitly installed packages in `/var/lib/kpm/world` to the target package):

```sh
kpm why libdrm
kpm why dbus
```

## Safe Cascade Removal & Orphan Pruning

- **Reverse-dependency guard:** `kpm remove <pkg>` checks installed dependents and refuses to break them unless `--force` or `--cascade` is explicitly requested.
- **Cascade removal (reverse topological order):** preview before removing with `--dry-run`:

```sh
kpm remove --cascade --dry-run sway
sudo kpm remove --cascade sway
```

- **Orphan pruning (reverse topological order):** `kpm autoremove` finds packages no longer reachable from `/var/lib/kpm/world` and removes leaves before their dependencies:

```sh
kpm autoremove --dry-run
sudo kpm autoremove
```

## Parallel Build Waves (`kryx-src`)

For repository maintainers and CI matrix builds, `kryx-src` uses the same `tsort.Waves()` engine to partition ports into independent parallel layers:

```sh
kryx-src plan --ports-dir /usr/ports --wave 0 all
```
