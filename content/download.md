---
title: Download KryX Linux
description: Official RootFS tarballs, Live ISO images, SHA-256 checksums, signify verification instructions, and plain-text curl endpoints.
---

# Download KryX Linux

KryX Linux ships as a lean `x86_64` RootFS tarball (ideal for chroot installs from any live Linux USB), a bootable Live/Installer ISO, and an OCI container image for CI and `kryx-src` builds.

> **Terminal / Live-ISO Endpoints:** You can fetch plain-text installation instructions, checksums, and the release public key directly from a console:
>
> - `curl -s https://kryx.anrix.org/install.txt | less`
> - `curl -fsSLO https://kryx.anrix.org/SHA256SUMS.txt`
> - `curl -fsSLO https://kryx.anrix.org/kryx-release.pub`

## Verify Downloads (`SHA-256` & `signify`)

Always verify downloaded archives before extracting or writing to removable media.

### 1. Verify Archive Integrity with SHA-256

```sh
curl -fsSLO https://kryx.anrix.org/SHA256SUMS.txt
sha256sum -c --ignore-missing SHA256SUMS.txt
```

### 2. Trust the KryX Release Public Key

The official KryX Ed25519 release public key (`kryx-ed25519-pub-v1`, pre-installed in the Live ISO at `/etc/kpm/keys/kryx-release.pub`) has:

- **Key ID:** `2528a8ef6bb01ca3`
- **SHA-256 Fingerprint:** `SHA256:JSio72uwHKPFO8HgwqaTitUg26aaKcZxLlwRHIjtSg8`

Import the public key into `kpm` so `kpm sync` cryptographically verifies `index.json` against `index.json.sig` before updating the local cache:

```sh
curl -fsSLO https://kryx.anrix.org/kryx-release.pub
sudo kpm key trust kryx-release.pub --name kryx-official
kpm key list
```

Enable mandatory signature verification in `/etc/kpm.ini`:

```ini
[repo]
verify_index_signature = true
pubkey = /etc/kpm/keys/kryx-release.pub
```

Then verify a repository sync:

```sh
sudo kpm sync --verify
```

## Live ISO Installation (`kryx-linux-x86_64.iso`)

`kryx-linux-x86_64.iso` is a hybrid **GRUB (BIOS + UEFI)** minimal live image powered by CRUX's `linux-6.18.63` ISO kernel, `kpm` (with integrated `ports` and CRUX symlinks), `signify`, `/etc/kpm/keys/kryx-release.pub`, and the `setup` / `setup-chroot` installer pre-configured. After partitioning and mounting your target disk at `/mnt`:

```sh
setup /mnt
setup-chroot /mnt
```

## Bootstrap from Any Live Linux ISO

Because `kpm` (including integrated `ports`) is a static Go binary (`CGO_ENABLED=0`), you can also install KryX Linux onto `/mnt` from any x86_64 live environment (CRUX, Arch, Debian, Fedora, or Alpine):

```sh
setup-repo --root /mnt --key https://kryx.anrix.org/kryx-release.pub --verify
kpm --root /mnt install --deps kpm kernel grub2 grub2-efi efibootmgr
setup-chroot /mnt
```

See the full [Installation Guide](/docs/installation.html) for partitioning, bootloader setup, and post-install configuration.
