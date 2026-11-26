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
)

// DepNode is a compact dependency graph entry emitted into public/deps-graph.json
// and consumed by both functions/deps.js (Cloudflare Pages) and sitegen -serve.
type DepNode struct {
	Collection  string   `json:"c,omitempty"`
	Version     string   `json:"v,omitempty"`
	Prebuilt    bool     `json:"b,omitempty"`
	URL         string   `json:"u"`
	Depends     []string `json:"d,omitempty"`
	MakeDepends []string `json:"m,omitempty"`
}

// BuildDepsGraph builds the unified package/port dependency graph across all collections.
func BuildDepsGraph(idx RepoIndex, portsByCol map[string][]PortRecipe) map[string]DepNode {
	graph := make(map[string]DepNode, len(idx.Packages)+256)

	// 1. Seed from all discovered ports first.
	for _, col := range portCollectionsOrder {
		for _, p := range portsByCol[col] {
			if _, exists := graph[p.Name]; exists {
				continue
			}
			_, hasBin := idx.Packages[p.Name]
			u := fmt.Sprintf("/ports/%s/%s.html", col, p.DirName)
			if hasBin {
				u = "/packages/" + p.Name + ".html"
			}
			graph[p.Name] = DepNode{
				Collection:  col,
				Version:     p.FullVersion(),
				Prebuilt:    hasBin,
				URL:         u,
				Depends:     uniqueStrings(p.Depends),
				MakeDepends: uniqueStrings(p.MakeDepends),
			}
		}
	}

	// 2. Merge/add binary index entries (ensuring prebuilt packages always exist in graph).
	for name, pkg := range idx.Packages {
		e := latestEntry(pkg)
		cur, exists := graph[name]
		col := e.Collection
		if (col == "" || col == "rolling" || col == "pkg-tools") && exists && cur.Collection != "" {
			col = cur.Collection
		}
		col = normalizeCollection(col, name)
		deps := uniqueStrings(e.Depends)
		if len(deps) == 0 && exists {
			deps = cur.Depends
		}
		mdeps := uniqueStrings(e.MakeDepends)
		if len(mdeps) == 0 && exists {
			mdeps = cur.MakeDepends
		}
		graph[name] = DepNode{
			Collection:  col,
			Version:     defaultStr(e.FullVersion(), cur.Version),
			Prebuilt:    true,
			URL:         "/packages/" + name + ".html",
			Depends:     deps,
			MakeDepends: mdeps,
		}
	}

	return graph
}

func uniqueStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// GenerateDepsAssets writes public/deps-graph.json and the static /deps/index.html form page.
func GenerateDepsAssets(outDir string, site SiteContext, graph map[string]DepNode) error {
	raw, err := json.Marshal(graph)
	if err != nil {
		return fmt.Errorf("marshal deps-graph.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "deps-graph.json"), raw, 0o644); err != nil {
		return err
	}

	var body strings.Builder
	body.WriteString("<h1>Dependency Explorer</h1>\n")
	fmt.Fprintf(&body, "<p class=\"meta-note\">Zero-JavaScript transitive dependency tree (<code>kpm deps</code>) and reverse-dependency explorer (<code>kpm why</code>) across %d indexed packages &amp; ports.</p>\n", len(graph))
	body.WriteString(renderDepsForm("", "tree", false))

	body.WriteString("<h2>Example Graphs</h2>\n<ul>\n")
	body.WriteString("<li><a href=\"/deps?pkg=wlroots\">wlroots (dependency tree)</a> &middot; <a href=\"/deps?pkg=wlroots&amp;include_make=1\">with make deps</a></li>\n")
	body.WriteString("<li><a href=\"/deps?pkg=grub2\">grub2 (dependency tree)</a> &middot; <a href=\"/deps?pkg=grub2&amp;format=dot\">Graphviz DOT</a></li>\n")
	body.WriteString("<li><a href=\"/deps?pkg=dinit-kryx\">dinit-kryx (init DAG deps)</a></li>\n")
	body.WriteString("<li><a href=\"/deps?pkg=dbus&amp;mode=why\">dbus (Required By / reverse deps)</a></li>\n")
	body.WriteString("</ul>\n")

	body.WriteString("<h2>Terminal &amp; Graphviz Export</h2>\n")
	body.WriteString("<pre><code># Plain-text ASCII dependency tree\ncurl -s \"https://kryx.anrix.org/deps?pkg=wlroots&amp;format=txt\"\n\n# Reverse dependencies (why is a package needed?)\ncurl -s \"https://kryx.anrix.org/deps?pkg=dbus&amp;mode=why&amp;format=txt\"\n\n# Export Graphviz DOT and render SVG locally\ncurl -s \"https://kryx.anrix.org/deps?pkg=grub2&amp;format=dot\" | dot -Tsvg &gt; grub2.svg</code></pre>\n")

	return writeHTMLPage(filepath.Join(outDir, "deps", "index.html"), site, PageMeta{
		Title:       "Dependency Explorer",
		Description: "Zero-JavaScript transitive dependency tree and reverse-dependency explorer for KryX Linux packages and ports.",
		ActiveNav:   "ports",
		Canonical:   "/deps/",
	}, body.String())
}

func renderDepsForm(pkg, mode string, includeMake bool) string {
	if mode != "why" {
		mode = "tree"
	}
	var sb strings.Builder
	sb.WriteString("<form class=\"search-form\" action=\"/deps\" method=\"get\" role=\"search\">\n")
	sb.WriteString("  <label class=\"sr-only\" for=\"pkg\">Package or port name</label>\n")
	fmt.Fprintf(&sb, "  <input id=\"pkg\" type=\"search\" name=\"pkg\" value=\"%s\" placeholder=\"Enter package or port name (e.g. wlroots, grub2, dbus)...\">\n",
		html.EscapeString(pkg))
	sb.WriteString("  <label class=\"sr-only\" for=\"mode\">Mode</label>\n")
	sb.WriteString("  <select id=\"mode\" name=\"mode\">\n")
	if mode == "tree" {
		sb.WriteString("    <option value=\"tree\" selected>Dependencies (tree)</option>\n")
		sb.WriteString("    <option value=\"why\">Required By (why)</option>\n")
	} else {
		sb.WriteString("    <option value=\"tree\">Dependencies (tree)</option>\n")
		sb.WriteString("    <option value=\"why\" selected>Required By (why)</option>\n")
	}
	sb.WriteString("  </select>\n")
	checked := ""
	if includeMake {
		checked = " checked"
	}
	fmt.Fprintf(&sb, "  <label class=\"check-inline\"><input type=\"checkbox\" name=\"include_make\" value=\"1\"%s> include make deps</label>\n", checked)
	sb.WriteString("  <button type=\"submit\">Inspect</button>\n")
	sb.WriteString("</form>\n")
	return sb.String()
}

type depTreeLine struct {
	Prefix  string
	Name    string
	Node    DepNode
	Found   bool
	Cycle   bool
	Repeat  bool
	IsMake  bool
}

func buildDepTreeLines(graph map[string]DepNode, root string, includeMake bool, maxDepth, maxNodes int) ([]depTreeLine, int, bool) {
	var lines []depTreeLine
	visited := make(map[string]bool)
	truncated := false

	var walk func(name, prefix string, isLast, isRoot, isMake bool, depth int, stack map[string]bool)
	walk = func(name, prefix string, isLast, isRoot, isMake bool, depth int, stack map[string]bool) {
		if len(lines) >= maxNodes {
			truncated = true
			return
		}
		node, ok := graph[name]
		branch := ""
		childPrefix := prefix
		if !isRoot {
			if isLast {
				branch = prefix + "`-- "
				childPrefix = prefix + "    "
			} else {
				branch = prefix + "|-- "
				childPrefix = prefix + "|   "
			}
		}

		if stack[name] {
			lines = append(lines, depTreeLine{Prefix: branch, Name: name, Node: node, Found: ok, Cycle: true, IsMake: isMake})
			return
		}
		if visited[name] && !isRoot {
			lines = append(lines, depTreeLine{Prefix: branch, Name: name, Node: node, Found: ok, Repeat: true, IsMake: isMake})
			return
		}
		visited[name] = true
		lines = append(lines, depTreeLine{Prefix: branch, Name: name, Node: node, Found: ok, IsMake: isMake})

		if !ok || depth >= maxDepth {
			return
		}

		type childSpec struct {
			name   string
			isMake bool
		}
		var children []childSpec
		for _, d := range node.Depends {
			children = append(children, childSpec{name: d, isMake: false})
		}
		if includeMake {
			for _, m := range node.MakeDepends {
				children = append(children, childSpec{name: m, isMake: true})
			}
		}
		if len(children) == 0 {
			return
		}

		stack[name] = true
		for i, ch := range children {
			walk(ch.name, childPrefix, i == len(children)-1, false, ch.isMake, depth+1, stack)
		}
		delete(stack, name)
	}

	walk(root, "", true, true, false, 0, make(map[string]bool))
	return lines, len(visited), truncated
}

type reverseDepEntry struct {
	Name   string
	Node   DepNode
	IsMake bool
}

func findDirectReverseDeps(graph map[string]DepNode, target string, includeMake bool) []reverseDepEntry {
	var out []reverseDepEntry
	for name, node := range graph {
		if name == target {
			continue
		}
		matched := false
		for _, d := range node.Depends {
			if d == target {
				out = append(out, reverseDepEntry{Name: name, Node: node, IsMake: false})
				matched = true
				break
			}
		}
		if !matched && includeMake {
			for _, m := range node.MakeDepends {
				if m == target {
					out = append(out, reverseDepEntry{Name: name, Node: node, IsMake: true})
					break
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func renderDepDOT(graph map[string]DepNode, root string, includeMake bool, maxNodes int) string {
	var sb strings.Builder
	sb.WriteString("digraph kryx_deps {\n")
	sb.WriteString("  rankdir=LR;\n")
	sb.WriteString("  node [shape=box, fontname=\"monospace\", fontsize=10];\n")

	seen := make(map[string]bool)
	queue := []string{root}
	seen[root] = true

	for len(queue) > 0 && len(seen) <= maxNodes {
		cur := queue[0]
		queue = queue[1:]
		node, ok := graph[cur]
		if !ok {
			continue
		}
		for _, d := range node.Depends {
			fmt.Fprintf(&sb, "  %q -> %q;\n", cur, d)
			if !seen[d] && len(seen) < maxNodes {
				seen[d] = true
				queue = append(queue, d)
			}
		}
		if includeMake {
			for _, m := range node.MakeDepends {
				fmt.Fprintf(&sb, "  %q -> %q [style=dashed, label=\"make\"];\n", cur, m)
				if !seen[m] && len(seen) < maxNodes {
					seen[m] = true
					queue = append(queue, m)
				}
			}
		}
	}
	sb.WriteString("}\n")
	return sb.String()
}

func serveDepsRequest(w http.ResponseWriter, r *http.Request, outDir string, site SiteContext, loadGraph func() (map[string]DepNode, error)) {
	pkg := strings.TrimSpace(r.URL.Query().Get("pkg"))
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	if mode != "why" {
		mode = "tree"
	}
	includeMake := r.URL.Query().Get("include_make") == "1" || strings.EqualFold(r.URL.Query().Get("include_make"), "true")
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))

	if pkg == "" && format != "txt" && format != "dot" {
		http.ServeFile(w, r, filepath.Join(outDir, "deps", "index.html"))
		return
	}

	graph, err := loadGraph()
	if err != nil {
		http.Error(w, "dependency graph unavailable", http.StatusInternalServerError)
		return
	}

	node, exists := graph[pkg]

	if format == "dot" {
		w.Header().Set("Content-Type", "text/vnd.graphviz; charset=utf-8")
		if !exists {
			fmt.Fprintf(w, "// package %q not found\n", pkg)
			return
		}
		_, _ = w.Write([]byte(renderDepDOT(graph, pkg, includeMake, 120)))
		return
	}

	if format == "txt" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if mode == "why" {
			revs := findDirectReverseDeps(graph, pkg, includeMake)
			if !exists && len(revs) == 0 {
				fmt.Fprintf(w, "# package %q not found in KryX dependency graph\n", pkg)
				return
			}
			fmt.Fprintf(w, "# Required By %s (%d direct dependent(s))\n", pkg, len(revs))
			for _, rv := range revs {
				tag := "runtime"
				if rv.IsMake {
					tag = "make"
				}
				fmt.Fprintf(w, "%-24s %-10s %-8s %s%s\n", rv.Name, defaultStr(rv.Node.Collection, "-"), tag, strings.TrimRight(site.SiteURL, "/"), rv.Node.URL)
			}
			return
		}
		if !exists {
			fmt.Fprintf(w, "# package %q not found in KryX dependency graph\n", pkg)
			return
		}
		lines, total, trunc := buildDepTreeLines(graph, pkg, includeMake, 8, 200)
		fmt.Fprintf(w, "# Dependency tree for %s (%d unique node(s))\n", pkg, total)
		for _, ln := range lines {
			extra := ""
			if ln.IsMake {
				extra += " [make]"
			}
			if ln.Node.Prebuilt {
				extra += " [prebuilt]"
			}
			if ln.Cycle {
				extra += " (cycle)"
			} else if ln.Repeat {
				extra += " (*)"
			}
			fmt.Fprintf(w, "%s%s%s\n", ln.Prefix, ln.Name, extra)
		}
		if trunc {
			fmt.Fprintln(w, "# ... truncated at 200 lines")
		}
		return
	}

	var body strings.Builder
	body.WriteString("<h1>Dependency Explorer</h1>\n")
	body.WriteString(renderDepsForm(pkg, mode, includeMake))

	if mode == "why" {
		revs := findDirectReverseDeps(graph, pkg, includeMake)
		if !exists && len(revs) == 0 {
			fmt.Fprintf(&body, "<p>Package or port <code>%s</code> was not found in the dependency graph.</p>\n", html.EscapeString(pkg))
			fmt.Fprintf(&body, "<p class=\"meta-note\">Try <a href=\"/search?q=%s\">searching for %s</a> instead.</p>\n",
				html.EscapeString(pkg), html.EscapeString(pkg))
		} else {
			if exists && node.URL != "" {
				fmt.Fprintf(&body, "<h2>Required By: <a href=\"%s\">%s</a> <code>%s</code></h2>\n",
					html.EscapeString(node.URL), html.EscapeString(pkg), html.EscapeString(node.Version))
			} else {
				fmt.Fprintf(&body, "<h2>Required By: <code>%s</code></h2>\n", html.EscapeString(pkg))
			}
			fmt.Fprintf(&body, "<p class=\"meta-note\">%d direct dependent(s) &middot; <a href=\"/deps?pkg=%s\">View dependency tree</a> &middot; <a href=\"/deps?pkg=%s&amp;mode=why&amp;format=txt\">Plain text</a></p>\n",
				len(revs), html.EscapeString(pkg), html.EscapeString(pkg))
			if len(revs) == 0 {
				body.WriteString("<p>No other indexed packages or ports declare a direct dependency on this package.</p>\n")
			} else {
				body.WriteString("<div class=\"table-wrap\">\n<table>\n<thead>\n<tr><th>Package</th><th>Version</th><th>Collection</th><th>Dependency</th><th>Explore</th></tr>\n</thead>\n<tbody>\n")
				for _, rv := range revs {
					kind := "runtime"
					if rv.IsMake {
						kind = "make"
					}
					fmt.Fprintf(&body, "<tr><td><a href=\"%s\">%s</a></td><td><code>%s</code></td><td><a href=\"/ports/%s/\">%s</a></td><td><span class=\"kind-tag\">%s</span></td><td><a href=\"/deps?pkg=%s\">tree</a></td></tr>\n",
						html.EscapeString(rv.Node.URL),
						html.EscapeString(rv.Name),
						html.EscapeString(defaultStr(rv.Node.Version, "—")),
						html.EscapeString(rv.Node.Collection),
						html.EscapeString(rv.Node.Collection),
						kind,
						html.EscapeString(rv.Name),
					)
				}
				body.WriteString("</tbody>\n</table>\n</div>\n")
			}
		}
	} else if !exists {
		fmt.Fprintf(&body, "<p>Package or port <code>%s</code> was not found in the dependency graph.</p>\n", html.EscapeString(pkg))
		fmt.Fprintf(&body, "<p class=\"meta-note\">Try <a href=\"/search?q=%s\">searching for %s</a> instead.</p>\n",
			html.EscapeString(pkg), html.EscapeString(pkg))
	} else {
		lines, total, trunc := buildDepTreeLines(graph, pkg, includeMake, 8, 200)
		fmt.Fprintf(&body, "<h2>Dependency Tree: <a href=\"%s\">%s</a> <code>%s</code></h2>\n",
			html.EscapeString(node.URL), html.EscapeString(pkg), html.EscapeString(node.Version))
		makeParam := ""
		if includeMake {
			makeParam = "&amp;include_make=1"
		}
		fmt.Fprintf(&body, "<p class=\"meta-note\">%d unique node(s) &middot; <a href=\"/deps?pkg=%s&amp;mode=why\">Required By (why)</a> &middot; <a href=\"/deps?pkg=%s%s&amp;format=txt\">Plain text</a> &middot; <a href=\"/deps?pkg=%s%s&amp;format=dot\">Graphviz DOT</a></p>\n",
			total, html.EscapeString(pkg), html.EscapeString(pkg), makeParam, html.EscapeString(pkg), makeParam)
		body.WriteString("<pre class=\"dep-tree\"><code>")
		for _, ln := range lines {
			body.WriteString(html.EscapeString(ln.Prefix))
			if ln.Found && ln.Node.URL != "" {
				fmt.Fprintf(&body, "<a href=\"%s\">%s</a>", html.EscapeString(ln.Node.URL), html.EscapeString(ln.Name))
			} else {
				body.WriteString(html.EscapeString(ln.Name))
			}
			if ln.Node.Version != "" {
				fmt.Fprintf(&body, " (%s)", html.EscapeString(ln.Node.Version))
			}
			if ln.IsMake {
				body.WriteString(" [make]")
			}
			if ln.Node.Prebuilt {
				body.WriteString(" [prebuilt]")
			}
			if ln.Cycle {
				body.WriteString(" (cycle)")
			} else if ln.Repeat {
				body.WriteString(" (*)")
			}
			body.WriteByte('\n')
		}
		if trunc {
			body.WriteString("... truncated at 200 lines\n")
		}
		body.WriteString("</code></pre>\n")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderPageShell(site, PageMeta{
		Title:       fmt.Sprintf("Dependencies: %s", pkg),
		Description: fmt.Sprintf("Dependency graph for %s in KryX Linux.", pkg),
		ActiveNav:   "ports",
		Canonical:   "/deps/",
	}, body.String())))
}
