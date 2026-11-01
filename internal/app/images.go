package app

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

// imageCache keeps the pictures articles and feed icons use. The Markdown
// view asks for one on a goroutine of its own; the sidebar asks for an icon
// while it draws, so that path starts the download and shows the icon when it
// arrives.
type imageCache struct {
	mu      sync.Mutex
	entries map[string]*imgEntry
	post    func(func())
	client  *http.Client
}

type imgEntry struct {
	done    bool
	loading bool
	ready   chan struct{} // closed when the download finishes
	src     ui.ImageSource
	err     error
}

func newImageCache(post func(func())) *imageCache {
	return &imageCache{
		entries: map[string]*imgEntry{},
		post:    post,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

// cached returns an image that has loaded, and starts loading one that has
// not. It never blocks the frame.
func (c *imageCache) cached(raw string) ui.ImageSource {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	c.mu.Lock()
	e := c.entries[raw]
	if e == nil {
		e = &imgEntry{}
		c.entries[raw] = e
	}
	if e.done {
		src := e.src
		c.mu.Unlock()
		return src
	}
	start := !e.loading
	if start {
		e.loading = true
		e.ready = make(chan struct{})
	}
	c.mu.Unlock()
	if start {
		go c.download(raw, e)
	}
	return nil
}

// load fetches an image and returns it, for the Markdown view, which calls it
// off the main thread.
func (c *imageCache) load(raw string) (ui.ImageSource, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("app: empty image URL")
	}
	c.mu.Lock()
	e := c.entries[raw]
	if e == nil {
		e = &imgEntry{}
		c.entries[raw] = e
	}
	if e.done {
		src, err := e.src, e.err
		c.mu.Unlock()
		return src, err
	}
	if e.loading {
		ready := e.ready
		c.mu.Unlock()
		<-ready
		c.mu.Lock()
		src, err := e.src, e.err
		c.mu.Unlock()
		return src, err
	}
	e.loading = true
	e.ready = make(chan struct{})
	c.mu.Unlock()
	c.download(raw, e)
	c.mu.Lock()
	src, err := e.src, e.err
	c.mu.Unlock()
	return src, err
}

// download fetches one image and wakes whoever waits for it.
func (c *imageCache) download(raw string, e *imgEntry) {
	src, err := fetchImage(c.client, raw)
	c.mu.Lock()
	e.src, e.err, e.loading, e.done = src, err, false, true
	close(e.ready)
	c.mu.Unlock()
	if err == nil && c.post != nil {
		c.post(func() {})
	}
}

func fetchImage(client *http.Client, raw string) (ui.ImageSource, error) {
	data, err := readImage(client, raw)
	if err != nil {
		return nil, err
	}
	if looksLikeSVG(data, raw) {
		return ui.ParseSVG(data)
	}
	return ui.DecodeBitmap(data)
}

func readImage(client *http.Client, raw string) ([]byte, error) {
	if strings.HasPrefix(raw, "data:") {
		return decodeDataURL(raw)
	}
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "zrss/0.1")
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,image/gif,image/svg+xml,*/*;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("app: image: %s", resp.Status)
	}
	const limit = 8 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("app: image is larger than %d MiB", limit>>20)
	}
	return data, nil
}

func decodeDataURL(raw string) ([]byte, error) {
	comma := strings.IndexByte(raw, ',')
	if comma < 0 {
		return nil, fmt.Errorf("app: malformed data URL")
	}
	meta, body := raw[:comma], raw[comma+1:]
	if strings.HasSuffix(strings.ToLower(meta), ";base64") {
		return base64.StdEncoding.DecodeString(body)
	}
	return []byte(body), nil
}

func looksLikeSVG(data []byte, raw string) bool {
	if strings.Contains(strings.ToLower(raw), "svg") {
		return true
	}
	head := strings.TrimSpace(string(data))
	if len(head) > 256 {
		head = head[:256]
	}
	return strings.HasPrefix(strings.ToLower(head), "<svg") ||
		strings.HasPrefix(strings.ToLower(head), "<?xml") && strings.Contains(strings.ToLower(head), "<svg")
}
