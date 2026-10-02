package safety

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// VirusTotal looks files up by their SHA-256 with the person's own API
// key. Only the hash is sent: files are never uploaded.
type VirusTotal struct {
	Key    string
	Base   string // https://www.virustotal.com, unless a test serves its own
	Client *http.Client
}

// vtResult is what VirusTotal's engines said about one file.
type vtResult struct {
	RepackOnly bool
	Known      bool
	Malicious  int
	Suspicious int
	Engines    int
}

var errVTKey = errors.New("VirusTotal refused the API key")

func (v VirusTotal) lookup(ctx context.Context, sha string) (vtResult, error) {
	base := v.Base
	if base == "" {
		base = "https://www.virustotal.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v3/files/"+sha, nil)
	if err != nil {
		return vtResult{}, err
	}
	req.Header.Set("x-apikey", v.Key)
	client := v.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return vtResult{}, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return vtResult{}, nil // never seen
	case http.StatusUnauthorized, http.StatusForbidden:
		return vtResult{}, errVTKey
	case http.StatusTooManyRequests:
		return vtResult{}, errors.New("VirusTotal's limit for the key was reached; try again later")
	default:
		return vtResult{}, fmt.Errorf("VirusTotal answered %s", resp.Status)
	}
	var body struct {
		Data struct {
			Attributes struct {
				Stats   map[string]int `json:"last_analysis_stats"`
				Results map[string]struct {
					Category string `json:"category"`
					Result   string `json:"result"`
				} `json:"last_analysis_results"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&body); err != nil {
		return vtResult{}, fmt.Errorf("VirusTotal's answer: %w", err)
	}
	s := body.Data.Attributes.Stats
	r := vtResult{Known: true, Malicious: s["malicious"], Suspicious: s["suspicious"]}
	for _, n := range s {
		r.Engines += n
	}
	var labels []string
	for _, result := range body.Data.Attributes.Results {
		if result.Category == "malicious" || result.Category == "suspicious" {
			labels = append(labels, result.Result)
		}
	}
	r.RepackOnly = len(labels) >= r.Malicious+r.Suspicious && repackOnly(labels)
	return r, nil
}
