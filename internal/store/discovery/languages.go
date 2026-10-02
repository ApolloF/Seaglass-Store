package discovery

import (
	"regexp"
	"slices"
	"strings"
)

// languageNames maps what sources write to the names the installer and
// download language choices use (internal/installer). Region variants map
// to their language: the filter asks "has Spanish", not which Spanish.
var languageNames = map[string]string{
	"english": "English", "eng": "English", "en": "English",
	"russian": "Russian", "rus": "Russian", "ru": "Russian",
	"german": "German", "ger": "German", "deu": "German", "de": "German",
	"french": "French", "fre": "French", "fra": "French", "fr": "French",
	"spanish": "Spanish", "spa": "Spanish", "esp": "Spanish", "es": "Spanish",
	"italian": "Italian", "ita": "Italian", "it": "Italian",
	"japanese": "Japanese", "jap": "Japanese", "jpn": "Japanese", "ja": "Japanese",
	"korean": "Korean", "kor": "Korean", "ko": "Korean",
	"chinese": "Chinese", "chi": "Chinese", "chs": "Chinese", "cht": "Chinese", "zh": "Chinese",
	"simplified chinese": "Chinese", "traditional chinese": "Chinese",
	"polish": "Polish", "pol": "Polish", "pl": "Polish",
	"portuguese": "Portuguese", "por": "Portuguese", "pt": "Portuguese",
	"brazilian portuguese": "Brazilian Portuguese", "portuguese brazil": "Brazilian Portuguese", "br": "Brazilian Portuguese", "pt-br": "Brazilian Portuguese",
	"turkish": "Turkish", "tur": "Turkish", "tr": "Turkish",
	"ukrainian": "Ukrainian", "ukr": "Ukrainian", "uk": "Ukrainian",
	"czech": "Czech", "cze": "Czech", "cz": "Czech",
	"dutch": "Dutch", "dut": "Dutch", "nl": "Dutch",
	"arabic": "Arabic", "ara": "Arabic", "ar": "Arabic",
	"swedish": "Swedish", "swe": "Swedish",
	"hungarian": "Hungarian", "hun": "Hungarian",
	"thai": "Thai", "tha": "Thai",
	"norwegian": "Norwegian", "danish": "Danish", "finnish": "Finnish", "greek": "Greek", "romanian": "Romanian", "vietnamese": "Vietnamese", "indonesian": "Indonesian",
}

var (
	languageSplit  = regexp.MustCompile(`[,;/+&|]+|\s+and\s+`)
	languagePrefix = regexp.MustCompile(`(?i)^\s*(text|audio|voice(?:s| language)?|interface(?: language)?|subtitles?)\s*:\s*`)
	regionSuffix   = regexp.MustCompile(`\s*[–—-]\s*`)
	multiToken     = regexp.MustCompile(`(?i)^multi\s*\d*$`)
)

// ParseLanguages reads a source's language claim. It returns the languages
// it recognizes and whether it recognized the whole claim: "MULTi9" or an
// unknown word leaves the list incomplete, and no language is guessed.
func ParseLanguages(claim string) (langs []string, complete bool) {
	complete = strings.TrimSpace(claim) != ""
	for _, part := range languageSplit.Split(claim, -1) {
		part = languagePrefix.ReplaceAllString(part, "")
		part = strings.Trim(strings.TrimSpace(part), ".()[]")
		if part == "" {
			continue
		}
		low := strings.ToLower(part)
		// "Portuguese – Brazil", "Spanish – Latin America".
		if base, region, ok := strings.Cut(regionSuffix.ReplaceAllString(low, "|"), "|"); ok {
			if base == "portuguese" && strings.Contains(region, "brazil") {
				low = "brazilian portuguese"
			} else {
				low = base
			}
		}
		name := languageNames[low]
		if name == "" {
			// Short codes run together ("RUS/ENG" split already; "ENGRUS" not).
			complete = false
			if !multiToken.MatchString(low) && len(languages(low)) > 0 {
				name = languages(low)[0]
			}
		}
		if name != "" && !slices.Contains(langs, name) {
			langs = append(langs, name)
		}
	}
	if langs == nil {
		langs = []string{}
	}
	return langs, complete && len(langs) > 0
}

// languages finds whole language names inside a longer phrase
// ("English (full audio)").
func languages(s string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(s, func(r rune) bool { return !(r >= 'a' && r <= 'z') }) {
		if len(w) > 3 {
			if n := languageNames[w]; n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}
