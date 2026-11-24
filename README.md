# KryX Linux Website (`https://kryx.anrix.org`)

Ultra-fast, text-first, **zero-JavaScript** static website for **KryX Linux**, built with a standalone pure-Go (`CGO_ENABLED=0`, stdlib-only) static site generator (`cmd/sitegen`) and self-hosted **Atkinson Hyperlegible Next** typography.

## Features

- **0 bytes of client-side JavaScript:** Works identically in modern graphical browsers and terminal browsers (`lynx`, `w3m`, `links`).
- **Single-font typography:** Self-hosted, Latin-subsetted `Atkinson Hyperlegible Next` WOFF2 files in `static/fonts/` with local-first override and `font-display: swap`.
- **Comfortable 72ch layout:** Soft light & automatic dark color schemes (`prefers-color-scheme: dark`) plus clean `@media print` rules for Handbook PDF export.
- **Build-time generated pages:**
  - `/packages/` & `/packages/<pkg>.html` from `index.json` (live fetch with offline fixture fallback)
  - `/ports/` & `/ports/<collection>/` from local or CI-cloned ports trees (with `Pkgfile` source)
  - `/man/` & `/man/<name>.html` converted directly from roff man pages (`kpm.8`, `kpm-index.8`, `kryx-src.8`, `ports.8`)
  - `/docs/` Handbook with automatic Table of Contents and clickable `#` heading anchors
  - `/news/` & `/feed.xml` (valid RSS 2.0 XML)
  - `/status/` repository summary, CI badges, and signing key fingerprint
  - Plain-text live-ISO endpoints: `/install.txt`, `/SHA256SUMS.txt`, `/kryx-release.pub`, `/packages.txt`

## Local Development

```sh
# Build static site (offline using bundled fixture & local sibling repos if present)
make build-offline

# Build with live index.json fetch from kryx-linux/ports
make build

# Run vet, unit tests, full build, and strict 0-byte JS check
make verify

# Build and serve locally at http://127.0.0.1:8080/
make serve
```

### Refreshing Fonts

The subsetted `Atkinson Hyperlegible Next` WOFF2 files are committed in `static/fonts/`. To re-fetch them from upstream:

```sh
make fonts
```

### Passing Custom Paths to `sitegen`

```sh
go run ./cmd/sitegen \
  -out public \
  -site-url https://kryx.anrix.org \
  -index-url https://github.com/kryx-linux/ports/releases/download/rolling-x86_64/index.json \
  -ports-dir ../ports \
  -man-dirs ../kpm/man,data/man
```

## Deploying to Cloudflare Pages (`kryx.anrix.org`)

### Option 1: Direct Cloudflare Pages Git Integration
1. In the Cloudflare Dashboard, go to **Workers & Pages → Create → Pages → Connect to Git** and select this repository.
2. Configure build settings:
   - **Framework preset:** `None`
   - **Build command:** `CGO_ENABLED=0 go run ./cmd/sitegen -out public -site-url https://kryx.anrix.org`
   - **Build output directory:** `public`
   - **Environment variables:** `GO_VERSION=1.24` (if needed by your Pages build image)
3. Under **Custom domains**, add `kryx.anrix.org`. Cloudflare automatically provisions TLS and CNAME records for the `anrix.org` zone.

### Option 2: GitHub Actions (`.github/workflows/deploy.yml`)
1. Create a Cloudflare Pages project named `kryx-linux` and bind the custom domain `kryx.anrix.org`.
2. Add repository secrets `CLOUDFLARE_API_TOKEN` (Pages Edit permission) and `CLOUDFLARE_ACCOUNT_ID`.
3. Every push to `main` (and the daily `04:00 UTC` schedule) clones the latest `kryx-linux/ports` collections, fetches `index.json`, enforces the 0-byte JS check, and deploys `public/` to Cloudflare Pages.
