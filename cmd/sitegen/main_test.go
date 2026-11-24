package main

import (
	"encoding/xml"
	"io/fs"
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
		"docs/configuration.html",
		"packages/index.html",
		"packages/kpm.html",
		"packages/wlroots.html",
		"ports/index.html",
		"ports/rolling/index.html",
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
}
