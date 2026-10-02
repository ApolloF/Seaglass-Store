package installer

import (
	"errors"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/ApolloF/Seaglass/internal/torrent"
)

var packWords = regexp.MustCompile(`[^a-z]+`)
var packLanguages = map[string]string{
	"english": "English", "eng": "English", "french": "French", "german": "German", "italian": "Italian",
	"spanish": "Spanish", "russian": "Russian", "polish": "Polish", "japanese": "Japanese", "korean": "Korean",
	"portuguese": "Portuguese", "brazilian": "Brazilian Portuguese", "brazilianportuguese": "Brazilian Portuguese", "chinese": "Chinese", "arabic": "Arabic",
	"turkish": "Turkish", "czech": "Czech", "dutch": "Dutch", "ukrainian": "Ukrainian",
}

// PackLanguage recognizes explicitly optional/selective packs only. Core data,
// unnamed archives, and files mentioning more than one language stay selected.
func PackLanguage(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".bin", ".doi", ".pak", ".arc", ".zip", ".7z":
	default:
		return ""
	}
	stem := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	stem = strings.ReplaceAll(stem, "brazilian-portuguese", "brazilianportuguese")
	words := packWords.Split(stem, -1)
	if !slices.Contains(words, "selective") && !slices.Contains(words, "optional") {
		return ""
	}
	found := ""
	for _, word := range words {
		if l := packLanguages[word]; l != "" {
			if found != "" && found != l {
				return ""
			}
			found = l
		}
	}
	return found
}

func PackChoices(files []torrent.File) []string {
	out := []string{}
	for _, f := range files {
		if l := PackLanguage(f.Name); l != "" && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	slices.Sort(out)
	return out
}

// SelectPacks never changes priorities on core or unrecognized files.
func SelectPacks(files []torrent.File, language string) (wanted, skipped []int, err error) {
	choices := PackChoices(files)
	if language != "*" && !slices.ContainsFunc(choices, func(l string) bool { return strings.EqualFold(l, language) }) {
		return nil, nil, errors.New("this torrent has no recognized optional pack for that language")
	}
	for _, f := range files {
		if l := PackLanguage(f.Name); l != "" {
			if language == "*" || strings.EqualFold(l, language) {
				wanted = append(wanted, f.Index)
			} else {
				skipped = append(skipped, f.Index)
			}
		}
	}
	return wanted, skipped, nil
}
