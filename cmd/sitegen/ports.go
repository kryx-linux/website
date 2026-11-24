package main

import (
	"bufio"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PortRecipe represents a parsed CRUX/KryX Pkgfile discovered in a ports tree.
type PortRecipe struct {
	Collection  string
	DirName     string
	RelPath     string // e.g. "rolling/gports"
	Name        string
	Version     string
	Release     string
	Description string
	URL         string
	Maintainer  string
	Depends     []string
	MakeDepends []string
	RawPkgfile  string
}

func (p PortRecipe) FullVersion() string {
	if p.Release == "" {
		return p.Version
	}
	return p.Version + "-" + p.Release
}

var portCollectionsOrder = []string{"pkg-tools", "kernel", "core", "opt", "xorg", "contrib"}

var collectionDescriptions = map[string]string{
	"pkg-tools": "KryX Linux first-party package manager & toolchain ports (kpm with integrated ports, zig)",
	"kernel":    "KryX Linux rolling LTS kernel recipes (linux6.18)",
	"core":      "Essential CRUX 3.8-derived base system, toolchain, and runtime libraries",
	"opt":       "Optional system utilities, GRUB bootloaders, Wayland stack, and applications",
	"xorg":      "X.Org libraries, Wayland/DRM compatibility, fonts, and graphics drivers",
	"contrib":   "Community-maintained third-party CRUX/KryX ports",
}

// DiscoverPorts scans portsDir for standard CRUX/KryX collections (rolling, core, opt, xorg, contrib).
// If portsDir is absent or empty, falls back to synthesizing port entries from idx so standalone builds succeed.
func DiscoverPorts(portsDir string, idx RepoIndex) (byCollection map[string][]PortRecipe, byName map[string]PortRecipe) {
	byCollection = make(map[string][]PortRecipe)
	byName = make(map[string]PortRecipe)

	if strings.TrimSpace(portsDir) != "" {
		for _, col := range portCollectionsOrder {
			colDir := filepath.Join(portsDir, col)
			entries, err := os.ReadDir(colDir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
					continue
				}
				pkgfilePath := filepath.Join(colDir, e.Name(), "Pkgfile")
				raw, err := os.ReadFile(pkgfilePath)
				if err != nil {
					continue
				}
				pr := parsePkgfileStatic(col, e.Name(), string(raw))
				byCollection[col] = append(byCollection[col], pr)
				if _, exists := byName[pr.Name]; !exists {
					byName[pr.Name] = pr
				}
			}
		}
	}

	// Ensure every indexed binary package is also represented in ports when building without a full ports clone.
	if len(byName) == 0 {
		for name, pkg := range idx.Packages {
			e := latestEntry(pkg)
			col := defaultStr(e.Collection, "pkg-tools")
			pr := PortRecipe{
				Collection:  col,
				DirName:     name,
				RelPath:     col + "/" + name,
				Name:        name,
				Version:     e.Pkgver,
				Release:     e.Pkgrel,
				Description: e.Description,
				Depends:     e.Depends,
				MakeDepends: e.MakeDepends,
			}
			byCollection[col] = append(byCollection[col], pr)
			byName[name] = pr
		}
	}

	for col := range byCollection {
		sort.Slice(byCollection[col], func(i, j int) bool {
			return byCollection[col][i].Name < byCollection[col][j].Name
		})
	}
	return byCollection, byName
}

func parsePkgfileStatic(col, dirName, raw string) PortRecipe {
	pr := PortRecipe{
		Collection: col,
		DirName:    dirName,
		RelPath:    col + "/" + dirName,
		Name:       dirName,
		RawPkgfile: raw,
	}
	vars := make(map[string]string)

	scanner := bufio.NewScanner(strings.NewReader(raw))
	inHeader := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if !inHeader {
				continue
			}
			body := strings.TrimSpace(strings.TrimPrefix(line, "#"))
			if colon := strings.IndexByte(body, ':'); colon > 0 {
				k := strings.ToLower(strings.TrimSpace(body[:colon]))
				v := strings.TrimSpace(body[colon+1:])
				switch k {
				case "description":
					pr.Description = v
				case "url":
					pr.URL = v
				case "maintainer":
					pr.Maintainer = v
				case "depends on", "depends":
					pr.Depends = append(pr.Depends, splitDepList(v)...)
				case "make depends on", "make depends", "makedepends", "build depends on", "build depends":
					pr.MakeDepends = append(pr.MakeDepends, splitDepList(v)...)
				}
			}
			continue
		}
		inHeader = false
		if strings.HasPrefix(line, "build()") || strings.HasPrefix(line, "build ()") {
			break
		}
		if eq := strings.IndexByte(line, '='); eq > 0 {
			k := strings.TrimSpace(line[:eq])
			v := strings.TrimSpace(line[eq+1:])
			v = strings.Trim(v, `"'`)
			if !strings.ContainsAny(k, " \t()") {
				v = expandSimpleVars(v, vars)
				vars[k] = v
				switch k {
				case "name":
					if v != "" {
						pr.Name = v
					}
				case "version":
					pr.Version = v
				case "release":
					pr.Release = v
				}
			}
		}
	}
	return pr
}

func expandSimpleVars(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
		s = strings.ReplaceAll(s, "$"+k, v)
	}
	return s
}

func splitDepList(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	var out []string
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// GeneratePortsPages emits /ports/index.html, /ports/<col>/index.html, and recipe pages for rolling ports.
func GeneratePortsPages(outDir string, site SiteContext, byCol map[string][]PortRecipe, idx RepoIndex) error {
	portsOutDir := filepath.Join(outDir, "ports")
	if err := os.MkdirAll(portsOutDir, 0o755); err != nil {
		return err
	}

	totalPorts := 0
	for _, col := range portCollectionsOrder {
		totalPorts += len(byCol[col])
	}

	var indexBody strings.Builder
	indexBody.WriteString("<h1>Ports Tree</h1>\n")
	fmt.Fprintf(&indexBody, "<p class=\"meta-note\">%d port recipe(s) across KryX &amp; CRUX collections.</p>\n", totalPorts)
	indexBody.WriteString("<p>Sync all local collections natively with <code>gports -u</code> (pure-Go <code>git</code>/<code>rsync</code>/<code>httpup</code>) and build any port from source with <code>kpm build --deps --sync &lt;port&gt;</code>.</p>\n")

	indexBody.WriteString("<h2>Collections</h2>\n<div class=\"table-wrap\">\n<table>\n<thead>\n<tr><th>Collection</th><th>Ports</th><th>Description</th></tr>\n</thead>\n<tbody>\n")
	for _, col := range portCollectionsOrder {
		list := byCol[col]
		desc := collectionDescriptions[col]
		fmt.Fprintf(&indexBody, "<tr><td><a href=\"/ports/%s/\"><strong>%s</strong></a></td><td>%d</td><td>%s</td></tr>\n",
			html.EscapeString(col), html.EscapeString(col), len(list), html.EscapeString(desc))
	}
	indexBody.WriteString("</tbody>\n</table>\n</div>\n")

	indexBody.WriteString("<h2>Quick Commands</h2>\n<pre><code>gports check\nsudo gports -u\ngports -l\ngports -d\nkpm build --deps --sync --install &lt;port&gt;</code></pre>\n")

	if err := writeHTMLPage(filepath.Join(portsOutDir, "index.html"), site, PageMeta{
		Title:       "Ports",
		Description: "Browse KryX Linux and CRUX port collections (rolling, core, opt, xorg, contrib).",
		ActiveNav:   "ports",
		Canonical:   "/ports/",
	}, indexBody.String()); err != nil {
		return err
	}

	// Per-collection pages (/ports/<col>/index.html)
	for _, col := range portCollectionsOrder {
		colDir := filepath.Join(portsOutDir, col)
		if err := os.MkdirAll(colDir, 0o755); err != nil {
			return err
		}
		list := byCol[col]
		letters := make(map[string]bool)
		for _, p := range list {
			letters[alphaBucket(p.Name)] = true
		}

		var body strings.Builder
		body.WriteString("<p class=\"meta-note\"><a href=\"/ports/\">&larr; All collections</a></p>\n")
		fmt.Fprintf(&body, "<h1>Ports: <code>%s</code></h1>\n", html.EscapeString(col))
		fmt.Fprintf(&body, "<p>%s (%d port(s)).</p>\n", html.EscapeString(collectionDescriptions[col]), len(list))

		if len(list) == 0 {
			body.WriteString("<p class=\"meta-note\">No local ports scanned for this collection in the current build snapshot. Sync locally with <code>gports -u " + html.EscapeString(col) + "</code>.</p>\n")
		} else {
			body.WriteString(renderAlphaJumpBar(letters))
			body.WriteString("<div class=\"table-wrap\">\n<table>\n<thead>\n<tr><th>Port</th><th>Version</th><th>Binary</th><th>Description</th></tr>\n</thead>\n<tbody>\n")
			lastBucket := ""
			for _, p := range list {
				bucket := alphaBucket(p.Name)
				rowAttr := ""
				if bucket != lastBucket {
					rowAttr = fmt.Sprintf(" id=\"letter-%s\"", bucket)
					lastBucket = bucket
				}
				_, hasBin := idx.Packages[p.Name]
				portLink := ""
				if hasBin {
					portLink = fmt.Sprintf("<a href=\"/packages/%s.html\"><strong>%s</strong></a>", html.EscapeString(p.Name), html.EscapeString(p.Name))
				} else if col == "rolling" && p.RawPkgfile != "" {
					portLink = fmt.Sprintf("<a href=\"/ports/%s/%s.html\"><strong>%s</strong></a>", html.EscapeString(col), html.EscapeString(p.DirName), html.EscapeString(p.Name))
				} else {
					portLink = fmt.Sprintf("<strong>%s</strong>", html.EscapeString(p.Name))
				}
				binStatus := "source"
				if hasBin {
					binStatus = fmt.Sprintf("<a href=\"/packages/%s.html\">prebuilt</a>", html.EscapeString(p.Name))
				}
				fmt.Fprintf(&body, "<tr%s><td>%s</td><td><code>%s</code></td><td>%s</td><td>%s</td></tr>\n",
					rowAttr,
					portLink,
					html.EscapeString(defaultStr(p.FullVersion(), "—")),
					binStatus,
					html.EscapeString(p.Description),
				)

				// Emit recipe page for rolling ports even if not yet in index.json.
				if col == "rolling" && p.RawPkgfile != "" {
					if err := emitPortDetailPage(filepath.Join(colDir, p.DirName+".html"), site, p, idx); err != nil {
						return err
					}
				}
			}
			body.WriteString("</tbody>\n</table>\n</div>\n")
		}

		if err := writeHTMLPage(filepath.Join(colDir, "index.html"), site, PageMeta{
			Title:       fmt.Sprintf("%s — Ports", col),
			Description: fmt.Sprintf("KryX Linux %s port collection (%d ports).", col, len(list)),
			ActiveNav:   "ports",
			Canonical:   "/ports/" + col + "/",
		}, body.String()); err != nil {
			return err
		}
	}
	return nil
}

func emitPortDetailPage(dst string, site SiteContext, p PortRecipe, idx RepoIndex) error {
	var body strings.Builder
	fmt.Fprintf(&body, "<p class=\"meta-note\"><a href=\"/ports/%s/\">&larr; %s collection</a></p>\n",
		html.EscapeString(p.Collection), html.EscapeString(p.Collection))
	fmt.Fprintf(&body, "<h1>%s <code>%s</code></h1>\n", html.EscapeString(p.Name), html.EscapeString(p.FullVersion()))
	if p.Description != "" {
		fmt.Fprintf(&body, "<p>%s</p>\n", html.EscapeString(p.Description))
	}
	fmt.Fprintf(&body, "<h2>Build from Source</h2>\n<pre><code>kpm build --deps --sync --install %s</code></pre>\n", html.EscapeString(p.RelPath))
	body.WriteString("<h2>Runtime Dependencies</h2>\n" + renderDepChips(p.Depends, idx.Packages))
	body.WriteString("<h2>Make Dependencies</h2>\n" + renderDepChips(p.MakeDepends, idx.Packages))
	fmt.Fprintf(&body, "<h2>Pkgfile</h2>\n<pre><code>%s</code></pre>\n", html.EscapeString(p.RawPkgfile))

	return writeHTMLPage(dst, site, PageMeta{
		Title:       fmt.Sprintf("%s — %s Port", p.Name, p.Collection),
		Description: defaultStr(p.Description, fmt.Sprintf("Pkgfile recipe for %s.", p.Name)),
		ActiveNav:   "ports",
		Canonical:   fmt.Sprintf("/ports/%s/%s.html", p.Collection, p.DirName),
	}, body.String())
}
