package app

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/installer"
	"github.com/ApolloF/Seaglass/internal/safety"
	"github.com/ApolloF/Seaglass/internal/store/jobs"
	"github.com/ApolloF/Seaglass/internal/torrent"
)

type InstallerLanguage struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type DownloadLanguageOptions struct {
	Game      []string            `json:"game"`
	Installer []InstallerLanguage `json:"installer"`
	Torrent   bool                `json:"torrent"`
	Note      string              `json:"note"`
}

func languageEditable(j jobs.Job) bool {
	return j.State == jobs.Queued || j.State == jobs.Downloading || j.State == jobs.Paused || j.State == jobs.Downloaded || j.State == jobs.Blocked || (j.State == jobs.Failed && j.Safety != nil)
}

func (s *StoreService) languageOptions(ctx context.Context, j jobs.Job) (DownloadLanguageOptions, []torrent.File, error) {
	o := DownloadLanguageOptions{Game: slices.Clone(j.Languages), Installer: []InstallerLanguage{}}
	if o.Game == nil {
		o.Game = []string{}
	}
	if j.Hash == "" && j.Safety == nil {
		o.Note = "Torrent languages become available when metadata arrives. Resume the download if it is paused."
	}
	var files []torrent.File
	if j.Hash != "" {
		eng, err := s.c.store.engine(ctx)
		if err != nil {
			return o, nil, err
		}
		files, err = eng.Files(ctx, j.Hash)
		if err != nil {
			return o, nil, err
		}
		if packs := installer.PackChoices(files); len(packs) > 0 {
			o.Game, o.Torrent = packs, true
		}
	}
	if j.Safety != nil {
		main := safety.MainFile(root(j))
		kind, setup := installer.Detect(root(j), main, j.Installer)
		if kind == installer.Inno {
			tool, err := exec.LookPath("innoextract.exe")
			if err != nil {
				o.Note = "Installer languages cannot be listed without innoextract. Choose ‘Ask in the installer’ to select them there."
			} else {
				languages, err := installer.InnoLanguages(ctx, tool, setup)
				if err != nil {
					o.Note = "Installer language inspection failed: " + err.Error()
				} else {
					for id, name := range languages {
						o.Installer = append(o.Installer, InstallerLanguage{id, name})
					}
					slices.SortFunc(o.Installer, func(a, b InstallerLanguage) int { return strings.Compare(a.Name, b.Name) })
				}
			}
		}
	}
	return o, files, nil
}

func (s *StoreService) DownloadLanguages(id string) (DownloadLanguageOptions, error) {
	if err := s.on(); err != nil {
		return DownloadLanguageOptions{}, err
	}
	j, ok := s.c.store.jobs.Get(id)
	if !ok {
		return DownloadLanguageOptions{}, jobs.ErrNotFound
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, 75*time.Second)
	defer cancel()
	o, _, err := s.languageOptions(ctx, j)
	return o, err
}

// SetDownloadLanguages changes known optional packs and keeps manual installation
// selected, so newly requested files can finish and be checked first.
func (s *StoreService) SetDownloadLanguages(id, language, setupLanguage string, ask bool) (jobs.Job, error) {
	if err := s.on(); err != nil {
		return jobs.Job{}, err
	}
	j, ok := s.c.store.jobs.Get(id)
	if !ok {
		return j, jobs.ErrNotFound
	}
	if !languageEditable(j) {
		return j, errors.New("wait until this download is ready or pause it to change languages")
	}
	if !s.c.store.pipe.claim(id) {
		return j, errors.New("this download is busy")
	}
	defer s.c.store.pipe.release(id)
	ctx, cancel := context.WithTimeout(s.c.ctx, 75*time.Second)
	defer cancel()
	o, files, err := s.languageOptions(ctx, j)
	if err != nil {
		return j, err
	}
	if language != "*" && !slices.ContainsFunc(o.Game, func(l string) bool { return strings.EqualFold(l, language) }) && !(language == "English" && len(o.Game) == 0) {
		return j, errors.New("choose a language listed for this download")
	}
	if setupLanguage != "" && !slices.ContainsFunc(o.Installer, func(l InstallerLanguage) bool { return l.ID == setupLanguage }) {
		return j, errors.New("choose a language listed by this installer")
	}
	incomplete := false
	if o.Torrent {
		eng, err := s.c.store.engine(ctx)
		if err != nil {
			return j, err
		}
		wanted, skipped, err := installer.SelectPacks(files, language)
		if err != nil {
			return j, err
		}
		if err := applyPackPriorities(ctx, eng, j.Hash, wanted, skipped); err != nil {
			return j, err
		}
		for _, f := range files {
			if slices.Contains(wanted, f.Index) && f.Progress < 1 {
				incomplete = true
			}
		}
		if incomplete {
			if err := eng.Pause(ctx, j.Hash); err != nil {
				return j, err
			}
		}

	}
	j, err = s.c.store.jobs.Update(id, func(j *jobs.Job) bool {
		j.Language, j.SetupLanguage, j.AskInstaller, j.AutoInstall = language, setupLanguage, ask, false
		j.LanguageApplied = o.Torrent
		if incomplete {
			j.State, j.Safety, j.Finished, j.Seeding = jobs.Paused, nil, 0, false
			j.PendingLanguages = true
		}
		return true
	})
	if err != nil {
		return j, err
	}
	s.c.emit(EventStoreJobs, s.c.store.jobs.All())
	s.c.store.wake()
	return j, nil
}

func applyPackPriorities(ctx context.Context, eng torrent.Engine, hash string, wanted, skipped []int) error {
	if len(wanted) > 0 {
		if err := eng.SetSkipped(ctx, hash, wanted, false); err != nil {
			return err
		}
	}
	if len(skipped) > 0 {
		if err := eng.SetSkipped(ctx, hash, skipped, true); err != nil {
			return err
		}
	}
	return nil
}

// prepareDefaultLanguage waits for metadata before selecting explicit optional packs.
func (st *storeState) prepareDefaultLanguage(ctx context.Context, eng torrent.Engine, j jobs.Job, t *torrent.Torrent) (bool, error) {
	if j.Language == "" || j.LanguageApplied || (j.State != jobs.Queued && j.State != jobs.Downloading) || t.State == torrent.Metadata {
		return false, nil
	}
	files, err := eng.Files(ctx, t.Hash)
	if err != nil {
		return false, err
	}
	if len(files) == 0 {
		return false, nil
	}
	choices := installer.PackChoices(files)
	if j.Language == "*" || slices.ContainsFunc(choices, func(l string) bool { return strings.EqualFold(l, j.Language) }) {
		wanted, skipped, err := installer.SelectPacks(files, j.Language)
		if err != nil {
			return false, err
		}
		if err := applyPackPriorities(ctx, eng, t.Hash, wanted, skipped); err != nil {
			return false, err
		}
	}
	_, err = st.jobs.Update(j.ID, func(j *jobs.Job) bool { j.LanguageApplied = true; return true })
	return true, err
}

func (st *storeState) verifyPendingLanguages(ctx context.Context, eng torrent.Engine, j jobs.Job, t *torrent.Torrent) error {
	if !j.PendingLanguages || (t.State != torrent.Complete && t.State != torrent.Seeding) {
		return nil
	}
	files, err := eng.Files(ctx, t.Hash)
	if err != nil {
		return err
	}
	wanted, _, err := installer.SelectPacks(files, j.Language)
	if err != nil {
		return err
	}
	ready := len(wanted) > 0
	for _, file := range files {
		if slices.Contains(wanted, file.Index) && file.Progress < 1 {
			ready = false
		}
	}
	if ready {
		_, err = st.jobs.Update(j.ID, func(j *jobs.Job) bool { j.PendingLanguages = false; return true })
	}
	return err
}
