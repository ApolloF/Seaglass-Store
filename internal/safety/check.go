package safety

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// Options say what to check and how strictly.
type Options struct {
	Root   string // the download: a folder or one file
	SHA256 string // the main file's, as the feed gives it; "" when it doesn't
	// BlockDetections blocks a download Defender or several VirusTotal
	// engines flag; off, they're warnings.
	DisablePayloadScanning bool
	BlockDetections        bool
	VirusTotal             *VirusTotal // nil when the person gave no key

	// For tests.
	defender func(context.Context, string) ([]string, error)
	signer   func(string) (string, error)
}

// Check runs every check on a download.
func Check(ctx context.Context, o Options) Report {
	if o.defender == nil {
		o.defender = defenderScan
	}
	if o.signer == nil {
		o.signer = platform.Signer
	}
	detect := Warn
	if o.BlockDetections {
		detect = Block
	}
	r := Report{Checked: time.Now().Unix()}
	add := func(check string, l Level, format string, args ...any) {
		r.Findings = append(r.Findings, Finding{check, l, fmt.Sprintf(format, args...)})
	}

	main := MainFile(o.Root)
	if main != "" {
		r.Main, _ = filepath.Rel(o.Root, main)
		if r.Main == "." {
			r.Main = filepath.Base(main)
		}
		sha, err := FileSHA256(main)
		switch {
		case err != nil:
			add("integrity", Warn, "%s couldn't be read: %v", r.Main, err)
		case o.SHA256 != "" && !strings.EqualFold(sha, o.SHA256):
			add("integrity", Block, "%s isn't the file the feed lists: its SHA-256 differs. It was changed or replaced.", r.Main)
		case o.SHA256 != "":
			add("integrity", OK, "%s is the file the feed lists (SHA-256 matches).", r.Main)
		default:
			add("integrity", Info, "The feed gives no checksum, so the installer can't be compared with what it lists. The download itself was verified piece by piece.")
		}
		r.SHA256 = sha
		if name, err := o.signer(main); err == nil {
			add("signature", OK, "Signed by %s.", name)
		} else {
			add("signature", Info, "%s isn't digitally signed, so its publisher can't be confirmed.", r.Main)
		}
	} else {
		add("integrity", Info, "No installer was found in the download; it may be an archive or the game itself.")
	}

	r.Findings = append(r.Findings, checkFiles(o.Root, main)...)

	if o.DisablePayloadScanning {
		r.PayloadSkipped = true
		add("defender", Info, "Payload scanning is disabled. Defender and VirusTotal were skipped; integrity and file checks still ran.")
	} else {
		switch threats, err := o.defender(ctx, o.Root); {
		case errors.Is(err, ErrNoDefender):
			add("defender", Warn, "Microsoft Defender isn't available here (another antivirus may have replaced it), so it didn't scan the files.")
		case err != nil:
			add("defender", Warn, "Microsoft Defender's scan didn't finish: %v", err)
		case len(threats) > 0:
			level := detect
			if repackOnly(threats) {
				level = Warn
			}
			add("defender", level, "Microsoft Defender found: %s. HackTool and PUA labels can occur in repacks; review them before installing.", strings.Join(threats, ", "))
		default:
			add("defender", OK, "Microsoft Defender found nothing.")
		}

		if o.VirusTotal != nil && r.SHA256 != "" {
			switch v, err := o.VirusTotal.lookup(ctx, r.SHA256); {
			case err != nil:
				add("virustotal", Warn, "VirusTotal couldn't be asked: %v", err)
			case !v.Known:
				add("virustotal", Info, "VirusTotal hasn't seen %s before.", r.Main)
			case v.Malicious >= 3 && v.RepackOnly:
				add("virustotal", Warn, "%d of %d engines flag repack-related tools. This does not establish a false positive; review the report.", v.Malicious, v.Engines)
			case v.Malicious >= 3:
				add("virustotal", detect, "%d of %d VirusTotal engines call %s malicious.", v.Malicious, v.Engines, r.Main)
			case v.Malicious > 0 || v.Suspicious > 0:
				add("virustotal", Warn, "%d of %d VirusTotal engines flag %s (often a false alarm with few engines).", v.Malicious+v.Suspicious, v.Engines, r.Main)
			default:
				add("virustotal", OK, "None of %d VirusTotal engines flag %s.", v.Engines, r.Main)
			}
		}
	}
	r.Verdict = verdict(r.Findings)
	return r
}
