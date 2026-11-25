package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// PortRecipe represents a parsed CRUX/KryX Pkgfile discovered in a ports tree.
type PortRecipe struct {
	Collection  string
	DirName     string
	RelPath     string // e.g. "pkg-tools/kpm"
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

var portCollectionsOrder = []string{"pkg-tools", "kernel", "system", "core", "opt", "xorg", "contrib", "compat-32"}

var collectionDescriptions = map[string]string{
	"pkg-tools": "KryX Linux first-party package manager & toolchain ports (kpm with integrated ports, zig)",
	"kernel":    "KryX Linux rolling LTS kernel recipes (linux6.18)",
	"system":    "KryX Linux first-party system & init ports (dinit, dinit-kryx, turnstile)",
	"core":      "Essential CRUX 3.8-derived base system, toolchain, and runtime libraries",
	"opt":       "Optional system utilities, GRUB bootloaders, Wayland stack, and applications",
	"xorg":      "X.Org libraries, Wayland/DRM compatibility, fonts, and graphics drivers",
	"contrib":   "Community-maintained third-party CRUX/KryX ports",
	"compat-32": "32-bit multilib compatibility libraries for Steam, Wine, and legacy x86 software",
}

// normalizeCollection maps legacy collection names (like "rolling") onto active KryX collections.
func normalizeCollection(col, pkgName string) string {
	col = strings.TrimSpace(col)
	if col == "" || col == "rolling" {
		if pkgName == "kernel" || strings.HasPrefix(pkgName, "linux") {
			return "kernel"
		}
		return "pkg-tools"
	}
	return col
}

// DiscoverPorts scans portsDir (and sibling collection repos such as ../core, ../opt, ../xorg, ../contrib)
// for KryX/CRUX collections (pkg-tools, kernel, core, opt, xorg, contrib).
// When offline is false, any collection still missing locally is fetched directly from GitHub tarballs.
// If still completely empty (e.g. offline standalone build), falls back to synthesizing from idx.
func DiscoverPorts(portsDir string, idx RepoIndex, offline bool) (byCollection map[string][]PortRecipe, byName map[string]PortRecipe) {
	byCollection = make(map[string][]PortRecipe)
	byName = make(map[string]PortRecipe)

	addRecipes := func(col string, recipes []PortRecipe) {
		for _, pr := range recipes {
			byCollection[col] = append(byCollection[col], pr)
			if _, exists := byName[pr.Name]; !exists {
				byName[pr.Name] = pr
			}
		}
	}

	if root := strings.TrimSpace(portsDir); root != "" {
		parent := filepath.Dir(filepath.Clean(root))
		for _, col := range portCollectionsOrder {
			candidates := []string{filepath.Join(root, col)}
			if parent != "" && parent != root {
				candidates = append(candidates, filepath.Join(parent, col))
			}
			for _, colDir := range candidates {
				recipes := scanCollectionDir(col, colDir)
				if len(recipes) > 0 {
					addRecipes(col, recipes)
					break
				}
			}
		}
	}

	// Fetch any missing collections directly from GitHub tarballs when online.
	// This keeps /ports/ accurate both in GitHub Actions and on Cloudflare Pages direct builds.
	if !offline {
		var missing []string
		for _, col := range portCollectionsOrder {
			if len(byCollection[col]) == 0 {
				missing = append(missing, col)
			}
		}
		if len(missing) > 0 {
			fetched := fetchMissingCollections(missing)
			for col, recipes := range fetched {
				if len(byCollection[col]) == 0 && len(recipes) > 0 {
					addRecipes(col, recipes)
				}
			}
		}
	}

	// Offline/last-resort fallback: synthesize port entries from idx so standalone offline builds succeed.
	if len(byName) == 0 {
		for name, pkg := range idx.Packages {
			e := latestEntry(pkg)
			col := normalizeCollection(e.Collection, name)
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

func scanCollectionDir(col, colDir string) []PortRecipe {
	entries, err := os.ReadDir(colDir)
	if err != nil {
		return nil
	}
	var out []PortRecipe
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		pkgfilePath := filepath.Join(colDir, e.Name(), "Pkgfile")
		raw, err := os.ReadFile(pkgfilePath)
		if err != nil {
			continue
		}
		out = append(out, parsePkgfileStatic(col, e.Name(), string(raw)))
	}
	return out
}

func fetchMissingCollections(missing []string) map[string][]PortRecipe {
	result := make(map[string][]PortRecipe)
	var mu sync.Mutex
	var wg sync.WaitGroup

	client := &http.Client{Timeout: 45 * time.Second}
	needMetaPorts := false
	metaWant := make(map[string]bool)
	var standalone []string

	for _, col := range missing {
		switch col {
		case "pkg-tools", "kernel", "system":
			needMetaPorts = true
			metaWant[col] = true
		default:
			standalone = append(standalone, col)
		}
	}

	if needMetaPorts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := fetchRepoTarballPorts(client, "kryx-linux/ports", []string{"main"}, "", metaWant)
			mu.Lock()
			for col, list := range got {
				result[col] = append(result[col], list...)
			}
			mu.Unlock()
		}()
	}

	for _, col := range standalone {
		col := col
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := fetchRepoTarballPorts(client, "kryx-linux/"+col, []string{"main", "lts", "crux", "3.8"}, col, nil)
			mu.Lock()
			for c, list := range got {
				result[c] = append(result[c], list...)
			}
			mu.Unlock()
		}()
	}

	wg.Wait()
	return result
}

func fetchRepoTarballPorts(client *http.Client, repo string, branches []string, fixedCol string, multiCols map[string]bool) map[string][]PortRecipe {
	for _, br := range branches {
		url := fmt.Sprintf("https://github.com/%s/archive/refs/heads/%s.tar.gz", repo, br)
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}
		got, err := extractPortsFromTarGz(resp.Body, fixedCol, multiCols)
		resp.Body.Close()
		if err == nil && len(got) > 0 {
			return got
		}
	}
	return nil
}

// extractPortsFromTarGz reads Pkgfiles directly from a GitHub repo archive stream.
// If fixedCol != "", expects "<top>/<port>/Pkgfile".
// Otherwise expects "<top>/<col>/<port>/Pkgfile" for collections present in multiCols.
func extractPortsFromTarGz(r io.Reader, fixedCol string, multiCols map[string]bool) (map[string][]PortRecipe, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	out := make(map[string][]PortRecipe)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return out, err
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}
		clean := path.Clean(strings.TrimPrefix(hdr.Name, "./"))
		parts := strings.Split(clean, "/")
		var col, dirName string
		if fixedCol != "" {
			if len(parts) != 3 || parts[2] != "Pkgfile" || strings.HasPrefix(parts[1], ".") {
				continue
			}
			col = fixedCol
			dirName = parts[1]
		} else {
			if len(parts) != 4 || parts[3] != "Pkgfile" || strings.HasPrefix(parts[2], ".") {
				continue
			}
			if !multiCols[parts[1]] {
				continue
			}
			col = parts[1]
			dirName = parts[2]
		}
		raw, err := io.ReadAll(io.LimitReader(tr, 256<<10))
		if err != nil {
			return out, err
		}
		out[col] = append(out[col], parsePkgfileStatic(col, dirName, string(raw)))
	}
	return out, nil
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

// GeneratePortsPages emits /ports/index.html, /ports/<col>/index.html, and recipe pages for KryX first-party ports.
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
	indexBody.WriteString("<p>Sync all local collections natively with <code>ports -u</code> (pure-Go <code>git</code>/<code>rsync</code>/<code>httpup</code> integrated into <code>kpm</code>) and build any port from source with <code>kpm build --deps --sync &lt;port&gt;</code>.</p>\n")

	indexBody.WriteString("<h2>Collections</h2>\n<div class=\"table-wrap\">\n<table>\n<thead>\n<tr><th>Collection</th><th>Ports</th><th>Description</th></tr>\n</thead>\n<tbody>\n")
	for _, col := range portCollectionsOrder {
		list := byCol[col]
		desc := collectionDescriptions[col]
		fmt.Fprintf(&indexBody, "<tr><td><a href=\"/ports/%s/\"><strong>%s</strong></a></td><td>%d</td><td>%s</td></tr>\n",
			html.EscapeString(col), html.EscapeString(col), len(list), html.EscapeString(desc))
	}
	indexBody.WriteString("</tbody>\n</table>\n</div>\n")

	indexBody.WriteString("<h2>Quick Commands</h2>\n<pre><code>ports check\nsudo ports -u\nports -l\nports -d\nkpm build --deps --sync --install &lt;port&gt;</code></pre>\n")

	if err := writeHTMLPage(filepath.Join(portsOutDir, "index.html"), site, PageMeta{
		Title:       "Ports",
		Description: "Browse KryX Linux and CRUX port collections (pkg-tools, kernel, core, opt, xorg, contrib).",
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
			body.WriteString("<p class=\"meta-note\">No local ports scanned for this collection in the current build snapshot. Sync locally with <code>ports -u " + html.EscapeString(col) + "</code>.</p>\n")
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
				isFirstParty := col == "pkg-tools" || col == "kernel"
				portLink := ""
				if hasBin {
					portLink = fmt.Sprintf("<a href=\"/packages/%s.html\"><strong>%s</strong></a>", html.EscapeString(p.Name), html.EscapeString(p.Name))
				} else if isFirstParty && p.RawPkgfile != "" {
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

				// Emit recipe page for first-party KryX ports even if not yet in index.json.
				if isFirstParty && p.RawPkgfile != "" {
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
