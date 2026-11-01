// Package reader turns article HTML into Markdown for the native reader.
//
// MyGo's ui/markdown widget renders CommonMark with GFM tables,
// strikethrough and fenced code, but it does not understand raw HTML.
// ToMarkdown therefore rewrites an article into that subset, dropping
// scripts, styles, hidden nodes and unsafe URLs along the way.
package reader

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// ToMarkdown converts an article's HTML into safe GitHub-flavoured Markdown
// that ui/markdown can show. base resolves relative links and images.
func ToMarkdown(src string, base *url.URL) string {
	if !strings.Contains(src, "<") {
		return plainInputToMarkdown(src)
	}
	c := &converter{base: base}
	return c.renderDocument(parseHTML(src))
}

// PlainText returns the readable text of an HTML fragment.
func PlainText(src string) string {
	if !strings.Contains(src, "<") {
		return plainInputToText(src)
	}
	c := &converter{plain: true}
	return c.renderDocument(parseHTML(src))
}

// Summary returns at most n runes of readable text, ending on a word
// boundary where possible, without trailing space.
func Summary(src string, n int) string {
	if n <= 0 {
		return ""
	}
	text := strings.Join(strings.Fields(PlainText(src)), " ")
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= n {
		return text
	}
	cut := runes[:n]
	// Only retreat to a space when the cut would split a word. Chinese
	// text has no spaces, so it is cut on a rune boundary instead.
	if !unicode.IsSpace(runes[n]) {
		for i := len(cut) - 1; i >= 0; i-- {
			if unicode.IsSpace(cut[i]) {
				cut = cut[:i]
				break
			}
		}
	}
	return strings.TrimRightFunc(string(cut), unicode.IsSpace)
}

type converter struct {
	base  *url.URL
	plain bool
}

func parseHTML(src string) *html.Node {
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return nil
	}
	return doc
}

func (c *converter) renderDocument(n *html.Node) string {
	if n == nil {
		return ""
	}
	blocks := c.renderContainer(n)
	out := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if b = strings.TrimSpace(b); b != "" {
			out = append(out, b)
		}
	}
	return collapseBlankLines(strings.Join(out, "\n\n"))
}

// inlineWriter accumulates inline text. Whitespace is kept pending until a
// non-space rune arrives, which makes it possible to place spaces outside
// emphasis markers and to drop whitespace between CJK characters.
type inlineWriter struct {
	b         strings.Builder
	pending   bool
	last      rune
	has       bool
	lineStart bool
}

func (w *inlineWriter) addRaw(s string) {
	if s == "" {
		return
	}
	w.b.WriteString(s)
	w.pending = false
	for _, r := range s {
		w.last = r
		w.has = true
		w.lineStart = r == '\n'
	}
}

func (w *inlineWriter) flushSpace() {
	if !w.pending {
		return
	}
	w.pending = false
	if !w.has || w.lineStart || isCJK(w.last) {
		return
	}
	w.b.WriteByte(' ')
	w.last = ' '
}

func (w *inlineWriter) writeText(s string) {
	for _, r := range s {
		if unicode.IsSpace(r) {
			w.pending = true
			continue
		}
		if w.pending {
			w.pending = false
			if w.has && !w.lineStart && !suppressSpace(w.last, r) {
				w.b.WriteByte(' ')
			}
		}
		w.b.WriteRune(r)
		w.last = r
		w.has = true
		w.lineStart = false
	}
}

func (w *inlineWriter) hardBreak() {
	w.pending = false
	if !w.has || w.lineStart {
		return
	}
	w.b.WriteString("  \n")
	w.last = '\n'
	w.lineStart = true
}

func (w *inlineWriter) lineBreak() {
	w.pending = false
	if !w.has || w.lineStart {
		return
	}
	w.b.WriteByte('\n')
	w.last = '\n'
	w.lineStart = true
}

func (w *inlineWriter) finish() string {
	w.pending = false
	return strings.TrimSpace(w.b.String())
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r) ||
		(r >= 0x3000 && r <= 0x303F) || // CJK symbols and punctuation
		(r >= 0xFF00 && r <= 0xFFEF) // fullwidth forms
}

func suppressSpace(a, b rune) bool {
	if isCJK(a) && isCJK(b) {
		return true
	}
	if isCJK(a) {
		switch b {
		case ',', '.', ';', ':', '!', '?', ')', ']', '}':
			return true
		}
	}
	return false
}

// collapseBlankLines caps runs of three or more blank lines at two blank
// lines, leaving fenced code blocks untouched.
func collapseBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	inFence := false
	var fenceChar byte
	var fenceLen int
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if inFence {
			out = append(out, line)
			if isFenceLine(trimmed, fenceChar) && len(leadingFence(trimmed)) >= fenceLen {
				inFence = false
			}
			continue
		}
		if f := leadingFence(trimmed); len(f) >= 3 {
			inFence = true
			fenceChar = f[0]
			fenceLen = len(f)
			blank = 0
			out = append(out, line)
			continue
		}
		if trimmed == "" {
			blank++
			if blank > 2 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func leadingFence(s string) string {
	i := 0
	for i < len(s) && s[i] == ' ' && i < 3 {
		i++
	}
	if i >= len(s) || (s[i] != '`' && s[i] != '~') {
		return ""
	}
	ch := s[i]
	j := i
	for j < len(s) && s[j] == ch {
		j++
	}
	return s[i:j]
}

func isFenceLine(s string, ch byte) bool {
	f := leadingFence(s)
	return len(f) >= 3 && f[0] == ch
}

var ignoredTags = map[string]bool{
	"script": true, "style": true, "noscript": true, "template": true,
	"iframe": true, "object": true, "embed": true, "svg": true,
	"canvas": true, "form": true, "input": true, "button": true,
	"select": true, "textarea": true, "video": true, "audio": true,
	"map": true, "area": true, "link": true, "meta": true,
	// Document furniture that must never leak into article text.
	"head": true, "title": true, "base": true, "source": true,
	"track": true, "param": true, "col": true, "colgroup": true,
}

var blockTags = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true,
	"body": true, "caption": true, "center": true, "dd": true,
	"details": true, "dialog": true, "div": true, "dl": true, "dt": true,
	"fieldset": true, "figcaption": true, "figure": true, "footer": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"header": true, "hgroup": true, "hr": true, "html": true, "li": true,
	"main": true, "nav": true, "ol": true, "p": true, "pre": true,
	"section": true, "summary": true, "table": true, "tbody": true,
	"td": true, "tfoot": true, "th": true, "thead": true, "tr": true,
	"ul": true,
}

var containerTags = map[string]bool{
	"address": true, "article": true, "aside": true, "body": true,
	"center": true, "details": true, "dialog": true, "div": true,
	"fieldset": true, "footer": true, "header": true, "hgroup": true,
	"html": true, "main": true, "nav": true, "section": true,
	"summary": true, "tbody": true, "td": true, "tfoot": true,
	"th": true, "thead": true, "tr": true,
}

func lower(s string) string { return strings.ToLower(s) }

func isBlockTag(tag string) bool { return blockTags[tag] }

func attr(n *html.Node, name string) (string, bool) {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val, true
		}
	}
	return "", false
}

func attrValue(n *html.Node, name string) string {
	v, _ := attr(n, name)
	return v
}

func (c *converter) skip(n *html.Node) bool {
	if n == nil || n.Type != html.ElementNode {
		return false
	}
	return ignoredTags[lower(n.Data)] || isHiddenNode(n)
}

func (c *converter) renderContainer(n *html.Node) []string {
	if n == nil {
		return nil
	}
	var out []string
	var w inlineWriter
	flush := func() {
		text := w.finish()
		w = inlineWriter{}
		if text != "" {
			out = append(out, text)
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.TextNode:
			w.writeText(child.Data)
		case html.ElementNode:
			if c.skip(child) {
				continue
			}
			tag := lower(child.Data)
			if isBlockTag(tag) {
				flush()
				out = append(out, c.renderBlock(child)...)
			} else {
				c.renderInlineNode(child, &w)
			}
		}
	}
	flush()
	return out
}

func (c *converter) renderBlock(n *html.Node) []string {
	if c.skip(n) {
		return nil
	}
	tag := lower(n.Data)
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		text := c.inlineText(n)
		if text == "" {
			return nil
		}
		if c.plain {
			return []string{text}
		}
		return []string{strings.Repeat("#", int(tag[1]-'0')) + " " + text}
	case "p":
		if text := c.inlineText(n); text != "" {
			return []string{text}
		}
		return nil
	case "blockquote":
		inner := c.renderContainer(n)
		if len(inner) == 0 {
			return nil
		}
		if c.plain {
			return inner
		}
		return []string{prefixLines(strings.Join(inner, "\n\n"), "> ")}
	case "pre":
		return c.renderPre(n)
	case "hr":
		if c.plain {
			return nil
		}
		return []string{"---"}
	case "ul", "ol":
		if text := c.renderList(n, 0); text != "" {
			return []string{text}
		}
		return nil
	case "li":
		marker := "- "
		if c.plain {
			marker = ""
		}
		lines := c.renderListItem(n, 0, marker)
		if len(lines) == 0 {
			return nil
		}
		return []string{strings.Join(lines, "\n")}
	case "figure":
		return c.renderFigure(n)
	case "figcaption":
		text := c.inlineText(n)
		if text == "" {
			return nil
		}
		if c.plain {
			return []string{text}
		}
		return []string{"*" + text + "*"}
	case "dl":
		return c.renderDL(n)
	case "dt":
		text := c.inlineText(n)
		if text == "" {
			return nil
		}
		if c.plain {
			return []string{text}
		}
		return []string{"**" + text + "**"}
	case "dd":
		if text := c.inlineText(n); text != "" {
			return []string{text}
		}
		return nil
	case "table":
		if table := c.renderTable(n); table != nil {
			return table
		}
		return c.renderContainer(n)
	default:
		if containerTags[tag] {
			return c.renderContainer(n)
		}
		if text := c.inlineText(n); text != "" {
			return []string{text}
		}
		return nil
	}
}

func (c *converter) inlineText(n *html.Node) string {
	var w inlineWriter
	c.renderInlineChildren(n, &w)
	return w.finish()
}

func (c *converter) renderInlineChildren(n *html.Node, w *inlineWriter) {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		c.renderInlineNode(child, w)
	}
}

func prefixLines(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = strings.TrimRight(prefix, " ")
		} else {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

func (c *converter) renderPre(n *html.Node) []string {
	lang := languageFromClass(attrValue(n, "class"))
	code := n
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && lower(child.Data) == "code" {
			code = child
			if lang == "" {
				lang = languageFromClass(attrValue(child, "class"))
			}
			break
		}
	}
	raw := rawText(code)
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	raw = strings.TrimPrefix(raw, "\n")
	raw = strings.TrimRight(raw, "\n")
	if c.plain {
		if strings.TrimSpace(raw) == "" {
			return nil
		}
		return []string{raw}
	}
	fenceLen := 3
	if run := maxBacktickRun(raw); run+1 > fenceLen {
		fenceLen = run + 1
	}
	fence := strings.Repeat("`", fenceLen)
	var b strings.Builder
	b.WriteString(fence)
	b.WriteString(lang)
	b.WriteByte('\n')
	b.WriteString(raw)
	if raw != "" {
		b.WriteByte('\n')
	}
	b.WriteString(fence)
	return []string{b.String()}
}

func languageFromClass(class string) string {
	for _, field := range strings.Fields(class) {
		for _, prefix := range []string{"language-", "lang-"} {
			if strings.HasPrefix(field, prefix) {
				lang := strings.TrimPrefix(field, prefix)
				if isLanguageToken(lang) {
					return lang
				}
			}
		}
	}
	return ""
}

func isLanguageToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) &&
			r != '-' && r != '_' && r != '+' && r != '#' && r != '.' {
			return false
		}
	}
	return true
}

func maxBacktickRun(s string) int {
	best, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			if run > best {
				best = run
			}
		} else {
			run = 0
		}
	}
	return best
}

func rawText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		for child := x.FirstChild; child != nil; child = child.NextSibling {
			switch child.Type {
			case html.TextNode:
				b.WriteString(child.Data)
			case html.ElementNode:
				if lower(child.Data) == "br" {
					b.WriteByte('\n')
				} else {
					walk(child)
				}
			}
		}
	}
	walk(n)
	return b.String()
}

func (c *converter) renderList(n *html.Node, depth int) string {
	ordered := lower(n.Data) == "ol"
	start := 1
	if ordered {
		if v, ok := attr(n, "start"); ok {
			if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				start = parsed
			}
		}
	}
	var lines []string
	index := 0
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode || lower(child.Data) != "li" || c.skip(child) {
			continue
		}
		index++
		marker := ""
		if !c.plain {
			if ordered {
				marker = fmt.Sprintf("%d. ", start+index-1)
			} else {
				marker = "- "
			}
		}
		lines = append(lines, c.renderListItem(child, depth, marker)...)
	}
	return strings.Join(lines, "\n")
}

func (c *converter) renderListItem(li *html.Node, depth int, marker string) []string {
	indent := strings.Repeat("  ", depth)
	contentIndent := indent + strings.Repeat(" ", len(marker))
	var lines []string
	var w inlineWriter
	started := false

	emitText := func() {
		text := w.finish()
		w = inlineWriter{}
		if text == "" {
			return
		}
		for _, part := range strings.Split(text, "\n") {
			if !started {
				lines = append(lines, indent+marker+part)
				started = true
			} else {
				lines = append(lines, contentIndent+part)
			}
		}
	}
	emitBlock := func(block string) {
		block = strings.TrimSpace(block)
		if block == "" {
			return
		}
		if started {
			lines = append(lines, "")
		}
		for _, part := range strings.Split(block, "\n") {
			if !started {
				lines = append(lines, indent+marker+part)
				started = true
			} else {
				lines = append(lines, contentIndent+part)
			}
		}
	}

	for child := li.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.TextNode:
			w.writeText(child.Data)
		case html.ElementNode:
			if c.skip(child) {
				continue
			}
			tag := lower(child.Data)
			if tag == "ul" || tag == "ol" {
				emitText()
				if !started {
					lines = append(lines, strings.TrimRight(indent+marker, " "))
					started = true
				}
				nested := c.renderList(child, depth+1)
				if nested != "" {
					lines = append(lines, strings.Split(nested, "\n")...)
				}
				continue
			}
			if isBlockTag(tag) {
				emitText()
				for _, block := range c.renderBlock(child) {
					emitBlock(block)
				}
				continue
			}
			c.renderInlineNode(child, &w)
		}
	}
	emitText()
	return lines
}

func (c *converter) renderFigure(n *html.Node) []string {
	var out []string
	var w inlineWriter
	caption := ""
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.TextNode:
			w.writeText(child.Data)
		case html.ElementNode:
			if c.skip(child) {
				continue
			}
			if lower(child.Data) == "figcaption" {
				caption = textOnly(child)
				continue
			}
			c.renderInlineNode(child, &w)
		}
	}
	if media := w.finish(); media != "" {
		out = append(out, media)
	}
	if caption != "" {
		if c.plain {
			out = append(out, caption)
		} else {
			out = append(out, "*"+caption+"*")
		}
	}
	return out
}

func (c *converter) renderDL(n *html.Node) []string {
	var out []string
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode || c.skip(child) {
			continue
		}
		switch lower(child.Data) {
		case "dt":
			text := c.inlineText(child)
			if text == "" {
				continue
			}
			if c.plain {
				out = append(out, text)
			} else {
				out = append(out, "**"+text+"**")
			}
		case "dd":
			if text := c.inlineText(child); text != "" {
				out = append(out, text)
			}
		case "div":
			out = append(out, c.renderDL(child)...)
		}
	}
	return out
}

func (c *converter) renderTable(n *html.Node) []string {
	var rows [][]string
	var collect func(*html.Node)
	collect = func(parent *html.Node) {
		for child := parent.FirstChild; child != nil; child = child.NextSibling {
			if child.Type != html.ElementNode || c.skip(child) {
				continue
			}
			switch lower(child.Data) {
			case "tr":
				rows = append(rows, c.tableRow(child))
			case "table":
				// A nested table is part of a cell's text, not a new row.
			default:
				collect(child)
			}
		}
	}
	collect(n)
	if len(rows) == 0 {
		return nil
	}
	cols := 0
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if cols == 0 {
		return nil
	}
	pad := func(row []string) []string {
		out := make([]string, cols)
		copy(out, row)
		return out
	}
	if c.plain {
		var lines []string
		for _, row := range rows {
			cells := pad(row)
			lines = append(lines, strings.Join(cells, " "))
		}
		return []string{strings.Join(lines, "\n")}
	}
	renderRow := func(row []string) string {
		cells := pad(row)
		for i, cell := range cells {
			cells[i] = strings.ReplaceAll(cell, "|", `\|`)
		}
		return "| " + strings.Join(cells, " | ") + " |"
	}
	lines := []string{renderRow(rows[0])}
	separator := make([]string, cols)
	for i := range separator {
		separator[i] = "---"
	}
	lines = append(lines, "| "+strings.Join(separator, " | ")+" |")
	for _, row := range rows[1:] {
		lines = append(lines, renderRow(row))
	}
	return []string{strings.Join(lines, "\n")}
}

func (c *converter) tableRow(tr *html.Node) []string {
	var cells []string
	for child := tr.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode || c.skip(child) {
			continue
		}
		switch lower(child.Data) {
		case "td", "th":
			cells = append(cells, textOnly(child))
		}
	}
	return cells
}

// textOnly flattens a subtree to unformatted text. It is used for table
// cells and figure captions, where Markdown markup would be noise.
func textOnly(n *html.Node) string {
	var w inlineWriter
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		for child := x.FirstChild; child != nil; child = child.NextSibling {
			switch child.Type {
			case html.TextNode:
				w.writeText(child.Data)
			case html.ElementNode:
				if lower(child.Data) == "br" {
					w.writeText(" ")
					continue
				}
				if ignoredTags[lower(child.Data)] || isHiddenNode(child) {
					continue
				}
				walk(child)
			}
		}
	}
	walk(n)
	return w.finish()
}

type fragment struct {
	text        string
	lead, trail bool
}

func (c *converter) inlineFragment(n *html.Node) fragment {
	var w inlineWriter
	c.renderInlineChildren(n, &w)
	f := fragment{text: w.b.String()}
	if w.pending {
		if w.has {
			f.trail = true
		} else {
			f.lead = true
		}
	}
	return f
}

func (w *inlineWriter) emitFragment(f fragment, open, close string) {
	if strings.TrimSpace(f.text) == "" {
		if f.lead || f.trail {
			w.pending = true
		}
		return
	}
	if f.lead {
		w.pending = true
	}
	w.flushSpace()
	if open != "" {
		w.addRaw(open)
	}
	w.addRaw(f.text)
	if close != "" {
		w.addRaw(close)
	}
	if f.trail {
		w.pending = true
	}
}

func (c *converter) renderInlineNode(n *html.Node, w *inlineWriter) {
	if n == nil {
		return
	}
	switch n.Type {
	case html.TextNode:
		w.writeText(n.Data)
		return
	case html.ElementNode:
		if c.skip(n) {
			return
		}
	default:
		return
	}
	tag := lower(n.Data)
	switch tag {
	case "br":
		if c.plain {
			w.lineBreak()
		} else {
			w.hardBreak()
		}
	case "img":
		c.renderImage(n, w)
	case "a":
		c.renderAnchor(n, w)
	case "strong", "b":
		c.renderWrapped(n, w, "**", "**")
	case "em", "i":
		c.renderWrapped(n, w, "*", "*")
	case "del", "s", "strike":
		c.renderWrapped(n, w, "~~", "~~")
	case "code":
		c.renderCode(n, w)
	case "mark", "sub", "sup", "u", "small", "big", "span", "font",
		"abbr", "cite", "q", "time", "label", "ins", "bdi", "bdo",
		"ruby", "rt", "rp", "kbd", "samp", "var", "dfn", "wbr":
		c.renderInlineChildren(n, w)
	default:
		if isBlockTag(tag) {
			if blocks := c.renderBlock(n); len(blocks) > 0 {
				w.writeText(strings.Join(blocks, " "))
			}
			return
		}
		c.renderInlineChildren(n, w)
	}
}

func (c *converter) renderWrapped(n *html.Node, w *inlineWriter, open, close string) {
	f := c.inlineFragment(n)
	if c.plain {
		w.emitFragment(f, "", "")
		return
	}
	w.emitFragment(f, open, close)
}

func (c *converter) renderAnchor(n *html.Node, w *inlineWriter) {
	if c.plain {
		c.renderInlineChildren(n, w)
		return
	}
	dest, ok := c.resolveURL(attrValue(n, "href"))
	if !ok {
		c.renderInlineChildren(n, w)
		return
	}
	f := c.inlineFragment(n)
	if strings.TrimSpace(f.text) == "" {
		return
	}
	if f.lead {
		w.pending = true
	}
	w.flushSpace()
	w.addRaw("[")
	w.addRaw(f.text)
	w.addRaw("](")
	w.addRaw(dest)
	w.addRaw(")")
	if f.trail {
		w.pending = true
	}
}

func (c *converter) renderImage(n *html.Node, w *inlineWriter) {
	if c.plain {
		return
	}
	src := attrValue(n, "src")
	if src == "" {
		src = attrValue(n, "data-src")
	}
	if src == "" {
		src = firstSrcsetURL(attrValue(n, "srcset"))
	}
	if src == "" || isTrackingImage(n) {
		return
	}
	dest, ok := c.resolveURL(src)
	if !ok {
		return
	}
	alt := normalizeAlt(attrValue(n, "alt"))
	w.flushSpace()
	w.addRaw("![" + alt + "](" + dest + ")")
}

func normalizeAlt(alt string) string {
	var w inlineWriter
	w.writeText(alt)
	s := w.finish()
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "[", `\[`)
	s = strings.ReplaceAll(s, "]", `\]`)
	return s
}

func firstSrcsetURL(srcset string) string {
	for _, part := range strings.Split(srcset, ",") {
		fields := strings.Fields(part)
		if len(fields) > 0 {
			return fields[0]
		}
	}
	return ""
}

func isTrackingImage(n *html.Node) bool {
	for _, name := range []string{"width", "height"} {
		if v, ok := attr(n, name); ok {
			if dim, known := parseDimension(v); known && dim <= 1 {
				return true
			}
		}
	}
	style := strings.ToLower(attrValue(n, "style"))
	for _, decl := range strings.Split(style, ";") {
		key, value, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "width", "height":
			if dim, known := parseDimension(value); known && dim <= 1 {
				return true
			}
		}
	}
	return false
}

func parseDimension(s string) (int, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(s[:i])
	if err != nil {
		return 0, false
	}
	rest := strings.TrimSpace(s[i:])
	if rest != "" && rest != "px" {
		return 0, false
	}
	return n, true
}

func (c *converter) renderCode(n *html.Node, w *inlineWriter) {
	raw := rawText(n)
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	raw = strings.ReplaceAll(raw, "\n", " ")
	raw = strings.ReplaceAll(raw, "\t", " ")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	if c.plain {
		w.writeText(raw)
		return
	}
	fence := strings.Repeat("`", maxBacktickRun(raw)+1)
	body := raw
	if strings.HasPrefix(body, "`") || strings.HasSuffix(body, "`") {
		body = " " + body + " "
	}
	w.flushSpace()
	w.addRaw(fence + body + fence)
}

func (c *converter) resolveURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	lowerRaw := strings.ToLower(raw)
	for _, scheme := range []string{"javascript:", "data:", "vbscript:"} {
		if strings.HasPrefix(lowerRaw, scheme) {
			return "", false
		}
	}
	if strings.HasPrefix(raw, "//") {
		if c.base == nil {
			return "", false
		}
		u, err := url.Parse("https:" + raw)
		if err != nil || u.Host == "" {
			return "", false
		}
		return escapeDestination(u), true
	}
	u, err := url.Parse(strings.ReplaceAll(raw, " ", "%20"))
	if err != nil {
		return "", false
	}
	if u.Scheme == "" {
		if c.base == nil {
			return "", false
		}
		u = c.base.ResolveReference(u)
	}
	scheme := strings.ToLower(u.Scheme)
	if (scheme != "http" && scheme != "https") || u.Host == "" {
		return "", false
	}
	return escapeDestination(u), true
}

func escapeDestination(u *url.URL) string {
	s := u.String()
	s = strings.ReplaceAll(s, "(", "%28")
	s = strings.ReplaceAll(s, ")", "%29")
	s = strings.ReplaceAll(s, " ", "%20")
	s = strings.ReplaceAll(s, "<", "%3C")
	s = strings.ReplaceAll(s, ">", "%3E")
	return s
}

func isHiddenNode(n *html.Node) bool {
	if n == nil || n.Type != html.ElementNode {
		return false
	}
	if _, ok := attr(n, "hidden"); ok {
		return true
	}
	if v, ok := attr(n, "aria-hidden"); ok && strings.EqualFold(strings.TrimSpace(v), "true") {
		return true
	}
	compact := strings.Join(strings.Fields(strings.ToLower(attrValue(n, "style"))), "")
	return strings.Contains(compact, "display:none") ||
		strings.Contains(compact, "visibility:hidden")
}

// splitPlainParagraphs normalises line endings, decodes entities and groups
// non-blank input lines into paragraphs.
func splitPlainParagraphs(src string) [][]string {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	src = html.UnescapeString(src)
	lines := strings.Split(src, "\n")
	var paragraphs [][]string
	var current []string
	flush := func() {
		if len(current) > 0 {
			paragraphs = append(paragraphs, current)
			current = nil
		}
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	return paragraphs
}

func normalizeLine(line string) string {
	var w inlineWriter
	w.writeText(line)
	return w.finish()
}

func plainInputToMarkdown(src string) string {
	var paragraphs []string
	for _, lines := range splitPlainParagraphs(src) {
		var rendered []string
		for _, line := range lines {
			if text := normalizeLine(line); text != "" {
				rendered = append(rendered, escapeLineStart(text))
			}
		}
		if len(rendered) > 0 {
			paragraphs = append(paragraphs, strings.Join(rendered, "\n"))
		}
	}
	return strings.Join(paragraphs, "\n\n")
}

func plainInputToText(src string) string {
	var paragraphs []string
	for _, lines := range splitPlainParagraphs(src) {
		var w inlineWriter
		for i, line := range lines {
			if i > 0 {
				w.writeText(" ")
			}
			w.writeText(line)
		}
		if text := w.finish(); text != "" {
			paragraphs = append(paragraphs, text)
		}
	}
	return strings.Join(paragraphs, "\n\n")
}

func escapeLineStart(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '#', '>', '-', '*', '+':
		return `\` + s
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i > 0 && i < len(s) && s[i] == '.' {
		return s[:i] + `\` + s[i:]
	}
	return s
}
