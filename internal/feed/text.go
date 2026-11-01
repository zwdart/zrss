package feed

import (
	"strings"

	xhtml "golang.org/x/net/html"
)

// plainBlockTags get a space at their boundaries so removed markup does not
// glue adjacent words together.
var plainBlockTags = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true,
	"br": true, "dd": true, "div": true, "dl": true, "dt": true,
	"fieldset": true, "figcaption": true, "figure": true, "footer": true,
	"form": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true, "header": true, "hr": true, "li": true,
	"nav": true, "ol": true, "p": true, "pre": true, "section": true,
	"table": true, "td": true, "th": true, "tr": true, "ul": true,
}

// PlainText turns an HTML fragment into readable text: tags removed,
// entities decoded, whitespace collapsed.
func PlainText(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}

	z := xhtml.NewTokenizer(strings.NewReader(s))
	var b strings.Builder
	skipping := ""

	for {
		switch tt := z.Next(); tt {
		case xhtml.ErrorToken:
			return strings.Join(strings.Fields(b.String()), " ")
		case xhtml.TextToken:
			if skipping == "" {
				b.Write(z.Text())
			}
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			name, _ := z.TagName()
			tag := strings.ToLower(string(name))
			if skipping != "" {
				continue
			}
			if tt == xhtml.StartTagToken && (tag == "script" || tag == "style") {
				skipping = tag
				continue
			}
			if plainBlockTags[tag] {
				b.WriteByte(' ')
			}
		case xhtml.EndTagToken:
			name, _ := z.TagName()
			tag := strings.ToLower(string(name))
			if skipping != "" {
				if tag == skipping {
					skipping = ""
				}
				continue
			}
			if plainBlockTags[tag] {
				b.WriteByte(' ')
			}
		}
	}
}
