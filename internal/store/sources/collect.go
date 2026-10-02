package sources

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
)

type CollectOptions struct {
	Pages, Details, Resolve int
	Query                   string
}

func Collect(ctx context.Context, client *Client, start string, opts CollectOptions) (Snapshot, error) {
	if opts.Pages < 1 || opts.Pages > 10 || opts.Details < 0 || opts.Details > 20 || opts.Resolve < 0 || opts.Resolve > 3 {
		return Snapshot{}, errors.New("pages must be 1..10, details 0..20, torrent resolutions 0..3")
	}
	var first []byte
	var entries []Entry
	var evidence []Evidence
	var warnings []string
	seen := map[string]bool{}
	next := start
	for page := 0; page < opts.Pages && next != ""; page++ {
		if seen[next] {
			warnings = append(warnings, "Stopped repeated pagination URL")
			break
		}
		seen[next] = true
		doc, err := client.FetchDocument(ctx, next)
		if err != nil {
			if page == 0 {
				return Snapshot{}, err
			}
			warnings = append(warnings, fmt.Sprintf("Pagination stopped at %s: %v", next, err))
			break
		}
		parsed, err := Parse(client.source, doc.URL, doc.Body)
		if err != nil {
			if page == 0 {
				return Snapshot{}, err
			}
			warnings = append(warnings, fmt.Sprintf("Pagination parse stopped at %s: %v", doc.URL, err))
			break
		}
		if first == nil {
			first = doc.Body
		}
		evidence = append(evidence, documentEvidence(doc, len(parsed)))
		entries = append(entries, parsed...)
		if len(entries) > 5000 {
			return Snapshot{}, errors.New("catalog snapshot exceeds 5000 entries")
		}
		if len(parsed) == 0 {
			break
		}
		if page+1 < opts.Pages {
			next, err = NextPage(client.source, doc.URL, doc.Body)
			if err != nil {
				return Snapshot{}, err
			}
		}
	}
	entries = Search(unique(entries), opts.Query)
	followed := 0
	for i := range entries {
		if followed >= opts.Details {
			break
		}
		if !entries[i].SummaryOnly {
			continue
		}
		followed++
		doc, err := client.FetchDocument(ctx, entries[i].PageURL)
		if err != nil {
			entries[i].Warnings = append(entries[i].Warnings, "Detail fetch failed: "+err.Error())
			continue
		}
		parsed, err := Parse(client.source, doc.URL, doc.Body)
		if err != nil {
			entries[i].Warnings = append(entries[i].Warnings, "Detail parse failed: "+err.Error())
			continue
		}
		evidence = append(evidence, documentEvidence(doc, len(parsed)))
		matched := false
		for _, candidate := range parsed {
			if candidate.PageURL == doc.URL || candidate.ID == entries[i].ID {
				entries[i] = candidate
				matched = true
				break
			}
		}
		if !matched {
			entries[i].Warnings = append(entries[i].Warnings, "Detail page returned no matching release")
		}
	}
	if opts.Resolve > 0 {
		resolver, err := NewResolver(true)
		if err != nil {
			return Snapshot{}, err
		}
		defer resolver.Close()
		resolveEntries(ctx, resolver, entries, opts.Resolve)
	}
	snapshot := NewSnapshot(client.source, first, entries)
	snapshot.Documents, snapshot.Warnings = evidence, warnings
	return snapshot, nil
}

func resolveEntries(ctx context.Context, resolver *Resolver, entries []Entry, budget int) {
	attempts := 0
	seen := map[string]bool{}
	for i := range entries {
		if len(entries[i].Transports) > 0 {
			continue
		}
		// Prefer the observed ordinary File-Me route over hosts that often require CAPTCHA.
		order := make([]int, len(entries[i].References))
		for j := range order {
			order[j] = j
		}
		sort.SliceStable(order, func(a, b int) bool {
			return referencePriority(entries[i].References[order[a]]) < referencePriority(entries[i].References[order[b]])
		})
		for _, j := range order {
			ref := &entries[i].References[j]
			if ref.Kind != "torrent" {
				continue
			}
			if _, err := ResolverURL(*ref); err != nil {
				ref.State, ref.Reason = "manual-required", err.Error()
				continue
			}
			if seen[ref.URL] || attempts >= budget {
				continue
			}
			seen[ref.URL] = true
			attempts++
			transport, err := resolver.Resolve(ctx, *ref)
			if err != nil {
				var resolution *ResolutionError
				ref.State, ref.Reason = "failed", err.Error()
				if errors.As(err, &resolution) {
					ref.State, ref.Reason = resolution.State, resolution.Reason
				}
				// Respect per-IP host cooldowns instead of trying several more files immediately.
				if ref.State == "rate-limited" {
					return
				}
				continue
			}
			entries[i].Transports = append(entries[i].Transports, transport)
			ref.State = "resolved"
			entries[i].Warnings = append(entries[i].Warnings, "Torrent metadata was validated; game identity and payload safety remain unverified")
			break
		}
	}
}

func referencePriority(ref Reference) int {
	if u, err := referenceURL(ref.URL); err == nil && u.Hostname() == "file-me.top" {
		return 0
	}
	return 1
}
func documentEvidence(doc Document, count int) Evidence {
	hash := sha256.Sum256(doc.Body)
	return Evidence{URL: doc.URL, SHA256: hex.EncodeToString(hash[:]), Bytes: len(doc.Body), Entries: count}
}
