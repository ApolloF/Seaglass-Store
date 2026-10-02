package safety

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// ErrNoDefender means Microsoft Defender's command-line scanner isn't
// there (another antivirus replaced it, or it's turned off).
var ErrNoDefender = errors.New("Microsoft Defender isn't available")

// defenderExe finds MpCmdRun.exe: the newest platform version first,
// then the copy in Program Files.
func defenderExe() string {
	if platform.ProgramData != "" {
		base := filepath.Join(platform.ProgramData, "Microsoft", "Windows Defender", "Platform")
		if es, err := os.ReadDir(base); err == nil {
			var dirs []string
			for _, e := range es {
				if e.IsDir() {
					dirs = append(dirs, e.Name())
				}
			}
			slices.Reverse(dirs) // versions sort as text well enough: 4.18.26080.4-0 after …3-0
			for _, d := range dirs {
				if p := filepath.Join(base, d, "MpCmdRun.exe"); platform.IsFile(p) {
					return p
				}
			}
		}
	}
	if p := filepath.Join(platform.ProgramFiles, "Windows Defender", "MpCmdRun.exe"); platform.IsFile(p) {
		return p
	}
	return ""
}

// defenderScan scans path (a file or folder) and returns the threats
// found, without removing anything: the person decides.
func defenderScan(ctx context.Context, path string) ([]string, error) {
	exe := defenderExe()
	if exe == "" {
		return nil, ErrNoDefender
	}
	cmd := exec.CommandContext(ctx, exe, "-Scan", "-ScanType", "3", "-File", path, "-DisableRemediation")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	code := 0
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		code = exit.ExitCode()
	case err != nil:
		return nil, err
	}
	threats := parseThreats(out.String())
	switch {
	case code == 2 || len(threats) > 0:
		if len(threats) == 0 {
			threats = []string{"a threat Defender didn't name"}
		}
		return threats, nil
	case code == 0:
		return nil, nil
	}
	return nil, fmt.Errorf("Defender's scan stopped (exit code %d): %s", code, lastLine(out.String()))
}

// parseThreats reads the threat names from MpCmdRun's output, whose lines
// look like "Threat                  : Virus:DOS/EICAR_Test_File".
func parseThreats(out string) []string {
	var ts []string
	for _, l := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(l, ":")
		if ok && strings.TrimSpace(k) == "Threat" {
			if v = strings.TrimSpace(v); v != "" && !slices.Contains(ts, v) {
				ts = append(ts, v)
			}
		}
	}
	return ts
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
