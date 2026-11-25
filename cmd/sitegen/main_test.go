package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"encoding/xml"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMarkdownAutoTOCAndAnchors(t *testing.T) {
	md := `---
title: Test Guide
description: Testing markdown parser
toc: true
---

# Test Guide

Intro with **bold**, *italic*, ` + "`inline_code`" + `, and [link](/docs/).

## Section One

Paragraph text.

### Sub Section

` + "```sh\nkpm sync\n```" + `

## Section Two

| Col A | Col B |
| :--- | :--- |
| ` + "`kpm`" + ` | fast |
`
	doc := ParseMarkdown(md, true)
	if doc.Meta.Title != "Test Guide" {
		t.Fatalf("unexpected title: %q", doc.Meta.Title)
	}
	if !strings.Contains(doc.HTML, `<nav class="toc"`) {
		t.Fatalf("expected auto-TOC in rendered HTML: %s", doc.HTML)
	}
	if !strings.Contains(doc.HTML, `id="section-one"`) || !strings.Contains(doc.HTML, `class="heading-anchor"`) {
		t.Fatalf("expected heading anchor links: %s", doc.HTML)
	}
	if !strings.Contains(doc.HTML, `<code>inline_code</code>`) {
		t.Fatalf("expected inline code preserved verbatim: %s", doc.HTML)
	}
	if !strings.Contains(doc.HTML, `<table>`) {
		t.Fatalf("expected rendered table: %s", doc.HTML)
	}
}

func TestBuildSiteEndToEndAndZeroJS(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(t.TempDir(), "public")

	err = BuildSite(BuildOptions{
		OutDir:     outDir,
		SiteURL:    "https://kryx.anrix.org",
		ContentDir: filepath.Join(repoRoot, "content"),
		StaticDir:  filepath.Join(repoRoot, "static"),
		DataDir:    filepath.Join(repoRoot, "data"),
		ManDirs:    []string{filepath.Join(repoRoot, "data", "man")},
		Offline:    true,
		BuildTime:  time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildSite failed: %v", err)
	}

	required := []string{
		"index.html",
		"style.css",
		"kryx.svg",
		"download/index.html",
		"docs/index.html",
		"docs/installation.html",
		"docs/kpm.html",
		"docs/building-ports.html",
		"docs/packaging-deb.html",
		"docs/configuration.html",
		"packages/index.html",
		"packages/kpm.html",
		"packages/wlroots.html",
		"ports/index.html",
		"ports/kpm-tools/index.html",
		"man/index.html",
		"man/kpm.8.html",
		"man/kpm-index.8.html",
		"man/kryx-src.8.html",
		"man/ports.8.html",
		"news/index.html",
		"status/index.html",
		"feed.xml",
		"install.txt",
		"SHA256SUMS.txt",
		"kryx-release.pub",
		"packages.txt",
		"ports.txt",
		"search-index.json",
		"search/index.html",
	}
	for _, rel := range required {
		p := filepath.Join(outDir, filepath.FromSlash(rel))
		fi, err := os.Stat(p)
		if err != nil || fi.Size() == 0 {
			t.Fatalf("expected non-empty output %s (err=%v)", rel, err)
		}
	}

	// Enforce strict 0-byte JavaScript invariant across every file in outDir.
	err = filepath.WalkDir(outDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".js") {
			t.Fatalf("forbidden JavaScript file found in output: %s", path)
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".html") {
			data, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			lower := strings.ToLower(string(data))
			if strings.Contains(lower, "<script") {
				t.Fatalf("forbidden <script> tag found in %s", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Validate RSS 2.0 XML syntax.
	feedBytes, err := os.ReadFile(filepath.Join(outDir, "feed.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed rssDocument
	if err := xml.Unmarshal(feedBytes, &parsed); err != nil {
		t.Fatalf("invalid feed.xml: %v", err)
	}
	if len(parsed.Channel.Items) < 2 {
		t.Fatalf("expected at least 2 RSS items, got %d", len(parsed.Channel.Items))
	}

	// Verify strictly 8 top-level navigation items on index.html.
	homeHTML, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	navStart := strings.Index(string(homeHTML), `<nav class="site-nav"`)
	navEnd := strings.Index(string(homeHTML)[navStart:], `</nav>`)
	navBlock := string(homeHTML)[navStart : navStart+navEnd]
	if got := strings.Count(navBlock, "<a "); got != 8 {
		t.Fatalf("expected strictly 8 top-nav links, got %d:\n%s", got, navBlock)
	}
	if !strings.Contains(string(homeHTML), `action="/search"`) {
		t.Fatalf("expected zero-JS search form on homepage")
	}

	// Verify search-index.json parses and contains expected kinds.
	rawIdx, err := os.ReadFile(filepath.Join(outDir, "search-index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []SearchEntry
	if err := json.Unmarshal(rawIdx, &entries); err != nil {
		t.Fatalf("invalid search-index.json: %v", err)
	}
	if len(entries) < 10 {
		t.Fatalf("expected >= 10 search entries, got %d", len(entries))
	}

	// Verify server-side zero-JS search handler (lynx/w3m/curl compatible).
	handler := NewSiteHandler(outDir, SiteContext{
		SiteURL:   "https://kryx.anrix.org",
		BuildTime: time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC),
	})

	// 1. HTML query matching binary package + docs
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/search?q=wlroots&scope=packages", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /search?q=wlroots, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "/packages/wlroots.html") {
		t.Fatalf("expected wlroots package link in search results:\n%s", body)
	}
	if strings.Contains(strings.ToLower(body), "<script") {
		t.Fatalf("forbidden <script> tag in /search response")
	}

	// 2. Plain-text query for curl / terminal users
	recTxt := httptest.NewRecorder()
	reqTxt := httptest.NewRequest(http.MethodGet, "/search?q=kpm&format=txt", nil)
	handler.ServeHTTP(recTxt, reqTxt)
	if recTxt.Code != http.StatusOK {
		t.Fatalf("expected 200 for /search?q=kpm&format=txt, got %d", recTxt.Code)
	}
	if ct := recTxt.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("expected text/plain content-type, got %q", ct)
	}
	if !strings.Contains(recTxt.Body.String(), "https://kryx.anrix.org/packages/kpm.html") {
		t.Fatalf("expected plain-text kpm URL, got:\n%s", recTxt.Body.String())
	}

	// 3. XSS escaping on hostile query parameter
	recXSS := httptest.NewRecorder()
	reqXSS := httptest.NewRequest(http.MethodGet, "/search?q=%3Cscript%3Ealert(1)%3C/script%3E", nil)
	handler.ServeHTTP(recXSS, reqXSS)
	if recXSS.Code != http.StatusOK {
		t.Fatalf("expected 200 for escaped query, got %d", recXSS.Code)
	}
	if strings.Contains(strings.ToLower(recXSS.Body.String()), "<script") {
		t.Fatalf("unescaped <script> leaked into /search HTML:\n%s", recXSS.Body.String())
	}

	// 4. Empty query serves static /search/index.html
	recEmpty := httptest.NewRecorder()
	reqEmpty := httptest.NewRequest(http.MethodGet, "/search", nil)
	handler.ServeHTTP(recEmpty, reqEmpty)
	if recEmpty.Code != http.StatusOK || !strings.Contains(recEmpty.Body.String(), "Example Queries") {
		t.Fatalf("expected static search page on empty query, got status=%d", recEmpty.Code)
	}
}

func TestDiscoverPortsSiblingReposAndTarGz(t *testing.T) {
	ws := t.TempDir()
	writePkg := func(rel, content string) {
		t.Helper()
		p := filepath.Join(ws, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writePkg("ports/kpm-tools/kpm/Pkgfile", "# Description: kpm\nname=kpm\nversion=0.6.0\nrelease=1\n")
	writePkg("ports/kernel/linux6.18/Pkgfile", "# Description: kernel\nname=kernel\nversion=6.18.64\nrelease=2\n")
	writePkg("core/zlib/Pkgfile", "# Description: zlib\nname=zlib\nversion=1.4\nrelease=1\n")
	writePkg("opt/grub2/Pkgfile", "# Description: grub2\nname=grub2\nversion=2.16\nrelease=1\n")

	byCol, byName := DiscoverPorts(filepath.Join(ws, "ports"), RepoIndex{}, true)
	if len(byCol["kpm-tools"]) != 1 || len(byCol["kernel"]) != 1 || len(byCol["core"]) != 1 || len(byCol["opt"]) != 1 {
		t.Fatalf("unexpected sibling discovery counts: kpm-tools=%d kernel=%d core=%d opt=%d",
			len(byCol["kpm-tools"]), len(byCol["kernel"]), len(byCol["core"]), len(byCol["opt"]))
	}
	if byName["kernel"].Collection != "kernel" || byName["zlib"].Collection != "core" {
		t.Fatalf("unexpected collection mapping: %+v", byName)
	}

	// Verify tarball streaming extraction for both single-collection and multi-collection GitHub archives.
	makeTarGz := func(files map[string]string) *bytes.Buffer {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		for name, body := range files {
			hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
			if err := tw.WriteHeader(hdr); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		return &buf
	}

	coreTar := makeTarGz(map[string]string{
		"core-main/bash/Pkgfile": "# Description: GNU Bash\nname=bash\nversion=5.3\nrelease=1\n",
		"core-main/README.md":    "ignore me",
	})
	gotCore, err := extractPortsFromTarGz(coreTar, "core", nil)
	if err != nil || len(gotCore["core"]) != 1 || gotCore["core"][0].Name != "bash" {
		t.Fatalf("extractPortsFromTarGz(core) failed: got=%+v err=%v", gotCore, err)
	}

	metaTar := makeTarGz(map[string]string{
		"ports-main/kpm-tools/kpm/Pkgfile":      "# Description: kpm\nname=kpm\nversion=0.6.0\nrelease=1\n",
		"ports-main/kernel/linux6.18/Pkgfile":   "# Description: kernel\nname=kernel\nversion=6.18.64\nrelease=2\n",
		"ports-main/ignored/foo/Pkgfile":        "# Description: skip\nname=foo\nversion=1\nrelease=1\n",
	})
	gotMeta, err := extractPortsFromTarGz(metaTar, "", map[string]bool{"kpm-tools": true, "kernel": true})
	if err != nil || len(gotMeta["kpm-tools"]) != 1 || len(gotMeta["kernel"]) != 1 || len(gotMeta["ignored"]) != 0 {
		t.Fatalf("extractPortsFromTarGz(meta) failed: got=%+v err=%v", gotMeta, err)
	}
}
