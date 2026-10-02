package sources

import "testing"

func TestSelectiveSizeQualifiersAndAlternativesArePreserved(t *testing.T) {
	if size, minimum := parseSize("from 3.4 GB [Selective Download]"); size != 3400000000 || !minimum {
		t.Fatalf("%d %v", size, minimum)
	}
	options := parseSizeOptions("88.5/86.1 GB [Selective Download]")
	if len(options) != 2 || options[0] != 88500000000 || options[1] != 86100000000 {
		t.Fatal(options)
	}
	if size, _ := parseSize("88.5/86.1 GB"); size != 0 {
		t.Fatal("alternative chosen implicitly")
	}
	if size, _ := parseSize("4 GB plus 20 GB"); size != 0 {
		t.Fatal("unrecognized qualifier guessed")
	}
}

func TestSplitDODILanguagesAndUpdateOnlyReleases(t *testing.T) {
	s := sourceFor(t, "dodi")
	data := `<article><h1 class="entry-title">123- Example: Patch from v1.0 to v1.1 [DODI Repack]</h1><div class="entry-content"><p>Language:<br>– Text: English, French<br>– Audio: English<br>Repack Size: 88.5/86.1 GB [Selective Download]</p></div></article>`
	entries, err := Parse(s, s.StartURL, []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	e := entries[0]
	if e.ReleaseKind != "update" || e.LanguageClaim != "Text: English, French; Audio: English" || len(e.SizeOptionsBytes) != 2 || e.SizeBytes != 0 {
		t.Fatalf("%+v", e)
	}
}

func TestReleaseQualifiersAreRemovedWithoutDroppingEditionNames(t *testing.T) {
	for _, tc := range []struct{ raw, title, version string }{
		{"123- Example: Premium Edition (+ Update 8 + All DLCs + MULTi14) (From 66.4 GB) [DODI Repack]", "Example: Premium Edition", ""},
		{"123- Example (From 9.5 GB) – [DODI Repack]", "Example", ""},
		{"123- Example (Deluxe Edition) [DODI Repack]", "Example (Deluxe Edition)", ""},
		{"Example v1.31/v1.32 + HD Mod", "Example", "v1.31/v1.32"},
	} {
		title, version := cleanTitle("dodi", tc.raw)
		if title != tc.title || version != tc.version {
			t.Errorf("%s: %s / %s", tc.raw, title, version)
		}
	}
}
