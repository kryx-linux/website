// Command sitegen builds the ultra-fast, zero-JavaScript KryX Linux static website
// (https://kryx.anrix.org) using only the Go standard library.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const defaultIndexURL = "https://github.com/kryx-linux/ports/releases/download/rolling-x86_64/index.json"

// BuildOptions controls a site generation run.
type BuildOptions struct {
	OutDir     string
	SiteURL    string
	ContentDir string
	StaticDir  string
	DataDir    string
	IndexURL   string
	IndexFile  string
	PortsDir   string
	ManDirs    []string
	Offline    bool
	BuildTime  time.Time
}

func main() {
	outDir := flag.String("out", "public", "output directory for generated static website")
	siteURL := flag.String("site-url", "https://kryx.anrix.org", "canonical public base URL")
	contentDir := flag.String("content", "content", "Markdown content directory")
	staticDir := flag.String("static", "static", "static assets directory (CSS, fonts, SVGs)")
	dataDir := flag.String("data", "data", "data directory (release.json, index.fixture.json, man/)")
	indexURL := flag.String("index-url", defaultIndexURL, "live index.json URL for /packages/")
	indexFile := flag.String("index-file", "", "explicit local index.json path (overrides -index-url)")
	portsDir := flag.String("ports-dir", "", "local CRUX/KryX ports tree directory (auto-detects ../ports if present)")
	manDirsCSV := flag.String("man-dirs", "", "comma-separated directories containing roff man pages")
	offline := flag.Bool("offline", false, "skip live HTTP fetch of index.json and use local fixture")
	serveAddr := flag.String("serve", "", "optional HTTP listen address to serve -out (e.g. :8080)")
	flag.Parse()

	if strings.TrimSpace(*serveAddr) != "" {
		fmt.Printf("Serving %s on http://%s\n", *outDir, strings.TrimPrefix(*serveAddr, ":"))
		if err := http.ListenAndServe(*serveAddr, http.FileServer(http.Dir(*outDir))); err != nil {
			fmt.Fprintf(os.Stderr, "sitegen: serve failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	resolvedPorts := strings.TrimSpace(*portsDir)
	if resolvedPorts == "" {
		if fi, err := os.Stat("../ports"); err == nil && fi.IsDir() {
			resolvedPorts = "../ports"
		}
	}

	var manDirs []string
	if strings.TrimSpace(*manDirsCSV) != "" {
		manDirs = strings.Split(*manDirsCSV, ",")
	} else {
		manDirs = []string{"../kpm/man", filepath.Join(*dataDir, "man")}
	}

	opts := BuildOptions{
		OutDir:     *outDir,
		SiteURL:    *siteURL,
		ContentDir: *contentDir,
		StaticDir:  *staticDir,
		DataDir:    *dataDir,
		IndexURL:   *indexURL,
		IndexFile:  *indexFile,
		PortsDir:   resolvedPorts,
		ManDirs:    manDirs,
		Offline:    *offline,
		BuildTime:  time.Now().UTC(),
	}

	if err := BuildSite(opts); err != nil {
		fmt.Fprintf(os.Stderr, "sitegen: %v\n", err)
		os.Exit(1)
	}
}

// BuildSite renders the entire KryX Linux static website into opts.OutDir.
func BuildSite(opts BuildOptions) error {
	if opts.BuildTime.IsZero() {
		opts.BuildTime = time.Now().UTC()
	}
	site := SiteContext{
		SiteURL:   strings.TrimRight(opts.SiteURL, "/"),
		BuildTime: opts.BuildTime,
	}

	// Clean and create output directory.
	if err := os.RemoveAll(opts.OutDir); err != nil {
		return fmt.Errorf("clean %s: %w", opts.OutDir, err)
	}
	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", opts.OutDir, err)
	}

	// 1. Copy static assets (style.css, kryx.svg, kryx-release.pub, fonts/*.woff2).
	if err := copyDir(opts.StaticDir, opts.OutDir); err != nil {
		return fmt.Errorf("copy static assets: %w", err)
	}

	// 2. Load release metadata and binary repository index.json.
	rel, err := loadReleaseInfo(filepath.Join(opts.DataDir, "release.json"))
	if err != nil {
		return err
	}
	idx, idxSource, err := LoadIndex(opts.IndexFile, opts.IndexURL, filepath.Join(opts.DataDir, "index.fixture.json"), opts.Offline)
	if err != nil {
		return err
	}

	// 3. Discover ports tree (local, GitHub tarballs when online, or fallback from index.json).
	portsByCol, portsByName := DiscoverPorts(opts.PortsDir, idx, opts.Offline)

	// 4. Render Home (/).
	if err := renderMarkdownPage(filepath.Join(opts.ContentDir, "index.md"), filepath.Join(opts.OutDir, "index.html"), site, "home", "/", false); err != nil {
		return err
	}

	// 5. Render Download (/download/) with generated artifacts table + Markdown guide.
	if err := renderDownloadPage(filepath.Join(opts.ContentDir, "download.md"), filepath.Join(opts.OutDir, "download", "index.html"), site, rel); err != nil {
		return err
	}

	// 6. Render Documentation / Handbook (/docs/).
	if err := buildDocsSection(filepath.Join(opts.ContentDir, "docs"), filepath.Join(opts.OutDir, "docs"), site); err != nil {
		return err
	}

	// 7. Render Web Man Pages (/man/).
	manPages, err := DiscoverManPages(opts.ManDirs)
	if err != nil {
		return err
	}
	if err := buildManSection(filepath.Join(opts.OutDir, "man"), site, manPages); err != nil {
		return err
	}

	// 8. Render Packages (/packages/ + /packages.txt) and Ports (/ports/).
	if err := GeneratePackagesPages(opts.OutDir, site, idx, portsByName); err != nil {
		return err
	}
	if err := GeneratePortsPages(opts.OutDir, site, portsByCol, idx); err != nil {
		return err
	}

	// 9. Render News (/news/) and RSS 2.0 Feed (/feed.xml).
	newsPosts, err := buildNewsSection(filepath.Join(opts.ContentDir, "news"), filepath.Join(opts.OutDir, "news"), site)
	if err != nil {
		return err
	}
	if err := GenerateRSSFeed(opts.OutDir, site.SiteURL, newsPosts, opts.BuildTime); err != nil {
		return err
	}

	// 10. Render Status (/status/).
	statusHTML := renderStatusPage(site, idx, idxSource, portsByCol, rel, len(manPages))
	if err := writeHTMLPage(filepath.Join(opts.OutDir, "status", "index.html"), site, PageMeta{
		Title:       "Status",
		Description: "KryX Linux repository health, package counts, signing key fingerprint, and CI build status.",
		ActiveNav:   "status",
		Canonical:   "/status/",
	}, statusHTML); err != nil {
		return err
	}

	// 11. Emit terminal-friendly plain-text endpoints (/install.txt, /SHA256SUMS.txt).
	if err := generatePlainTextEndpoints(opts.OutDir, site, rel); err != nil {
		return err
	}

	fmt.Printf("Built KryX Linux static site -> %s (%d packages, %d man pages, %d news posts)\n",
		opts.OutDir, len(idx.Packages), len(manPages), len(newsPosts))
	return nil
}

func loadReleaseInfo(path string) (ReleaseInfo, error) {
	var rel ReleaseInfo
	raw, err := os.ReadFile(path)
	if err != nil {
		return rel, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &rel); err != nil {
		return rel, fmt.Errorf("parse %s: %w", path, err)
	}
	return rel, nil
}

func renderMarkdownPage(srcMD, dstHTML string, site SiteContext, activeNav, canonical string, autoTOC bool) error {
	raw, err := os.ReadFile(srcMD)
	if err != nil {
		return fmt.Errorf("read %s: %w", srcMD, err)
	}
	doc := ParseMarkdown(string(raw), autoTOC)
	return writeHTMLPage(dstHTML, site, PageMeta{
		Title:       doc.Meta.Title,
		Description: doc.Meta.Description,
		ActiveNav:   activeNav,
		Canonical:   canonical,
	}, doc.HTML)
}

func renderDownloadPage(srcMD, dstHTML string, site SiteContext, rel ReleaseInfo) error {
	raw, err := os.ReadFile(srcMD)
	if err != nil {
		return fmt.Errorf("read %s: %w", srcMD, err)
	}
	doc := ParseMarkdown(string(raw), false)
	tableHTML := renderDownloadArtifactsTable(rel)
	// Inject the artifact table right after the intro paragraph/callout before "## Verify".
	combined := doc.HTML
	if idx := strings.Index(combined, "<h2"); idx >= 0 {
		combined = combined[:idx] + tableHTML + combined[idx:]
	} else {
		combined = combined + tableHTML
	}
	return writeHTMLPage(dstHTML, site, PageMeta{
		Title:       defaultStr(doc.Meta.Title, "Download"),
		Description: doc.Meta.Description,
		ActiveNav:   "download",
		Canonical:   "/download/",
	}, combined)
}

func buildDocsSection(srcDir, outDir string, site SiteContext) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("read docs dir %s: %w", srcDir, err)
	}

	type docItem struct {
		slug  string
		title string
		order int
	}
	var guides []docItem
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if rerr != nil {
			return rerr
		}
		doc := ParseMarkdown(string(raw), true)
		slug := strings.TrimSuffix(e.Name(), ".md")
		if slug != "index" {
			guides = append(guides, docItem{slug: slug, title: doc.Meta.Title, order: doc.Meta.Order})
		}
	}
	sort.Slice(guides, func(i, j int) bool {
		if guides[i].order != guides[j].order {
			return guides[i].order < guides[j].order
		}
		return guides[i].slug < guides[j].slug
	})

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if rerr != nil {
			return rerr
		}
		slug := strings.TrimSuffix(e.Name(), ".md")
		doc := ParseMarkdown(string(raw), slug != "index")

		body := doc.HTML
		canonical := "/docs/" + slug + ".html"
		dst := filepath.Join(outDir, slug+".html")
		if slug == "index" {
			canonical = "/docs/"
			dst = filepath.Join(outDir, "index.html")
		} else {
			body = "<p class=\"meta-note\"><a href=\"/docs/\">&larr; Handbook Index</a></p>\n" + body
		}

		if err := writeHTMLPage(dst, site, PageMeta{
			Title:       doc.Meta.Title,
			Description: doc.Meta.Description,
			ActiveNav:   "docs",
			Canonical:   canonical,
		}, body); err != nil {
			return err
		}
	}
	return nil
}

func buildManSection(outDir string, site SiteContext, pages []ManPage) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	var idxBody strings.Builder
	idxBody.WriteString("<p class=\"meta-note\"><a href=\"/docs/\">&larr; Handbook</a></p>\n")
	idxBody.WriteString("<h1>Web Manual Pages</h1>\n")
	idxBody.WriteString("<p>Generated directly from roff manual sources (<code>man(7)</code>) into semantic HTML.</p>\n")
	idxBody.WriteString("<div class=\"table-wrap\">\n<table>\n<thead>\n<tr><th>Manual</th><th>Section</th><th>Summary</th></tr>\n</thead>\n<tbody>\n")
	for _, mp := range pages {
		fmt.Fprintf(&idxBody, "<tr><td><a href=\"/man/%s.html\"><strong>%s</strong></a></td><td>%s</td><td>%s</td></tr>\n",
			html.EscapeString(mp.Filename),
			html.EscapeString(mp.Title),
			html.EscapeString(mp.Section),
			html.EscapeString(mp.Summary),
		)
		pageBody := "<p class=\"meta-note\"><a href=\"/man/\">&larr; All Man Pages</a></p>\n" + mp.HTML
		if err := writeHTMLPage(filepath.Join(outDir, mp.Filename+".html"), site, PageMeta{
			Title:       mp.Title + " — Manual",
			Description: defaultStr(mp.Summary, "Manual page for "+mp.Title),
			ActiveNav:   "docs",
			Canonical:   "/man/" + mp.Filename + ".html",
		}, pageBody); err != nil {
			return err
		}
	}
	idxBody.WriteString("</tbody>\n</table>\n</div>\n")

	return writeHTMLPage(filepath.Join(outDir, "index.html"), site, PageMeta{
		Title:       "Manual Pages",
		Description: "Web manual pages for kpm(8), kpm-index(8), kryx-src(8), and ports(8).",
		ActiveNav:   "docs",
		Canonical:   "/man/",
	}, idxBody.String())
}

func buildNewsSection(srcDir, outDir string, site SiteContext) ([]NewsPost, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, fmt.Errorf("read news dir %s: %w", srcDir, err)
	}
	var posts []NewsPost
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if rerr != nil {
			return nil, rerr
		}
		doc := ParseMarkdown(string(raw), false)
		slug := strings.TrimSuffix(e.Name(), ".md")
		parsedDate, _ := time.Parse("2006-01-02", doc.Meta.Date)
		posts = append(posts, NewsPost{
			Slug:        slug,
			Title:       defaultStr(doc.Meta.Title, slug),
			Date:        doc.Meta.Date,
			ParsedDate:  parsedDate,
			Description: doc.Meta.Description,
			HTML:        doc.HTML,
		})
	}
	sort.Slice(posts, func(i, j int) bool {
		if posts[i].Date != posts[j].Date {
			return posts[i].Date > posts[j].Date
		}
		return posts[i].Slug > posts[j].Slug
	})

	var idxBody strings.Builder
	idxBody.WriteString("<h1>News &amp; Releases</h1>\n")
	idxBody.WriteString("<p class=\"meta-note\">Subscribe via <a href=\"/feed.xml\">RSS 2.0 (/feed.xml)</a>.</p>\n")
	for _, p := range posts {
		fmt.Fprintf(&idxBody, "<article>\n<h2><a href=\"/news/%s.html\">%s</a></h2>\n",
			html.EscapeString(p.Slug), html.EscapeString(p.Title))
		if p.Date != "" {
			fmt.Fprintf(&idxBody, "<p class=\"meta-note\"><time datetime=\"%s\">%s</time></p>\n",
				html.EscapeString(p.Date), html.EscapeString(p.Date))
		}
		if p.Description != "" {
			fmt.Fprintf(&idxBody, "<p>%s</p>\n", html.EscapeString(p.Description))
		}
		idxBody.WriteString("</article>\n")

		postBody := fmt.Sprintf("<p class=\"meta-note\"><a href=\"/news/\">&larr; All News</a> &middot; <time datetime=\"%s\">%s</time></p>\n%s",
			html.EscapeString(p.Date), html.EscapeString(p.Date), p.HTML)
		if err := writeHTMLPage(filepath.Join(outDir, p.Slug+".html"), site, PageMeta{
			Title:       p.Title,
			Description: p.Description,
			ActiveNav:   "news",
			Canonical:   "/news/" + p.Slug + ".html",
		}, postBody); err != nil {
			return nil, err
		}
	}

	if err := writeHTMLPage(filepath.Join(outDir, "index.html"), site, PageMeta{
		Title:       "News",
		Description: "KryX Linux release announcements and engineering news.",
		ActiveNav:   "news",
		Canonical:   "/news/",
	}, idxBody.String()); err != nil {
		return nil, err
	}
	return posts, nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	})
}
