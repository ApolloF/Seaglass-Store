package safety

import (
	"regexp"
	"strings"
)

var toolLabel = regexp.MustCompile(`(?i)(^|[.:/!_ -])(hacktool|pua|pup|riskware|not-a-virus)([.:/!_ -]|$)`)
var malwareLabel = regexp.MustCompile(`(?i)(trojan|ransom|stealer|backdoor|worm|spyware|rootkit|miner|virus:)`)

// Tool classifications are warnings, never an allowlist for a file or source.
// Unknown and mixed malware results retain the person's blocking policy.
func repackOnly(labels []string) bool {
	if len(labels) == 0 {
		return false
	}
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if malwareLabel.MatchString(strings.ReplaceAll(strings.ToLower(label), "not-a-virus", "")) || !toolLabel.MatchString(label) {
			return false
		}
	}
	return true
}
