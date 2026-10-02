package installer

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode"
)

// innoNames are Inno Setup's internal names for its languages (the files
// it ships in Languages\ and the common unofficial ones), by the names
// feeds use. /LANG= takes these.
var innoNames = map[string]string{
	"english": "english", "german": "german", "french": "french", "spanish": "spanish", "italian": "italian",
	"polish": "polish", "russian": "russian", "ukrainian": "ukrainian", "portuguese": "portuguese",
	"brazilianportuguese": "brazilianportuguese", "portuguesebrazil": "brazilianportuguese", "portuguesebrazilian": "brazilianportuguese",
	"japanese": "japanese", "korean": "korean", "turkish": "turkish", "czech": "czech", "dutch": "dutch",
	"hungarian": "hungarian", "finnish": "finnish", "danish": "danish", "norwegian": "norwegian", "swedish": "swedish",
	"hebrew": "hebrew", "arabic": "arabic", "greek": "greek", "slovak": "slovak", "slovenian": "slovenian",
	"bulgarian": "bulgarian", "catalan": "catalan", "icelandic": "icelandic", "armenian": "armenian", "corsican": "corsican",
	"chinesesimplified": "chinesesimplified", "simplifiedchinese": "chinesesimplified", "chinese": "chinesesimplified",
	"chinesetraditional": "chinesetraditional", "traditionalchinese": "chinesetraditional",
	"spanishlatinamerica": "spanish", "latamspanish": "spanish", "thai": "thai", "vietnamese": "vietnamese", "romanian": "romanian",
}

// InnoLanguage is Inno Setup's internal name for a language as a feed
// names it ("Brazilian Portuguese" → "brazilianportuguese"); "" when it
// isn't one Inno knows.
func InnoLanguage(name string) string {
	k := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, name)
	return innoNames[k]
}

// reListedLanguage reads innoextract's --list-languages lines, like
// " - english: English" or "german (German)".
var reListedLanguage = regexp.MustCompile(`^\s*(?:-\s*)?([A-Za-z0-9_]+)\s*(?::|\()\s*([^)]*)\)?\s*$`)

// InnoLanguages asks innoextract (when it's there) which languages a setup
// program offers, as internal name → display name.
func InnoLanguages(ctx context.Context, innoextract, setup string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, innoextract, "--list-languages", "--color", "0", setup)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return parseLanguages(out.String()), nil
}

func parseLanguages(out string) map[string]string {
	langs := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		if m := reListedLanguage.FindStringSubmatch(strings.TrimRight(l, "\r")); m != nil {
			langs[strings.ToLower(m[1])] = strings.TrimSpace(m[2])
		}
	}
	return langs
}
