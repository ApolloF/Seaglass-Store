package enrich

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Steam answers, in the one place that knows their shape. Reviews and
// summaries come from the store's public appreviews endpoint rather than
// api.steampowered.com's IUserReviewsService: without a key that interface
// ignores the filter, language and day-range parameters and leaves out
// persona names, so it can't serve the "recent" ordering or a language.

const reviewsPerPage = 20

func reviewsURL(appID int) string {
	return "https://steamcommunity.com/app/" + strconv.Itoa(appID) + "/reviews/"
}

// ---- most-played chart ----

type chartAnswer struct {
	Response struct {
		Ranks []struct {
			Rank  int `json:"rank"`
			AppID int `json:"appid"`
		} `json:"ranks"`
	} `json:"response"`
}

func (c *Client) fetchChart(ctx context.Context) (Popularity, error) {
	const u = "https://api.steampowered.com/ISteamChartsService/GetMostPlayedGames/v1/"
	b, err := c.fetch(ctx, request{url: u})
	if err != nil {
		return Popularity{}, err
	}
	p, err := parseChart(b)
	if err != nil {
		c.formatFailed(u)
		return Popularity{}, err
	}
	p.FetchedAt = c.now().Unix()
	return p, nil
}

func parseChart(b []byte) (Popularity, error) {
	var a chartAnswer
	if err := json.Unmarshal(b, &a); err != nil {
		return Popularity{}, &errFormat{"the chart isn't JSON"}
	}
	ranks := map[int]int{}
	for i, r := range a.Response.Ranks {
		if r.AppID <= 0 {
			continue
		}
		rank := r.Rank
		if rank <= 0 {
			rank = i + 1
		}
		if _, dup := ranks[r.AppID]; !dup {
			ranks[r.AppID] = rank
		}
	}
	// An empty chart is Steam breaking, not nobody playing anything; it
	// must not replace a good cached chart.
	if len(ranks) == 0 {
		return Popularity{}, &errFormat{"the chart is empty"}
	}
	return Popularity{Ranks: ranks, State: StateOK}, nil
}

// ---- review summary ----

type querySummary struct {
	Desc     string `json:"review_score_desc"`
	Positive int    `json:"total_positive"`
	Negative int    `json:"total_negative"`
	Total    int    `json:"total_reviews"`
}

type reviewsAnswer struct {
	Success      int          `json:"success"`
	QuerySummary querySummary `json:"query_summary"`
	Cursor       string       `json:"cursor"`
	Reviews      []struct {
		ID     string `json:"recommendationid"`
		Author struct {
			SteamID         string `json:"steamid"`
			PersonaName     string `json:"personaname"`
			PlaytimeForever int    `json:"playtime_forever"`
			PlaytimeAt      int    `json:"playtime_at_review"`
		} `json:"author"`
		Language string `json:"language"`
		Text     string `json:"review"`
		Created  int64  `json:"timestamp_created"`
		VotedUp  bool   `json:"voted_up"`
		VotesUp  int    `json:"votes_up"`
		Funny    int    `json:"votes_funny"`
	} `json:"reviews"`
}

// histogramAnswer has a daily tally for the last 30 days, which is what
// Steam's own "recent reviews" score is made from.
type histogramAnswer struct {
	Success int `json:"success"`
	Results struct {
		Recent []struct {
			Up   int `json:"recommendations_up"`
			Down int `json:"recommendations_down"`
		} `json:"recent"`
	} `json:"results"`
}

func percent(pos, total int) int {
	if total <= 0 {
		return 0
	}
	return pos * 100 / total
}

func parseSummary(b []byte) (Score, error) {
	var a reviewsAnswer
	if err := json.Unmarshal(b, &a); err != nil || a.Success != 1 {
		return Score{}, &errFormat{"the review summary isn't what the store sends"}
	}
	s := a.QuerySummary
	if s.Total < 0 || s.Positive < 0 || s.Negative < 0 {
		return Score{}, &errFormat{"the review summary has negative counts"}
	}
	return Score{Label: s.Desc, Percent: percent(s.Positive, s.Total), Total: s.Total}, nil
}

// parseRecent adds up the last 30 days. Steam gives no label for it, so
// there is none; Total 0 means Steam gave nothing.
func parseRecent(b []byte) (Score, bool) {
	var a histogramAnswer
	if json.Unmarshal(b, &a) != nil || a.Success != 1 {
		return Score{}, false
	}
	var up, down int
	for _, d := range a.Results.Recent {
		if d.Up < 0 || d.Down < 0 {
			return Score{}, false
		}
		up += d.Up
		down += d.Down
	}
	return Score{Percent: percent(up, up+down), Total: up + down}, true
}

func (c *Client) fetchSummary(ctx context.Context, appID int) (ReviewSummary, error) {
	id := strconv.Itoa(appID)
	u := "https://store.steampowered.com/appreviews/" + id + "?json=1&filter=all&language=all&purchase_type=all&num_per_page=0&cursor=*"
	b, err := c.fetch(ctx, request{url: u})
	if err != nil {
		return ReviewSummary{}, err
	}
	overall, err := parseSummary(b)
	if err != nil {
		c.formatFailed(u)
		return ReviewSummary{}, err
	}
	sum := ReviewSummary{AppID: appID, Overall: overall, URL: reviewsURL(appID), FetchedAt: c.now().Unix(), State: StateOK}
	// The recent score is a bonus: if Steam won't give it, the overall
	// score still stands and Recent stays empty.
	if hb, err := c.fetch(ctx, request{url: "https://store.steampowered.com/appreviewhistogram/" + id + "?l=english"}); err == nil {
		if recent, ok := parseRecent(hb); ok {
			sum.Recent = recent
		}
	} else if ctx.Err() != nil {
		return ReviewSummary{}, ctx.Err()
	}
	return sum, nil
}

// ---- review pages ----

var languageName = regexp.MustCompile(`^[a-z]{2,20}$`)

// cleanReviewQuery turns a query into what is actually asked of Steam, so
// "", "*" and the first page share a cache entry and bad input can't reach
// the URL.
func cleanReviewQuery(q ReviewQuery) ReviewQuery {
	if q.Cursor == "" {
		q.Cursor = "*"
	}
	if q.Filter != ReviewsRecent {
		q.Filter = ReviewsHelpful
	}
	q.Language = strings.ToLower(strings.TrimSpace(q.Language))
	if q.Language == "" || !languageName.MatchString(q.Language) {
		q.Language = "all"
	}
	return q
}

func reviewKey(q ReviewQuery) string {
	return strings.Join([]string{strconv.Itoa(q.AppID), q.Filter, q.Language, q.Cursor}, "|")
}

func (c *Client) fetchReviews(ctx context.Context, q ReviewQuery) (ReviewPage, error) {
	filter := "all"
	if q.Filter == ReviewsRecent {
		filter = "recent"
	}
	u := "https://store.steampowered.com/appreviews/" + strconv.Itoa(q.AppID) + "?json=1&filter=" + filter +
		"&language=" + url.QueryEscape(q.Language) + "&purchase_type=all&num_per_page=" + strconv.Itoa(reviewsPerPage) +
		"&cursor=" + url.QueryEscape(q.Cursor)
	b, err := c.fetch(ctx, request{url: u})
	if err != nil {
		return ReviewPage{}, err
	}
	page, err := parseReviews(b, q)
	if err != nil {
		c.formatFailed(u)
		return ReviewPage{}, err
	}
	return page, nil
}

const maxReviewText = 8000

func parseReviews(b []byte, q ReviewQuery) (ReviewPage, error) {
	var a reviewsAnswer
	if err := json.Unmarshal(b, &a); err != nil || a.Success != 1 {
		return ReviewPage{}, &errFormat{"the reviews aren't what the store sends"}
	}
	page := ReviewPage{AppID: q.AppID, Reviews: make([]Review, 0, len(a.Reviews)), Cursor: a.Cursor, State: StateOK}
	for _, r := range a.Reviews {
		if r.ID == "" {
			continue
		}
		author := strings.TrimSpace(r.Author.PersonaName)
		if author == "" {
			author = r.Author.SteamID
		}
		link := reviewsURL(q.AppID)
		if validSteamID(r.Author.SteamID) {
			link = "https://steamcommunity.com/profiles/" + r.Author.SteamID + "/recommended/" + strconv.Itoa(q.AppID) + "/"
		}
		page.Reviews = append(page.Reviews, Review{
			ID:               r.ID,
			Author:           cleanText(author, 80),
			Recommended:      r.VotedUp,
			Text:             plainText(r.Text, maxReviewText),
			Language:         r.Language,
			Helpful:          max(r.VotesUp, 0),
			Funny:            max(r.Funny, 0),
			PlaytimeAtReview: max(r.Author.PlaytimeAt, 0),
			PlaytimeForever:  max(r.Author.PlaytimeForever, 0),
			Posted:           r.Created,
			URL:              link,
		})
	}
	// Steam hands back the same cursor (or none) when the reviews run out,
	// and a short page means the same.
	page.More = len(a.Reviews) >= reviewsPerPage && a.Cursor != "" && a.Cursor != q.Cursor
	return page, nil
}

func validSteamID(s string) bool {
	if len(s) < 5 || len(s) > 20 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ---- critic ----

type appDetailsAnswer map[string]struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
}

func (c *Client) fetchCritic(ctx context.Context, appID int) (Critic, error) {
	u := "https://store.steampowered.com/api/appdetails?filters=metacritic&appids=" + strconv.Itoa(appID)
	b, err := c.fetch(ctx, request{url: u})
	if err != nil {
		return Critic{}, err
	}
	cr, err := parseCritic(b, appID)
	if err != nil {
		c.formatFailed(u)
		return Critic{}, err
	}
	cr.FetchedAt = c.now().Unix()
	return cr, nil
}

func parseCritic(b []byte, appID int) (Critic, error) {
	var a appDetailsAnswer
	if err := json.Unmarshal(b, &a); err != nil || a == nil {
		return Critic{}, &errFormat{"the store data isn't JSON"}
	}
	cr := Critic{AppID: appID, State: StateOK}
	r, ok := a[strconv.Itoa(appID)]
	if !ok {
		return Critic{}, &errFormat{"the store data has no entry for this game"}
	}
	// A game the store doesn't know, or one with no Metacritic entry
	// (Steam then sends [] for data), simply has no score.
	if !r.Success || len(r.Data) == 0 || r.Data[0] != '{' {
		return cr, nil
	}
	var d struct {
		Metacritic struct {
			Score int    `json:"score"`
			URL   string `json:"url"`
		} `json:"metacritic"`
	}
	if err := json.Unmarshal(r.Data, &d); err != nil {
		return Critic{}, &errFormat{"the Metacritic entry isn't readable"}
	}
	if s := d.Metacritic.Score; s >= 1 && s <= 100 {
		cr.Score = s
		cr.URL = metacriticURL(d.Metacritic.URL)
	}
	return cr, nil
}

// metacriticURL keeps Steam's link only if it really points at Metacritic.
func metacriticURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return ""
	}
	h := strings.ToLower(u.Hostname())
	if h != "metacritic.com" && !strings.HasSuffix(h, ".metacritic.com") {
		return ""
	}
	return u.String()
}
