package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIRequiresOptInAndRejectsInvalidOrNetworkOfflineOptions(t *testing.T) {
	for _, args := range [][]string{
		{"--source", "fitgirl"},
		{"--private-sources", "--source", "dodi", "--pages", "0"},
		{"--private-sources", "--source", "dodi", "--resolve-torrents", "4"},
		{"--private-sources", "--source", "dodi", "--input", "unused", "--pages", "2"},
		{"--private-sources", "--source", "dodi", "--input", "unused", "--search", "game"},
		{"--private-sources", "--source", "dodi", "--search", "game", "--url", "https://dodi-repacks.site/"},
	} {
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}

func TestCLIOfflineParserProducesJSONAndEmptySearch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.html")
	markup := `<article><h1 class="entry-title">Example [DODI Repack]</h1><div class="entry-content">Repack Size: 2 GB</div></article>`
	if err := os.WriteFile(path, []byte(markup), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run([]string{"--private-sources", "--source", "dodi", "--input", path}, &output); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"title": "Example"`)) {
		t.Fatal(output.String())
	}
	output.Reset()
	if err := run([]string{"--private-sources", "--source", "dodi", "--input", path, "--query", "missing"}, &output); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"entries": []`)) {
		t.Fatal(output.String())
	}
}

func TestManualTorrentNeedsExactlyOneRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.html")
	markup := `<body class="search search-no-results"><h1 class="page-title">Nothing Found</h1></body>`
	if err := os.WriteFile(path, []byte(markup), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--private-sources", "--source", "dodi", "--input", path, "--torrent-file", "unused"}, &bytes.Buffer{}); err == nil {
		t.Fatal("unbound manual torrent accepted")
	}
}
