package sources

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Document struct {
	URL         string
	Body        []byte
	ContentType string
}
type HTTPError struct {
	Status     int
	RetryAfter string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("source returned HTTP %d (Retry-After: %q)", e.Status, e.RetryAfter)
}

type cachedDocument struct {
	data                        []byte
	etag, finalURL, contentType string
}
type Client struct {
	source       Source
	http         *http.Client
	validate     func(string) (*url.URL, error)
	allowTorrent bool
	mu           sync.Mutex
	last         time.Time
	cache        map[string]cachedDocument
}

// NewClient has no proxy, cookies or authenticated browser session. Each dial pins vetted public IPs.
func NewClient(id string, enabled bool) (*Client, error) {
	source, err := PrivateSource(id, enabled)
	if err != nil {
		return nil, err
	}
	c := newClient(source.ValidateURL)
	c.source = source
	return c, nil
}
func newClient(validate func(string) (*url.URL, error)) *Client {
	transport := &http.Transport{DialContext: publicDial, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 10 * time.Second, MaxIdleConnsPerHost: 1}
	client := &http.Client{Transport: transport, Timeout: 20 * time.Second}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("redirect limit exceeded")
		}
		_, err := validate(req.URL.String())
		return err
	}
	return &Client{http: client, validate: validate, cache: map[string]cachedDocument{}}
}
func publicDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != "443" {
		return nil, errors.New("only HTTPS port 443 is allowed")
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, errors.New("host has no addresses")
	}
	for _, address := range addresses {
		if !publicIP(address.IP) {
			return nil, errors.New("DNS returned a non-public address")
		}
	}
	var lastErr error
	dialer := net.Dialer{Timeout: 10 * time.Second}
	for _, address := range addresses {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(address.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}
func (c *Client) Fetch(ctx context.Context, raw string) ([]byte, error) {
	doc, err := c.FetchDocument(ctx, raw)
	return doc.Body, err
}
func (c *Client) FetchDocument(ctx context.Context, raw string) (Document, error) {
	return c.request(ctx, http.MethodGet, raw, nil)
}

// request serializes requests and respects a two-second interval even after failure.
func (c *Client) request(ctx context.Context, method, raw string, form url.Values) (Document, error) {
	if _, err := c.validate(raw); err != nil {
		return Document{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if wait := time.Until(c.last.Add(2 * time.Second)); wait > 0 {
		if err := waitContext(ctx, wait); err != nil {
			return Document{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	c.last = time.Now()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, raw, body)
	if err != nil {
		return Document{}, err
	}
	req.Header.Set("User-Agent", "Seaglass-CatalogLab/0.2 (private metadata experiment)")
	req.Header.Set("Accept", "application/rss+xml, application/xml, text/html;q=0.9, application/x-bittorrent;q=0.8")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Referer", raw)
	}
	cached := c.cache[raw]
	if c.allowTorrent {
		cached = cachedDocument{}
	}
	if method == http.MethodGet && cached.etag != "" {
		req.Header.Set("If-None-Match", cached.etag)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Document{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		if method != http.MethodGet || len(cached.data) == 0 {
			return Document{}, errors.New("304 without a cached document")
		}
		return Document{URL: cached.finalURL, Body: append([]byte(nil), cached.data...), ContentType: cached.contentType}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return Document{}, &HTTPError{Status: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After")}
	}
	finalURL := raw
	if resp.Request != nil {
		finalURL = resp.Request.URL.String()
	}
	media := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	isHTML := media == "text/html" || media == "application/rss+xml" || media == "application/xml" || media == "text/xml"
	u, _ := url.Parse(finalURL)
	isTorrent := c.allowTorrent && strings.HasSuffix(strings.ToLower(u.Path), ".torrent") && (media == "application/x-bittorrent" || media == "application/octet-stream" || media == "application/force-download")
	if !isHTML && !isTorrent {
		return Document{}, errors.New("unexpected response content type")
	}
	if resp.ContentLength > MaxDocumentBytes {
		return Document{}, errors.New("source response exceeds 4 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxDocumentBytes+1))
	if err != nil {
		return Document{}, err
	}
	if len(data) > MaxDocumentBytes {
		return Document{}, errors.New("source response exceeds 4 MiB")
	}
	if method == http.MethodGet && !c.allowTorrent && (len(c.cache) < 25 || len(cached.data) > 0) {
		c.cache[raw] = cachedDocument{data: append([]byte(nil), data...), etag: resp.Header.Get("ETag"), finalURL: finalURL, contentType: media}
	}
	return Document{URL: finalURL, Body: data, ContentType: media}, nil
}
func waitContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func (c *Client) Close() { c.http.CloseIdleConnections() }
