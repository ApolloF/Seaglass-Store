package qbit

import "strings"

// setINI returns an INI file's text with key set to value in section,
// leaving everything else as qBittorrent wrote it. Keys use qBittorrent's
// backslashes, as in "WebUI\Port".
func setINI(text, section, key, value string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	header := "[" + section + "]"
	in, end := false, -1 // end: the line after the section's last non-blank line
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			if in {
				break
			}
			in = t == header
			if in {
				end = i + 1
			}
			continue
		}
		if !in {
			continue
		}
		if k, _, ok := strings.Cut(t, "="); ok && strings.TrimSpace(k) == key {
			lines[i] = key + "=" + value
			return strings.Join(lines, "\n")
		}
		if t != "" {
			end = i + 1
		}
	}
	if end < 0 {
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		return strings.Join(append(lines, header, key+"="+value, ""), "\n")
	}
	lines = append(lines[:end], append([]string{key + "=" + value}, lines[end:]...)...)
	return strings.Join(lines, "\n")
}
