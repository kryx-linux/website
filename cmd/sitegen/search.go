package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// SearchEntry is a compact record emitted into public/search-index.json
// and consumed by both the Cloudflare Pages Function (functions/search.js)
// and sitegen's local -serve HTTP handler. Zero client-side JS is ever sent.
type SearchEntry struct {
	Kind        string `json:"k"`           // package, port, doc, man, news
	Name        string `json:"n"`           // package/port name or page title
	Version     string `json:"v,omitempty"` // version-release for packages/ports
	Collection  string `json:"c,omitempty"` // collection name for packages/ports
	Description string `json:"d,omitempty"` // concise summary
	URL         string `json:"u"`           // canonical path starting with /
}

// DocRecord captures indexed metadata for a rendered Handbook document.
type DocRecord struct {
	Slug        string
	Title       string
	Description string
	URL         string
}

var validSearchScopes = []struct {
	Value string
	Label string
}{
	{"all", "All"},
	{"packages", "Packages"},
	{"ports", "Ports"},
	{"docs", "Handbook"},
	{"man", "Man Pages"},
	{"news", "News"},
}

// renderSearchForm returns a pure-HTML <form method="get" action="/search"> that
// works identically in lynx, w3m, links, curl, and graphical browsers.
func renderSearchForm(query, scope, placeholder string) string {
	scope = normalizeSearchScope(scope)
	if strings.TrimSpace(placeholder) == "" {
		placeholder = "Search packages, ports, docs, man pages..."
	}
	var sb strings.Builder
	sb.WriteString("<form class=\"search-form\" action=\"/search\" method=\"get\" role=\"search\">\n")
	sb.WriteString("  <label class=\"sr-only\" for=\"q\">Search query</label>\n")
	fmt.Fprintf(&sb, "  <input id=\"q\" type=\"search\" name=\"q\" value=\"%s\" placeholder=\"%s\">\n",
		html.EscapeString(query), html.EscapeString(placeholder))
	sb.WriteString("  <label class=\"sr-only\" for=\"scope\">Scope</label>\n")
	sb.WriteString("  <select id=\"scope\" name=\"scope\">\n")
	for _, opt := range validSearchScopes {
		sel := ""
		if opt.Value == scope {
			sel = " selected"
		}
		fmt.Fprintf(&sb, "    <option value=\"%s\"%s>%s</option>\n",
			html.EscapeString(opt.Value), sel, html.EscapeString(opt.Label))
	}
	sb.WriteString("  </select>\n")
	sb.WriteString("  <button type=\"submit\">Search</button>\n")
	sb.WriteString("</form>\n")
	return sb.String()
}

func normalizeSearchScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "packages", "package", "pkg":
		return "packages"
	case "ports", "port":
		return "ports"
	case "docs", "doc", "handbook":
		return "docs"
	case "man", "manpages":
		return "man"
	case "news":
		return "news"
	default:
		return "all"
	}
}

// BuildSearchIndex builds the unified search catalog across binary packages,
// ports tree recipes, Handbook guides, man pages, and news posts.
func BuildSearchIndex(idx RepoIndex, portsByCol map[string][]PortRecipe, portsByName map[string]PortRecipe, docs []DocRecord, manPages []ManPage, newsPosts []NewsPost) []SearchEntry {
	entries := make([]SearchEntry, 0, len(idx.Packages)+len(docs)+len(manPages)+len(newsPosts)+256)

	// 1. Binary packages
	pkgNames := make([]string, 0, len(idx.Packages))
	for name := range idx.Packages {
		pkgNames = append(pkgNames, name)
	}
	sort.Strings(pkgNames)
	for _, name := range pkgNames {
		e := latestEntry(idx.Packages[name])
		col := e.Collection
		desc := e.Description
		if pr, ok := portsByName[name]; ok {
			if col == "" || col == "rolling" || col == "pkg-tools" {
				col = pr.Collection
			}
			if desc == "" {
				desc = pr.Description
			}
		}
		col = normalizeCollection(col, name)
		entries = append(entries, SearchEntry{
			Kind:        "package",
			Name:        name,
			Version:     e.FullVersion(),
			Collection:  col,
			Description: desc,
			URL:         "/packages/" + name + ".html",
		})
	}

	// 2. Ports recipes (every collection now has its own static detail page)
	for _, col := range portCollectionsOrder {
		for _, p := range portsByCol[col] {
			u := fmt.Sprintf("/ports/%s/%s.html", col, p.DirName)
			entries = append(entries, SearchEntry{
				Kind:        "port",
				Name:        p.Name,
				Version:     p.FullVersion(),
				Collection:  col,
				Description: p.Description,
				URL:         u,
			})
		}
	}

	// 3. Handbook guides
	for _, d := range docs {
		entries = append(entries, SearchEntry{
			Kind:        "doc",
			Name:        d.Title,
			Description: d.Description,
			URL:         d.URL,
		})
	}

	// 4. Web man pages
	for _, mp := range manPages {
		entries = append(entries, SearchEntry{
			Kind:        "man",
			Name:        mp.Title,
			Description: mp.Summary,
			URL:         "/man/" + mp.Filename + ".html",
		})
	}

	// 5. News posts
	for _, np := range newsPosts {
		entries = append(entries, SearchEntry{
			Kind:        "news",
			Name:        np.Title,
			Version:     np.Date,
			Description: np.Description,
			URL:         "/news/" + np.Slug + ".html",
		})
	}

	return entries
}

// GenerateSearchAssets writes public/search-index.json, public/ports.txt, and
// public/search/index.html.
func GenerateSearchAssets(outDir string, site SiteContext, entries []SearchEntry, portsByCol map[string][]PortRecipe) error {
	// 1. /search-index.json (compact JSON for Cloudflare Pages Function & local server)
	rawJSON, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("marshal search-index.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "search-index.json"), rawJSON, 0o644); err != nil {
		return err
	}

	// 2. /ports.txt (plain-text index for curl | grep and lynx/w3m)
	var portsTxt strings.Builder
	for _, col := range portCollectionsOrder {
		for _, p := range portsByCol[col] {
			fmt.Fprintf(&portsTxt, "%-10s %-26s %-16s %s\n", col, p.Name, defaultStr(p.FullVersion(), "-"), p.Description)
		}
	}
	if err := os.WriteFile(filepath.Join(outDir, "ports.txt"), []byte(portsTxt.String()), 0o644); err != nil {
		return err
	}

	// 3. /search/index.html (zero-JS HTML search page)
	var body strings.Builder
	body.WriteString("<h1>Search KryX Linux</h1>\n")
	fmt.Fprintf(&body, "<p class=\"meta-note\">Server-side, zero-JavaScript search across %d indexed packages, ports, Handbook guides, man pages, and news posts. Works natively in <code>lynx</code>, <code>w3m</code>, <code>links</code>, and <code>curl</code>.</p>\n", len(entries))
	body.WriteString(renderSearchForm("", "all", "e.g. kpm, wlroots, dinit, grub2, deb..."))

	body.WriteString("<h2>Example Queries</h2>\n<ul>\n")
	body.WriteString("<li><a href=\"/search?q=kpm\">kpm</a> — package manager, man page, and Handbook guide</li>\n")
	body.WriteString("<li><a href=\"/search?q=dinit&amp;scope=ports\">dinit (ports)</a> — opt-in service supervision stack</li>\n")
	body.WriteString("<li><a href=\"/search?q=wlroots&amp;scope=packages\">wlroots (packages)</a> — prebuilt Wayland compositor library</li>\n")
	body.WriteString("<li><a href=\"/search?q=deb&amp;scope=docs\">deb (handbook)</a> — packaging <code>.deb</code> archives into a <code>Pkgfile</code></li>\n")
	body.WriteString("</ul>\n")

	body.WriteString("<h2>Terminal &amp; Text-Browser Search</h2>\n")
	body.WriteString("<p>Query directly from a shell or live ISO without a graphical browser:</p>\n")
	body.WriteString("<pre><code># Plain-text server-side search\ncurl -s \"https://kryx.anrix.org/search?q=wlroots&amp;format=txt\"\n\n# Interactive text browsers\nlynx \"https://kryx.anrix.org/search?q=dinit\"\nw3m  \"https://kryx.anrix.org/search?q=dinit\"\n\n# Local grep over plain-text indices\ncurl -s https://kryx.anrix.org/packages.txt | grep -i wlroots\ncurl -s https://kryx.anrix.org/ports.txt    | grep -i dinit</code></pre>\n")

	return writeHTMLPage(filepath.Join(outDir, "search", "index.html"), site, PageMeta{
		Title:       "Search",
		Description: "Zero-JavaScript search across KryX Linux binary packages, ports recipes, Handbook docs, and man pages.",
		ActiveNav:   "",
		Canonical:   "/search/",
	}, body.String())
}

// SearchResult pairs a matched SearchEntry with its relevance score.
type SearchResult struct {
	Entry SearchEntry
	Score int
}

// FilterSearchEntries ranks entries matching every token in query within scope.
func FilterSearchEntries(entries []SearchEntry, query, scope string, limit int) []SearchResult {
	scope = normalizeSearchScope(scope)
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	if len(tokens) == 0 {
		return nil
	}
	fullQuery := strings.Join(tokens, " ")

	var matches []SearchResult
	for _, e := range entries {
		if !entryMatchesScope(e.Kind, scope) {
			continue
		}
		nameL := strings.ToLower(e.Name)
		colL := strings.ToLower(e.Collection)
		descL := strings.ToLower(e.Description)
		kindL := strings.ToLower(e.Kind)

		allFound := true
		score := 0
		for _, tok := range tokens {
			switch {
			case nameL == tok:
				score += 1000
			case strings.HasPrefix(nameL, tok):
				score += 500
			case strings.Contains(nameL, tok):
				score += 250
			case colL != "" && colL == tok:
				score += 140
			case colL != "" && strings.Contains(colL, tok):
				score += 100
			case strings.Contains(descL, tok):
				score += 60
			case kindL == tok:
				score += 40
			default:
				allFound = false
			}
			if !allFound {
				break
			}
		}
		if !allFound {
			continue
		}
		if nameL == fullQuery {
			score += 600
		} else if strings.HasPrefix(nameL, fullQuery) {
			score += 200
		}
		// Prefer prebuilt packages slightly over source-only ports on equal scores.
		switch e.Kind {
		case "package":
			score += 25
		case "port":
			score += 15
		case "doc":
			score += 12
		case "man":
			score += 10
		}
		matches = append(matches, SearchResult{Entry: e, Score: score})
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		if matches[i].Entry.Name != matches[j].Entry.Name {
			return matches[i].Entry.Name < matches[j].Entry.Name
		}
		return matches[i].Entry.Kind < matches[j].Entry.Kind
	})
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

func entryMatchesScope(kind, scope string) bool {
	switch scope {
	case "all":
		return true
	case "packages":
		return kind == "package"
	case "ports":
		return kind == "port"
	case "docs":
		return kind == "doc"
	case "man":
		return kind == "man"
	case "news":
		return kind == "news"
	default:
		return true
	}
}

// NewSiteHandler returns an http.Handler serving static files from outDir plus
// server-side zero-JS /search, /browse, /deps, and /theme handlers (matching
// website/functions/*.js on Cloudflare Pages).
func NewSiteHandler(outDir string, site SiteContext) http.Handler {
	fileServer := http.FileServer(http.Dir(outDir))
	var (
		loadOnce sync.Once
		entries  []SearchEntry
		loadErr  error

		depsOnce  sync.Once
		depsGraph map[string]DepNode
		depsErr   error
	)
	loadIndex := func() ([]SearchEntry, error) {
		loadOnce.Do(func() {
			raw, err := os.ReadFile(filepath.Join(outDir, "search-index.json"))
			if err != nil {
				loadErr = err
				return
			}
			loadErr = json.Unmarshal(raw, &entries)
		})
		return entries, loadErr
	}
	loadDeps := func() (map[string]DepNode, error) {
		depsOnce.Do(func() {
			raw, err := os.ReadFile(filepath.Join(outDir, "deps-graph.json"))
			if err != nil {
				depsErr = err
				return
			}
			depsErr = json.Unmarshal(raw, &depsGraph)
		})
		return depsGraph, depsErr
	}

	mux := http.NewServeMux()
	handleSearch := func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		scope := normalizeSearchScope(r.URL.Query().Get("scope"))
		format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))

		if q == "" && format != "txt" {
			http.ServeFile(w, r, filepath.Join(outDir, "search", "index.html"))
			return
		}

		idxEntries, err := loadIndex()
		if err != nil {
			http.Error(w, "search index unavailable", http.StatusInternalServerError)
			return
		}

		allMatches := FilterSearchEntries(idxEntries, q, scope, 0)
		total := len(allMatches)
		shown := allMatches
		if len(shown) > 150 {
			shown = shown[:150]
		}

		if format == "txt" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintf(w, "# KryX Linux search: q=%q scope=%s (%d match(es))\n", q, scope, total)
			for _, m := range shown {
				e := m.Entry
				fmt.Fprintf(w, "%-8s %-10s %-24s %-14s %s%s\n",
					e.Kind, defaultStr(e.Collection, "-"), e.Name, defaultStr(e.Version, "-"),
					strings.TrimRight(site.SiteURL, "/"), e.URL)
			}
			return
		}

		var body strings.Builder
		body.WriteString("<h1>Search KryX Linux</h1>\n")
		body.WriteString(renderSearchForm(q, scope, "Search packages, ports, docs, man pages..."))

		if total == 0 {
			fmt.Fprintf(&body, "<p>No results found for <code>%s</code> in scope <code>%s</code>.</p>\n",
				html.EscapeString(q), html.EscapeString(scope))
			body.WriteString("<p class=\"meta-note\">Try fewer keywords, switch scope to <em>All</em>, or browse <a href=\"/browse\">/browse</a>, <a href=\"/packages/\">/packages/</a>, and <a href=\"/ports/\">/ports/</a>.</p>\n")
		} else {
			if total > len(shown) {
				fmt.Fprintf(&body, "<p class=\"meta-note\">Showing top %d of %d matches for <code>%s</code> (scope: <code>%s</code>).</p>\n",
					len(shown), total, html.EscapeString(q), html.EscapeString(scope))
			} else {
				fmt.Fprintf(&body, "<p class=\"meta-note\">%d match(es) for <code>%s</code> (scope: <code>%s</code>).</p>\n",
					total, html.EscapeString(q), html.EscapeString(scope))
			}
			body.WriteString("<div class=\"table-wrap\">\n<table>\n<thead>\n<tr><th>Type</th><th>Name</th><th>Version</th><th>Collection</th><th>Description</th></tr>\n</thead>\n<tbody>\n")
			for _, m := range shown {
				e := m.Entry
				colCell := "—"
				if e.Collection != "" {
					colCell = fmt.Sprintf("<a href=\"/ports/%s/\">%s</a>", html.EscapeString(e.Collection), html.EscapeString(e.Collection))
				}
				verCell := "—"
				if e.Version != "" {
					verCell = "<code>" + html.EscapeString(e.Version) + "</code>"
				}
				fmt.Fprintf(&body, "<tr><td><span class=\"kind-tag\">%s</span></td><td><a href=\"%s\">%s</a></td><td>%s</td><td>%s</td><td>%s</td></tr>\n",
					html.EscapeString(e.Kind),
					html.EscapeString(e.URL),
					html.EscapeString(e.Name),
					verCell,
					colCell,
					html.EscapeString(e.Description),
				)
			}
			body.WriteString("</tbody>\n</table>\n</div>\n")
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(renderPageShell(site, PageMeta{
			Title:       fmt.Sprintf("Search: %s", q),
			Description: "KryX Linux search results.",
			Canonical:   "/search/",
		}, body.String())))
	}

	handleBrowse := func(w http.ResponseWriter, r *http.Request) {
		serveBrowseRequest(w, r, site, loadIndex)
	}
	handleDeps := func(w http.ResponseWriter, r *http.Request) {
		serveDepsRequest(w, r, outDir, site, loadDeps)
	}

	mux.HandleFunc("/search", handleSearch)
	mux.HandleFunc("/search/", handleSearch)
	mux.HandleFunc("/browse", handleBrowse)
	mux.HandleFunc("/browse/", handleBrowse)
	mux.HandleFunc("/deps", handleDeps)
	mux.HandleFunc("/deps/", handleDeps)
	mux.HandleFunc("/theme", serveThemeRequest)
	mux.HandleFunc("/theme/", serveThemeRequest)
	mux.Handle("/", fileServer)
	return withThemeMiddleware(mux)
}

type browseItem struct {
	Name        string
	Version     string
	Collection  string
	Description string
	Prebuilt    bool
	URL         string
	PortURL     string
}

func buildBrowseCatalog(entries []SearchEntry) []browseItem {
	byName := make(map[string]*browseItem, len(entries))
	for _, e := range entries {
		if e.Kind != "port" {
			continue
		}
		if _, exists := byName[e.Name]; exists {
			continue
		}
		byName[e.Name] = &browseItem{
			Name:        e.Name,
			Version:     e.Version,
			Collection:  e.Collection,
			Description: e.Description,
			Prebuilt:    false,
			URL:         e.URL,
			PortURL:     e.URL,
		}
	}
	for _, e := range entries {
		if e.Kind != "package" {
			continue
		}
		if cur, exists := byName[e.Name]; exists {
			cur.Prebuilt = true
			cur.URL = e.URL
			if e.Version != "" {
				cur.Version = e.Version
			}
			if e.Collection != "" && cur.Collection == "" {
				cur.Collection = e.Collection
			}
			if e.Description != "" && cur.Description == "" {
				cur.Description = e.Description
			}
		} else {
			byName[e.Name] = &browseItem{
				Name:        e.Name,
				Version:     e.Version,
				Collection:  e.Collection,
				Description: e.Description,
				Prebuilt:    true,
				URL:         e.URL,
			}
		}
	}
	items := make([]browseItem, 0, len(byName))
	for _, it := range byName {
		items = append(items, *it)
	}
	return items
}

func serveBrowseRequest(w http.ResponseWriter, r *http.Request, site SiteContext, loadIndex func() ([]SearchEntry, error)) {
	idxEntries, err := loadIndex()
	if err != nil {
		http.Error(w, "catalog unavailable", http.StatusInternalServerError)
		return
	}

	qp := r.URL.Query()
	col := strings.TrimSpace(qp.Get("col"))
	kind := strings.ToLower(strings.TrimSpace(qp.Get("kind")))
	if kind != "prebuilt" && kind != "source" {
		kind = "all"
	}
	sortBy := strings.ToLower(strings.TrimSpace(qp.Get("sort")))
	if sortBy != "col" && sortBy != "version" {
		sortBy = "name"
	}
	q := strings.TrimSpace(qp.Get("q"))
	format := strings.ToLower(strings.TrimSpace(qp.Get("format")))

	per := 50
	if rawPer, err := strconv.Atoi(strings.TrimSpace(qp.Get("per"))); err == nil && rawPer >= 10 && rawPer <= 200 {
		per = rawPer
	}
	page := 1
	if rawPage, err := strconv.Atoi(strings.TrimSpace(qp.Get("page"))); err == nil && rawPage > 0 {
		page = rawPage
	}

	tokens := strings.Fields(strings.ToLower(q))
	catalog := buildBrowseCatalog(idxEntries)
	filtered := make([]browseItem, 0, len(catalog))
	for _, it := range catalog {
		if col != "" && it.Collection != col {
			continue
		}
		if kind == "prebuilt" && !it.Prebuilt {
			continue
		}
		if kind == "source" && it.Prebuilt {
			continue
		}
		if len(tokens) > 0 {
			hay := strings.ToLower(it.Name + " " + it.Collection + " " + it.Description)
			ok := true
			for _, tok := range tokens {
				if !strings.Contains(hay, tok) {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
		}
		filtered = append(filtered, it)
	}

	sort.Slice(filtered, func(i, j int) bool {
		switch sortBy {
		case "col":
			if filtered[i].Collection != filtered[j].Collection {
				return filtered[i].Collection < filtered[j].Collection
			}
		case "version":
			if filtered[i].Version != filtered[j].Version {
				return filtered[i].Version > filtered[j].Version
			}
		}
		return filtered[i].Name < filtered[j].Name
	})

	if format == "txt" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "# KryX Linux browse: col=%q kind=%s sort=%s q=%q (%d item(s))\n", col, kind, sortBy, q, len(filtered))
		for _, it := range filtered {
			status := "source"
			if it.Prebuilt {
				status = "prebuilt"
			}
			fmt.Fprintf(w, "%-8s %-10s %-26s %-16s %s%s\n",
				status, defaultStr(it.Collection, "-"), it.Name, defaultStr(it.Version, "-"),
				strings.TrimRight(site.SiteURL, "/"), it.URL)
		}
		return
	}

	total := len(filtered)
	totalPages := (total + per - 1) / per
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * per
	end := start + per
	if end > total {
		end = total
	}
	slice := filtered[start:end]

	buildBrowseURL := func(p int, extraFormat string) string {
		v := url.Values{}
		if col != "" {
			v.Set("col", col)
		}
		if kind != "all" {
			v.Set("kind", kind)
		}
		if sortBy != "name" {
			v.Set("sort", sortBy)
		}
		if q != "" {
			v.Set("q", q)
		}
		if per != 50 {
			v.Set("per", strconv.Itoa(per))
		}
		if p > 1 {
			v.Set("page", strconv.Itoa(p))
		}
		if extraFormat != "" {
			v.Set("format", extraFormat)
		}
		if enc := v.Encode(); enc != "" {
			return "/browse?" + enc
		}
		return "/browse"
	}

	var body strings.Builder
	body.WriteString("<h1>Browse Packages &amp; Ports</h1>\n")
	body.WriteString("<p class=\"meta-note\">Zero-JavaScript server-side filter across all prebuilt binary packages and source ports. Works natively in <code>lynx</code>, <code>w3m</code>, <code>links</code>, and <code>curl</code>.</p>\n")

	body.WriteString("<form class=\"filter-bar\" action=\"/browse\" method=\"get\" role=\"search\">\n")
	body.WriteString("  <label class=\"sr-only\" for=\"b-q\">Filter query</label>\n")
	fmt.Fprintf(&body, "  <input id=\"b-q\" type=\"search\" name=\"q\" value=\"%s\" placeholder=\"Filter by name or description...\">\n", html.EscapeString(q))
	body.WriteString("  <label class=\"sr-only\" for=\"b-col\">Collection</label>\n")
	body.WriteString("  <select id=\"b-col\" name=\"col\">\n")
	body.WriteString("    <option value=\"\">All collections</option>\n")
	for _, c := range portCollectionsOrder {
		sel := ""
		if c == col {
			sel = " selected"
		}
		fmt.Fprintf(&body, "    <option value=\"%s\"%s>%s</option>\n", html.EscapeString(c), sel, html.EscapeString(c))
	}
	body.WriteString("  </select>\n")
	body.WriteString("  <label class=\"sr-only\" for=\"b-kind\">Availability</label>\n")
	body.WriteString("  <select id=\"b-kind\" name=\"kind\">\n")
	for _, opt := range [][2]string{{"all", "All (binary + source)"}, {"prebuilt", "Prebuilt binary only"}, {"source", "Source port only"}} {
		sel := ""
		if opt[0] == kind {
			sel = " selected"
		}
		fmt.Fprintf(&body, "    <option value=\"%s\"%s>%s</option>\n", opt[0], sel, opt[1])
	}
	body.WriteString("  </select>\n")
	body.WriteString("  <label class=\"sr-only\" for=\"b-sort\">Sort order</label>\n")
	body.WriteString("  <select id=\"b-sort\" name=\"sort\">\n")
	for _, opt := range [][2]string{{"name", "Sort: Name"}, {"col", "Sort: Collection"}, {"version", "Sort: Version"}} {
		sel := ""
		if opt[0] == sortBy {
			sel = " selected"
		}
		fmt.Fprintf(&body, "    <option value=\"%s\"%s>%s</option>\n", opt[0], sel, opt[1])
	}
	body.WriteString("  </select>\n")
	body.WriteString("  <button type=\"submit\">Filter</button>\n")
	body.WriteString("</form>\n")

	fmt.Fprintf(&body, "<p class=\"meta-note\">%d matching item(s) &middot; page %d of %d &middot; <a href=\"%s\">Plain text (curl)</a> &middot; <a href=\"/browse\">Reset</a></p>\n",
		total, page, totalPages, html.EscapeString(buildBrowseURL(1, "txt")))

	if total == 0 {
		body.WriteString("<p>No packages or ports matched the active filter.</p>\n")
	} else {
		body.WriteString("<div class=\"table-wrap\">\n<table>\n<thead>\n<tr><th>Name</th><th>Version</th><th>Collection</th><th>Status</th><th>Description</th><th>Graph</th></tr>\n</thead>\n<tbody>\n")
		for _, it := range slice {
			statusTag := `<span class="kind-tag">source</span>`
			if it.Prebuilt {
				statusTag = `<span class="badge-ok">prebuilt</span>`
			}
			colCell := "—"
			if it.Collection != "" {
				colCell = fmt.Sprintf("<a href=\"/browse?col=%s\">%s</a>", html.EscapeString(it.Collection), html.EscapeString(it.Collection))
			}
			fmt.Fprintf(&body, "<tr><td><a href=\"%s\">%s</a></td><td><code>%s</code></td><td>%s</td><td>%s</td><td>%s</td><td><a href=\"/deps?pkg=%s\">deps</a></td></tr>\n",
				html.EscapeString(it.URL),
				html.EscapeString(it.Name),
				html.EscapeString(defaultStr(it.Version, "—")),
				colCell,
				statusTag,
				html.EscapeString(it.Description),
				html.EscapeString(it.Name),
			)
		}
		body.WriteString("</tbody>\n</table>\n</div>\n")

		if totalPages > 1 {
			body.WriteString("<nav class=\"pagination\" aria-label=\"Pagination\">\n")
			if page > 1 {
				fmt.Fprintf(&body, "  <a class=\"btn\" href=\"%s\">&larr; Previous</a>\n", html.EscapeString(buildBrowseURL(page-1, "")))
			} else {
				body.WriteString("  <span class=\"page-disabled\">&larr; Previous</span>\n")
			}
			fmt.Fprintf(&body, "  <span class=\"page-info\">Page %d of %d</span>\n", page, totalPages)
			if page < totalPages {
				fmt.Fprintf(&body, "  <a class=\"btn\" href=\"%s\">Next &rarr;</a>\n", html.EscapeString(buildBrowseURL(page+1, "")))
			} else {
				body.WriteString("  <span class=\"page-disabled\">Next &rarr;</span>\n")
			}
			body.WriteString("</nav>\n")
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderPageShell(site, PageMeta{
		Title:       "Browse Packages & Ports",
		Description: "Filter and paginate KryX Linux binary packages and source ports without JavaScript.",
		ActiveNav:   "packages",
		Canonical:   "/browse",
	}, body.String())))
}

func safeNextPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "/"
	}
	if strings.ContainsAny(raw, "\r\n\\") {
		return "/"
	}
	return raw
}

func serveThemeRequest(w http.ResponseWriter, r *http.Request) {
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	next := safeNextPath(r.URL.Query().Get("next"))
	switch mode {
	case "light", "dark":
		http.SetCookie(w, &http.Cookie{
			Name:     "kryx_theme",
			Value:    mode,
			Path:     "/",
			MaxAge:   31536000,
			SameSite: http.SameSiteLaxMode,
		})
	default:
		http.SetCookie(w, &http.Cookie{
			Name:     "kryx_theme",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			SameSite: http.SameSiteLaxMode,
		})
	}
	http.Redirect(w, r, next, http.StatusFound)
}

type themeCaptureWriter struct {
	header http.Header
	code   int
	buf    bytes.Buffer
}

func (tw *themeCaptureWriter) Header() http.Header         { return tw.header }
func (tw *themeCaptureWriter) WriteHeader(statusCode int)  { tw.code = statusCode }
func (tw *themeCaptureWriter) Write(p []byte) (int, error) { return tw.buf.Write(p) }

func withThemeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("kryx_theme")
		if err != nil || (cookie.Value != "light" && cookie.Value != "dark") {
			next.ServeHTTP(w, r)
			return
		}
		theme := cookie.Value
		cw := &themeCaptureWriter{header: make(http.Header), code: http.StatusOK}
		next.ServeHTTP(cw, r)

		for k, vals := range cw.header {
			if strings.EqualFold(k, "Content-Length") {
				continue
			}
			for _, v := range vals {
				w.Header().Add(k, v)
			}
		}
		w.Header().Add("Vary", "Cookie")

		payload := cw.buf.Bytes()
		ct := cw.header.Get("Content-Type")
		if strings.Contains(strings.ToLower(ct), "text/html") {
			payload = bytes.Replace(payload, []byte(`<html lang="en">`), []byte(`<html lang="en" data-theme="`+theme+`">`), 1)
		}
		w.WriteHeader(cw.code)
		_, _ = w.Write(payload)
	})
}
