---
title: Installation Guide
description: Step-by-step guide to installing KryX Linux from the Live ISO, RootFS tarball, or any x86_64 live Linux environment.
order: 1
toc: true
---

# Installation Guide

KryX Linux can be installed either from the official hybrid **GRUB (BIOS + UEFI)** Live ISO (`kryx-linux-x86_64.iso` with `setup` & `setup-chroot`) or bootstrapped from **any** x86_64 live Linux environment using static `kpm` and `setup-repo`.

## 1. Partition and Format Target Disk

Identify your target block device (for example `/dev/nvme0n1` or `/dev/sda`) with `lsblk`. A clean UEFI layout uses an EFI System Partition (FAT32) and a Linux root partition (ext4, XFS, or Btrfs):

```sh
cfdisk /dev/nvme0n1
mkfs.fat -F32 /dev/nvme0n1p1
mkfs.ext4 /dev/nvme0n1p2

mount /dev/nvme0n1p2 /mnt
mkdir -p /mnt/boot/efi
mount /dev/nvme0n1p1 /mnt/boot/efi
```

## 2. Install Base System onto `/mnt`

### Option A: From the KryX Live ISO (`setup`)

The KryX Live ISO ships with `/etc/kpm/keys/kryx-release.pub` (Key ID `2528a8ef6bb01ca3`) pre-configured. Bring up networking (`dhcpcd` or `wpa_supplicant`) and run:

```sh
setup /mnt
```

`setup` initializes `/mnt/var/lib/pkg/db`, copies `/etc/kpm.ini`, `/etc/kpm/keys/kryx-release.pub`, and network config into `/mnt`, runs `kpm --root=/mnt sync`, installs the base package set via `kpm --root=/mnt install --deps`, and hands off cleanly to `setup-chroot /mnt`.

### Option B: Extract the Official RootFS Tarball

```sh
curl -fsSLO https://kryx.anrix.org/SHA256SUMS.txt
curl -fsSLO https://github.com/kryx-linux/iso/releases/latest/download/kryx-rootfs-x86_64-rolling.tar.xz
sha256sum -c --ignore-missing SHA256SUMS.txt
tar -xpf kryx-rootfs-x86_64-rolling.tar.xz -C /mnt
```

### Option C: Initialize Repositories Directly onto `/mnt` (`setup-repo`)

```sh
setup-repo \
  --root /mnt \
  --key https://kryx.anrix.org/kryx-release.pub \
  --verify
```

This creates `/mnt/etc/kpm.ini`, `/mnt/etc/gports/sources.list`, `/mnt/etc/pkgadd.conf`, `/mnt/var/lib/pkg/db`, and syncs the verified `index.json` into `/mnt/var/cache/kpm/`.

## 3. Install Kernel, Bootloader, and Core Tools

Install binary packages into `/mnt` with automatic runtime dependency resolution (if you used Option B or C, or want extra packages):

```sh
kpm --root=/mnt install --deps kpm gports kernel grub2 grub2-efi efibootmgr
```

## 4. Enter the Target System (`setup-chroot`)

Mount pseudo-filesystems (`/dev`, `/proc`, `/sys`, `/run`), bind DNS configuration, and chroot:

```sh
setup-chroot /mnt
```

Inside the chroot, verify package database and ABI health:

```sh
kpm doctor
kpm check --files
kpm check --broken
```

## 5. Essential System Configuration & GRUB Bootloader

Configure `/etc/fstab`, hostname, timezone, and root password:

```sh
passwd
echo "kryx" > /etc/hostname
ln -sf /usr/share/zoneinfo/UTC /etc/localtime
```

Install **GRUB** and generate `/boot/grub/grub.cfg`:

```sh
# UEFI (x86_64):
grub-install --target=x86_64-efi --efi-directory=/boot/efi --bootloader-id=KryX

# Or Legacy BIOS (example on /dev/sda):
# grub-install --target=i386-pc /dev/sda

grub-mkconfig -o /boot/grub/grub.cfg
```

## 6. Sync Local Ports Tree (Optional)

If you plan to compile custom ports or overlays locally:

```sh
gports check
gports -u
```

## 7. Reboot

Exit the chroot, unmount cleanly, and reboot:

```sh
exit
umount -R /mnt
reboot
```
