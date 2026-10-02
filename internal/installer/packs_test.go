package installer

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ApolloF/Seaglass/internal/torrent"
)

func TestOnlyRecognizedOptionalLanguagePacksCanBeSkipped(t *testing.T) {
	files := []torrent.File{
		{Index: 0, Name: "Game/fg-selective-english.bin"}, {Index: 1, Name: "Game/fg-selective-german.bin"},
		{Index: 2, Name: "Game/fg-01.bin"}, {Index: 3, Name: "Game/english.pak"},
		{Index: 4, Name: "Game/fg-optional-bonus.bin"}, {Index: 5, Name: "Game/fg-selective-english-german.bin"},
		{Index: 6, Name: "Game/fg-optional-german.exe"},
	}
	wanted, skipped, err := SelectPacks(files, "English")
	if err != nil || !reflect.DeepEqual(wanted, []int{0}) || !reflect.DeepEqual(skipped, []int{1}) {
		t.Fatalf("selection: %v %v %v", wanted, skipped, err)
	}
	if _, _, err := SelectPacks(files, "Klingon"); err == nil {
		t.Fatal("unavailable language accepted")
	}
	wanted, skipped, err = SelectPacks(files, "*")
	if err != nil || len(wanted) != 2 || len(skipped) != 0 {
		t.Fatalf("all packs: %v %v %v", wanted, skipped, err)
	}
}

func TestInteractiveInstallShowsLanguageAndComponentQuestions(t *testing.T) {
	for _, kind := range []Kind{Inno, NSIS, MSI} {
		c, err := command(Request{Kind: kind, File: `C:\download\setup.exe`, Dir: `C:\Games\Game`, Language: "english", Ask: true})
		if err != nil {
			t.Fatal(err)
		}
		for _, silent := range []string{"/VERYSILENT", "/SUPPRESSMSGBOXES", "/S ", "/qn", "/LANG="} {
			if strings.Contains(c.Args, silent) {
				t.Errorf("interactive %s still silent: %s", kind, c.Args)
			}
		}
	}
}
