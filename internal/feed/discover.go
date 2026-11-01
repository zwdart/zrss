package feed

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"

	xhtml "golang.org/x/net/html"
)

const maxPageBytes = 4 << 20

const pageAccept = "text/html, application/xhtml+xml, application/xml;q=0.9, */*;q=0.8"

var feedLinkTypes = map[string]bool{
	"application/rss+xml":  true,
	"application/atom+xml": true,
	"application/rdf+xml":  true,
	"application/xml":      true,
	"text/xml":             true,
}

var candidatePaths = []string{
	"/feed",
	"/feed/",
	"/rss",
	"/rss/",
	"/rss.xml",
	"/feed.xml",
	"/atom.xml",
	"/index.xml",
	"/rss2.xml",
	"/feed/rss",
	"/feeds/posts/default",
}

// Discover fetches an HTML page and returns the feed URLs its
// <link rel="alternate"> elements advertise, resolved and unique.
func (f *Fetcher) Discover(ctx context.Context, siteURL string) ([]string, error) {
	siteURL = strings.TrimSpace(siteURL)
	if siteURL == "" {
		return nil, errors.New("feed: empty URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, siteURL, nil)
	if err != nil {
		return nil, fmt.Errorf("feed: create request: %w", err)
	}
	req.Header.Set("User-Agent", f.userAgent())
	req.Header.Set("Accept", pageAccept)

	resp, err := f.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("feed: discover %s: %w", siteURL, err)
	}
	defer resp.Body.Close()

	finalURL := siteURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("feed: discover %s: unexpected status %s", siteURL, resp.Status)
	}

	body, err := readLimited(resp.Body, maxPageBytes)
	if err != nil {
		return nil, fmt.Errorf("feed: read %s: %w", siteURL, err)
	}

	base, err := url.Parse(finalURL)
	if err != nil {
		return nil, fmt.Errorf("feed: discover %s: invalid final URL: %w", siteURL, err)
	}

	// A URL that already points at a feed is its own discovery result.
	if _, err := Parse(body, base); err == nil {
		return []string{finalURL}, nil
	}

	contentType := trimmed(resp.Header.Get("Content-Type"))
	if contentType != "" {
		if mediaType, _, err := mime.ParseMediaType(contentType); err == nil {
			mediaType = strings.ToLower(mediaType)
			if mediaType != "text/html" && mediaType != "application/xhtml+xml" {
				return nil, fmt.Errorf("feed: discover %s: unexpected content type %s", siteURL, mediaType)
			}
		}
	}

	return discoverHTML(body, base), nil
}

// Candidates returns the common feed paths of a site, resolved.
func Candidates(siteURL string) []string {
	siteURL = strings.TrimSpace(siteURL)
	if siteURL == "" {
		return nil
	}
	if !strings.Contains(siteURL, "://") && !strings.HasPrefix(siteURL, "//") {
		siteURL = "https://" + siteURL
	}
	u, err := url.Parse(siteURL)
	if err != nil || u.Host == "" {
		return nil
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		scheme = "https"
	}
	if scheme != "http" && scheme != "https" {
		return nil
	}

	origin := &url.URL{Scheme: scheme, Host: u.Host}
	out := make([]string, 0, len(candidatePaths))
	for _, path := range candidatePaths {
		ref := *origin
		ref.Path = path
		out = append(out, ref.String())
	}
	return out
}

func discoverHTML(data []byte, base *url.URL) []string {
	page, err := xhtml.Parse(bytes.NewReader(data))
	if err != nil {
		return []string{}
	}

	var links, anchors []string
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			switch strings.ToLower(n.Data) {
			case "link":
				if hasRel(n, "alternate") && isFeedLinkType(nodeAttr(n, "type")) {
					if u := resolveURL(base, nodeAttr(n, "href")); u != "" {
						links = appendUnique(links, u)
					}
				}
			case "a":
				href := nodeAttr(n, "href")
				if u := resolveURL(base, href); u != "" && hasFeedPath(u) {
					anchors = appendUnique(anchors, u)
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(page)

	for _, u := range anchors {
		links = appendUnique(links, u)
	}
	if links == nil {
		return []string{}
	}
	return links
}

func hasRel(n *xhtml.Node, want string) bool {
	for _, rel := range strings.Fields(strings.ToLower(nodeAttr(n, "rel"))) {
		if rel == want {
			return true
		}
	}
	return false
}

func isFeedLinkType(raw string) bool {
	mediaType := strings.ToLower(trimmed(strings.SplitN(raw, ";", 2)[0]))
	return feedLinkTypes[mediaType]
}

func hasFeedPath(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	path := strings.ToLower(u.Path)
	return strings.HasSuffix(path, ".rss") ||
		strings.HasSuffix(path, ".xml") ||
		strings.HasSuffix(path, ".atom")
}

func nodeAttr(n *xhtml.Node, name string) string {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, name) {
			return trimmed(attr.Val)
		}
	}
	return ""
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
