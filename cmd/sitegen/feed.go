package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// NewsPost represents a parsed Markdown announcement under content/news/.
type NewsPost struct {
	Slug        string
	Title       string
	Date        string
	ParsedDate  time.Time
	Description string
	HTML        string
}

type rssDocument struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	AtomNS  string     `xml:"xmlns:atom,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title         string     `xml:"title"`
	Link          string     `xml:"link"`
	Description   string     `xml:"description"`
	Language      string     `xml:"language"`
	LastBuildDate string     `xml:"lastBuildDate"`
	AtomLink      rssAtomRef `xml:"atom:link"`
	Items         []rssItem  `xml:"item"`
}

type rssAtomRef struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
}

// GenerateRSSFeed writes a valid RSS 2.0 XML feed to <outDir>/feed.xml.
func GenerateRSSFeed(outDir, siteURL string, posts []NewsPost, buildTime time.Time) error {
	base := strings.TrimRight(siteURL, "/")
	items := make([]rssItem, 0, len(posts))
	for _, p := range posts {
		link := base + "/news/" + p.Slug + ".html"
		pub := p.ParsedDate
		if pub.IsZero() {
			pub = buildTime
		}
		items = append(items, rssItem{
			Title:       p.Title,
			Link:        link,
			GUID:        link,
			PubDate:     pub.UTC().Format(time.RFC1123Z),
			Description: p.Description,
		})
	}

	doc := rssDocument{
		Version: "2.0",
		AtomNS:  "http://www.w3.org/2005/Atom",
		Channel: rssChannel{
			Title:         "KryX Linux News",
			Link:          base + "/news/",
			Description:   "Release announcements and engineering updates from KryX Linux.",
			Language:      "en-us",
			LastBuildDate: buildTime.UTC().Format(time.RFC1123Z),
			AtomLink: rssAtomRef{
				Href: base + "/feed.xml",
				Rel:  "self",
				Type: "application/rss+xml",
			},
			Items: items,
		},
	}

	encoded, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal rss feed: %w", err)
	}
	payload := append([]byte(xml.Header), encoded...)
	payload = append(payload, '\n')
	return os.WriteFile(filepath.Join(outDir, "feed.xml"), payload, 0o644)
}
