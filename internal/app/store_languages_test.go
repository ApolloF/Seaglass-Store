package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/store/jobs"
	"github.com/ApolloF/Seaglass/internal/torrent"
)

type languageEngine struct {
	torrent.Engine
	files           []torrent.File
	wanted, skipped []int
	err             error
}

func (e *languageEngine) Files(context.Context, string) ([]torrent.File, error) {
	return e.files, e.err
}
func (e *languageEngine) SetSkipped(_ context.Context, _ string, indexes []int, skip bool) error {
	if e.err != nil {
		return e.err
	}
	if skip {
		e.skipped = append(e.skipped, indexes...)
	} else {
		e.wanted = append(e.wanted, indexes...)
	}
	return nil
}

func TestDefaultEnglishSelectionWaitsForMetadataAndPreservesCore(t *testing.T) {
	c := testStoreCore(t)
	j, err := c.store.jobs.Add(jobs.Job{Title: "Game", Language: "English"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	e := &languageEngine{files: []torrent.File{{Index: 0, Name: "fg-01.bin"}, {Index: 1, Name: "fg-selective-english.bin"}, {Index: 2, Name: "fg-selective-german.bin"}}}
	tor := &torrent.Torrent{Hash: "aa", State: torrent.Metadata}
	if prepared, err := c.store.prepareDefaultLanguage(context.Background(), e, j, tor); prepared || err != nil {
		t.Fatalf("metadata: %v %v", prepared, err)
	}
	tor.State = torrent.Downloading
	if prepared, err := c.store.prepareDefaultLanguage(context.Background(), e, j, tor); !prepared || err != nil {
		t.Fatalf("selection: %v %v", prepared, err)
	}
	if !reflect.DeepEqual(e.wanted, []int{1}) || !reflect.DeepEqual(e.skipped, []int{2}) {
		t.Fatalf("core file priorities changed: %+v", e)
	}
	got, _ := c.store.jobs.Get(j.ID)
	if !got.LanguageApplied {
		t.Fatal("selection not persisted")
	}
	if prepared, err := c.store.prepareDefaultLanguage(context.Background(), e, got, tor); prepared || err != nil {
		t.Fatal("priorities applied twice")
	}
	e.err = errors.New("engine failed")
	got.LanguageApplied = false
	if _, err := c.store.prepareDefaultLanguage(context.Background(), e, got, tor); err == nil {
		t.Fatal("engine error swallowed")
	}
}

func TestMissingEnglishPackKeepsAllFiles(t *testing.T) {
	c := testStoreCore(t)
	j, _ := c.store.jobs.Add(jobs.Job{Title: "Game", Language: "English"}, time.Now())
	e := &languageEngine{files: []torrent.File{{Index: 1, Name: "fg-selective-german.bin"}}}
	prepared, err := c.store.prepareDefaultLanguage(context.Background(), e, j, &torrent.Torrent{Hash: "aa", State: torrent.Downloading})
	if !prepared || err != nil || len(e.wanted)+len(e.skipped) != 0 {
		t.Fatalf("unconfirmed English caused file skipping: %+v %v", e, err)
	}
}

func TestNewLanguagePacksMustCompleteBeforeAnotherSafetyCheck(t *testing.T) {
	c := testStoreCore(t)
	j, _ := c.store.jobs.Add(jobs.Job{Title: "Game", Language: "German"}, time.Now())
	j, _ = c.store.jobs.Update(j.ID, func(j *jobs.Job) bool { j.State = jobs.Paused; j.PendingLanguages = true; return true })
	e := &languageEngine{files: []torrent.File{{Index: 1, Name: "fg-selective-german.bin", Progress: 0}}}
	tor := &torrent.Torrent{Hash: "aa", State: torrent.Complete}
	if err := c.store.verifyPendingLanguages(context.Background(), e, j, tor); err != nil {
		t.Fatal(err)
	}
	got, _ := c.store.jobs.Update(j.ID, func(j *jobs.Job) bool { return j.Sync(tor, time.Now()) })
	if !got.PendingLanguages || got.State == jobs.Downloaded {
		t.Fatal("stale completion allowed an incomplete pack to be installed")
	}
	e.files[0].Progress = 1
	if err := c.store.verifyPendingLanguages(context.Background(), e, got, tor); err != nil {
		t.Fatal(err)
	}
	got, _ = c.store.jobs.Update(j.ID, func(j *jobs.Job) bool { return j.Sync(tor, time.Now()) })
	if got.PendingLanguages || got.State != jobs.Downloaded || got.Safety != nil {
		t.Fatalf("completed pack didn't return to checks: %+v", got)
	}
}

func TestLanguageChoicePersistsAndDisablesAutomaticInstall(t *testing.T) {
	c := testStoreCore(t)
	j, _ := c.store.jobs.Add(jobs.Job{Title: "Game", Language: "English", Languages: []string{"English", "German"}, AutoInstall: true}, time.Now())
	c.store.jobs.Update(j.ID, func(j *jobs.Job) bool { j.State = jobs.Paused; return true })
	s := NewStoreService(c)
	if _, err := s.SetDownloadLanguages(j.ID, "Klingon", "", false); err == nil {
		t.Fatal("unknown language accepted")
	}
	if _, err := s.SetDownloadLanguages(j.ID, "German", "unlisted", false); err == nil {
		t.Fatal("unknown installer language accepted")
	}
	got, err := s.SetDownloadLanguages(j.ID, "German", "", true)
	if err != nil || got.Language != "German" || !got.AskInstaller || got.AutoInstall {
		t.Fatalf("selection: %+v %v", got, err)
	}
}
