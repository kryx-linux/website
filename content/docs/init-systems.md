---
title: Init Systems (rc & dinit)
description: Choosing and switching between KryX Linux's default CRUX SysVinit (rc) and the opt-in dinit supervising DAG init with turnstile and rc.d compatibility.
order: 5
toc: true
---

# Init Systems (`rc` & `dinit`)

KryX Linux supports a **dual-init architecture** without package conflicts:

- **Default: CRUX SysVinit + BSD-style `rc`** — Zero-overhead, sequential, transparent `/etc/rc.conf` and `/etc/rc.d/*` scripts (compatible with all 91 CRUX service scripts out of the box).
- **Opt-in: `dinit` (`0.23.0`) + `dinit-kryx` + `turnstile`** — A fast, modern, non-systemd supervising DAG init system with true file-descriptor readiness notification (`dbus` → `iwd`), clean service restarts, per-user session/service management (`turnstile`), and an automatic `rc-compat` bridge for legacy `/etc/rc.d/*` services.

Both init systems can be installed side by side (`dinit` uses `--shutdown-prefix=dinit-` so it never conflicts with `core/sysvinit`) and switched safely via `kryx-init` or a single GRUB kernel parameter (`init=/usr/sbin/dinit`).

## Default Init: CRUX `rc` (`/etc/rc.conf`)

Services are configured in `/etc/rc.conf`:

```sh
HOSTNAME=kryx
TIMEZONE=UTC
SERVICES=(lo dbus iwd dhcpcd)
```

Manage services directly via `/etc/rc.d/<service>`:

```sh
sudo /etc/rc.d/iwd status
sudo /etc/rc.d/dhcpcd restart
```

## Opt-In `dinit` During Installation

Both `setup` and the guided `setup-kryx` TUI installer accept `--init dinit` (available in online/hybrid mode or when `dinit` is present in the local pool):

```sh
setup --root /mnt --profile base --init dinit --hostname kryx
```

When `--init dinit` is selected, `setup`:

1. Installs `system/dinit`, `system/dinit-kryx`, and (on `base`/`devel` profiles) `system/turnstile`.
2. Adds `init=/usr/sbin/dinit` to `/etc/default/grub` (`GRUB_CMDLINE_LINUX_DEFAULT`).
3. Enables only the daemons that are actually installed in `/etc/dinit.d/boot.d/` (`dbus`, `iwd`, `dhcpcd`, `turnstiled`).

## Switching an Existing System (`rc` ↔ `dinit`)

Install the `system` collection init stack via `kpm`:

```sh
sudo kpm sync
sudo kpm install --deps dinit dinit-kryx turnstile
```

Inspect and switch the default GRUB init with `kryx-init`:

```sh
# Show active PID 1, GRUB default, and enabled dinit services
kryx-init status

# Switch GRUB default to dinit and regenerate /boot/grub/grub.cfg
sudo kryx-init use dinit

# Roll back to CRUX SysVinit / rc at any time
sudo kryx-init use rc
```

> **Instant boot-time fallback:** Even after running `kryx-init use dinit`, `core/sysvinit` and `/etc/rc` remain untouched. Press `e` in GRUB and remove `init=/usr/sbin/dinit` to boot straight into CRUX `rc` on any boot.

## Managing `dinit` Services

Enable or disable supervised services in `/etc/dinit.d/boot.d/`:

```sh
kryx-init list
sudo kryx-init enable seatd sshd chronyd
sudo kryx-init disable bluetoothd
```

Control running services live with `dinitctl`:

```sh
dinitctl list
sudo dinitctl start sshd
sudo dinitctl restart iwd
sudo dinitctl status dbus
```

When PID 1 is `dinit`, `/etc/profile.d/dinit-kryx.sh` automatically prepends `/usr/libexec/dinit-kryx/bin` to `PATH` so standard `reboot`, `poweroff`, `halt`, and `shutdown` commands execute `dinit-reboot` / `dinit-poweroff` cleanly.

## Boot DAG & `rc-compat` Bridge

`dinit-kryx` structures boot cleanly:

1. `early-filesystems` mounts `/proc`, `/sys`, `/dev`, `/dev/pts`, `/dev/shm`, `/run`, and `efivarfs` while keeping `/` read-only.
2. `udevd` and `udev-trigger` populate hardware devices.
3. `local-fs` runs `fsck -A -T -C -a`, remounts `/` read-write (`starts-rwfs`), mounts `/etc/fstab`, and enables swap.
4. `hostname`, `lo`, `gettys` (`tty1`–`tty6`, waiting for `hostname`), and enabled `/etc/dinit.d/boot.d/*` services start in parallel.
5. `rc-compat` reads `SERVICES=(...)` from `/etc/rc.conf` and automatically starts any CRUX `/etc/rc.d/<svc>` script that is **not** already enabled natively under `/etc/dinit.d/boot.d/`, then runs `/etc/rc.local` if executable.

## Session Management (`turnstile`)

`system/turnstile` provides `turnstiled` and `/lib/security/pam_turnstile.so`, automatically creating `$XDG_RUNTIME_DIR` (`/run/user/$UID`) and starting per-user `dinit` services (e.g. PipeWire, Wayland helpers) on login without `elogind` or `systemd`.
