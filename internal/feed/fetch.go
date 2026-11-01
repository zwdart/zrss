package feed

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
)

const (
	defaultUserAgent = "zrss/0.1"
	feedAccept       = "application/rss+xml, application/atom+xml, application/rdf+xml, application/xml;q=0.9, text/xml;q=0.9, text/html;q=0.8, */*;q=0.1"
	maxFeedBytes     = 12 << 20
	maxRedirects     = 10
	fetchTimeout     = 25 * time.Second
)

var defaultHTTPClient = &http.Client{
	Timeout:   fetchTimeout,
	Transport: newDefaultTransport(),
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("feed: stopped after %d redirects", maxRedirects)
		}
		return nil
	},
}

// newDefaultTransport returns a transport that honors the usual proxy
// environment variables and, on Windows, the proxy configured in the system
// settings. Many readers run behind a local proxy without setting environment
// variables, and without this a feed appears to be down.
func newDefaultTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = proxyFromSettings
	return t
}

// Fetcher retrieves feeds over HTTP.
type Fetcher struct {
	Client    *http.Client
	UserAgent string
}

// FetchResult is the outcome of fetching one feed.
type FetchResult struct {
	Doc          *Document
	NotModified  bool
	ETag         string
	LastModified string
	FinalURL     string
	FetchedAt    time.Time
}

// NewFetcher returns a Fetcher with a 25 second timeout and a descriptive
// default user agent.
func NewFetcher() *Fetcher {
	return &Fetcher{Client: defaultHTTPClient, UserAgent: defaultUserAgent}
}

func (f *Fetcher) httpClient() *http.Client {
	if f != nil && f.Client != nil {
		return f.Client
	}
	return defaultHTTPClient
}

func (f *Fetcher) userAgent() string {
	if f != nil {
		if ua := strings.TrimSpace(f.UserAgent); ua != "" {
			return ua
		}
	}
	return defaultUserAgent
}

// Fetch downloads and parses a feed. ETag and Last-Modified are sent as
// conditional request headers when non-empty.
func (f *Fetcher) Fetch(ctx context.Context, feedURL, etag, modified string) (*FetchResult, error) {
	feedURL = strings.TrimSpace(feedURL)
	if feedURL == "" {
		return nil, errors.New("feed: empty URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("feed: create request: %w", err)
	}
	req.Header.Set("User-Agent", f.userAgent())
	req.Header.Set("Accept", feedAccept)
	if etag = strings.TrimSpace(etag); etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if modified = strings.TrimSpace(modified); modified != "" {
		req.Header.Set("If-Modified-Since", modified)
	}

	resp, err := f.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("feed: fetch %s: %w", feedURL, err)
	}
	defer resp.Body.Close()

	result := &FetchResult{
		ETag:         trimmed(resp.Header.Get("ETag")),
		LastModified: trimmed(resp.Header.Get("Last-Modified")),
		FinalURL:     feedURL,
		FetchedAt:    time.Now().UTC(),
	}
	if resp.Request != nil && resp.Request.URL != nil {
		result.FinalURL = resp.Request.URL.String()
	}

	if resp.StatusCode == http.StatusNotModified {
		result.NotModified = true
		return result, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("feed: %s: unexpected status %s", feedURL, resp.Status)
	}

	body, err := readLimited(resp.Body, maxFeedBytes)
	if err != nil {
		return nil, fmt.Errorf("feed: read %s: %w", feedURL, err)
	}
	body = decodeBodyCharset(body, resp.Header.Get("Content-Type"))

	var base *url.URL
	if resp.Request != nil {
		base = resp.Request.URL
	}
	doc, err := Parse(body, base)
	if err != nil {
		return nil, fmt.Errorf("%w (from %s)", err, feedURL)
	}
	result.Doc = doc
	return result, nil
}

// decodeBodyCharset converts a feed to UTF-8 when its Content-Type names a
// charset and the XML declaration does not. encoding/xml only consults the
// declaration, so a GBK feed served with a GBK header would be misread.
func decodeBodyCharset(body []byte, contentType string) []byte {
	if xmlDeclaresEncoding(body) {
		return body
	}
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return body
	}
	label := strings.TrimSpace(params["charset"])
	if label == "" {
		return body
	}
	switch strings.ToLower(label) {
	case "utf-8", "utf8", "us-ascii", "ascii":
		return body
	}
	r, err := charset.NewReaderLabel(label, bytes.NewReader(body))
	if err != nil {
		return body
	}
	decoded, err := io.ReadAll(r)
	if err != nil {
		return body
	}
	return decoded
}

func xmlDeclaresEncoding(body []byte) bool {
	body = bytes.TrimPrefix(body, utf8BOM)
	body = bytes.TrimSpace(body)
	if !bytes.HasPrefix(body, []byte("<?xml")) {
		return false
	}
	end := bytes.Index(body, []byte("?>"))
	if end < 0 {
		return false
	}
	return bytes.Contains(body[:end], []byte("encoding"))
}

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("body exceeds %d MiB limit", limit>>20)
	}
	return body, nil
}
