package app

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

// TestImageCacheDownloadsOncePerURL checks that the list, the reader and the
// icons share one download instead of starting one each.
func TestImageCacheDownloadsOncePerURL(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatalf("encode PNG: %v", err)
	}
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()

	cache := newImageCache(nil)
	const callers = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			<-start
			src, err := cache.load(srv.URL)
			if err != nil {
				errs <- err
				return
			}
			if src == nil {
				errs <- errors.New("image cache returned no image")
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("downloaded the image %d times, want 1", got)
	}
}
