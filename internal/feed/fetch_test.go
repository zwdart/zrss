package feed

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
)

const fetchFixture = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
  <title>Fetch Test</title>
  <link>/</link>
  <item><title>One</title><link>posts/1</link><guid>one</guid></item>
</channel></rss>`

type requestInfo struct {
	userAgent, accept, ifNoneMatch, ifModifiedSince string
}

func TestFetchAndConditional(t *testing.T) {
	const lastModified = "Mon, 02 Jan 2006 15:04:05 GMT"

	var mu sync.Mutex
	var seen []requestInfo

	mux := http.NewServeMux()
	mux.HandleFunc("/feed.xml", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, requestInfo{
			userAgent:       r.Header.Get("User-Agent"),
			accept:          r.Header.Get("Accept"),
			ifNoneMatch:     r.Header.Get("If-None-Match"),
			ifModifiedSince: r.Header.Get("If-Modified-Since"),
		})
		mu.Unlock()

		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Last-Modified", lastModified)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		io.WriteString(w, fetchFixture)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := NewFetcher()
	feedURL := srv.URL + "/feed.xml"
	res, err := f.Fetch(context.Background(), feedURL, "", "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.NotModified {
		t.Error("NotModified = true on first fetch")
	}
	if res.Doc == nil || res.Doc.Title != "Fetch Test" {
		t.Fatalf("Doc = %+v", res.Doc)
	}
	if res.Doc.Items[0].Link != srv.URL+"/posts/1" {
		t.Errorf("item link = %q, want resolved against final URL", res.Doc.Items[0].Link)
	}
	if res.ETag != `"v1"` || res.LastModified != lastModified {
		t.Errorf("ETag/LastModified = %q / %q", res.ETag, res.LastModified)
	}
	if res.FinalURL != feedURL {
		t.Errorf("FinalURL = %q, want %q", res.FinalURL, feedURL)
	}
	if res.FetchedAt.IsZero() || time.Since(res.FetchedAt) > time.Minute {
		t.Errorf("FetchedAt = %s", res.FetchedAt)
	}

	mu.Lock()
	first := seen[len(seen)-1]
	mu.Unlock()
	if first.userAgent != defaultUserAgent {
		t.Errorf("User-Agent = %q, want default", first.userAgent)
	}
	if !strings.Contains(first.accept, "application/rss+xml") || !strings.Contains(first.accept, "application/atom+xml") {
		t.Errorf("Accept = %q", first.accept)
	}

	res2, err := f.Fetch(context.Background(), feedURL, res.ETag, res.LastModified)
	if err != nil {
		t.Fatalf("conditional Fetch: %v", err)
	}
	if !res2.NotModified {
		t.Error("NotModified = false on 304")
	}
	if res2.Doc != nil {
		t.Errorf("Doc = %+v, want nil on 304", res2.Doc)
	}
	if res2.FinalURL != feedURL || res2.FetchedAt.IsZero() {
		t.Errorf("metadata = %+v", res2)
	}

	mu.Lock()
	second := seen[len(seen)-1]
	mu.Unlock()
	if second.ifNoneMatch != `"v1"` {
		t.Errorf("If-None-Match = %q", second.ifNoneMatch)
	}
	if second.ifModifiedSince != lastModified {
		t.Errorf("If-Modified-Since = %q", second.ifModifiedSince)
	}

	custom := &Fetcher{Client: srv.Client(), UserAgent: "zrss-test/9"}
	if _, err := custom.Fetch(context.Background(), feedURL, "", ""); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	last := seen[len(seen)-1]
	mu.Unlock()
	if last.userAgent != "zrss-test/9" {
		t.Errorf("User-Agent = %q, want custom", last.userAgent)
	}
}

func TestFetchUsesContentTypeCharsetWithoutDeclaration(t *testing.T) {
	source := `<rss version="2.0"><channel>
  <title>中文标题</title>
  <description>简介</description>
  <item><title>条目</title><link>https://example.com/1</link></item>
</channel></rss>`
	encoded, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(source))
	if err != nil {
		t.Fatalf("encode GBK: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml; charset=gbk")
		_, _ = w.Write(encoded)
	}))
	defer srv.Close()

	res, err := NewFetcher().Fetch(context.Background(), srv.URL, "", "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.Doc.Title != "中文标题" || res.Doc.Items[0].Title != "条目" {
		t.Errorf("decoded titles = %q / %q", res.Doc.Title, res.Doc.Items[0].Title)
	}
}

func TestFetchRedirectUsesFinalURLAsBase(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final/feed.xml", http.StatusFound)
	})
	mux.HandleFunc("/final/feed.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		io.WriteString(w, fetchFixture)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := NewFetcher().Fetch(context.Background(), srv.URL+"/redirect", "", "")
	if err != nil {
		t.Fatal(err)
	}
	wantFinal := srv.URL + "/final/feed.xml"
	if res.FinalURL != wantFinal {
		t.Errorf("FinalURL = %q, want %q", res.FinalURL, wantFinal)
	}
	if res.Doc.Items[0].Link != srv.URL+"/final/posts/1" {
		t.Errorf("item link = %q, want base from final URL", res.Doc.Items[0].Link)
	}
}

func TestFetchStatusAndParseErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/boom", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusInternalServerError)
	})
	mux.HandleFunc("/html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<html><body>not a feed</body></html>")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := NewFetcher()
	if _, err := f.Fetch(context.Background(), srv.URL+"/boom", "", ""); err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("status error = %v, want 500", err)
	}
	if _, err := f.Fetch(context.Background(), srv.URL+"/html", "", ""); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("parse error = %v", err)
	}
	if _, err := f.Fetch(context.Background(), "", "", ""); err == nil {
		t.Error("empty URL: want error")
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	return len(p), nil
}

func TestFetchBodyLimit(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		io.CopyN(w, zeroReader{}, int64(maxFeedBytes)+1)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	_, err := NewFetcher().Fetch(context.Background(), srv.URL+"/big", "", "")
	if err == nil {
		t.Fatal("want body limit error")
	}
	if !strings.Contains(err.Error(), "exceeds") || !strings.Contains(err.Error(), "12 MiB") {
		t.Errorf("error = %v, want clear size limit message", err)
	}
}
