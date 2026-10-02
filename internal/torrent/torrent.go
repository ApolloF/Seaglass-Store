// Package torrent is the experimental store's download engine: what
// Seaglass needs from a BitTorrent client, whichever client runs it.
package torrent

import "context"

// State is where a torrent is, as Seaglass tells them apart.
type State string

const (
	Queued      State = "queued"      // waiting for a free download slot
	Metadata    State = "metadata"    // fetching the torrent's file list from peers
	Downloading State = "downloading" // receiving data (or trying to: stalled counts too)
	Paused      State = "paused"      // stopped before it was complete
	Checking    State = "checking"    // verifying pieces already on disk
	Seeding     State = "seeding"     // complete, uploading to others
	Complete    State = "complete"    // complete and stopped
	Failed      State = "failed"      // the client gave up (disk error, missing files)
)

// Torrent is one torrent's progress.
type Torrent struct {
	Hash      string  `json:"hash"`
	Name      string  `json:"name"`
	Tag       string  `json:"tag"` // Seaglass's own id for it, set when added
	State     State   `json:"state"`
	Size      int64   `json:"size"` // bytes wanted (files skipped don't count)
	Done      int64   `json:"done"` // bytes downloaded and verified
	Progress  float64 `json:"progress"`
	DownSpeed int64   `json:"downSpeed"` // bytes per second
	UpSpeed   int64   `json:"upSpeed"`
	Seeds     int     `json:"seeds"` // connected
	Peers     int     `json:"peers"` // connected
	ETA       int64   `json:"eta"`   // seconds; 0 when unknown
	SavePath  string  `json:"savePath"`
	Ratio     float64 `json:"ratio"`
}

// File is one file in a torrent.
type File struct {
	Index    int     `json:"index"`
	Name     string  `json:"name"` // path inside the torrent, with forward slashes
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"`
	Skip     bool    `json:"skip"` // not downloaded
}

// AddOptions say where a torrent goes and how Seaglass finds it again.
type AddOptions struct {
	SavePath string
	Tag      string // unique; the client's hash isn't known before a magnet's metadata arrives
	Paused   bool
}

// Interface is a network interface the client can bind to.
type Interface struct {
	ID   string `json:"id"`   // the client's id for it
	Name string `json:"name"` // as Windows names it ("Ethernet", "Wi-Fi", a VPN adapter)
}

// Network is how the client connects. The zero value isn't useful; start
// from DefaultNetwork.
type Network struct {
	// Interface binds all traffic to one network interface (a VPN adapter,
	// say), by the client's id for it; "" uses any. While a bound
	// interface is missing, nothing is sent or received at all.
	Interface string `json:"interface"`
	Address   string `json:"address"` // one address of Interface; "" uses all of them
	Port      int    `json:"port"`    // incoming connections; 0 picks a random one at each start
	UPnP      bool   `json:"upnp"`    // forward Port on the router with UPnP / NAT-PMP

	Proxy         string  `json:"proxy"` // none, socks5, http
	ProxyHost     string  `json:"proxyHost"`
	ProxyPort     int     `json:"proxyPort"`
	ProxyUser     string  `json:"proxyUser"`  // "" for none; the password is kept encrypted, apart
	ProxyPeers    bool    `json:"proxyPeers"` // peer connections go through the proxy too, not only trackers
	ProxyPassword string  `json:"-"`          // filled in from the encrypted store when applied
	Encryption    string  `json:"encryption"` // prefer, require, off
	DHT           bool    `json:"dht"`        // find peers without trackers
	PeX           bool    `json:"pex"`        // peers tell each other about peers
	LSD           bool    `json:"lsd"`        // find peers on the local network
	Anonymous     bool    `json:"anonymous"`  // don't tell peers and trackers which client this is
	DownLimit     int     `json:"downLimit"`  // KiB/s; 0 for no limit
	UpLimit       int     `json:"upLimit"`    // KiB/s; 0 for no limit
	MaxActive     int     `json:"maxActive"`  // downloads at once
	SeedRatio     float64 `json:"seedRatio"`  // stop seeding at this upload/download ratio; 0 stops when complete, -1 never
}

// DefaultNetwork is how a new install connects.
func DefaultNetwork() Network {
	return Network{UPnP: true, Proxy: "none", ProxyPort: 1080, Encryption: "prefer", DHT: true, PeX: true, LSD: true, MaxActive: 2, SeedRatio: 1}
}

// Engine is a BitTorrent client Seaglass drives. Torrents are found by
// the tag they were added with.
type Engine interface {
	Add(ctx context.Context, source string, opts AddOptions) error
	List(ctx context.Context) ([]Torrent, error)
	Files(ctx context.Context, hash string) ([]File, error)
	SetSkipped(ctx context.Context, hash string, indexes []int, skip bool) error
	Pause(ctx context.Context, hashes ...string) error
	Resume(ctx context.Context, hashes ...string) error
	Remove(ctx context.Context, deleteFiles bool, hashes ...string) error
	Apply(ctx context.Context, n Network) error
	Interfaces(ctx context.Context) ([]Interface, error)
	Addresses(ctx context.Context, iface string) ([]string, error)
}

// Normalize returns n with out-of-range values replaced, so whatever
// comes from a settings file or the interface is safe to apply.
func (n Network) Normalize() Network {
	d := DefaultNetwork()
	if n.Port < 0 || n.Port > 65535 {
		n.Port = 0
	}
	switch n.Proxy {
	case "none", "socks5", "http":
	default:
		n.Proxy = d.Proxy
	}
	if n.ProxyPort < 1 || n.ProxyPort > 65535 {
		n.ProxyPort = 1080
	}
	if n.Proxy == "none" {
		n.ProxyPeers = false
	}
	switch n.Encryption {
	case "prefer", "require", "off":
	default:
		n.Encryption = d.Encryption
	}
	n.DownLimit, n.UpLimit = max(n.DownLimit, 0), max(n.UpLimit, 0)
	if n.MaxActive < 1 || n.MaxActive > 10 {
		n.MaxActive = d.MaxActive
	}
	if n.SeedRatio < 0 {
		n.SeedRatio = -1
	}
	if n.Interface == "" {
		n.Address = ""
	}
	return n
}
