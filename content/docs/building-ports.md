---
title: Building Ports (Pkgfile & kryx-src)
description: How to write CRUX/KryX Pkgfiles with Make Depends, build locally with kpm build, and automate builds with kryx-src.
order: 3
toc: true
---

# Building Ports (`Pkgfile` & `kryx-src`)

KryX Linux keeps standard CRUX `Pkgfile` compatibility while adding optional build-only dependency tracking (`# Make depends on:`), Nix-like direct repository builds, and the `kryx-src` build machine.

## Anatomy of a KryX `Pkgfile`

```sh
# Description: Fast static tool written in Go
# URL:         https://github.com/example/mytool
# Maintainer:  Your Name, you at example dot org
# Depends on:  glibc
# Make depends on: go

name=mytool
version=1.2.0
release=1
source=(https://github.com/example/$name/archive/v$version/$name-$version.tar.gz)

build() {
    cd $name-$version
    CGO_ENABLED=0 go build -trimpath -o $name ./cmd/$name
    install -D -m 0755 $name "$PKG/usr/bin/$name"
}
```

### Backward-Compatible `# Make depends on:`

- Stock CRUX `pkgmk` treats `# Make depends on:` as a comment, so recipes stay 100% compatible upstream.
- `kpm build --deps --sync` and `kryx-src pkg --deps --sync-index` satisfy build-only tools (such as `go`, `cmake`, `meson`, `llvm`) from prebuilt binaries without adding them to runtime `depends` in `index.json`.

## Building Locally with `kpm build`

```sh
# Build a port from /usr/ports using binary build-deps
sudo kpm build --deps --sync --install mytool

# Build directly from the current directory (Pkgfile or .kryx/Pkgfile)
sudo kpm build . --deps --install

# Build directly from a Git URL (Nix-like direct repo build)
sudo kpm build https://github.com/kryx-linux/kpm@main --deps --install
```

## Syncing and Diffing Ports with `ports` (`kpm ports`)

`ports` is built directly into `kpm` (invoked via `/usr/bin/ports` or `kpm ports`) as a zero-`os/exec`, pure-Go ports synchronizer (`git`, `rsync`, `httpup`, `tarball`) with automatic CRUX driver fallback:

```sh
ports check            # Validate /etc/ports/sources.list and DNS/TCP reachability
sudo ports -u          # Sync all collections concurrently (kryx-linux/{core,opt,xorg,contrib,ports})
ports -l               # List local ports (<collection>/<port>)
ports -d               # Diff local port versions against /var/lib/pkg/db
ports -s wayland       # Search port names and descriptions
ports status           # Inspect configured collections and local sync state
```

## Automated Builds & CI Staging with `kryx-src`

`kryx-src` never hardcodes `/usr/ports`. It resolves the ports tree via `--ports-dir` / `-P`, `$KRYX_PORTS_DIR`, `$KPM_PORTS_DIR`, `kpm.ini`, or workspace auto-detection (`./ports`, `./pkg-tools`, `./kernel`, `./core`, `./Pkgfile`):

```sh
kryx-src lint --ports-dir ./ports
kryx-src plan --ports-dir ./ports --max-ports 20 --compact pkg-tools/kpm
kryx-src pkg --ports-dir ./ports --work-root /tmp/work --out-dir ./dist --deps --sync-index pkg-tools/kpm
kryx-src stage --packages ./dist --ports-dir ./ports --index ./dist/index.json --upload-dir ./dist/upload
kryx-src sign --signify --key ./kryx-release.sec --file ./dist/index.json
```

See [`kryx-src(8)`](/man/kryx-src.8.html) and [`ports(8)`](/man/ports.8.html) for full flag documentation.
