package main

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// FrontMatter holds simple key: value metadata from leading YAML front matter.
type FrontMatter struct {
	Title       string
	Description string
	Date        string
	Order       int
	TOC         string // "true", "false", or "" (auto)
}

// Heading records a rendered heading for auto-TOC generation.
type Heading struct {
	Level int
	Text  string
	ID    string
}

// RenderedDoc is the output of parsing and rendering a Markdown document.
type RenderedDoc struct {
	Meta     FrontMatter
	Headings []Heading
	HTML     string
	TOCHTML  string
	RawBody  string
}

// ParseMarkdown parses optional front matter and converts Markdown to semantic HTML5
// with clickable # heading anchors and an optional auto-generated Table of Contents.
func ParseMarkdown(input string, defaultAutoTOC bool) RenderedDoc {
	meta, body := splitFrontMatter(input)
	body = strings.ReplaceAll(body, "\r\n", "\n")

	htmlOut, headings := renderMarkdownBody(body)
	wantTOC := false
	switch strings.ToLower(strings.TrimSpace(meta.TOC)) {
	case "true", "yes", "1":
		wantTOC = true
	case "false", "no", "0":
		wantTOC = false
	default:
		if defaultAutoTOC && countTOCHeadings(headings) >= 3 {
			wantTOC = true
		}
	}

	var tocHTML string
	if wantTOC {
		tocHTML = BuildTOC(headings)
		if tocHTML != "" {
			htmlOut = injectTOCAfterH1(htmlOut, tocHTML)
		}
	}

	return RenderedDoc{
		Meta:     meta,
		Headings: headings,
		HTML:     htmlOut,
		TOCHTML:  tocHTML,
		RawBody:  body,
	}
}

func countTOCHeadings(hs []Heading) int {
	n := 0
	for _, h := range hs {
		if h.Level == 2 || h.Level == 3 {
			n++
		}
	}
	return n
}

// BuildTOC renders a semantic <nav class="toc"> for h2 and h3 headings.
func BuildTOC(headings []Heading) string {
	var items []Heading
	for _, h := range headings {
		if h.Level == 2 || h.Level == 3 {
			items = append(items, h)
		}
	}
	if len(items) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<nav class=\"toc\" aria-label=\"Table of Contents\">\n")
	sb.WriteString("<p class=\"toc-title\">Contents</p>\n<ul>\n")
	for _, h := range items {
		cls := ""
		if h.Level == 3 {
			cls = " class=\"toc-l3\""
		}
		fmt.Fprintf(&sb, "<li%s><a href=\"#%s\">%s</a></li>\n", cls, html.EscapeString(h.ID), html.EscapeString(h.Text))
	}
	sb.WriteString("</ul>\n</nav>\n")
	return sb.String()
}

func injectTOCAfterH1(htmlBody, tocHTML string) string {
	if idx := strings.Index(htmlBody, "</h1>"); idx >= 0 {
		pos := idx + len("</h1>")
		return htmlBody[:pos] + "\n" + tocHTML + htmlBody[pos:]
	}
	return tocHTML + htmlBody
}

func splitFrontMatter(input string) (FrontMatter, string) {
	var fm FrontMatter
	norm := strings.ReplaceAll(input, "\r\n", "\n")
	if !strings.HasPrefix(norm, "---\n") {
		return fm, norm
	}
	rest := norm[4:]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return fm, norm
	}
	header := rest[:end]
	body := strings.TrimLeft(rest[end+5:], "\n")
	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(line[:colon]))
		v := strings.TrimSpace(line[colon+1:])
		v = strings.Trim(v, `"'`)
		switch k {
		case "title":
			fm.Title = v
		case "description":
			fm.Description = v
		case "date":
			fm.Date = v
		case "toc":
			fm.TOC = v
		case "order":
			fmt.Sscanf(v, "%d", &fm.Order)
		}
	}
	return fm, body
}

func renderMarkdownBody(body string) (string, []Heading) {
	lines := strings.Split(body, "\n")
	var out strings.Builder
	var headings []Heading
	usedIDs := make(map[string]int)

	uniqueSlug := func(text string) string {
		base := slugify(text)
		if base == "" {
			base = "section"
		}
		if n, exists := usedIDs[base]; exists {
			usedIDs[base] = n + 1
			return fmt.Sprintf("%s-%d", base, n+1)
		}
		usedIDs[base] = 1
		return base
	}

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			i++
			continue
		}

		// Fenced code block (``` or ~~~)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence := trimmed[:3]
			lang := strings.TrimSpace(trimmed[3:])
			i++
			var codeLines []string
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), fence) {
				codeLines = append(codeLines, lines[i])
				i++
			}
			if i < len(lines) {
				i++ // skip closing fence
			}
			if lang != "" {
				fmt.Fprintf(&out, "<pre><code class=\"language-%s\">%s</code></pre>\n",
					html.EscapeString(lang), html.EscapeString(strings.Join(codeLines, "\n")))
			} else {
				fmt.Fprintf(&out, "<pre><code>%s</code></pre>\n",
					html.EscapeString(strings.Join(codeLines, "\n")))
			}
			continue
		}

		// ATX Headings (# .. ######)
		if strings.HasPrefix(trimmed, "#") {
			level := 0
			for level < len(trimmed) && level < 6 && trimmed[level] == '#' {
				level++
			}
			if level > 0 && level < len(trimmed) && trimmed[level] == ' ' {
				rawText := strings.TrimSpace(trimmed[level+1:])
				plainText := stripInlineMarkdown(rawText)
				id := uniqueSlug(plainText)
				headings = append(headings, Heading{Level: level, Text: plainText, ID: id})
				if level == 1 {
					fmt.Fprintf(&out, "<h1 id=\"%s\">%s</h1>\n", html.EscapeString(id), renderInline(rawText))
				} else {
					fmt.Fprintf(&out, "<h%d id=\"%s\">%s<a class=\"heading-anchor\" href=\"#%s\" aria-label=\"Link to %s\">#</a></h%d>\n",
						level, html.EscapeString(id), renderInline(rawText),
						html.EscapeString(id), html.EscapeString(plainText), level)
				}
				i++
				continue
			}
		}

		// Horizontal rule
		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			out.WriteString("<hr>\n")
			i++
			continue
		}

		// Blockquote (> ...)
		if strings.HasPrefix(trimmed, ">") {
			var qLines []string
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), ">") {
				t := strings.TrimSpace(lines[i])
				t = strings.TrimPrefix(t, ">")
				if strings.HasPrefix(t, " ") {
					t = t[1:]
				}
				qLines = append(qLines, t)
				i++
			}
			inner, _ := renderMarkdownBody(strings.Join(qLines, "\n"))
			out.WriteString("<blockquote>\n")
			out.WriteString(inner)
			out.WriteString("</blockquote>\n")
			continue
		}

		// GFM pipe table
		if strings.Contains(trimmed, "|") && i+1 < len(lines) && isTableSeparator(lines[i+1]) {
			headerCells := splitTableRow(trimmed)
			i += 2
			out.WriteString("<div class=\"table-wrap\">\n<table>\n<thead>\n<tr>\n")
			for _, cell := range headerCells {
				fmt.Fprintf(&out, "<th>%s</th>\n", renderInline(cell))
			}
			out.WriteString("</tr>\n</thead>\n<tbody>\n")
			for i < len(lines) {
				rowTrim := strings.TrimSpace(lines[i])
				if rowTrim == "" || !strings.Contains(rowTrim, "|") {
					break
				}
				cells := splitTableRow(rowTrim)
				out.WriteString("<tr>\n")
				for _, cell := range cells {
					fmt.Fprintf(&out, "<td>%s</td>\n", renderInline(cell))
				}
				out.WriteString("</tr>\n")
				i++
			}
			out.WriteString("</tbody>\n</table>\n</div>\n")
			continue
		}

		// Unordered list (- or *)
		if isUnorderedListItem(trimmed) {
			out.WriteString("<ul>\n")
			for i < len(lines) && isUnorderedListItem(strings.TrimSpace(lines[i])) {
				itemText := strings.TrimSpace(strings.TrimSpace(lines[i])[2:])
				i++
				for i < len(lines) && (strings.HasPrefix(lines[i], "  ") || strings.HasPrefix(lines[i], "\t")) &&
					!isUnorderedListItem(strings.TrimSpace(lines[i])) && strings.TrimSpace(lines[i]) != "" {
					itemText += " " + strings.TrimSpace(lines[i])
					i++
				}
				fmt.Fprintf(&out, "<li>%s</li>\n", renderInline(itemText))
			}
			out.WriteString("</ul>\n")
			continue
		}

		// Ordered list (1. ...)
		if item, ok := parseOrderedListItem(trimmed); ok {
			out.WriteString("<ol>\n")
			for i < len(lines) {
				txt, matched := parseOrderedListItem(strings.TrimSpace(lines[i]))
				if !matched {
					break
				}
				i++
				for i < len(lines) && (strings.HasPrefix(lines[i], "  ") || strings.HasPrefix(lines[i], "\t")) &&
					strings.TrimSpace(lines[i]) != "" {
					txt += " " + strings.TrimSpace(lines[i])
					i++
				}
				fmt.Fprintf(&out, "<li>%s</li>\n", renderInline(txt))
			}
			_ = item
			out.WriteString("</ol>\n")
			continue
		}

		// Paragraph
		var pLines []string
		for i < len(lines) {
			cur := strings.TrimSpace(lines[i])
			if cur == "" || strings.HasPrefix(cur, "#") || strings.HasPrefix(cur, "```") ||
				strings.HasPrefix(cur, "~~~") || strings.HasPrefix(cur, ">") ||
				isUnorderedListItem(cur) || cur == "---" || cur == "***" {
				break
			}
			if _, ok := parseOrderedListItem(cur); ok {
				break
			}
			if strings.Contains(cur, "|") && i+1 < len(lines) && isTableSeparator(lines[i+1]) {
				break
			}
			pLines = append(pLines, cur)
			i++
		}
		if len(pLines) > 0 {
			fmt.Fprintf(&out, "<p>%s</p>\n", renderInline(strings.Join(pLines, " ")))
		}
	}

	return out.String(), headings
}

func isUnorderedListItem(s string) bool {
	return strings.HasPrefix(s, "- ") || strings.HasPrefix(s, "* ")
}

func parseOrderedListItem(s string) (string, bool) {
	dot := strings.IndexByte(s, '.')
	if dot <= 0 || dot+1 >= len(s) || s[dot+1] != ' ' {
		return "", false
	}
	for j := 0; j < dot; j++ {
		if s[j] < '0' || s[j] > '9' {
			return "", false
		}
	}
	return strings.TrimSpace(s[dot+2:]), true
}

func isTableSeparator(line string) bool {
	t := strings.TrimSpace(line)
	if !strings.Contains(t, "|") || !strings.Contains(t, "-") {
		return false
	}
	for _, r := range t {
		if r != '|' && r != '-' && r != ':' && r != ' ' && r != '\t' {
			return false
		}
	}
	return true
}

func splitTableRow(line string) []string {
	t := strings.TrimSpace(line)
	t = strings.TrimPrefix(t, "|")
	t = strings.TrimSuffix(t, "|")
	raw := strings.Split(t, "|")
	out := make([]string, len(raw))
	for i, c := range raw {
		out[i] = strings.TrimSpace(c)
	}
	return out
}

// renderInline parses inline code, links, bold, and italic safely without breaking code spans.
func renderInline(s string) string {
	var out strings.Builder
	n := len(s)
	i := 0
	for i < n {
		// Inline code span: `...`
		if s[i] == '`' {
			end := strings.IndexByte(s[i+1:], '`')
			if end >= 0 {
				code := s[i+1 : i+1+end]
				out.WriteString("<code>")
				out.WriteString(html.EscapeString(code))
				out.WriteString("</code>")
				i = i + 2 + end
				continue
			}
		}

		// Link: [label](href)
		if s[i] == '[' {
			if closeBracket, openParen, closeParen, ok := matchMarkdownLink(s, i); ok {
				label := s[i+1 : closeBracket]
				href := strings.TrimSpace(s[openParen+1 : closeParen])
				fmt.Fprintf(&out, "<a href=\"%s\">%s</a>", html.EscapeString(href), renderInline(label))
				i = closeParen + 1
				continue
			}
		}

		// Bold: **text**
		if i+1 < n && s[i] == '*' && s[i+1] == '*' {
			end := strings.Index(s[i+2:], "**")
			if end > 0 {
				inner := s[i+2 : i+2+end]
				out.WriteString("<strong>")
				out.WriteString(renderInline(inner))
				out.WriteString("</strong>")
				i = i + 4 + end
				continue
			}
		}

		// Italic: *text* (avoid matching standalone '* ' or '**')
		if s[i] == '*' && i+1 < n && s[i+1] != ' ' && s[i+1] != '*' {
			end := strings.IndexByte(s[i+1:], '*')
			if end > 0 {
				inner := s[i+1 : i+1+end]
				out.WriteString("<em>")
				out.WriteString(renderInline(inner))
				out.WriteString("</em>")
				i = i + 2 + end
				continue
			}
		}

		out.WriteString(html.EscapeString(s[i : i+1]))
		i++
	}
	return out.String()
}

func matchMarkdownLink(s string, start int) (closeBracket, openParen, closeParen int, ok bool) {
	depth := 0
	for j := start; j < len(s); j++ {
		switch s[j] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				if j+1 < len(s) && s[j+1] == '(' {
					pDepth := 0
					for k := j + 1; k < len(s); k++ {
						if s[k] == '(' {
							pDepth++
						} else if s[k] == ')' {
							pDepth--
							if pDepth == 0 {
								return j, j + 1, k, true
							}
						}
					}
				}
				return 0, 0, 0, false
			}
		}
	}
	return 0, 0, 0, false
}

var reInlineCode = regexp.MustCompile("`([^`]*)`")
var reInlineLink = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)

func stripInlineMarkdown(s string) string {
	s = reInlineLink.ReplaceAllString(s, "$1")
	s = reInlineCode.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "*", "")
	return strings.TrimSpace(s)
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var sb strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			sb.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_' || r == '/' || r == '.':
			if !prevDash && sb.Len() > 0 {
				sb.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.TrimRight(sb.String(), "-")
}
