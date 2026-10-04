package main

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ApolloF/Seaglass/internal/update"
)

// writeRelease puts a release's files in a folder, signed for tag with k.
func writeRelease(t *testing.T, k ed25519.PrivateKey, tag string) string {
	dir := t.TempDir()
	sums := map[string]string{}
	for _, n := range signed {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("MZ "+n), 0o644); err != nil {
			t.Fatal(err)
		}
		sums[n], _ = update.FileSHA256(p)
	}
	list := update.FormatSums(sums)
	_ = os.WriteFile(filepath.Join(dir, update.SumsAsset), list, 0o644)
	_ = os.WriteFile(filepath.Join(dir, update.SigAsset), update.EncodeSig(ed25519.Sign(k, update.SignedMessage(tag, list))), 0o644)
	return dir
}

func TestCheckSignedBeforeMirroring(t *testing.T) {
	pub, k, _ := ed25519.GenerateKey(nil)
	keys := []ed25519.PublicKey{pub}
	dir := writeRelease(t, k, "v1.10.0-store.2")
	if err := checkSigned(dir, "v1.10.0-store.2", keys); err != nil {
		t.Fatalf("a signed release was refused: %v", err)
	}
	if err := checkSigned(dir, "v1.10.0-store.3", keys); err == nil {
		t.Error("a signature for another tag was accepted")
	}
	other, _, _ := ed25519.GenerateKey(nil)
	if err := checkSigned(dir, "v1.10.0-store.2", []ed25519.PublicKey{other}); err == nil {
		t.Error("a release signed with another key was accepted")
	}
	_ = os.WriteFile(filepath.Join(dir, update.InstallerAsset), []byte("MZ swapped"), 0o644)
	if err := checkSigned(dir, "v1.10.0-store.2", keys); err == nil {
		t.Error("a file that doesn't match SHA256SUMS was accepted")
	}
}

func TestMirroredFiles(t *testing.T) {
	got := mirrored()
	for _, want := range []string{update.InstallerAsset, update.ExeAsset, update.InstallerAsset + ".sha256", update.SumsAsset, update.SigAsset} {
		if !slices.Contains(got, want) {
			t.Errorf("mirror leaves out %s", want)
		}
	}
}

func TestMirrorRefusesBadTags(t *testing.T) {
	for _, tag := range []string{"../../etc", "main", "v1.2.0 --repo x"} {
		if err := mirror(tag, "ApolloF/Seaglass-Store-Releases"); err == nil {
			t.Errorf("mirror(%q) went ahead", tag)
		}
	}
}

func TestNotesFileFallsBack(t *testing.T) {
	dir := t.TempDir()
	p, err := notesFile("v0.0.1-store.99", dir)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "Seaglass Store Edition v0.0.1-store.99\n" {
		t.Errorf("notes %q", b)
	}
}

func TestFeedEnvSwapsTheToken(t *testing.T) {
	env := []string{"PATH=x", "GH_TOKEN=ci", "GITHUB_TOKEN=ci2", "HOME=y"}
	got := feedEnv(env, "feed")
	if want := []string{"PATH=x", "HOME=y", "GH_TOKEN=feed"}; !slices.Equal(got, want) {
		t.Errorf("feedEnv = %v, want %v", got, want)
	}
	if got := feedEnv(env, ""); !slices.Equal(got, env) {
		t.Errorf("without a feed token the environment changed: %v", got)
	}
}
