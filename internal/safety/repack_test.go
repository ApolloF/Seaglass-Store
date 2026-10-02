package safety

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkippedPayloadStillBlocksWrongChecksum(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "setup.exe"), exe)
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("VirusTotal was contacted with scanning disabled") }))
	defer server.Close()
	o := Options{DisablePayloadScanning: true, BlockDetections: true, SHA256: strings.Repeat("0", 64), VirusTotal: &VirusTotal{Key: "test", Base: server.URL},
		defender: func(context.Context, string) ([]string, error) {
			t.Error("Defender ran with scanning disabled")
			return nil, nil
		}}
	r := check(t, dir, o)
	if !r.PayloadSkipped || r.Verdict != Blocked || r.SHA256 == "" {
		t.Fatalf("integrity checks skipped: %+v", r)
	}
}

func TestToolLabelsRemainWarningsButMixedMalwareBlocks(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "setup.exe"), exe)
	for _, test := range []struct {
		labels  []string
		verdict Verdict
	}{
		{[]string{"HackTool:Win32/Crack", "PUA:Win32/Pack"}, Caution},
		{[]string{"HackTool:Win32/Crack", "Trojan:Win32/Stealer"}, Blocked},
		{[]string{"a threat Defender didn't name"}, Blocked},
		{[]string{"Trojan.HackTool"}, Blocked},
	} {
		r := check(t, dir, Options{BlockDetections: true, defender: func(context.Context, string) ([]string, error) { return test.labels, nil }})
		if r.Verdict != test.verdict {
			t.Errorf("%v: %+v", test.labels, r)
		}
	}
}

func TestVirusTotalRequiresAllDetectionLabelsBeforeToolClassification(t *testing.T) {
	for _, test := range []struct {
		body     string
		toolOnly bool
	}{
		{`{"data":{"attributes":{"last_analysis_stats":{"malicious":3},"last_analysis_results":{"a":{"category":"malicious","result":"HackTool.Crack"},"b":{"category":"malicious","result":"PUA.Tool"},"c":{"category":"malicious","result":"Riskware.Tool"}}}}}`, true},
		{`{"data":{"attributes":{"last_analysis_stats":{"malicious":3},"last_analysis_results":{"a":{"category":"malicious","result":"HackTool.Crack"}}}}}`, false},
		{`{"data":{"attributes":{"last_analysis_stats":{"malicious":3},"last_analysis_results":{"a":{"category":"malicious","result":"HackTool.Crack"},"b":{"category":"malicious","result":"PUA.Tool"},"c":{"category":"malicious","result":"Trojan.Generic"}}}}}`, false},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(test.body)) }))
		v, err := (VirusTotal{Base: server.URL}).lookup(context.Background(), strings.Repeat("a", 64))
		server.Close()
		if err != nil || v.RepackOnly != test.toolOnly {
			t.Fatalf("result: %+v, %v", v, err)
		}
	}
}
