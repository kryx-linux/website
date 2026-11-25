---
title: Packaging .deb Archives into a Pkgfile
description: How to cleanly repackage Debian/Ubuntu .deb binaries into reproducible CRUX/KryX Pkgfile recipes with multiarch fixes, .nostrip, and ELF dependency audits.
order: 4
toc: true
---

# Packaging `.deb` Archives into a `Pkgfile`

Rather than extracting `.deb` files directly onto `/` (which leaves untracked files and breaks clean uninstalls), the CRUX/KryX way is to write a short, reproducible **`Pkgfile`** that unpacks the `.deb` into `$PKG`.

This gives you:
- Full file tracking in `/var/lib/pkg/db` and `/etc/pkgadd.conf` conflict protection.
- One-command clean removal (`kpm remove <pkg>`).
- Automatic shared-library verification via `kryx-src audit-deps` and `kpm check --broken`.

## 1. Inspect the `.deb` First

A `.deb` file is a standard Unix `ar` archive containing three members: `debian-binary`, `control.tar.*` (metadata + scripts), and `data.tar.*` (the actual files).

Inspect the package name, version, dependencies, and any maintainer scripts before writing a `Pkgfile`:

```sh
# List members inside the .deb
ar t myapp_1.2.3_amd64.deb

# Read Debian control metadata (Package, Version, Depends, Description)
bsdtar -xOf myapp_1.2.3_amd64.deb 'control.tar.*' | tar -xOf - ./control

# Check whether any postinst/prerm script exists (never execute blindly!)
bsdtar -xOf myapp_1.2.3_amd64.deb 'control.tar.*' | tar -tf -
```

## 2. Standard `.deb` `Pkgfile` Template

Create a local port directory (for example under `/usr/ports/local/myapp/`) with a `Pkgfile`:

```sh
# Description: Example prebuilt application repackaged from upstream .deb
# URL:         https://example.org/myapp
# Maintainer:  Your Name, you at example dot org
# Depends on:  glibc zlib openssl

name=myapp
version=1.2.3
release=1
source=(https://example.org/releases/${name}_${version}_amd64.deb)

build() {
	# 1. Extract data.tar.* directly into $PKG (handles .gz, .xz, .zst, .bz2)
	bsdtar -xOf "${SRC}/${name}_${version}_amd64.deb" 'data.tar.*' \
		| bsdtar -xpf - -C "$PKG"

	# 2. Normalize Debian multiarch library paths to CRUX/KryX /usr/lib and /lib
	if [ -d "$PKG/usr/lib/x86_64-linux-gnu" ]; then
		cp -a "$PKG/usr/lib/x86_64-linux-gnu/." "$PKG/usr/lib/"
		rm -rf "$PKG/usr/lib/x86_64-linux-gnu"
	fi
	if [ -d "$PKG/lib/x86_64-linux-gnu" ]; then
		cp -a "$PKG/lib/x86_64-linux-gnu/." "$PKG/lib/"
		rm -rf "$PKG/lib/x86_64-linux-gnu"
	fi

	# 3. Drop Debian-only packaging noise
	rm -rf "$PKG/usr/share/lintian" "$PKG/usr/share/doc/${name}/changelog.Debian.gz"
}
```

> **Tip (`ar` fallback):** If you prefer standard `binutils` `ar` + `tar` instead of `bsdtar`, replace step 1 with:
> `ar x "${SRC}/${name}_${version}_amd64.deb" && tar -xpf data.tar.* -C "$PKG"`

## 3. Electron, Chrome & Proprietary `.deb` Apps (`.nostrip`)

Prebuilt Electron/Chromium apps (VS Code, Discord, Obsidian, Slack, Google Chrome) bundle V8 snapshots, ASAR archives, and already-stripped `.so` libraries. Running `strip` on them can corrupt embedded payloads or break signatures.

Add a **`.nostrip`** file next to your `Pkgfile` to skip ELF stripping for the bundled directory:

```text
^opt/myapp/.*
^usr/share/myapp/.*
```

If the app ships a Chromium sandbox helper (`chrome-sandbox`), ensure it keeps setuid root (`4755`) inside `build()`:

```sh
if [ -f "$PKG/opt/myapp/chrome-sandbox" ]; then
	chmod 4755 "$PKG/opt/myapp/chrome-sandbox"
fi
```

## 4. Mapping Debian Dependencies to KryX Packages

Debian package names include ABI suffixes (`t64`, `6`, `1g`). Use this quick reference when writing `# Depends on:` in your `Pkgfile`:

| Debian Package | KryX / CRUX Port |
| :--- | :--- |
| `libc6` | `glibc` |
| `zlib1g` | `zlib` |
| `libssl3` / `libssl3t64` | `openssl` |
| `libcurl4` / `libcurl4t64` | `curl` |
| `libgtk-3-0` / `libgtk-3-0t64` | `gtk3` |
| `libglib2.0-0` / `libglib2.0-0t64` | `glib` |
| `libnss3` | `nss` |
| `libasound2` / `libasound2t64` | `alsa-lib` |
| `libx11-6` / `libxcb1` | `xorg-libx11` / `xorg-libxcb` |
| `libdrm2` / `libgbm1` | `libdrm` / `mesa` |
| `libwayland-client0` | `wayland` |
| `libxkbcommon0` | `libxkbcommon` |
| `libcups2` / `libcups2t64` | `cups` |
| `libsecret-1-0` | `libsecret` |
| `libdbus-1-3` | `dbus` |

## 5. Build, Audit ELF Shared Libraries & Install

Build the package from your local directory, audit its real ELF `DT_NEEDED` dependencies, and install cleanly via `kpm`:

```sh
# 1. Build the .pkg.tar.zst archive and generate .footprint
cd /usr/ports/local/myapp
kpm build --update-footprint --ignore-md5sum .

# 2. Discover exact KryX runtime package deps from ELF DT_NEEDED & shebangs
kryx-src audit-deps /var/cache/kpm/packages/myapp#1.2.3-1.pkg.tar.zst

# 3. Install cleanly into /var/lib/pkg/db
sudo kpm install /var/cache/kpm/packages/myapp#1.2.3-1.pkg.tar.zst

# 4. Verify zero missing shared libraries
kpm check --broken myapp
```

To uninstall completely with zero leftover files:

```sh
sudo kpm remove myapp
```

## 6. Handling Debian Maintainer Scripts Safely

Never run Debian `postinst` or `prerm` scripts directly—they frequently invoke `dpkg`, `apt-key`, `update-alternatives`, or `systemctl`. Instead, inspect them:

```sh
bsdtar -xOf myapp_1.2.3_amd64.deb 'control.tar.*' | tar -xOf - ./postinst
```

Only translate the safe desktop/system actions you actually need:
- **Desktop entries / MIME types:** run `update-desktop-database -q` or `update-mime-database /usr/share/mime` after installing.
- **Icons:** run `gtk-update-icon-cache -q -f /usr/share/icons/hicolor`.
- **Background daemons:** write a small `/etc/dinit.d/<name>` service (for `dinit`) or `/etc/rc.d/<name>` script (for CRUX `rc`) instead of Debian's systemd unit.
