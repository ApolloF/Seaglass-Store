package wishlist

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestAnImportQueuesAtMostItsShareOfSearches(t *testing.T) {
	q := OpenSearches(filepath.Join(t.TempDir(), "searches.json"))
	var keys []string
	for i := 0; i < SearchesPerImport+50; i++ {
		keys = append(keys, fmt.Sprintf("steam:%d", i+1))
	}
	now := time.Now()
	n, err := q.Enqueue(keys, now)
	if err != nil || n != SearchesPerImport || q.Len() != SearchesPerImport {
		t.Fatalf("queued %d (%d) %v", n, q.Len(), err)
	}
	// Importing again counts the queued games without queueing more.
	if n, _ := q.Enqueue(keys[:10], now); n != 10 || q.Len() != SearchesPerImport {
		t.Errorf("again: %d, %d queued", n, q.Len())
	}
}

func TestSearchesAreSpacedAndNotRepeatedWithinADay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "searches.json")
	q := OpenSearches(path)
	now := time.Now()
	if _, err := q.Enqueue([]string{"steam:1", "steam:2"}, now); err != nil {
		t.Fatal(err)
	}
	key, _ := q.Next(now, nil)
	if key != "steam:1" {
		t.Fatalf("first: %q", key)
	}
	if err := q.Done(key, now); err != nil {
		t.Fatal(err)
	}
	if key, wait := q.Next(now.Add(time.Minute), nil); key != "" || wait != SearchSpacing-time.Minute {
		t.Errorf("within the spacing: %q, wait %v", key, wait)
	}
	// The queue and the searches survive a restart.
	q = OpenSearches(path)
	if key, _ := q.Next(now.Add(SearchSpacing), nil); key != "steam:2" {
		t.Errorf("after the spacing: %q", key)
	}
	if n, _ := q.Enqueue([]string{"steam:1"}, now.Add(time.Hour)); n != 0 {
		t.Error("a game searched an hour ago was queued again")
	}
	if n, _ := q.Enqueue([]string{"steam:1"}, now.Add(SearchAgain)); n != 1 {
		t.Error("a game searched a day ago wasn't queued again")
	}
}

func TestDroppingAGameTakesItOffTheQueueWithoutASearch(t *testing.T) {
	q := OpenSearches(filepath.Join(t.TempDir(), "searches.json"))
	now := time.Now()
	q.Enqueue([]string{"steam:1", "steam:2"}, now)
	if err := q.Drop("steam:1", now); err != nil {
		t.Fatal(err)
	}
	if key, wait := q.Next(now, nil); key != "steam:2" || wait != 0 {
		t.Errorf("next: %q %v", key, wait)
	}
	q.Done("steam:2", now)
	if key, wait := q.Next(now.Add(time.Hour), nil); key != "" || wait != 0 {
		t.Errorf("empty queue: %q %v", key, wait)
	}
}

func TestASearchThatMissedASourceGoesToTheBackAfterTheSpacing(t *testing.T) {
	q := OpenSearches(filepath.Join(t.TempDir(), "searches.json"))
	now := time.Now()
	q.Enqueue([]string{"steam:1", "steam:2"}, now)
	if err := q.Later("steam:1", nil, now); err != nil {
		t.Fatal(err)
	}
	if key, wait := q.Next(now, nil); key != "" || wait != SearchSpacing {
		t.Errorf("within the spacing: %q, wait %v", key, wait)
	}
	if key, _ := q.Next(now.Add(SearchSpacing), nil); key != "steam:2" || q.Len() != 2 {
		t.Errorf("after the spacing: %q, %d queued", key, q.Len())
	}
}

func TestTheSourcesThatAnsweredAreRememberedAcrossARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "searches.json")
	q := OpenSearches(path)
	now := time.Now()
	q.Enqueue([]string{"steam:1", "steam:2"}, now)
	q.Later("steam:1", []string{"fitgirl"}, now)
	q.Later("steam:2", []string{"fitgirl"}, now.Add(SearchSpacing))
	q.Later("steam:1", []string{"elamigos"}, now.Add(2*SearchSpacing))
	q = OpenSearches(path)
	if got := q.Answered("steam:1"); len(got) != 2 || got[0] != "fitgirl" || got[1] != "elamigos" {
		t.Errorf("answered after a restart: %v", got)
	}
	// Only a game a source may still be asked about is next.
	needsDODI := func(answered []string) bool { return len(answered) == 1 }
	if key, wait := q.Next(now.Add(3*SearchSpacing), needsDODI); key != "steam:2" || wait != 0 {
		t.Errorf("next ready game: %q %v", key, wait)
	}
	none := func([]string) bool { return false }
	if key, wait := q.Next(now.Add(3*SearchSpacing), none); key != "" || wait != 0 || q.Len() != 2 {
		t.Errorf("no game ready: %q %v, %d queued", key, wait, q.Len())
	}
}

func TestAGameIsGivenUpAfterSearchesThatKeepMissingASource(t *testing.T) {
	q := OpenSearches(filepath.Join(t.TempDir(), "searches.json"))
	now := time.Now()
	q.Enqueue([]string{"steam:1"}, now)
	for i := range SearchMisses {
		if q.Len() != 1 {
			t.Fatalf("given up after %d misses", i)
		}
		q.Later("steam:1", nil, now.Add(time.Duration(i)*SearchSpacing))
	}
	if q.Len() != 0 || len(q.Answered("steam:1")) != 0 {
		t.Errorf("still queued after %d misses", SearchMisses)
	}
	if n, _ := q.Enqueue([]string{"steam:1"}, now.Add(time.Hour)); n != 0 {
		t.Error("a game given up was queued again within a day")
	}
}
