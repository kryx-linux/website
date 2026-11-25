package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// RepoIndex mirrors kpm-index's index.json schema.
type RepoIndex struct {
	SchemaVersion int                    `json:"schema_version"`
	Packages      map[string]RepoPackage `json:"packages"`
}

// RepoPackage groups all indexed versions of one binary package.
type RepoPackage struct {
	Latest   string      `json:"latest"`
	Versions []RepoEntry `json:"versions"`
}

// RepoEntry describes a single binary package archive in index.json.
type RepoEntry struct {
	Pkgname     string   `json:"pkgname"`
	Pkgver      string   `json:"pkgver"`
	Pkgrel      string   `json:"pkgrel"`
	Arch        string   `json:"arch"`
	Filename    string   `json:"filename"`
	URL         string   `json:"url"`
	SHA256      string   `json:"sha256"`
	Size        int64    `json:"size"`
	Description string   `json:"description,omitempty"`
	Depends     []string `json:"depends,omitempty"`
	MakeDepends []string `json:"makedepends,omitempty"`
	Collection  string   `json:"collection,omitempty"`
}

func (e RepoEntry) FullVersion() string {
	if e.Pkgrel == "" {
		return e.Pkgver
	}
	return e.Pkgver + "-" + e.Pkgrel
}

// LoadIndex loads index.json from an explicit local file, or fetches indexURL
// (when offline is false), falling back gracefully to fallbackPath.
func LoadIndex(indexFile, indexURL, fallbackPath string, offline bool) (idx RepoIndex, source string, err error) {
	if strings.TrimSpace(indexFile) != "" {
		raw, rerr := os.ReadFile(indexFile)
		if rerr != nil {
			return RepoIndex{}, "", fmt.Errorf("read index file %q: %w", indexFile, rerr)
		}
		if jerr := json.Unmarshal(raw, &idx); jerr != nil {
			return RepoIndex{}, "", fmt.Errorf("parse index file %q: %w", indexFile, jerr)
		}
		normalizeIndex(&idx)
		return idx, indexFile, nil
	}

	if !offline && strings.TrimSpace(indexURL) != "" {
		client := &http.Client{Timeout: 12 * time.Second}
		resp, herr := client.Get(indexURL)
		if herr == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				raw, rerr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
				if rerr == nil && json.Unmarshal(raw, &idx) == nil && len(idx.Packages) > 0 {
					normalizeIndex(&idx)
					return idx, indexURL, nil
				}
			}
		}
	}

	raw, rerr := os.ReadFile(fallbackPath)
	if rerr != nil {
		return RepoIndex{}, "", fmt.Errorf("read fallback index %q: %w", fallbackPath, rerr)
	}
	if jerr := json.Unmarshal(raw, &idx); jerr != nil {
		return RepoIndex{}, "", fmt.Errorf("parse fallback index %q: %w", fallbackPath, jerr)
	}
	normalizeIndex(&idx)
	return idx, fallbackPath + " (offline fixture)", nil
}

func normalizeIndex(idx *RepoIndex) {
	if idx.Packages == nil {
		idx.Packages = make(map[string]RepoPackage)
	}
}

// GeneratePackagesPages emits /packages/index.html, /packages/<pkg>.html, and /packages.txt.
func GeneratePackagesPages(outDir string, site SiteContext, idx RepoIndex, portsByName map[string]PortRecipe) error {
	pkgOutDir := filepath.Join(outDir, "packages")
	if err := os.MkdirAll(pkgOutDir, 0o755); err != nil {
		return err
	}

	names := make([]string, 0, len(idx.Packages))
	for name := range idx.Packages {
		names = append(names, name)
	}
	sort.Strings(names)

	// Build reverse dependency index across binary packages.
	revDeps := make(map[string][]string)
	for _, name := range names {
		latest := latestEntry(idx.Packages[name])
		for _, dep := range latest.Depends {
			if dep != name {
				revDeps[dep] = append(revDeps[dep], name)
			}
		}
	}
	for k := range revDeps {
		sort.Strings(revDeps[k])
	}

	// Group by first letter for zero-JS alphabetical jump bar.
	lettersPresent := make(map[string]bool)
	for _, name := range names {
		lettersPresent[alphaBucket(name)] = true
	}

	var indexBody strings.Builder
	indexBody.WriteString("<h1>Binary Packages</h1>\n")
	fmt.Fprintf(&indexBody, "<p class=\"meta-note\">%d package(s) indexed in <code>rolling-x86_64</code>. Plain-text index: <a href=\"/packages.txt\">/packages.txt</a>.</p>\n", len(names))
	indexBody.WriteString("<p>Install any package binary-first with <code>kpm install --deps &lt;package&gt;</code> after running <code>kpm sync</code>.</p>\n")
	indexBody.WriteString(renderSearchForm("", "packages", "Search binary packages..."))
	indexBody.WriteString(renderAlphaJumpBar(lettersPresent))

	indexBody.WriteString("<div class=\"table-wrap\">\n<table>\n<thead>\n<tr><th>Package</th><th>Version</th><th>Collection</th><th>Size</th><th>Description</th></tr>\n</thead>\n<tbody>\n")
	lastBucket := ""
	var plainList strings.Builder
	for _, name := range names {
		e := latestEntry(idx.Packages[name])
		bucket := alphaBucket(name)
		rowAttr := ""
		if bucket != lastBucket {
			rowAttr = fmt.Sprintf(" id=\"letter-%s\"", bucket)
			lastBucket = bucket
		}
		col := e.Collection
		if col == "" || col == "rolling" {
			if pr, ok := portsByName[name]; ok && pr.Collection != "" {
				col = pr.Collection
			} else {
				col = normalizeCollection(col, name)
			}
		}
		desc := e.Description
		if desc == "" {
			if pr, ok := portsByName[name]; ok {
				desc = pr.Description
			}
		}
		fmt.Fprintf(&indexBody, "<tr%s><td><a href=\"/packages/%s.html\">%s</a></td><td><code>%s</code></td><td>%s</td><td>%s</td><td>%s</td></tr>\n",
			rowAttr,
			html.EscapeString(name),
			html.EscapeString(name),
			html.EscapeString(e.FullVersion()),
			html.EscapeString(col),
			html.EscapeString(formatSize(e.Size)),
			html.EscapeString(desc),
		)
		fmt.Fprintf(&plainList, "%-24s %-16s %-10s %s\n", name, e.FullVersion(), col, desc)
	}
	indexBody.WriteString("</tbody>\n</table>\n</div>\n")

	if err := writeHTMLPage(filepath.Join(pkgOutDir, "index.html"), site, PageMeta{
		Title:       "Packages",
		Description: fmt.Sprintf("Browse %d prebuilt x86_64 binary packages for KryX Linux.", len(names)),
		ActiveNav:   "packages",
		Canonical:   "/packages/",
	}, indexBody.String()); err != nil {
		return err
	}

	// Plain-text terminal package list (/packages.txt)
	if err := os.WriteFile(filepath.Join(outDir, "packages.txt"), []byte(plainList.String()), 0o644); err != nil {
		return err
	}

	// Individual package detail pages (/packages/<pkg>.html)
	for _, name := range names {
		pkg := idx.Packages[name]
		latest := latestEntry(pkg)
		portRec, hasPort := portsByName[name]

		desc := latest.Description
		if desc == "" && hasPort {
			desc = portRec.Description
		}
		col := latest.Collection
		if (col == "" || col == "rolling") && hasPort && portRec.Collection != "" {
			col = portRec.Collection
		}
		col = normalizeCollection(col, name)

		var body strings.Builder
		fmt.Fprintf(&body, "<p class=\"meta-note\"><a href=\"/packages/\">&larr; All packages</a></p>\n")
		fmt.Fprintf(&body, "<h1>%s <code>%s</code></h1>\n", html.EscapeString(name), html.EscapeString(latest.FullVersion()))
		if desc != "" {
			fmt.Fprintf(&body, "<p>%s</p>\n", html.EscapeString(desc))
		}

		body.WriteString("<h2>Install</h2>\n")
		fmt.Fprintf(&body, "<pre><code>kpm install --deps %s</code></pre>\n", html.EscapeString(name))

		body.WriteString("<h2>Metadata</h2>\n<div class=\"table-wrap\">\n<table>\n<tbody>\n")
		fmt.Fprintf(&body, "<tr><th>Package</th><td><code>%s</code></td></tr>\n", html.EscapeString(name))
		fmt.Fprintf(&body, "<tr><th>Latest Version</th><td><code>%s</code></td></tr>\n", html.EscapeString(latest.FullVersion()))
		fmt.Fprintf(&body, "<tr><th>Collection</th><td><a href=\"/ports/%s/\">%s</a></td></tr>\n", html.EscapeString(col), html.EscapeString(col))
		fmt.Fprintf(&body, "<tr><th>Architecture</th><td><code>%s</code></td></tr>\n", html.EscapeString(defaultStr(latest.Arch, "x86_64")))
		fmt.Fprintf(&body, "<tr><th>Archive Size</th><td>%s</td></tr>\n", html.EscapeString(formatSize(latest.Size)))
		if latest.SHA256 != "" {
			fmt.Fprintf(&body, "<tr><th>SHA-256</th><td><code>%s</code></td></tr>\n", html.EscapeString(latest.SHA256))
		}
		if hasPort && portRec.URL != "" {
			fmt.Fprintf(&body, "<tr><th>Upstream URL</th><td><a href=\"%s\">%s</a></td></tr>\n", html.EscapeString(portRec.URL), html.EscapeString(portRec.URL))
		}
		if latest.URL != "" {
			fmt.Fprintf(&body, "<tr><th>Binary Archive</th><td><a href=\"%s\">%s</a></td></tr>\n", html.EscapeString(latest.URL), html.EscapeString(latest.Filename))
		}
		body.WriteString("</tbody>\n</table>\n</div>\n")

		deps := latest.Depends
		if len(deps) == 0 && hasPort {
			deps = portRec.Depends
		}
		makeDeps := latest.MakeDepends
		if len(makeDeps) == 0 && hasPort {
			makeDeps = portRec.MakeDepends
		}

		body.WriteString("<h2>Runtime Dependencies</h2>\n")
		body.WriteString(renderDepChips(deps, idx.Packages))

		body.WriteString("<h2>Make Dependencies</h2>\n")
		body.WriteString(renderDepChips(makeDeps, idx.Packages))

		body.WriteString("<h2>Required By</h2>\n")
		body.WriteString(renderDepChips(revDeps[name], idx.Packages))

		if len(pkg.Versions) > 1 {
			body.WriteString("<h2>Available Versions</h2>\n<div class=\"table-wrap\">\n<table>\n<thead>\n<tr><th>Version</th><th>Arch</th><th>Size</th><th>Pin Command</th></tr>\n</thead>\n<tbody>\n")
			for i := len(pkg.Versions) - 1; i >= 0; i-- {
				v := pkg.Versions[i]
				fmt.Fprintf(&body, "<tr><td><code>%s</code></td><td>%s</td><td>%s</td><td><code>kpm install %s=%s</code></td></tr>\n",
					html.EscapeString(v.FullVersion()),
					html.EscapeString(defaultStr(v.Arch, "x86_64")),
					html.EscapeString(formatSize(v.Size)),
					html.EscapeString(name),
					html.EscapeString(v.FullVersion()),
				)
			}
			body.WriteString("</tbody>\n</table>\n</div>\n")
		}

		if hasPort && strings.TrimSpace(portRec.RawPkgfile) != "" {
			fmt.Fprintf(&body, "<h2>Pkgfile (<code>%s</code>)</h2>\n", html.EscapeString(portRec.RelPath))
			fmt.Fprintf(&body, "<pre><code>%s</code></pre>\n", html.EscapeString(portRec.RawPkgfile))
		}

		if err := writeHTMLPage(filepath.Join(pkgOutDir, name+".html"), site, PageMeta{
			Title:       fmt.Sprintf("%s %s — Packages", name, latest.FullVersion()),
			Description: defaultStr(desc, fmt.Sprintf("KryX Linux binary package %s %s.", name, latest.FullVersion())),
			ActiveNav:   "packages",
			Canonical:   "/packages/" + name + ".html",
		}, body.String()); err != nil {
			return err
		}
	}
	return nil
}

func latestEntry(p RepoPackage) RepoEntry {
	if len(p.Versions) == 0 {
		return RepoEntry{}
	}
	return p.Versions[len(p.Versions)-1]
}

func renderDepChips(deps []string, indexed map[string]RepoPackage) string {
	if len(deps) == 0 {
		return "<p class=\"meta-note\">None</p>\n"
	}
	var sb strings.Builder
	sb.WriteString("<ul class=\"dep-list\">\n")
	for _, d := range deps {
		if _, ok := indexed[d]; ok {
			fmt.Fprintf(&sb, "<li><a href=\"/packages/%s.html\">%s</a></li>\n", html.EscapeString(d), html.EscapeString(d))
		} else {
			fmt.Fprintf(&sb, "<li><span>%s</span></li>\n", html.EscapeString(d))
		}
	}
	sb.WriteString("</ul>\n")
	return sb.String()
}

func alphaBucket(name string) string {
	if name == "" {
		return "0"
	}
	c := strings.ToUpper(name[:1])[0]
	if c >= 'A' && c <= 'Z' {
		return string(c)
	}
	return "0"
}

func renderAlphaJumpBar(present map[string]bool) string {
	var sb strings.Builder
	sb.WriteString("<nav class=\"alpha-nav\" aria-label=\"Alphabetical index\">\n")
	buckets := []string{"0"}
	for ch := 'A'; ch <= 'Z'; ch++ {
		buckets = append(buckets, string(ch))
	}
	for _, b := range buckets {
		label := b
		if b == "0" {
			label = "0–9"
		}
		if present[b] {
			fmt.Fprintf(&sb, "<a href=\"#letter-%s\">%s</a>\n", b, label)
		} else {
			fmt.Fprintf(&sb, "<span>%s</span>\n", label)
		}
	}
	sb.WriteString("</nav>\n")
	return sb.String()
}

func formatSize(n int64) string {
	if n <= 0 {
		return "—"
	}
	const (
		kib = 1024
		mib = 1024 * kib
	)
	switch {
	case n >= mib:
		return fmt.Sprintf("%.1f MiB", float64(n)/float64(mib))
	case n >= kib:
		return fmt.Sprintf("%.1f KiB", float64(n)/float64(kib))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func defaultStr(v, fb string) string {
	if strings.TrimSpace(v) == "" {
		return fb
	}
	return v
}
