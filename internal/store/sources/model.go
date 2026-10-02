// Package catalog collects untrusted source metadata for the private store experiment.
package sources

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
	"unicode"
)

const Schema = "seaglass.catalog-preview/v1"

type Snapshot struct {
	Schema         string     `json:"schema"`
	Source         Source     `json:"source"`
	CollectedAt    time.Time  `json:"collectedAt"`
	DocumentSHA256 string     `json:"documentSha256"`
	Entries        []Entry    `json:"entries"`
	Groups         []Group    `json:"groups"`
	Documents      []Evidence `json:"documents,omitempty"`
	Warnings       []string   `json:"warnings,omitempty"`
}

type Entry struct {
	ID                 string      `json:"id"`
	SourceID           string      `json:"sourceId"`
	PageURL            string      `json:"pageUrl"`
	DocumentSHA256     string      `json:"documentSha256"`
	RawTitle           string      `json:"rawTitle"`
	Title              string      `json:"title"`
	TitleKey           string      `json:"titleKey"`
	Version            string      `json:"version,omitempty"`
	PublishedAt        *time.Time  `json:"publishedAt,omitempty"`
	UpdatedAt          *time.Time  `json:"updatedAt,omitempty"`
	SizeBytes          int64       `json:"sizeBytes,omitempty"`
	SizeIsMinimum      bool        `json:"sizeIsMinimum,omitempty"`
	SizeClaim          string      `json:"sizeClaim,omitempty"`
	SizeOptionsBytes   []int64     `json:"sizeOptionsBytes,omitempty"`
	InstalledSizeBytes int64       `json:"installedSizeBytes,omitempty"`
	LanguageClaim      string      `json:"languageClaim,omitempty"`
	ReleaseKind        string      `json:"releaseKind"`
	Transports         []Transport `json:"transports"`
	References         []Reference `json:"references"`
	NeedsReview        bool        `json:"needsReview"`
	SummaryOnly        bool        `json:"summaryOnly,omitempty"`
	Warnings           []string    `json:"warnings"`
}

type Evidence struct {
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Bytes   int    `json:"bytes"`
	Entries int    `json:"entries"`
}

type Transport struct {
	Kind           string `json:"kind"`
	URI            string `json:"uri"`
	InfoHash       string `json:"infoHash,omitempty"`
	TorrentName    string `json:"torrentName,omitempty"`
	SizeBytes      int64  `json:"sizeBytes,omitempty"`
	MetadataSHA256 string `json:"metadataSha256,omitempty"`
}

type Reference struct {
	URL    string `json:"url"`
	Label  string `json:"label"`
	Kind   string `json:"kind,omitempty"`
	State  string `json:"state,omitempty"`
	Reason string `json:"reason,omitempty"`
}

func NewSnapshot(source Source, data []byte, entries []Entry) Snapshot {
	sum := sha256.Sum256(data)
	return Snapshot{Schema: Schema, Source: source, CollectedAt: time.Now().UTC(),
		DocumentSHA256: hex.EncodeToString(sum[:]), Entries: entries, Groups: GroupEntries(entries)}
}

func titleKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func entryID(source, page string) string {
	sum := sha256.Sum256([]byte(source + "\x00" + page))
	return hex.EncodeToString(sum[:16])
}

// EntryID is the stable ID of a source article, derived from the source and
// its URL; the discovery index keys records by it.
func EntryID(source, page string) string { return entryID(source, page) }
