package main

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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

	// 2. Ports recipes (every collection)
	for _, col := range portCollectionsOrder {
		for _, p := range portsByCol[col] {
			_, hasBin := idx.Packages[p.Name]
			isFirstParty := col == "kpm-tools" || col == "kernel" || col == "system"
			u := fmt.Sprintf("/ports/%s/#letter-%s", col, alphaBucket(p.Name))
			if hasBin {
				u = "/packages/" + p.Name + ".html"
			} else if isFirstParty && p.RawPkgfile != "" {
				u = fmt.Sprintf("/ports/%s/%s.html", col, p.DirName)
			}
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
// server-side zero-JS search on /search (matching functions/search.js on Cloudflare Pages).
func NewSiteHandler(outDir string, site SiteContext) http.Handler {
	fileServer := http.FileServer(http.Dir(outDir))
	var (
		loadOnce sync.Once
		entries  []SearchEntry
		loadErr  error
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
			body.WriteString("<p class=\"meta-note\">Try fewer keywords, switch scope to <em>All</em>, or browse <a href=\"/packages/\">/packages/</a> and <a href=\"/ports/\">/ports/</a>.</p>\n")
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

	mux.HandleFunc("/search", handleSearch)
	mux.HandleFunc("/search/", handleSearch)
	mux.Handle("/", fileServer)
	return mux
}
