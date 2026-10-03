package sources

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

var ErrDisabled = errors.New("private catalog sources are disabled")

type Source struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	StartURL string `json:"startUrl"`
}

// PrivateSource requires a separate opt-in for every source request.
func PrivateSource(id string, enabled bool) (Source, error) {
	if !enabled {
		return Source{}, ErrDisabled
	}
	p, ok := Lookup(id)
	if !ok {
		return Source{}, fmt.Errorf("unknown source %q", id)
	}
	return p.Source, nil
}

func (s Source) ValidateURL(raw string) (*url.URL, error) {
	u, err := httpsURL(raw)
	if err != nil {
		return nil, err
	}
	if u.Hostname() != s.Host {
		return nil, errors.New("URL is outside the selected source")
	}
	return u, nil
}

func httpsURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Scheme != "https" || u.User != nil || u.Host == "" || u.Fragment != "" || (u.Port() != "" && u.Port() != "443") {
		return nil, errors.New("expected an HTTPS URL without credentials, fragment or custom port")
	}
	if !publicHost(u.Hostname()) {
		return nil, errors.New("non-public host")
	}
	return u, nil
}

func publicHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if ip := net.ParseIP(host); ip != nil {
		return publicIP(ip)
	}
	return strings.Contains(host, ".") && host != "localhost" && !strings.HasSuffix(host, ".localhost") &&
		!strings.HasSuffix(host, ".local") && !strings.HasSuffix(host, ".internal")
}

func publicIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	if ip.To4() == nil {
		_, global, _ := net.ParseCIDR("2000::/3")
		if !global.Contains(ip) {
			return false
		}
	}
	// Shared address space, link-local transition endpoints and documentation ranges are not origin servers.
	for _, cidr := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "2001::/32", "2001:10::/28", "2001:20::/28", "2002::/16", "64:ff9b::/96"} {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) {
			return false
		}
	}
	return true
}
