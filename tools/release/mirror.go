package main

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ApolloF/Seaglass/internal/update"
)

// feedTokenEnv, when set, is the token gh uses to write to the releases-only
// repository. CI's own token can't write to another repository, and this
// one shouldn't be able to read the private source.
const feedTokenEnv = "SEAGLASS_FEED_TOKEN"

// mirror copies the published, signed release tag from this repository
// (where CI builds and signs it) to the public releases-only repository
// installed copies update from (edition.ReleasesRepo). It needs no release
// key: it copies the signature, checking it before the copy and again on
// what the copy serves.
func mirror(tag, to string) error {
	if _, err := versionNumber(tag); err != nil {
		return err
	}
	if strings.EqualFold(to, repo) {
		return errors.New(tag + " is already in " + to)
	}
	var src struct {
		IsDraft bool   `json:"isDraft"`
		Name    string `json:"name"`
	}
	if err := ghJSON(&src, "release", "view", tag, "--repo", repo, "--json", "isDraft,name"); err != nil {
		return err
	}
	if src.IsDraft {
		return errors.New(tag + " is still a draft in " + repo + ": sign and publish it first")
	}
	dir, err := os.MkdirTemp("", "wl-mirror-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	files := mirrored()
	args := []string{"release", "download", tag, "--repo", repo, "--dir", dir}
	for _, n := range files {
		args = append(args, "--pattern", n)
	}
	if err := gh(args...); err != nil {
		return err
	}
	if err := checkSigned(dir, tag, update.ReleaseKeys); err != nil {
		return fmt.Errorf("not copying %s: %w", tag, err)
	}

	notes, err := notesFile(tag, dir)
	if err != nil {
		return err
	}
	title := src.Name
	if title == "" {
		title = "Seaglass Store Edition " + tag
	}
	var dst struct {
		IsDraft bool `json:"isDraft"`
	}
	// A draft first, published once every file is up, so an updater never
	// sees the release half uploaded. A rerun picks up an existing draft.
	if ghJSONTo(&dst, "release", "view", tag, "--repo", to, "--json", "isDraft") != nil {
		if err := ghTo("release", "create", tag, "--repo", to, "--draft", "--title", title, "--notes-file", notes); err != nil {
			return err
		}
	} else if !dst.IsDraft {
		fmt.Println(tag, "is already published in", to)
		return verifyIn(to, tag)
	}
	up := []string{"release", "upload", tag, "--repo", to, "--clobber"}
	for _, n := range files {
		up = append(up, filepath.Join(dir, n))
	}
	if err := ghTo(up...); err != nil {
		return err
	}
	edit := []string{"release", "edit", tag, "--repo", to, "--draft=false"}
	if stable(tag) {
		edit = append(edit, "--latest", "--prerelease=false")
	} else {
		edit = append(edit, "--prerelease")
	}
	if err := ghTo(edit...); err != nil {
		return err
	}
	fmt.Println("Copied", tag, "to", to)
	return verifyIn(to, tag)
}

// mirrored lists the files a release carries: what the updater downloads,
// their .sha256 and the signed list.
func mirrored() []string {
	var out []string
	for _, n := range signed {
		out = append(out, n, n+".sha256")
	}
	return append(out, update.SumsAsset, update.SigAsset)
}

// checkSigned checks the downloaded release in dir the way an installed
// updater does: SHA256SUMS signed for tag with one of keys, and every file
// the updater may download matching it.
func checkSigned(dir, tag string, keys []ed25519.PublicKey) error {
	list, err := os.ReadFile(filepath.Join(dir, update.SumsAsset))
	if err != nil {
		return err
	}
	sig, err := os.ReadFile(filepath.Join(dir, update.SigAsset))
	if err != nil {
		return err
	}
	sums, err := update.VerifySums(keys, tag, list, sig)
	if err != nil {
		return err
	}
	for _, n := range signed {
		got, err := update.FileSHA256(filepath.Join(dir, n))
		if err != nil {
			return err
		}
		if sums[n] != got {
			return fmt.Errorf("%s doesn't match the signed SHA256SUMS", n)
		}
	}
	return nil
}

// notesFile is the release notes the copy shows (and the updater shows in
// Seaglass): docs/releases/<tag>.md, without the notes GitHub generated
// from the private repository's pull requests.
func notesFile(tag, dir string) (string, error) {
	p := filepath.Join("docs", "releases", tag+".md")
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	p = filepath.Join(dir, "notes.md")
	return p, os.WriteFile(p, []byte("Seaglass Store Edition "+tag+"\n"), 0o644)
}

// feedEnv is env with gh's token replaced by token, when there is one.
func feedEnv(env []string, token string) []string {
	if token == "" {
		return env
	}
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if !strings.EqualFold(k, "GH_TOKEN") && !strings.EqualFold(k, "GITHUB_TOKEN") {
			out = append(out, kv)
		}
	}
	return append(out, "GH_TOKEN="+token)
}

// ghTo and ghJSONTo run gh against the releases-only repository.
func ghTo(args ...string) error {
	cmd := exec.Command("gh", args...)
	cmd.Env = feedEnv(os.Environ(), os.Getenv(feedTokenEnv))
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func ghJSONTo(v any, args ...string) error {
	cmd := exec.Command("gh", args...)
	cmd.Env = feedEnv(os.Environ(), os.Getenv(feedTokenEnv))
	return jsonOutput(cmd, v)
}
