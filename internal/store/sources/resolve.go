package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/publicsuffix"
)

type ResolutionError struct{ State, Reason string }

func (e *ResolutionError) Error() string { return e.State + ": " + e.Reason }

type Resolver struct{ client *Client }

var hostIDRE = regexp.MustCompile(`^[a-zA-Z0-9]{12}(?:\.html)?$`)
var cdnRE = regexp.MustCompile(`^svr[0-9]+\.file-me\.top$`)

// NewResolver enables only normal file-host forms for torrent metadata, with an ephemeral cookie jar.
func NewResolver(enabled bool) (*Resolver, error) {
	if !enabled {
		return nil, ErrDisabled
	}
	c := newClient(fileHostURL)
	c.allowTorrent = true
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, err
	}
	c.http.Jar = jar
	return &Resolver{client: c}, nil
}
func (r *Resolver) Close() { r.client.Close() }

func fileHostURL(raw string) (*url.URL, error) {
	u, err := httpsURL(raw)
	if err != nil {
		return nil, err
	}
	if cdnRE.MatchString(u.Hostname()) && strings.HasSuffix(strings.ToLower(u.Path), ".torrent") {
		return u, nil
	}
	if u.Hostname() != "file-me.top" && u.Hostname() != "www.up-4ever.net" && u.Hostname() != "up-4ever.net" {
		return nil, errors.New("unreviewed torrent metadata host")
	}
	if u.RawQuery != "" || !hostIDRE.MatchString(strings.TrimPrefix(u.Path, "/")) {
		return nil, errors.New("unsupported file-host path")
	}
	return u, nil
}

func ResolverURL(ref Reference) (string, error) {
	if ref.Kind != "torrent" {
		return "", errors.New("reference is not in a torrent section")
	}
	u, err := referenceURL(ref.URL)
	if err != nil {
		return "", err
	}
	// Only these reviewed hosts get an HTTPS upgrade; never request the original HTTP link.
	if u.Scheme == "http" {
		u.Scheme = "https"
		u.Host = u.Hostname()
	}
	if _, err := fileHostURL(u.String()); err != nil {
		return "", err
	}
	return u.String(), nil
}

// Resolve follows at most two normal form submissions, honoring displayed waits and stopping at CAPTCHA.
func (r *Resolver) Resolve(ctx context.Context, ref Reference) (Transport, error) {
	raw, err := ResolverURL(ref)
	if err != nil {
		return Transport{}, &ResolutionError{State: "manual-required", Reason: err.Error()}
	}
	doc, err := r.client.FetchDocument(ctx, raw)
	if err != nil {
		return Transport{}, resolutionError(err)
	}
	for stage := 0; stage <= 2; stage++ {
		if doc.ContentType != "text/html" {
			return TorrentMetadata(doc.Body)
		}
		root, err := document(doc.Body)
		if err != nil {
			return Transport{}, resolutionError(err)
		}
		if challenge(root) {
			return Transport{}, &ResolutionError{State: "captcha-required", Reason: "file host requires a browser challenge; no solver configured"}
		}
		for _, alert := range find(root, func(n *html.Node) bool { return class(n, "alert-danger") }) {
			message := text(alert, false)
			if strings.Contains(strings.ToLower(message), "wait") {
				return Transport{}, &ResolutionError{State: "rate-limited", Reason: message}
			}
			return Transport{}, &ResolutionError{State: "host-error", Reason: message}
		}
		if stage == 2 {
			break
		}
		action, fields, wait, err := downloadForm(doc.URL, root)
		if err != nil {
			return Transport{}, &ResolutionError{State: "manual-required", Reason: err.Error()}
		}
		if err := waitContext(ctx, wait); err != nil {
			return Transport{}, err
		}
		doc, err = r.client.request(ctx, http.MethodPost, action, fields)
		if err != nil {
			return Transport{}, resolutionError(err)
		}
	}
	return Transport{}, &ResolutionError{State: "manual-required", Reason: "file host did not provide torrent metadata after normal download steps"}
}

func resolutionError(err error) error {
	var status *HTTPError
	if errors.As(err, &status) {
		state := "host-error"
		if status.Status == 403 || status.Status == 401 {
			state = "access-blocked"
		}
		if status.Status == 429 {
			state = "rate-limited"
		}
		return &ResolutionError{State: state, Reason: status.Error()}
	}
	return &ResolutionError{State: "failed", Reason: err.Error()}
}

func challenge(root *html.Node) bool {
	for _, n := range find(root, func(n *html.Node) bool { return n.Type == html.ElementNode }) {
		if class(n, "cf-turnstile") || class(n, "g-recaptcha") || class(n, "h-captcha") || attr(n, "id") == "challenge-form" {
			return true
		}
		if n.Data == "script" && strings.Contains(attr(n, "src"), "/cdn-cgi/challenge-platform/") {
			return true
		}
	}
	return false
}

func downloadForm(base string, root *html.Node) (string, url.Values, time.Duration, error) {
	for _, form := range find(root, func(n *html.Node) bool { return n.Data == "form" }) {
		fields := url.Values{}
		valid := true
		for _, input := range find(form, func(n *html.Node) bool { return n.Data == "input" }) {
			name, value := attr(input, "name"), attr(input, "value")
			if name == "" {
				continue
			}
			if len(value) > 512 {
				valid = false
				break
			}
			switch name {
			case "op", "id", "rand", "fname", "usr_login", "referer", "method_free", "method_premium":
				if fields.Has(name) {
					valid = false
					break
				}
				fields.Set(name, value)
			default:
				// A host asking for another value needs explicit review, not an invented token.
				valid = false
			}
		}
		if fields.Get("op") != "download1" && fields.Get("op") != "download2" {
			continue
		}
		if !valid || !strings.EqualFold(attr(form, "method"), "post") {
			return "", nil, 0, errors.New("unsupported download form fields or method")
		}
		if fields.Get("usr_login") != "" {
			return "", nil, 0, errors.New("authenticated downloads are not supported")
		}
		origin, _ := url.Parse(base)
		id := strings.TrimSuffix(strings.TrimPrefix(origin.Path, "/"), ".html")
		if fields.Get("id") != id {
			return "", nil, 0, errors.New("form ID differs from its source reference")
		}
		action, err := url.Parse(attr(form, "action"))
		if err != nil {
			return "", nil, 0, err
		}
		action = origin.ResolveReference(action)
		if _, err := fileHostURL(action.String()); err != nil || action.Host != origin.Host || action.Path != origin.Path {
			return "", nil, 0, errors.New("form action must stay on its original host and file")
		}
		fields.Set("referer", base)
		fields.Set("method_free", "Free Download")
		fields.Set("method_premium", "")
		wait := time.Duration(0)
		for _, node := range find(form, func(n *html.Node) bool { return attr(n, "id") == "seconds" }) {
			seconds, err := strconv.Atoi(strings.TrimSpace(text(node, false)))
			if err != nil || seconds < 0 || seconds > 60 {
				return "", nil, 0, errors.New("unsupported download countdown")
			}
			if time.Duration(seconds+1)*time.Second > wait {
				wait = time.Duration(seconds+1) * time.Second
			}
		}
		return action.String(), fields, wait, nil
	}
	return "", nil, 0, fmt.Errorf("no supported download form at %s", base)
}
