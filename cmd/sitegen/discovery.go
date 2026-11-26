package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	XMLNS   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

// GenerateSitemap emits a valid XML sitemap at <outDir>/sitemap.xml.
func GenerateSitemap(outDir, siteURL string, paths []string, buildTime time.Time) error {
	base := strings.TrimRight(siteURL, "/")
	lastmod := buildTime.UTC().Format("2006-01-02")

	seen := make(map[string]bool, len(paths))
	uniq := make([]string, 0, len(paths))
	for _, p := range paths {
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		if !seen[p] {
			seen[p] = true
			uniq = append(uniq, p)
		}
	}
	sort.Strings(uniq)

	doc := sitemapURLSet{
		XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  make([]sitemapURL, 0, len(uniq)),
	}
	for _, p := range uniq {
		doc.URLs = append(doc.URLs, sitemapURL{
			Loc:     base + p,
			LastMod: lastmod,
		})
	}

	encoded, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sitemap.xml: %w", err)
	}
	payload := append([]byte(xml.Header), encoded...)
	payload = append(payload, '\n')
	return os.WriteFile(filepath.Join(outDir, "sitemap.xml"), payload, 0o644)
}

// GenerateRobotsTxt emits <outDir>/robots.txt referencing sitemap.xml.
func GenerateRobotsTxt(outDir, siteURL string) error {
	base := strings.TrimRight(siteURL, "/")
	content := fmt.Sprintf("User-agent: *\nAllow: /\n\nSitemap: %s/sitemap.xml\n", base)
	return os.WriteFile(filepath.Join(outDir, "robots.txt"), []byte(content), 0o644)
}

// GenerateOpenSearch emits <outDir>/opensearch.xml so browsers offer KryX search
// directly from the address bar (zero client-side JS).
func GenerateOpenSearch(outDir, siteURL string) error {
	base := strings.TrimRight(siteURL, "/")
	xmlDoc := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<OpenSearchDescription xmlns="http://a9.com/-/spec/opensearch/1.1/">
  <ShortName>KryX Linux</ShortName>
  <Description>Search KryX Linux binary packages, ports, Handbook guides, and man pages.</Description>
  <InputEncoding>UTF-8</InputEncoding>
  <Image width="16" height="16" type="image/svg+xml">%s/kryx.svg</Image>
  <Url type="text/html" method="get" template="%s/search?q={searchTerms}"/>
</OpenSearchDescription>
`, base, base)
	return os.WriteFile(filepath.Join(outDir, "opensearch.xml"), []byte(xmlDoc), 0o644)
}

// Generate404Page emits <outDir>/404.html (served automatically by Cloudflare Pages).
func Generate404Page(outDir string, site SiteContext) error {
	var body strings.Builder
	body.WriteString("<h1>Page Not Found (404)</h1>\n")
	body.WriteString("<p>The requested page does not exist on KryX Linux. Try searching packages, ports, or Handbook documentation below:</p>\n")
	body.WriteString(renderSearchForm("", "all", "Search packages, ports, docs, man pages..."))
	body.WriteString("<h2>Quick Navigation</h2>\n<ul>\n")
	body.WriteString("<li><a href=\"/\">Home</a></li>\n")
	body.WriteString("<li><a href=\"/download/\">Download ISO &amp; RootFS</a></li>\n")
	body.WriteString("<li><a href=\"/docs/\">KryX Linux Handbook</a></li>\n")
	body.WriteString("<li><a href=\"/packages/\">Binary Packages</a> &middot; <a href=\"/browse\">Filter &amp; Browse</a></li>\n")
	body.WriteString("<li><a href=\"/ports/\">Ports Tree</a> &middot; <a href=\"/deps/\">Dependency Explorer</a></li>\n")
	body.WriteString("</ul>\n")

	return writeHTMLPage(filepath.Join(outDir, "404.html"), site, PageMeta{
		Title:       "404 Not Found",
		Description: "Requested page was not found on KryX Linux.",
		Canonical:   "/404.html",
	}, body.String())
}
