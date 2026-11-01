package feed

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const discoveryPage = `<!DOCTYPE html>
<html>
<head>
  <title>Site</title>
  <link rel="stylesheet" href="/style.css">
  <link rel="alternate" type="application/rss+xml" title="RSS" href="/feed.rss">
  <link rel="alternate" type="application/atom+xml" href="https://example.org/atom.xml">
  <link rel="alternate" type="application/rss+xml" href="/feed.rss">
  <link rel="alternate" type="application/feed+json" href="/feed.json">
  <link rel="alternate" type="text/xml" href="javascript:alert(1)">
  <link rel="ALTERNATE" type="application/rdf+xml" href="rdf.xml">
  <link rel="alternate" type="application/rss+xml" href="//mirror.example.net/rss">
</head>
<body><a href="/spare.xml">Spare feed</a></body>
</html>`

const anchorPage = `<!DOCTYPE html>
<html><body>
  <a href="/rss.xml">RSS</a>
  <a href="/page2.html">Other page</a>
  <a href="https://other.example/feed.atom">Atom</a>
  <a href="/rss.xml">Duplicate</a>
</body></html>`

func TestDiscover(t *testing.T) {
	var mu sync.Mutex
	var userAgent, accept string
	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		userAgent = r.Header.Get("User-Agent")
		accept = r.Header.Get("Accept")
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, discoveryPage)
	})
	mux.HandleFunc("/anchors", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, anchorPage)
	})
	mux.HandleFunc("/noct", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "")
		io.WriteString(w, `<html><head><link rel="alternate" type="application/atom+xml" href="/atom.xml"></head></html>`)
	})
	mux.HandleFunc("/feed", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		io.WriteString(w, fetchFixture)
	})
	mux.HandleFunc("/plain", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "hello")
	})
	mux.HandleFunc("/empty", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<html><head></head><body>No feeds</body></html>`)
	})
	mux.HandleFunc("/moved", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/sub/page", http.StatusFound)
	})
	mux.HandleFunc("/sub/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<html><head><link rel="alternate" type="application/rss+xml" href="feed.xml"></head></html>`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	f := NewFetcher()

	links, err := f.Discover(context.Background(), srv.URL+"/page")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := []string{
		srv.URL + "/feed.rss",
		"https://example.org/atom.xml",
		srv.URL + "/rdf.xml",
		"http://mirror.example.net/rss",
		srv.URL + "/spare.xml",
	}
	if len(links) != len(want) {
		t.Fatalf("links = %q, want %q", links, want)
	}
	for i := range want {
		if links[i] != want[i] {
			t.Errorf("links[%d] = %q, want %q", i, links[i], want[i])
		}
	}
	mu.Lock()
	gotUserAgent, gotAccept := userAgent, accept
	mu.Unlock()
	if gotUserAgent != defaultUserAgent {
		t.Errorf("User-Agent = %q", gotUserAgent)
	}
	if !strings.Contains(gotAccept, "text/html") {
		t.Errorf("Accept = %q", gotAccept)
	}

	anchors, err := f.Discover(context.Background(), srv.URL+"/anchors")
	if err != nil {
		t.Fatal(err)
	}
	wantAnchors := []string{srv.URL + "/rss.xml", "https://other.example/feed.atom"}
	if len(anchors) != len(wantAnchors) {
		t.Fatalf("anchor links = %q, want %q", anchors, wantAnchors)
	}
	for i := range wantAnchors {
		if anchors[i] != wantAnchors[i] {
			t.Errorf("anchors[%d] = %q, want %q", i, anchors[i], wantAnchors[i])
		}
	}

	noType, err := f.Discover(context.Background(), srv.URL+"/noct")
	if err != nil {
		t.Fatal(err)
	}
	if len(noType) != 1 || noType[0] != srv.URL+"/atom.xml" {
		t.Errorf("no content type links = %q", noType)
	}

	self, err := f.Discover(context.Background(), srv.URL+"/feed")
	if err != nil {
		t.Fatal(err)
	}
	if len(self) != 1 || self[0] != srv.URL+"/feed" {
		t.Errorf("feed page discovery = %q, want itself", self)
	}

	empty, err := f.Discover(context.Background(), srv.URL+"/empty")
	if err != nil {
		t.Fatal(err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("empty discovery = %#v, want empty non-nil slice", empty)
	}

	if _, err := f.Discover(context.Background(), srv.URL+"/plain"); err == nil || !strings.Contains(err.Error(), "content type") {
		t.Errorf("plain text error = %v", err)
	}

	moved, err := f.Discover(context.Background(), srv.URL+"/moved")
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != 1 || moved[0] != srv.URL+"/sub/feed.xml" {
		t.Errorf("redirected discovery = %q, want resolved against final URL", moved)
	}
}

func TestCandidates(t *testing.T) {
	got := Candidates("https://example.com/blog/")
	if len(got) == 0 {
		t.Fatal("Candidates returned nothing")
	}
	seen := make(map[string]bool)
	for _, candidate := range got {
		if !strings.HasPrefix(candidate, "https://example.com/") {
			t.Errorf("candidate = %q, want same origin", candidate)
		}
		if seen[candidate] {
			t.Errorf("duplicate candidate %q", candidate)
		}
		seen[candidate] = true
	}
	for _, want := range []string{"https://example.com/feed", "https://example.com/rss.xml", "https://example.com/atom.xml"} {
		if !seen[want] {
			t.Errorf("missing candidate %q in %q", want, got)
		}
	}

	if got := Candidates("http://example.com"); len(got) == 0 || got[0] != "http://example.com/feed" {
		t.Errorf("Candidates(http) = %q", got)
	}
	if got := Candidates("example.com"); len(got) == 0 || got[0] != "https://example.com/feed" {
		t.Errorf("Candidates(bare host) = %q", got)
	}
	if got := Candidates("ftp://example.com"); got != nil {
		t.Errorf("Candidates(ftp) = %q, want nil", got)
	}
	if got := Candidates(""); got != nil {
		t.Errorf("Candidates(empty) = %q, want nil", got)
	}
}
