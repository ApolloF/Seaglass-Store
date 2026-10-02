package qbit

import (
	"testing"

	"github.com/ApolloF/Seaglass/internal/torrent"
)

func TestPrefs(t *testing.T) {
	n := torrent.DefaultNetwork()
	n.Interface, n.Address, n.Port = "iftype53_32768", "10.8.0.2", 51413
	n.Proxy, n.ProxyHost, n.ProxyPort, n.ProxyUser, n.ProxyPassword = "socks5", "127.0.0.1", 1080, "me", "secret"
	n.Encryption, n.DownLimit, n.SeedRatio = "require", 2048, 0
	p := prefs(n)
	for k, want := range map[string]any{
		"current_network_interface": "iftype53_32768",
		"current_interface_address": "10.8.0.2",
		"random_port":               false,
		"listen_port":               51413,
		"proxy_type":                "SOCKS5",
		"proxy_auth_enabled":        true,
		"proxy_password":            "secret",
		"proxy_hostname_lookup":     true,
		"encryption":                1,
		"dl_limit":                  2048 * 1024,
		"up_limit":                  0,
		"max_ratio_enabled":         true,
		"max_ratio":                 0.0,
		"web_ui_address":            "127.0.0.1",
		"bypass_local_auth":         false,
	} {
		if p[k] != want {
			t.Errorf("%s = %v (%T), want %v (%T)", k, p[k], p[k], want, want)
		}
	}

	d := prefs(torrent.DefaultNetwork())
	if _, ok := d["listen_port"]; ok || d["random_port"] != true {
		t.Errorf("port 0 should pick a random port: listen_port %v, random_port %v", d["listen_port"], d["random_port"])
	}
	if d["proxy_type"] != "None" || d["proxy_auth_enabled"] != false || d["proxy_hostname_lookup"] != false {
		t.Errorf("no proxy: %v %v %v", d["proxy_type"], d["proxy_auth_enabled"], d["proxy_hostname_lookup"])
	}
	n.SeedRatio = -1
	if p := prefs(n); p["max_ratio_enabled"] != false {
		t.Error("seed ratio -1 should seed forever")
	}
}
