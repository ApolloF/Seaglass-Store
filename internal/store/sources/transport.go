package sources

import (
	"encoding/base32"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
)

// Magnet keeps only the identity and display name. A future engine must supply its own network policy.
func Magnet(raw string) (Transport, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "magnet" || u.Host != "" || u.Path != "" || u.User != nil || u.Fragment != "" {
		return Transport{}, errors.New("invalid magnet URI")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return Transport{}, err
	}
	xt := q["xt"]
	if len(xt) != 1 || !strings.HasPrefix(strings.ToLower(xt[0]), "urn:btih:") {
		return Transport{}, errors.New("expected one BitTorrent v1 identity")
	}
	hash := xt[0][9:]
	var decoded []byte
	switch len(hash) {
	case 40:
		decoded, err = hex.DecodeString(hash)
	case 32:
		decoded, err = base32.StdEncoding.DecodeString(strings.ToUpper(hash))
	default:
		err = errors.New("invalid info hash length")
	}
	if err != nil || len(decoded) != 20 {
		return Transport{}, errors.New("invalid info hash")
	}
	hash = hex.EncodeToString(decoded)
	clean := url.Values{"xt": {"urn:btih:" + hash}}
	if name := q.Get("dn"); len(name) <= 512 && name != "" {
		clean.Set("dn", name)
	}
	return Transport{Kind: "magnet", URI: "magnet:?" + clean.Encode(), InfoHash: hash}, nil
}
