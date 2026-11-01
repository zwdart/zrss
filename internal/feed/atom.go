package feed

import (
	"html"
	"net/url"
	"strings"
)

type atomDocument struct {
	Lang     string      `xml:"lang,attr"`
	Title    atomText    `xml:"title"`
	Subtitle atomText    `xml:"subtitle"`
	Tagline  atomText    `xml:"tagline"` // Atom 0.3
	ID       string      `xml:"id"`
	Links    []atomLink  `xml:"link"`
	Icon     string      `xml:"icon"`
	Logo     string      `xml:"logo"`
	Author   atomPerson  `xml:"author"`
	Entries  []atomEntry `xml:"entry"`
}

type atomText struct {
	Type  string `xml:"type,attr"`
	Mode  string `xml:"mode,attr"`
	Body  string `xml:",chardata"`
	Inner string `xml:",innerxml"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
	Href string `xml:"href,attr"`
}

type atomPerson struct {
	Name  string `xml:"name"`
	URI   string `xml:"uri"`
	Email string `xml:"email"`
}

type atomEntry struct {
	ID         string         `xml:"id"`
	Title      atomText       `xml:"title"`
	Links      []atomLink     `xml:"link"`
	Summary    atomText       `xml:"summary"`
	Contents   []atomContent  `xml:"content"`
	Published  string         `xml:"published"`
	Issued     string         `xml:"issued"` // Atom 0.3
	Updated    string         `xml:"updated"`
	Modified   string         `xml:"modified"` // Atom 0.3
	Author     atomPerson     `xml:"author"`
	Thumbnails []atomResource `xml:"thumbnail"`
	Enclosures []atomResource `xml:"enclosure"`
}

type atomContent struct {
	Type   string `xml:"type,attr"`
	Mode   string `xml:"mode,attr"`
	URL    string `xml:"url,attr"`
	Medium string `xml:"medium,attr"`
	Body   string `xml:",chardata"`
	Inner  string `xml:",innerxml"`
}

type atomResource struct {
	URL    string `xml:"url,attr"`
	Href   string `xml:"href,attr"`
	Type   string `xml:"type,attr"`
	Medium string `xml:"medium,attr"`
}

func fromAtom(raw *atomDocument, base *url.URL) *Document {
	description := atomTextValue(raw.Subtitle)
	if trimmed(description) == "" {
		description = atomTextValue(raw.Tagline)
	}
	doc := &Document{
		Feed: Feed{
			Title:       PlainText(atomTextValue(raw.Title)),
			Link:        resolveURL(base, atomAlternateLink(raw.Links)),
			Description: feedText(description),
			Language:    trimmed(raw.Lang),
			IconURL:     resolveURL(base, firstNonEmpty(raw.Icon, raw.Logo)),
			Author:      trimmed(raw.Author.Name),
			Type:        "atom",
		},
	}
	doc.Items = make([]Item, 0, len(raw.Entries))
	for i := range raw.Entries {
		doc.Items = append(doc.Items, atomEntryToItem(&raw.Entries[i], raw, base))
	}
	return doc
}

func atomEntryToItem(raw *atomEntry, feed *atomDocument, base *url.URL) Item {
	link := resolveURL(base, atomAlternateLink(raw.Links))

	summaryRaw := atomTextValue(raw.Summary)
	summary := PlainText(summaryRaw)

	content := ""
	for i := range raw.Contents {
		c := &raw.Contents[i]
		// media:content is an item child with a url attribute and is not
		// the entry body. Atom content elements never carry url.
		if trimmed(c.URL) != "" {
			continue
		}
		content = atomContentHTML(*c)
		break
	}
	if content == "" {
		content = trimmed(summaryRaw)
	}

	var image, enclosure string
	for _, thumb := range raw.Thumbnails {
		if u := resolveURL(base, firstNonEmpty(thumb.URL, thumb.Href)); u != "" {
			image = u
			break
		}
	}
	for _, enc := range raw.Enclosures {
		if u := resolveURL(base, firstNonEmpty(enc.URL, enc.Href)); u != "" {
			enclosure = u
			break
		}
	}
	if enclosure == "" {
		enclosure = resolveURL(base, atomLinkByRel(raw.Links, "enclosure"))
	}
	for _, media := range raw.Contents {
		if trimmed(media.URL) == "" {
			continue
		}
		u := resolveURL(base, media.URL)
		if u == "" {
			continue
		}
		if enclosure == "" {
			enclosure = u
		}
		if image == "" && isImageResource(media.Type, media.Medium, media.URL) {
			image = u
		}
	}

	guid := trimmed(raw.ID)
	if guid == "" {
		guid = link
	}

	author := trimmed(raw.Author.Name)
	if author == "" {
		// Atom entries inherit the feed author when they omit one.
		author = trimmed(feed.Author.Name)
	}

	published, _ := ParseDate(raw.Published)
	if published.IsZero() {
		published, _ = ParseDate(raw.Issued)
	}
	if published.IsZero() {
		published, _ = ParseDate(raw.Updated)
	}
	if published.IsZero() {
		published, _ = ParseDate(raw.Modified)
	}

	return Item{
		GUID:         guid,
		Link:         link,
		Title:        PlainText(atomTextValue(raw.Title)),
		Author:       author,
		Summary:      summary,
		Content:      content,
		ImageURL:     image,
		EnclosureURL: enclosure,
		Published:    published,
	}
}

// atomAlternateLink picks the entry or feed link, preferring HTML alternates.
func atomAlternateLink(links []atomLink) string {
	var fallback string
	for _, link := range links {
		rel := strings.ToLower(trimmed(link.Rel))
		if rel == "" {
			rel = "alternate"
		}
		if rel != "alternate" {
			continue
		}
		href := trimmed(link.Href)
		if href == "" {
			continue
		}
		typ := strings.ToLower(trimmed(link.Type))
		switch typ {
		case "", "text/html", "application/xhtml+xml":
			return href
		}
		if fallback == "" {
			fallback = href
		}
	}
	return fallback
}

func atomLinkByRel(links []atomLink, want string) string {
	for _, link := range links {
		if strings.EqualFold(trimmed(link.Rel), want) {
			if href := trimmed(link.Href); href != "" {
				return href
			}
		}
	}
	return ""
}

// atomTextValue returns the markup carried by a title or summary element.
func atomTextValue(t atomText) string {
	typ := strings.ToLower(trimmed(t.Type))
	if typ == "xhtml" || strings.EqualFold(trimmed(t.Mode), "xml") {
		if inner := trimmed(t.Inner); inner != "" {
			return inner
		}
	}
	return t.Body
}

// atomContentHTML keeps HTML content and escapes plain text content.
func atomContentHTML(c atomContent) string {
	typ := strings.ToLower(trimmed(c.Type))
	mode := strings.ToLower(trimmed(c.Mode))
	if typ == "xhtml" || typ == "application/xhtml+xml" || (typ == "" && mode == "xml") {
		if inner := trimmed(c.Inner); inner != "" {
			return inner
		}
		return trimmed(c.Body)
	}
	if mode == "escaped" {
		return trimmed(c.Body)
	}
	switch typ {
	case "html", "text/html":
		return trimmed(c.Body)
	case "", "text", "text/plain":
		return html.EscapeString(trimmed(c.Body))
	default:
		return trimmed(c.Body)
	}
}
