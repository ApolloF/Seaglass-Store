package sources

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(body)), ContentLength: -1}
}

func TestFetcherUsesETagAndReturnsDefensiveCopies(t *testing.T) {
	c, err := NewClient("fitgirl", true)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	calls := 0
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			if r.Header.Get("If-None-Match") != "" {
				t.Fatal("unexpected ETag")
			}
			resp := testResponse(200, "<html>example</html>")
			resp.Header.Set("ETag", `"one"`)
			return resp, nil
		}
		if r.Header.Get("If-None-Match") != `"one"` {
			t.Fatal("missing ETag")
		}
		return testResponse(304, ""), nil
	})
	data, err := c.Fetch(context.Background(), c.source.StartURL)
	if err != nil {
		t.Fatal(err)
	}
	data[0] = 'X'
	c.last = time.Time{}
	got, err := c.Fetch(context.Background(), c.source.StartURL)
	if err != nil || string(got) != "<html>example</html>" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestFetcherRejectsUnsafeRedirectBeforeContactingTarget(t *testing.T) {
	for _, target := range []string{"http://fitgirl-repacks.site/other", "https://evil.example/", "https://127.0.0.1/"} {
		t.Run(target, func(t *testing.T) {
			c, _ := NewClient("fitgirl", true)
			calls := 0
			c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				resp := testResponse(302, "")
				resp.Header.Set("Location", target)
				return resp, nil
			})
			if _, err := c.Fetch(context.Background(), c.source.StartURL); err == nil || calls != 1 {
				t.Fatalf("%v calls=%d", err, calls)
			}
		})
	}
}

func TestFetcherRejectsStatusesBodiesAndContentTypes(t *testing.T) {
	for _, name := range []string{"forbidden", "too-large", "binary", "uncached-304"} {
		t.Run(name, func(t *testing.T) {
			c, _ := NewClient("dodi", true)
			c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				resp := testResponse(200, "<html/>")
				switch name {
				case "forbidden":
					resp.StatusCode = 403
				case "too-large":
					resp.Body = io.NopCloser(strings.NewReader(strings.Repeat("x", MaxDocumentBytes+1)))
				case "binary":
					resp.Header.Set("Content-Type", "application/octet-stream")
				case "uncached-304":
					resp.StatusCode = 304
				}
				return resp, nil
			})
			if _, err := c.Fetch(context.Background(), c.source.StartURL); err == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
}

func TestFetchCancellationInterruptsRateLimitWait(t *testing.T) {
	c, _ := NewClient("dodi", true)
	c.last = time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Fetch(ctx, c.source.StartURL); err != context.Canceled {
		t.Fatal(err)
	}
}
