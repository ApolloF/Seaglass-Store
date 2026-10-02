package qbit

import "github.com/ApolloF/Seaglass/internal/torrent"

// prefs turns Seaglass's network settings into qBittorrent preferences
// (app/setPreferences). The Web UI stays on localhost whatever happens.
func prefs(n torrent.Network) map[string]any {
	p := map[string]any{
		"current_network_interface": n.Interface,
		"current_interface_address": n.Address,
		"random_port":               n.Port == 0,
		"upnp":                      n.UPnP,
		"proxy_type":                proxyTypes[n.Proxy],
		"proxy_ip":                  n.ProxyHost,
		"proxy_port":                n.ProxyPort,
		"proxy_auth_enabled":        n.Proxy != "none" && n.ProxyUser != "",
		"proxy_username":            n.ProxyUser,
		"proxy_password":            n.ProxyPassword,
		"proxy_peer_connections":    n.ProxyPeers,
		// Names are looked up through the proxy too, so DNS doesn't leak past it.
		"proxy_hostname_lookup": n.Proxy != "none",
		"proxy_bittorrent":      n.Proxy != "none",
		"encryption":            encryption[n.Encryption],
		"dht":                   n.DHT,
		"pex":                   n.PeX,
		"lsd":                   n.LSD,
		"anonymous_mode":        n.Anonymous,
		"dl_limit":              n.DownLimit * 1024,
		"up_limit":              n.UpLimit * 1024,
		"max_active_downloads":  n.MaxActive,
		"max_active_torrents":   n.MaxActive + 3, // seeding torrents need slots too
		"max_ratio_enabled":     n.SeedRatio >= 0,
		"max_ratio":             max(n.SeedRatio, 0),
		"max_ratio_act":         0, // stop the torrent (it stays, for its files)
		"web_ui_address":        "127.0.0.1",
		"web_ui_upnp":           false,
		"bypass_local_auth":     false,
	}
	if n.Port > 0 {
		p["listen_port"] = n.Port
	}
	return p
}

var proxyTypes = map[string]string{"none": "None", "socks5": "SOCKS5", "http": "HTTP"}

var encryption = map[string]int{"prefer": 0, "require": 1, "off": 2}
