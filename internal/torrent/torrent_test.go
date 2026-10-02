package torrent

import "testing"

// Values from a hand-edited settings file or a bad request fall back to
// something safe.
func TestNormalize(t *testing.T) {
	n := Network{Port: 70000, Proxy: "socks9", ProxyPort: 0, ProxyPeers: true, Encryption: "maybe",
		DownLimit: -5, UpLimit: -1, MaxActive: 99, SeedRatio: -3, Address: "10.0.0.2"}.Normalize()
	want := Network{Port: 0, Proxy: "none", ProxyPort: 1080, ProxyPeers: false, Encryption: "prefer",
		DownLimit: 0, UpLimit: 0, MaxActive: 2, SeedRatio: -1, Address: ""}
	if n != want {
		t.Errorf("normalized\n got %+v\nwant %+v", n, want)
	}
	if d := DefaultNetwork(); d.Normalize() != d {
		t.Errorf("the defaults change when normalized: %+v", d.Normalize())
	}
}
