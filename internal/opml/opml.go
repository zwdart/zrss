// Package opml reads and writes subscription lists in OPML, the format
// most feed readers use for import and export.
package opml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Feed is one subscription in a list.
type Feed struct {
	Title       string
	URL         string
	SiteURL     string
	Description string
}

// Folder groups subscriptions under one name and can hold child folders. An
// empty name holds the subscriptions shown at the top level of the sidebar.
type Folder struct {
	Name    string
	Feeds   []Feed
	Folders []Folder
}

// Document is a whole subscription list. Folders and top-level feeds keep
// the order they were written in.
type Document struct {
	Title   string
	Folders []Folder
	Feeds   []Feed
}

// Parse reads an OPML document. Folders keep their nesting, and outlines
// without a feed URL are treated as folders.
func Parse(data []byte) (*Document, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("opml: empty document")
	}
	var raw rawDocument
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	dec.CharsetReader = charsetReader
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("opml: decode: %w", err)
	}
	if !strings.EqualFold(raw.XMLName.Local, "opml") {
		return nil, fmt.Errorf("opml: root element is <%s>, want <opml>", raw.XMLName.Local)
	}

	doc := &Document{Title: firstText(raw.Head.Title, raw.Head.Outline.Title)}
	body := raw.Body.Outlines
	if len(body) == 0 {
		// Some exporters put the list in <head>, which is not standard but
		// harmless to accept.
		body = raw.Head.Outline.Children
	}
	collect(doc, body, nil)
	if doc.Title == "" {
		doc.Title = "Subscriptions"
	}
	return doc, nil
}

// collect walks the outline tree. A named outline with children is a folder;
// nested folders stay nested instead of being flattened into one name.
func collect(doc *Document, outlines []rawOutline, parent *Folder) {
	for _, outline := range outlines {
		if url := outline.feedURL(); url != "" {
			feed := Feed{
				Title:       firstText(outline.Text, outline.Title),
				URL:         url,
				SiteURL:     firstText(outline.HTMLURL, outline.HTMLURLLower),
				Description: strings.TrimSpace(outline.Description),
			}
			if parent == nil {
				doc.Feeds = append(doc.Feeds, feed)
			} else {
				parent.Feeds = append(parent.Feeds, feed)
			}
			// A feed outline normally has no children, but if it does, keep
			// them rather than dropping part of the list.
			if len(outline.Children) > 0 {
				collect(doc, outline.Children, parent)
			}
			continue
		}
		name := firstText(outline.Text, outline.Title)
		if name == "" {
			collect(doc, outline.Children, parent)
			continue
		}
		folder := Folder{Name: name}
		if parent == nil {
			doc.Folders = append(doc.Folders, folder)
			collect(doc, outline.Children, &doc.Folders[len(doc.Folders)-1])
		} else {
			parent.Folders = append(parent.Folders, folder)
			collect(doc, outline.Children, &parent.Folders[len(parent.Folders)-1])
		}
	}
}

// Encode writes a document in OPML 2.0, indented for humans and version
// control.
func Encode(doc *Document) ([]byte, error) {
	title := strings.TrimSpace(doc.Title)
	if title == "" {
		title = "Subscriptions"
	}
	raw := rawDocument{Version: "2.0"}
	raw.Head.Title = title
	for _, f := range doc.Feeds {
		raw.Body.Outlines = append(raw.Body.Outlines, feedOutline(f))
	}
	for _, folder := range doc.Folders {
		appendFolder(&raw.Body.Outlines, folder)
	}

	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(raw); err != nil {
		return nil, fmt.Errorf("opml: encode: %w", err)
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

func feedOutline(f Feed) rawOutline {
	return rawOutline{
		Type:        "rss",
		Text:        strings.TrimSpace(f.Title),
		Title:       strings.TrimSpace(f.Title),
		XMLURL:      strings.TrimSpace(f.URL),
		HTMLURL:     strings.TrimSpace(f.SiteURL),
		Description: strings.TrimSpace(f.Description),
	}
}

// appendFolder writes one folder, recursively. A folder with an empty name is
// a container for top-level feeds, so its contents are written in place.
func appendFolder(outlines *[]rawOutline, folder Folder) {
	name := strings.TrimSpace(folder.Name)
	if name == "" {
		for _, f := range folder.Feeds {
			*outlines = append(*outlines, feedOutline(f))
		}
		for _, child := range folder.Folders {
			appendFolder(outlines, child)
		}
		return
	}
	outline := rawOutline{Text: name, Title: name}
	for _, f := range folder.Feeds {
		outline.Children = append(outline.Children, feedOutline(f))
	}
	for _, child := range folder.Folders {
		appendFolder(&outline.Children, child)
	}
	*outlines = append(*outlines, outline)
}

func firstText(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

type rawDocument struct {
	XMLName xml.Name `xml:"opml"`
	Version string   `xml:"version,attr"`
	Head    rawHead  `xml:"head"`
	Body    rawBody  `xml:"body"`
}

type rawHead struct {
	Title   string     `xml:"title"`
	Outline rawOutline `xml:"outline"`
}

type rawBody struct {
	Outlines []rawOutline `xml:"outline"`
}

type rawOutline struct {
	Text         string       `xml:"text,attr"`
	Title        string       `xml:"title,attr"`
	Type         string       `xml:"type,attr"`
	XMLURL       string       `xml:"xmlUrl,attr"`
	XMLURLLower  string       `xml:"xmlurl,attr"`
	HTMLURL      string       `xml:"htmlUrl,attr"`
	HTMLURLLower string       `xml:"htmlurl,attr"`
	Description  string       `xml:"description,attr"`
	Children     []rawOutline `xml:"outline"`
}

func (o rawOutline) feedURL() string {
	return strings.TrimSpace(firstText(o.XMLURL, o.XMLURLLower))
}

// charsetReader lets Parse accept the encodings deathbed exporters use.
func charsetReader(label string, input io.Reader) (io.Reader, error) {
	label = strings.ToLower(strings.TrimSpace(label))
	switch label {
	case "", "utf-8", "utf8", "us-ascii", "ascii":
		return input, nil
	default:
		return nil, fmt.Errorf("opml: unsupported encoding %q (save as UTF-8)", label)
	}
}
