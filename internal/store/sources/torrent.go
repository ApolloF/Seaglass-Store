package sources

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

type bvalue struct {
	dict       map[string]bvalue
	list       []bvalue
	data       []byte
	integer    int64
	isInt      bool
	start, end int
}
type bparser struct {
	data       []byte
	pos, nodes int
}

// TorrentMetadata derives a v1 identity from the original info bytes without contacting any peer or tracker.
func TorrentMetadata(data []byte) (Transport, error) {
	if len(data) == 0 || len(data) > 2<<20 {
		return Transport{}, errors.New("torrent metadata must be between 1 byte and 2 MiB")
	}
	p := bparser{data: data}
	root, err := p.value(0)
	if err != nil || p.pos != len(data) || root.dict == nil {
		return Transport{}, errors.New("invalid torrent bencoding")
	}
	info, ok := root.dict["info"]
	if !ok || info.dict == nil {
		return Transport{}, errors.New("torrent has no info dictionary")
	}
	if _, ok := info.dict["meta version"]; ok {
		return Transport{}, errors.New("v2/hybrid torrent metadata is not supported yet")
	}
	name := string(info.dict["name"].data)
	if !safeComponent(name) {
		return Transport{}, errors.New("torrent name is not a safe file component")
	}
	length, files := info.dict["length"], info.dict["files"]
	var total int64
	if length.isInt && files.list == nil {
		total = length.integer
	} else if !length.isInt && files.list != nil && len(files.list) > 0 && len(files.list) <= 50000 {
		seen := map[string]bool{}
		for _, file := range files.list {
			if _, ok := file.dict["symlink path"]; ok || strings.Contains(string(file.dict["attr"].data), "l") {
				return Transport{}, errors.New("torrent symlinks are not supported")
			}
			size := file.dict["length"]
			path := file.dict["path"].list
			if !size.isInt || size.integer < 0 || len(path) == 0 || len(path) > 64 {
				return Transport{}, errors.New("invalid torrent file")
			}
			parts := make([]string, 0, len(path))
			for _, part := range path {
				if !safeComponent(string(part.data)) {
					return Transport{}, errors.New("unsafe torrent path")
				}
				parts = append(parts, string(part.data))
			}
			key := strings.ToLower(strings.Join(parts, "/"))
			if seen[key] {
				return Transport{}, errors.New("duplicate torrent path")
			}
			seen[key] = true
			if size.integer > 1e15-total {
				return Transport{}, errors.New("torrent size exceeds limit")
			}
			total += size.integer
		}
	} else {
		return Transport{}, errors.New("torrent must have one supported file layout")
	}
	if total <= 0 || total > 1e15 {
		return Transport{}, errors.New("invalid torrent size")
	}
	pieceLength := info.dict["piece length"]
	pieces := info.dict["pieces"].data
	if !pieceLength.isInt || pieceLength.integer <= 0 || pieceLength.integer > 64<<20 || len(pieces)%20 != 0 || int64(len(pieces)/20) != (total+pieceLength.integer-1)/pieceLength.integer {
		return Transport{}, errors.New("torrent piece table does not match its size")
	}
	hash := sha1.Sum(data[info.start:info.end])
	metadataHash := sha256.Sum256(data)
	identity := hex.EncodeToString(hash[:])
	magnet, err := Magnet("magnet:?" + url.Values{"xt": {"urn:btih:" + identity}, "dn": {name}}.Encode())
	if err != nil {
		return Transport{}, err
	}
	magnet.TorrentName, magnet.SizeBytes, magnet.MetadataSHA256 = name, total, hex.EncodeToString(metadataHash[:])
	return magnet, nil
}

func safeComponent(s string) bool {
	if s == "" || len(s) > 255 || !utf8.ValidString(s) || s == "." || s == ".." || strings.HasSuffix(s, ".") || strings.HasSuffix(s, " ") {
		return false
	}
	for _, r := range s {
		if r < 32 || r == 127 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return false
		}
	}
	base := strings.ToUpper(strings.SplitN(s, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" {
		return false
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return false
	}
	return true
}

func (p *bparser) value(depth int) (bvalue, error) {
	p.nodes++
	if depth > 64 || p.nodes > 100000 || p.pos >= len(p.data) {
		return bvalue{}, errors.New("bencode limit exceeded")
	}
	v := bvalue{start: p.pos}
	switch p.data[p.pos] {
	case 'i':
		p.pos++
		start := p.pos
		for p.pos < len(p.data) && p.data[p.pos] != 'e' {
			p.pos++
		}
		if p.pos >= len(p.data) {
			return v, errors.New("unterminated integer")
		}
		raw := string(p.data[start:p.pos])
		p.pos++
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || strconv.FormatInt(n, 10) != raw {
			return v, errors.New("noncanonical integer")
		}
		v.integer, v.isInt = n, true
	case 'd', 'l':
		dict := p.data[p.pos] == 'd'
		p.pos++
		if dict {
			v.dict = map[string]bvalue{}
		} else {
			v.list = []bvalue{}
		}
		previous := ""
		for p.pos < len(p.data) && p.data[p.pos] != 'e' {
			if dict {
				key, err := p.value(depth + 1)
				if err != nil || key.data == nil {
					return v, errors.New("invalid dictionary key")
				}
				name := string(key.data)
				if _, exists := v.dict[name]; exists || (len(v.dict) > 0 && name <= previous) {
					return v, errors.New("unsorted or duplicate dictionary key")
				}
				value, err := p.value(depth + 1)
				if err != nil {
					return v, err
				}
				v.dict[name] = value
				previous = name
			} else {
				child, err := p.value(depth + 1)
				if err != nil {
					return v, err
				}
				v.list = append(v.list, child)
			}
		}
		if p.pos >= len(p.data) {
			return v, errors.New("unterminated collection")
		}
		p.pos++
	default:
		start := p.pos
		for p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			p.pos++
		}
		if p.pos == start || p.pos >= len(p.data) || p.data[p.pos] != ':' {
			return v, errors.New("invalid byte string")
		}
		raw := string(p.data[start:p.pos])
		p.pos++
		n, err := strconv.Atoi(raw)
		if err != nil || strconv.Itoa(n) != raw || n > len(p.data)-p.pos {
			return v, fmt.Errorf("invalid byte length %q", raw)
		}
		v.data = p.data[p.pos : p.pos+n]
		p.pos += n
	}
	v.end = p.pos
	return v, nil
}
