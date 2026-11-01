// Package feed parses and fetches RSS and Atom feeds.
package feed

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
)

// Feed describes the channel-level metadata of a feed.
type Feed struct {
	Title       string
	Link        string
	Description string
	Language    string
	IconURL     string
	Author      string
	Type        string // "rss", "rdf" or "atom"
}

// Item is a single entry in a feed.
type Item struct {
	GUID         string
	Link         string
	Title        string
	Author       string
	Summary      string
	Content      string
	ImageURL     string
	EnclosureURL string
	Published    time.Time
}

// Document is a parsed feed and its items.
type Document struct {
	Feed
	Items []Item
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Parse reads RSS 0.91/2.0, RSS 1.0 (RDF) and Atom 1.0/0.3. data is the raw
// document; base resolves relative links when non-nil.
func Parse(data []byte, base *url.URL) (*Document, error) {
	data = bytes.TrimPrefix(data, utf8BOM)
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("feed: empty document")
	}
	root, err := rootElement(data)
	if err != nil {
		return nil, err
	}

	switch root {
	case "rss":
		var raw rssDocument
		if err := decodeXML(data, &raw); err != nil {
			return nil, err
		}
		return fromRSS(&raw, base, "rss"), nil
	case "rdf":
		var raw rdfDocument
		if err := decodeXML(data, &raw); err != nil {
			return nil, err
		}
		return fromRSS(raw.document(), base, "rdf"), nil
	case "feed":
		var raw atomDocument
		if err := decodeXML(data, &raw); err != nil {
			return nil, err
		}
		return fromAtom(&raw, base), nil
	default:
		return nil, fmt.Errorf("feed: unsupported document root <%s>", root)
	}
}

func newDecoder(data []byte) *xml.Decoder {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = charset.NewReaderLabel
	// Feeds in the wild are frequently not strictly well-formed.
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	return dec
}

func rootElement(data []byte) (string, error) {
	dec := newDecoder(data)
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", fmt.Errorf("feed: find root element: %w", err)
		}
		if start, ok := tok.(xml.StartElement); ok {
			return strings.ToLower(start.Name.Local), nil
		}
	}
}

func decodeXML(data []byte, v any) error {
	if err := newDecoder(data).Decode(v); err != nil {
		return fmt.Errorf("feed: decode: %w", err)
	}
	return nil
}

func trimmed(s string) string { return strings.TrimSpace(s) }

// feedText turns a channel description or subtitle into readable text.
func feedText(s string) string { return PlainText(s) }

// resolveURL resolves raw against base and only keeps absolute HTTP(S) URLs.
// Relative values are returned unchanged when no base is available.
func resolveURL(base *url.URL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.NewReplacer("\n", "", "\r", "", "\t", "").Replace(raw)

	if strings.HasPrefix(raw, "//") {
		scheme := "https"
		if base != nil && (base.Scheme == "http" || base.Scheme == "https") {
			scheme = base.Scheme
		}
		raw = scheme + ":" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if u.Scheme == "" {
		// Without a base there is nothing to resolve against; keep the
		// reference exactly as the feed supplied it.
		return raw
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	if u.Host == "" {
		return ""
	}
	return u.String()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func isImageResource(typ, medium, rawURL string) bool {
	typ = strings.ToLower(trimmed(typ))
	medium = strings.ToLower(trimmed(medium))
	if medium == "image" || strings.HasPrefix(typ, "image/") {
		return true
	}
	if typ != "" || medium != "" {
		return false
	}
	// Media RSS often omits type/medium; fall back to the URL extension.
	path := rawURL
	if u, err := url.Parse(rawURL); err == nil {
		path = u.Path
	}
	path = strings.ToLower(path)
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".avif", ".svg", ".bmp"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}
