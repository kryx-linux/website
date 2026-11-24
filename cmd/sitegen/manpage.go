package main

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ManPage represents a converted roff manual page.
type ManPage struct {
	Filename    string // e.g. "kpm.8"
	Name        string // e.g. "kpm"
	Section     string // e.g. "8"
	Title       string // e.g. "kpm(8)"
	Summary     string // Extracted from NAME section
	AliasOf     string // Non-empty when .so points to another page
	HTML        string
	Headings    []Heading
	SourcePath  string
}

// DiscoverManPages scans comma-separated directories (earlier directories win on duplicate names)
// and converts discovered roff man pages into clean semantic HTML5.
func DiscoverManPages(dirs []string) ([]ManPage, error) {
	found := make(map[string]string)
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			ext := filepath.Ext(e.Name())
			if len(ext) != 2 || ext[1] < '1' || ext[1] > '9' {
				continue
			}
			if _, exists := found[e.Name()]; !exists {
				found[e.Name()] = filepath.Join(dir, e.Name())
			}
		}
	}

	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)

	knownSet := make(map[string]bool, len(names))
	for _, n := range names {
		knownSet[n] = true
	}

	var pages []ManPage
	for _, fname := range names {
		srcPath := found[fname]
		raw, err := os.ReadFile(srcPath)
		if err != nil {
			continue
		}
		mp := convertRoffManPage(fname, srcPath, string(raw), found, knownSet)
		pages = append(pages, mp)
	}

	// Prioritize primary KryX man pages first in the index.
	priority := map[string]int{
		"kpm.8":          1,
		"kpm-index.8":    2,
		"kryx-src.8":     3,
		"ports.8":        4,
		"setup.8":        5,
		"setup-kryx.8":   6,
		"setup-chroot.8": 7,
		"setup-repo.8":   8,
	}
	sort.Slice(pages, func(i, j int) bool {
		pi, oki := priority[pages[i].Filename]
		pj, okj := priority[pages[j].Filename]
		if oki && okj {
			return pi < pj
		}
		if oki != okj {
			return oki
		}
		return pages[i].Filename < pages[j].Filename
	})
	return pages, nil
}

func convertRoffManPage(fname, srcPath, content string, allFiles map[string]string, knownSet map[string]bool) ManPage {
	sec := strings.TrimPrefix(filepath.Ext(fname), ".")
	base := strings.TrimSuffix(fname, filepath.Ext(fname))
	mp := ManPage{
		Filename:   fname,
		Name:       base,
		Section:    sec,
		Title:      fmt.Sprintf("%s(%s)", base, sec),
		SourcePath: srcPath,
	}

	// Handle roff `.so man8/kpm.8` alias pages cleanly.
	trim := strings.TrimSpace(content)
	if strings.HasPrefix(trim, ".so ") {
		target := filepath.Base(strings.TrimSpace(strings.TrimPrefix(trim, ".so ")))
		mp.AliasOf = target
		if targetPath, ok := allFiles[target]; ok {
			if targetRaw, err := os.ReadFile(targetPath); err == nil {
				content = string(targetRaw)
			}
		}
	}

	bodyHTML, headings, summary := renderRoffToHTML(content, knownSet)
	if mp.AliasOf != "" {
		notice := fmt.Sprintf("<p class=\"callout\"><strong>%s</strong> is documented jointly in <a href=\"/man/%s.html\">%s</a> (shown below).</p>\n",
			html.EscapeString(mp.Title), html.EscapeString(mp.AliasOf), html.EscapeString(mp.AliasOf))
		bodyHTML = notice + bodyHTML
		if summary == "" {
			summary = "Alias for " + mp.AliasOf
		}
	}
	mp.Summary = summary
	mp.Headings = headings
	toc := BuildTOC(headings)
	mp.HTML = fmt.Sprintf("<h1>%s</h1>\n", html.EscapeString(mp.Title))
	if summary != "" {
		mp.HTML += fmt.Sprintf("<p class=\"meta-note\">%s</p>\n", html.EscapeString(summary))
	}
	if toc != "" {
		mp.HTML += toc
	}
	mp.HTML += bodyHTML
	return mp
}

func renderRoffToHTML(input string, knownSet map[string]bool) (string, []Heading, string) {
	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	var out strings.Builder
	var headings []Heading
	usedIDs := make(map[string]int)

	uniqueSlug := func(text string) string {
		base := slugify(text)
		if base == "" {
			base = "sec"
		}
		if n, ok := usedIDs[base]; ok {
			usedIDs[base] = n + 1
			return fmt.Sprintf("%s-%d", base, n+1)
		}
		usedIDs[base] = 1
		return base
	}

	inP := false
	inDL := false
	inDD := false
	inPre := false
	expectDT := false
	inNameSec := false
	var nameSummary string

	closeP := func() {
		if inP {
			out.WriteString("</p>\n")
			inP = false
		}
	}
	closeDD := func() {
		if inDD {
			out.WriteString("</dd>\n")
			inDD = false
		}
	}
	closeDL := func() {
		closeDD()
		if inDL {
			out.WriteString("</dl>\n")
			inDL = false
		}
	}
	closePre := func() {
		if inPre {
			out.WriteString("</code></pre>\n")
			inPre = false
		}
	}

	appendText := func(frag string) {
		if frag == "" {
			return
		}
		if expectDT {
			if !inDL {
				out.WriteString("<dl class=\"man-dl\">\n")
				inDL = true
			}
			closeDD()
			fmt.Fprintf(&out, "<dt>%s</dt>\n<dd>", frag)
			expectDT = false
			inDD = true
			return
		}
		if inDD {
			out.WriteString(frag)
			out.WriteByte(' ')
			return
		}
		if !inP {
			out.WriteString("<p>")
			inP = true
		}
		out.WriteString(frag)
		out.WriteByte(' ')
	}

	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, " \t")

		// Comments
		if strings.HasPrefix(line, `.\"`) || strings.HasPrefix(line, `'"`) {
			continue
		}

		// Inside preformatted block (.nf .. .fi)
		if inPre {
			if strings.HasPrefix(line, ".fi") {
				closePre()
				continue
			}
			if strings.HasPrefix(line, ".") {
				continue
			}
			out.WriteString(html.EscapeString(unescapeRoffPlain(line)))
			out.WriteByte('\n')
			continue
		}

		if strings.TrimSpace(line) == "" {
			closeP()
			continue
		}

		if !strings.HasPrefix(line, ".") {
			if inNameSec && nameSummary == "" {
				plain := unescapeRoffPlain(line)
				if idx := strings.Index(plain, " - "); idx >= 0 {
					nameSummary = strings.TrimSpace(plain[idx+3:])
				} else {
					nameSummary = strings.TrimSpace(plain)
				}
			}
			appendText(renderRoffInline(line, knownSet))
			continue
		}

		// Macro line
		macro, arg := splitRoffMacro(line)
		switch macro {
		case "TH", "so", "RS", "RE":
			// Handled at page level or indentation wrapper
		case "SH":
			closeP()
			closeDL()
			closePre()
			title := strings.Trim(unescapeRoffPlain(arg), `"`)
			inNameSec = strings.EqualFold(title, "NAME")
			id := uniqueSlug(title)
			headings = append(headings, Heading{Level: 2, Text: title, ID: id})
			fmt.Fprintf(&out, "<h2 id=\"%s\">%s<a class=\"heading-anchor\" href=\"#%s\" aria-label=\"Link to %s\">#</a></h2>\n",
				html.EscapeString(id), html.EscapeString(title), html.EscapeString(id), html.EscapeString(title))
		case "SS":
			closeP()
			closeDL()
			closePre()
			title := strings.Trim(unescapeRoffPlain(arg), `"`)
			id := uniqueSlug(title)
			headings = append(headings, Heading{Level: 3, Text: title, ID: id})
			fmt.Fprintf(&out, "<h3 id=\"%s\">%s<a class=\"heading-anchor\" href=\"#%s\" aria-label=\"Link to %s\">#</a></h3>\n",
				html.EscapeString(id), html.EscapeString(title), html.EscapeString(id), html.EscapeString(title))
		case "PP", "LP", "P":
			closeP()
			closeDL()
		case "br":
			if inP || inDD {
				out.WriteString("<br>\n")
			}
		case "nf":
			closeP()
			closeDL()
			out.WriteString("<pre><code>")
			inPre = true
		case "fi":
			closePre()
		case "TP":
			closeP()
			closeDD()
			expectDT = true
		case "IP":
			closeP()
			closeDD()
			parts := splitRoffArgs(arg)
			tag := "•"
			if len(parts) > 0 && parts[0] != "" {
				tag = parts[0]
			}
			if !inDL {
				out.WriteString("<dl class=\"man-dl\">\n")
				inDL = true
			}
			fmt.Fprintf(&out, "<dt>%s</dt>\n<dd>", renderRoffInline(tag, knownSet))
			inDD = true
		case "B":
			appendText("<strong>" + renderRoffInline(unquoteRoff(arg), knownSet) + "</strong>")
		case "I":
			appendText("<em>" + renderRoffInline(unquoteRoff(arg), knownSet) + "</em>")
		case "BR", "BI", "RB", "RI", "IB", "IR":
			appendText(renderAlternatingRoff(macro, arg, knownSet))
		}
	}

	closeP()
	closeDL()
	closePre()

	return out.String(), headings, nameSummary
}

func splitRoffMacro(line string) (string, string) {
	rest := strings.TrimPrefix(line, ".")
	rest = strings.TrimLeft(rest, " \t")
	if idx := strings.IndexAny(rest, " \t"); idx >= 0 {
		return rest[:idx], strings.TrimSpace(rest[idx+1:])
	}
	return rest, ""
}

func splitRoffArgs(s string) []string {
	var args []string
	var cur strings.Builder
	inQuotes := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '"' {
			inQuotes = !inQuotes
			continue
		}
		if (ch == ' ' || ch == '\t') && !inQuotes {
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteByte(ch)
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
}

func unquoteRoff(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func renderAlternatingRoff(macro, arg string, knownSet map[string]bool) string {
	parts := splitRoffArgs(arg)
	if len(parts) == 0 {
		return ""
	}
	// Detect cross-reference like `.BR kpm (8)` or `.BR kryx\-src (8),`
	if macro == "BR" && len(parts) >= 2 && strings.HasPrefix(parts[1], "(") {
		name := unescapeRoffPlain(parts[0])
		rest := unescapeRoffPlain(strings.Join(parts[1:], ""))
		if m := reManRefSuffix.FindStringSubmatch(rest); len(m) == 3 {
			target := name + "." + m[1]
			if knownSet[target] {
				return fmt.Sprintf("<a href=\"/man/%s.html\"><strong>%s</strong>(%s)</a>%s",
					html.EscapeString(target), html.EscapeString(name), html.EscapeString(m[1]), html.EscapeString(m[2]))
			}
		}
	}

	var sb strings.Builder
	for i, p := range parts {
		style := macro[i%2]
		rendered := renderRoffInline(p, knownSet)
		switch style {
		case 'B':
			sb.WriteString("<strong>" + rendered + "</strong>")
		case 'I':
			sb.WriteString("<em>" + rendered + "</em>")
		default:
			sb.WriteString(rendered)
		}
	}
	return sb.String()
}

var reManRefSuffix = regexp.MustCompile(`^\(([1-9])\)(.*)$`)

func renderRoffInline(s string, knownSet map[string]bool) string {
	_ = knownSet
	var out strings.Builder
	openTag := ""
	closeTag := func() {
		if openTag != "" {
			out.WriteString("</" + openTag + ">")
			openTag = ""
		}
	}

	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			next := s[i+1]
			switch next {
			case 'f':
				if i+2 < len(s) {
					code := s[i+2]
					closeTag()
					switch code {
					case 'B':
						out.WriteString("<strong>")
						openTag = "strong"
					case 'I':
						out.WriteString("<em>")
						openTag = "em"
					case 'R', 'P':
						// reset
					}
					i += 2
					continue
				}
			case '-':
				out.WriteByte('-')
				i++
				continue
			case 'e', '\\':
				out.WriteByte('\\')
				i++
				continue
			case '&':
				i++
				continue
			case ' ':
				out.WriteString("&nbsp;")
				i++
				continue
			case '(':
				if i+3 < len(s) {
					sym := s[i+2 : i+4]
					switch sym {
					case "em":
						out.WriteString("&mdash;")
					case "en":
						out.WriteString("&ndash;")
					case "dq":
						out.WriteString("&quot;")
					case "bu":
						out.WriteString("&bull;")
					default:
						out.WriteString(html.EscapeString(sym))
					}
					i += 3
					continue
				}
			}
		}
		out.WriteString(html.EscapeString(s[i : i+1]))
	}
	closeTag()
	return out.String()
}

func unescapeRoffPlain(s string) string {
	r := strings.NewReplacer(
		`\-`, "-",
		`\e`, `\`,
		`\\`, `\`,
		`\&`, "",
		`\ `, " ",
		`\fB`, "",
		`\fI`, "",
		`\fR`, "",
		`\fP`, "",
		`\(em`, "—",
		`\(dq`, `"`,
	)
	return r.Replace(s)
}
