package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// HowLongToBeat has no public API, so this talks to it the way its own
// pages do, anonymously: the search page asks /api/search/site/init for a
// short-lived token and posts the search with it, and a game's page carries
// its data as embedded Next.js JSON. Both are read defensively; if the
// format changes the answer falls back to the cache (stale) or to
// "unavailable" with a link, never to guessed times.

const (
	hltbBase = "https://howlongtobeat.com"
	// The token is short-lived; asking again after a few minutes is cheap.
	hltbTokenTTL  = 10 * time.Minute
	hltbSearchTTL = 24 * time.Hour
	hltbPageSize  = 20
)

// hltbHit is one search result as the matcher needs it.
type hltbHit struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	Type          string `json:"type"` // game, dlc, mod, multi, ...
	Year          int    `json:"year"`
	Main          int    `json:"main"` // minutes
	MainExtras    int    `json:"mainExtras"`
	Completionist int    `json:"completionist"`
}

func gameURL(id int) string { return hltbBase + "/game/" + strconv.Itoa(id) }

func searchLink(title string) string {
	return hltbBase + "/?q=" + url.QueryEscape(strings.TrimSpace(title))
}

// minutes converts HowLongToBeat's seconds, rounding to the nearest minute.
func minutes(seconds int) int {
	if seconds <= 0 {
		return 0
	}
	return (seconds + 30) / 60
}

// ---- titles ----

var trailingYear = regexp.MustCompile(`\s*\((\d{4})\)\s*$`)

// splitYear takes an optional trailing "(2019)" off a title.
func splitYear(title string) (string, int) {
	m := trailingYear.FindStringSubmatchIndex(title)
	if m == nil {
		return title, 0
	}
	y, _ := strconv.Atoi(title[m[2]:m[3]])
	return title[:m[0]], y
}

// normTitle is the form two titles are compared in: lower case, letters
// and digits only, "&" read as "and", no trademark signs, no trailing year
// in parentheses. Anything else (a sequel number, a subtitle, "Remastered",
// "Edition") keeps titles apart.
func normTitle(title string) string {
	title, _ = splitYear(title)
	title = strings.NewReplacer("™", "", "®", "", "©", "", "&", " and ").Replace(title)
	var b strings.Builder
	for _, r := range strings.ToLower(title) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// searchTerms are the words sent to HowLongToBeat's search. Punctuation
// stays out of them: its search treats "2:" and "2" as different words.
func searchTerms(title string) []string {
	title, _ = splitYear(title)
	title = strings.NewReplacer("™", "", "®", "", "©", "").Replace(title)
	terms := strings.FieldsFunc(title, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(terms) > 8 {
		terms = terms[:8]
	}
	return terms
}

// matchTitle accepts a result only when its title equals the wanted one
// after normalisation. Several equal titles (a remake with the original's
// name) are told apart by year, and when that doesn't settle it there is
// no match. Mods never match on their own.
func matchTitle(hits []hltbHit, title string, year int) (hltbHit, bool) {
	_, titleYear := splitYear(title)
	if year == 0 {
		year = titleYear
	}
	want := normTitle(title)
	if want == "" {
		return hltbHit{}, false
	}
	var exact []hltbHit
	for _, h := range hits {
		if h.Type != "mod" && normTitle(h.Title) == want {
			exact = append(exact, h)
		}
	}
	switch len(exact) {
	case 0:
		return hltbHit{}, false
	case 1:
		return exact[0], true
	}
	if year == 0 {
		return hltbHit{}, false
	}
	for _, slack := range []int{0, 1} {
		var near []hltbHit
		for _, h := range exact {
			if h.Year != 0 && abs(h.Year-year) <= slack {
				near = append(near, h)
			}
		}
		if len(near) == 1 {
			return near[0], true
		}
		if len(near) > 1 {
			return hltbHit{}, false
		}
	}
	return hltbHit{}, false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// ---- retrieval ----

func (c *Client) hltbHeaders() map[string]string {
	return map[string]string{"Referer": hltbBase + "/", "Accept": "application/json, text/html;q=0.9"}
}

// hltbToken returns the search token, asking for a new one when the old
// one is missing, old, or refused.
func (c *Client) hltbToken(ctx context.Context, renew bool) (string, error) {
	c.tmu.Lock()
	defer c.tmu.Unlock()
	if !renew && c.hltbTok != "" && c.now().Sub(c.hltbTokT) < hltbTokenTTL {
		return c.hltbTok, nil
	}
	u := hltbBase + "/api/search/site/init?t=" + strconv.FormatInt(c.now().UnixMilli(), 10)
	b, err := c.fetch(ctx, request{url: u, header: c.hltbHeaders()})
	if err != nil {
		return "", err
	}
	var a struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(b, &a) != nil || a.Token == "" {
		c.formatFailed(u)
		return "", &errFormat{"no search token"}
	}
	c.hltbTok, c.hltbTokT = a.Token, c.now()
	return a.Token, nil
}

func (c *Client) dropToken() {
	c.tmu.Lock()
	c.hltbTok = ""
	c.tmu.Unlock()
}

// searchBody is what the site's own search page posts.
func searchBody(terms []string) []byte {
	type rng struct {
		Min *int `json:"min"`
		Max *int `json:"max"`
	}
	type yr struct {
		Min string `json:"min"`
		Max string `json:"max"`
	}
	type gameplay struct {
		Perspective string `json:"perspective"`
		Flow        string `json:"flow"`
		Genre       string `json:"genre"`
		Difficulty  string `json:"difficulty"`
	}
	type games struct {
		UserID       int      `json:"userId"`
		Platform     string   `json:"platform"`
		SortCategory string   `json:"sortCategory"`
		RangeCat     string   `json:"rangeCategory"`
		RangeTime    rng      `json:"rangeTime"`
		Gameplay     gameplay `json:"gameplay"`
		RangeYear    yr       `json:"rangeYear"`
		Modifier     string   `json:"modifier"`
	}
	type opts struct {
		Games games `json:"games"`
		Users struct {
			SortCategory string `json:"sortCategory"`
		} `json:"users"`
		Lists struct {
			SortCategory string `json:"sortCategory"`
		} `json:"lists"`
		Filter     string `json:"filter"`
		Sort       int    `json:"sort"`
		Randomizer int    `json:"randomizer"`
	}
	var body struct {
		SearchType    string   `json:"searchType"`
		SearchTerms   []string `json:"searchTerms"`
		SearchPage    int      `json:"searchPage"`
		Size          int      `json:"size"`
		SearchOptions opts     `json:"searchOptions"`
		UseCache      bool     `json:"useCache"`
	}
	body.SearchType, body.SearchTerms, body.SearchPage, body.Size, body.UseCache = "games", terms, 1, hltbPageSize, true
	body.SearchOptions.Games = games{SortCategory: "popular", RangeCat: "main"}
	body.SearchOptions.Users.SortCategory = "postcount"
	body.SearchOptions.Lists.SortCategory = "follows"
	b, _ := json.Marshal(body)
	return b
}

type searchAnswer struct {
	Data *[]struct {
		ID    int    `json:"game_id"`
		Name  string `json:"game_name"`
		Type  string `json:"game_type"`
		Main  int    `json:"comp_main"`
		Plus  int    `json:"comp_plus"`
		Full  int    `json:"comp_100"`
		Year  int    `json:"release_world"`
		Alias string `json:"game_alias"`
	} `json:"data"`
}

func parseSearch(b []byte) ([]hltbHit, error) {
	var a searchAnswer
	if err := json.Unmarshal(b, &a); err != nil || a.Data == nil {
		return nil, &errFormat{"the search results aren't what the site sends"}
	}
	hits := make([]hltbHit, 0, len(*a.Data))
	for _, g := range *a.Data {
		if g.ID <= 0 || strings.TrimSpace(g.Name) == "" {
			continue
		}
		hits = append(hits, hltbHit{ID: g.ID, Title: strings.TrimSpace(g.Name), Type: g.Type, Year: g.Year,
			Main: minutes(g.Main), MainExtras: minutes(g.Plus), Completionist: minutes(g.Full)})
	}
	return hits, nil
}

func (c *Client) fetchSearch(ctx context.Context, terms []string) ([]hltbHit, error) {
	const u = hltbBase + "/api/search/site"
	body := searchBody(terms)
	var b []byte
	for attempt := 0; ; attempt++ {
		tok, err := c.hltbToken(ctx, attempt > 0)
		if err != nil {
			return nil, err
		}
		h := c.hltbHeaders()
		h["Content-Type"] = "application/json"
		h["x-auth-token"] = tok
		b, err = c.fetch(ctx, request{method: "POST", url: u, header: h, body: body, forbiddenOK: attempt == 0})
		var se *statusError
		if attempt == 0 && errors.As(err, &se) && se.Status == 403 {
			// The token expired or was refused: ask for a new one, once.
			c.dropToken()
			continue
		}
		if err != nil {
			return nil, err
		}
		break
	}
	hits, err := parseSearch(b)
	if err != nil {
		c.formatFailed(u)
		return nil, err
	}
	return hits, nil
}

// searchHits runs a search through its own cache. With allowStale an old
// answer is better than none (the person is choosing from a list); the
// automatic matcher passes false so it never matches on old results.
func (c *Client) searchHits(ctx context.Context, title string, allowStale bool) ([]hltbHit, error) {
	terms := searchTerms(title)
	if len(terms) == 0 {
		return nil, errors.New("no title to search for")
	}
	key := strings.ToLower(strings.Join(terms, " "))
	hits, oc, err := resolve(c, ctx, kindSearch, key, hltbSearchTTL, func(ctx context.Context) ([]hltbHit, error) {
		return c.fetchSearch(ctx, terms)
	})
	if oc == failed || (oc == staleCache && !allowStale) {
		return nil, err
	}
	return hits, nil
}

var nextData = regexp.MustCompile(`(?s)<script id="__NEXT_DATA__"[^>]*>(.*?)</script>`)

type gamePage struct {
	Props struct {
		PageProps struct {
			Game struct {
				Data struct {
					Game []struct {
						ID    int    `json:"game_id"`
						Name  string `json:"game_name"`
						Type  string `json:"game_type"`
						Main  int    `json:"comp_main"`
						Plus  int    `json:"comp_plus"`
						Full  int    `json:"comp_100"`
						Alias string `json:"game_alias"`
					} `json:"game"`
				} `json:"data"`
			} `json:"game"`
		} `json:"pageProps"`
	} `json:"props"`
}

func parseGamePage(b []byte, id int) (hltbHit, error) {
	m := nextData.FindSubmatch(b)
	if m == nil {
		return hltbHit{}, &errFormat{"the game page has no embedded data"}
	}
	var p gamePage
	if err := json.Unmarshal(m[1], &p); err != nil {
		return hltbHit{}, &errFormat{"the game page data isn't readable"}
	}
	games := p.Props.PageProps.Game.Data.Game
	if len(games) == 0 || games[0].ID != id || strings.TrimSpace(games[0].Name) == "" {
		return hltbHit{}, &errFormat{"the game page is for another game"}
	}
	g := games[0]
	return hltbHit{ID: g.ID, Title: strings.TrimSpace(g.Name), Type: g.Type,
		Main: minutes(g.Main), MainExtras: minutes(g.Plus), Completionist: minutes(g.Full)}, nil
}

func (c *Client) fetchGame(ctx context.Context, id int) (hltbHit, error) {
	u := gameURL(id)
	b, err := c.fetch(ctx, request{url: u, header: c.hltbHeaders()})
	if err != nil {
		return hltbHit{}, err
	}
	hit, err := parseGamePage(b, id)
	if err != nil {
		c.formatFailed(u)
		return hltbHit{}, err
	}
	return hit, nil
}

// ---- public methods ----

// completionKey is the cache key for a query; "" when there is nothing to
// look up.
func completionKey(q CompletionQuery) string {
	if q.HLTBID > 0 {
		return "id:" + strconv.Itoa(q.HLTBID)
	}
	want := normTitle(q.Title)
	if want == "" {
		return ""
	}
	year := q.Year
	if year == 0 {
		_, year = splitYear(q.Title)
	}
	return "t:" + want + ":" + strconv.Itoa(year)
}

func (h hltbHit) completion(now time.Time, corrected bool) Completion {
	return Completion{HLTBID: h.ID, Title: h.Title, Main: h.Main, MainExtras: h.MainExtras, Completionist: h.Completionist,
		URL: gameURL(h.ID), Corrected: corrected, FetchedAt: now.Unix(), State: StateOK}
}

// noMatch is the answer when HowLongToBeat has nothing we'd stand behind;
// it is kept like any other answer so the site isn't asked again at once.
func noMatch(title string, now time.Time) Completion {
	return Completion{URL: searchLink(title), FetchedAt: now.Unix(), State: StateUnavailable,
		Error: "no confident match on HowLongToBeat"}
}

func (c *Client) fetchCompletion(ctx context.Context, q CompletionQuery) (Completion, error) {
	if q.HLTBID > 0 {
		hit, err := c.fetchGame(ctx, q.HLTBID)
		if err != nil {
			return Completion{}, err
		}
		return hit.completion(c.now(), true), nil
	}
	hits, err := c.searchHits(ctx, q.Title, false)
	if err != nil {
		return Completion{}, err
	}
	hit, ok := matchTitle(hits, q.Title, q.Year)
	if !ok {
		return noMatch(q.Title, c.now()), nil
	}
	return hit.completion(c.now(), false), nil
}

// failedCompletion is the answer when there is nothing cached and the
// lookup failed.
func failedCompletion(q CompletionQuery, err error) Completion {
	r := Completion{HLTBID: q.HLTBID, Corrected: q.HLTBID > 0, State: failState(err), Error: err.Error(), URL: searchLink(q.Title)}
	if q.HLTBID > 0 {
		r.URL = gameURL(q.HLTBID)
	}
	return r
}

// Completion returns HowLongToBeat times (cached seven days). With
// q.HLTBID set it reads that game; otherwise it searches q.Title and
// accepts only a conservative match (no sequel, remaster or DLC).
func (c *Client) Completion(ctx context.Context, q CompletionQuery) Completion {
	key := completionKey(q)
	if key == "" {
		return Completion{State: StateUnavailable, Error: "no title to look up", URL: searchLink("")}
	}
	v, oc, err := resolve(c, ctx, kindCompletion, key, CompletionTTL, func(ctx context.Context) (Completion, error) {
		return c.fetchCompletion(ctx, q)
	})
	switch oc {
	case fromCache, fetched:
		v = settled(v, StateOK)
	case staleCache:
		// A remembered "no match" has no times to call stale.
		if v.HLTBID == 0 {
			v = settled(v, StateOK)
		} else {
			v.State, v.Error = StateStale, err.Error()
		}
	default:
		v = failedCompletion(q, err)
	}
	return v
}

// settled labels a cached or fresh answer: matches are ok, a remembered
// lack of a match stays unavailable.
func settled(v Completion, ok string) Completion {
	if v.HLTBID == 0 {
		v.State = StateUnavailable
		if v.Error == "" {
			v.Error = "no confident match on HowLongToBeat"
		}
		return v
	}
	v.State, v.Error = ok, ""
	return v
}

// CachedCompletion returns cached times without asking HowLongToBeat.
func (c *Client) CachedCompletion(q CompletionQuery) (Completion, bool) {
	key := completionKey(q)
	if key == "" {
		return Completion{}, false
	}
	v, at, ok := cacheGet[Completion](c, kindCompletion, key)
	if !ok {
		return Completion{}, false
	}
	return settled(v, c.cachedState(at, CompletionTTL)), true
}

// Candidates searches HowLongToBeat so the person can choose the match.
func (c *Client) Candidates(ctx context.Context, title string) ([]Candidate, error) {
	hits, err := c.searchHits(ctx, title, true)
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(hits))
	for _, h := range hits {
		out = append(out, Candidate{HLTBID: h.ID, Title: h.Title, Year: h.Year, Type: strings.ToLower(strings.TrimSpace(h.Type)), Main: h.Main,
			MainExtras: h.MainExtras, Completionist: h.Completionist, URL: gameURL(h.ID)})
	}
	return out, nil
}
