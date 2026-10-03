import { describe, expect, it } from "vitest";
import {
  adoptPage,
  availabilityLabel,
  canPrepare,
  createSearchController,
  emptyQuery,
  hoursText,
  completionKind,
  languagesLine,
  providerText,
  publishedText,
  sizeLine,
  sourceLine,
  statusSummary,
  storeMode,
  withEnrichment,
  type SearchView,
} from "./storefront";
import type { BrowsePage, DiscoverySourceStatus, DiscoveryStatus, Enrichment, GameSummary, ProviderProgress, Release, SearchResult } from "./types";

const game = (key: string, extra: Partial<GameSummary> = {}): GameSummary => ({
  key, title: key, sourceBacked: true, sources: ["fitgirl"], releases: 1, publishedAt: 1, updatedAt: 1, sizeBytes: 0, languages: [], genres: [],
  installable: true, popularRank: 0, completionMain: 0, reviewPercent: 0, reviewTotal: 0, wishlisted: false, activity: false, ...extra,
});
const page = (...keys: string[]): BrowsePage => ({ games: keys.map((k) => game(k)), total: keys.length, unknown: 0, languages: [], genres: [] });
const result = (p: BrowsePage, extra: Partial<SearchResult> = {}): SearchResult => ({ query: emptyQuery(), page: p, other: [], remote: [], complete: true, ...extra });

/** A promise the test settles by hand. */
function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => ((resolve = res), (reject = rej)));
  return { promise, resolve, reject };
}
const flush = () => new Promise((r) => setTimeout(r, 0));

/** Timers the test advances itself. */
function fakeTimers() {
  let now = 0;
  let next = 1;
  const pending = new Map<number, { at: number; fn: () => void }>();
  return {
    setTimer: (fn: () => void, ms: number) => {
      pending.set(next, { at: now + ms, fn });
      return next++;
    },
    clearTimer: (h: unknown) => void pending.delete(h as number),
    advance(ms: number) {
      now += ms;
      for (const [id, t] of [...pending]) if (t.at <= now) (pending.delete(id), t.fn());
    },
    get count() {
      return pending.size;
    },
  };
}

function setup(over: { browse?: (q: { text: string }) => Promise<SearchResult>; search?: (q: { text: string }) => Promise<SearchResult> } = {}) {
  const timers = fakeTimers();
  const calls = { browse: [] as string[], search: [] as string[] };
  let view: SearchView | undefined;
  const c = createSearchController({
    browse: (q) => (calls.browse.push(q.text), over.browse ? over.browse(q) : Promise.resolve(result(page("local:" + q.text)))),
    search: (q) => (calls.search.push(q.text), over.search ? over.search(q) : Promise.resolve(result(page("remote:" + q.text)))),
    onChange: (v) => (view = v),
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
  });
  return { c, timers, calls, view: () => view! };
}
const q = (text: string) => ({ ...emptyQuery(), text });

describe("search controller", () => {
  it("asks the local index at once on every change", async () => {
    const { c, calls, view } = setup();
    c.update(q("e"));
    c.update(q("em"));
    expect(calls.browse).toEqual(["e", "em"]);
    await flush();
    expect(view().page?.games[0].key).toBe("local:em");
  });

  it("waits 600 ms before searching the sites and Steam", async () => {
    const { c, timers, calls } = setup();
    c.update(q("ember"));
    timers.advance(599);
    expect(calls.search).toEqual([]);
    timers.advance(1);
    expect(calls.search).toEqual(["ember"]);
  });

  it("does not search remotely under two characters", () => {
    const { c, timers, calls, view } = setup();
    c.update(q("e"));
    expect(timers.count).toBe(0);
    timers.advance(5000);
    expect(calls.search).toEqual([]);
    expect(view().searching).toBe(false);
    c.update(q(" e "));
    expect(timers.count).toBe(0);
  });

  it("restarts the wait while typing continues", () => {
    const { c, timers, calls } = setup();
    c.update(q("em"));
    timers.advance(500);
    c.update(q("emb"));
    timers.advance(500);
    expect(calls.search).toEqual([]);
    timers.advance(100);
    expect(calls.search).toEqual(["emb"]);
  });

  it("ignores a superseded remote answer", async () => {
    const slow = deferred<SearchResult>();
    const { c, timers, view } = setup({ search: (x) => (x.text === "ember" ? slow.promise : Promise.resolve(result(page("remote:" + x.text)))) });
    c.update(q("ember"));
    timers.advance(600);
    c.update(q("hollow"));
    timers.advance(600);
    await flush();
    expect(view().page?.games[0].key).toBe("remote:hollow");
    slow.resolve(result(page("remote:ember")));
    await flush();
    expect(view().page?.games[0].key).toBe("remote:hollow");
  });

  it("ignores a superseded local answer", async () => {
    const slow = deferred<SearchResult>();
    const { c, view } = setup({ browse: (x) => (x.text === "em" ? slow.promise : Promise.resolve(result(page("local:" + x.text)))) });
    c.update(q("em"));
    c.update(q("emb"));
    await flush();
    slow.resolve(result(page("local:em")));
    await flush();
    expect(view().page?.games[0].key).toBe("local:emb");
  });

  it("keeps local matches when the remote search fails", async () => {
    const { c, timers, view } = setup({ search: () => Promise.reject(new Error("offline")) });
    c.update(q("ember"));
    await flush();
    timers.advance(600);
    await flush();
    expect(view().page?.games[0].key).toBe("local:ember");
    expect(view().remoteError).toBe("offline");
    expect(view().searching).toBe(false);
  });

  it("keeps a failed provider's reason next to the other matches", async () => {
    const remote: ProviderProgress[] = [
      { id: "fitgirl", name: "FitGirl", state: "error", found: 0, error: "couldn't reach FitGirl", cached: false },
      { id: "steam", name: "Steam", state: "ok", found: 1, cached: false },
    ];
    const { c, timers, view } = setup({ search: () => Promise.resolve(result(page("a", "b"), { remote, other: [game("steam:1", { sourceBacked: false })] })) });
    c.update(q("ember"));
    timers.advance(600);
    await flush();
    expect(view().page?.games.map((g) => g.key)).toEqual(["a", "b"]);
    expect(view().remote[0].error).toBe("couldn't reach FitGirl");
    expect(view().other).toHaveLength(1);
  });

  it("returns to plain browsing when the text is cleared and drops the remote search", async () => {
    const slow = deferred<SearchResult>();
    const { c, timers, calls, view } = setup({ search: () => slow.promise });
    c.update(q("ember"));
    timers.advance(600);
    c.update(q(""));
    expect(timers.count).toBe(0);
    slow.resolve(result(page("remote:ember"), { other: [game("steam:1")] }));
    await flush();
    expect(calls.browse).toEqual(["ember", ""]);
    expect(view().page?.games[0].key).toBe("local:");
    expect(view().other).toEqual([]);
    expect(view().remote).toEqual([]);
  });

  it("keeps remote results while only a filter changes", async () => {
    const { c, timers, view } = setup({ search: () => Promise.resolve(result(page("r"), { other: [game("steam:1")] })) });
    c.update(q("ember"));
    timers.advance(600);
    await flush();
    c.update({ ...q("ember"), language: "French" });
    await flush();
    expect(view().other).toHaveLength(1);
    expect(view().page?.games[0].key).toBe("local:ember");
  });

  it("shows progress only for the current text", async () => {
    const { c, view } = setup();
    c.update(q("ember"));
    const remote: ProviderProgress[] = [{ id: "steam", name: "Steam", state: "loading", found: 0, cached: false }];
    c.progress({ text: "hollow", remote });
    expect(view().remote).toEqual([]);
    c.progress({ text: "ember", remote });
    expect(view().remote).toEqual(remote);
  });

  it("refreshes local matches without starting another remote search", async () => {
    const { c, timers, calls, view } = setup();
    c.update(q("ember"));
    timers.advance(600);
    await flush();
    c.refreshLocal();
    await flush();
    expect(calls.browse).toEqual(["ember", "ember"]);
    expect(calls.search).toEqual(["ember"]);
    expect(view().page?.games[0].key).toBe("local:ember");
  });

  it("drops late answers after it is disposed", async () => {
    const slow = deferred<SearchResult>();
    const { c, view } = setup({ browse: () => slow.promise });
    c.update(q("ember"));
    c.dispose();
    slow.resolve(result(page("late")));
    await flush();
    expect(view().page).toBeNull();
  });
});

describe("adoptPage", () => {
  it("keeps the pages already shown when the first page arrives again", () => {
    const current = ["a", "b", "c", "d"].map((k) => game(k));
    expect(adoptPage(current, page("a", "b")).map((g) => g.key)).toEqual(["a", "b", "c", "d"]);
  });
  it("starts over when the first page is different", () => {
    const current = ["a", "b", "c"].map((k) => game(k));
    expect(adoptPage(current, page("x", "y")).map((g) => g.key)).toEqual(["x", "y"]);
  });
});

const source = (over: Partial<DiscoverySourceStatus> = {}): DiscoverySourceStatus => ({
  id: "fitgirl", name: "FitGirl", enabled: true, state: "idle", releases: 0, recentAt: 0, backfillPage: 0, backfillDone: false, retryAt: 0,
  host: "fitgirl-repacks.site", search: true, paged: true, torrents: true, defaultOn: true, notes: [], ...over,
});
const status = (over: Partial<DiscoveryStatus> = {}): DiscoveryStatus => ({ enabled: true, setupNeeded: false, sources: [source()], games: 0, releases: 0, refreshing: false, paused: false, playing: false, stale: false, ...over });

describe("store mode", () => {
  it("asks the existing Store user to choose sources first", () => {
    expect(storeMode({ loaded: true, status: status({ setupNeeded: true }), shown: 3 })).toBe("setup");
  });
  it("shows games that exist even when discovery is off, as feed games do", () => {
    expect(storeMode({ loaded: true, status: status({ enabled: false, sources: [] }), shown: 2 })).toBe("ready");
  });
  it("says discovery is off when there is nothing to show", () => {
    expect(storeMode({ loaded: true, status: status({ enabled: false, sources: [] }), shown: 0 })).toBe("off");
  });
  it("says it is finding releases while the first pass has not finished", () => {
    expect(storeMode({ loaded: true, status: status(), shown: 0 })).toBe("finding");
  });
  it("says the sources can't be reached when every source failed", () => {
    expect(storeMode({ loaded: true, status: status({ sources: [source({ error: "no such host", recentAt: 0 })] }), shown: 0 })).toBe("unreachable");
  });
  it("shows cached games when offline", () => {
    expect(storeMode({ loaded: true, status: status({ stale: true, sources: [source({ error: "x", recentAt: 5 })] }), shown: 9 })).toBe("ready");
  });
  it("waits for the first answers", () => {
    expect(storeMode({ loaded: false, status: null, shown: 0 })).toBe("loading");
  });
});

describe("status text", () => {
  it("labels cached results", () => {
    expect(statusSummary(status({ stale: true })).text).toBe("Showing cached results");
  });
  it("mentions indexing of older releases", () => {
    expect(statusSummary(status({ sources: [source({ state: "backfill" })] })).text).toBe("Indexing older releases");
  });
  it("says when to try again after a failure", () => {
    const line = sourceLine(source({ state: "backoff", retryAt: 1600, releases: 4 }), 1000);
    expect(line).toContain("Trying again in 10 min");
  });
  it("shows backfill progress and a finished backfill", () => {
    expect(sourceLine(source({ state: "backfill", backfillPage: 14, releases: 1 }), 1000)).toContain("Indexing older releases (page 14)");
    expect(sourceLine(source({ backfillDone: true, releases: 2 }), 1000)).toContain("Older releases indexed");
  });
});

describe("labels", () => {
  it("writes completion times as hours", () => {
    expect(hoursText(0)).toBe("");
    expect(hoursText(45)).toBe("45 min");
    expect(hoursText(720)).toBe("12 h");
    expect(hoursText(750)).toBe("12½ h");
    expect(hoursText(30)).toBe("30 min");
  });
  it("labels HowLongToBeat's DLC and mods, and nothing for a game", () => {
    expect(completionKind("game")).toBe("");
    expect(completionKind("")).toBe("");
    expect(completionKind("dlc")).toBe("DLC");
    expect(completionKind("Mod")).toBe("Mod");
    expect(completionKind("expansion")).toBe("Expansion");
  });
  it("labels the source date as publication, and nothing when unknown", () => {
    expect(publishedText(0)).toBe("");
    expect(publishedText(1_790_000_000)).toMatch(/^Published \d+ \w+ 2026$/);
  });
  it("marks minimum sizes and unknown sizes", () => {
    expect(sizeLine({ sizeBytes: 2_000_000_000, sizeIsMinimum: true })).toBe("At least 2.0 GB");
    expect(sizeLine({ sizeBytes: 0 })).toBe("Size not stated");
    expect(sizeLine({ sizeBytes: 0, sizeClaim: "~10 GB" })).toBe("Size claimed: ~10 GB");
  });
  it("shows the raw language claim or that none is stated", () => {
    expect(languagesLine({ languages: ["English"] })).toBe("English");
    expect(languagesLine({ languages: [], languageClaim: "ENG/RUS" })).toBe("Claimed: ENG/RUS");
    expect(languagesLine({ languages: [] })).toBe("Languages not stated");
  });
  it("names provider states", () => {
    const p = (state: ProviderProgress["state"], extra: Partial<ProviderProgress> = {}): ProviderProgress => ({ id: "x", name: "X", state, found: 3, cached: false, ...extra });
    expect(providerText(p("loading"))).toBe("searching…");
    expect(providerText(p("ok"))).toBe("done (3)");
    expect(providerText(p("ok", { cached: true }))).toBe("cached (3)");
    expect(providerText(p("error", { error: "timed out" }))).toBe("failed: timed out");
    expect(providerText(p("skipped"))).toBe("skipped");
  });
  it("names availability and only prepares real, listed releases", () => {
    const r = (kind: Release["kind"], availability: Release["availability"]) => ({ kind, availability }) as Release;
    expect(availabilityLabel("unresolved")).toBe("Needs resolving");
    expect(canPrepare(r("release", "installable"))).toBe(true);
    expect(canPrepare(r("release", "manual"))).toBe(true);
    expect(canPrepare(r("update", "update-only"))).toBe(false);
    expect(canPrepare(r("release", "unavailable"))).toBe(false);
    expect(canPrepare(r("release", "summary"))).toBe(false);
  });
});

describe("withEnrichment", () => {
  const e = (state: "ok" | "unavailable"): Enrichment =>
    ({ key: "k", steamAppId: 1, popularRank: 4, reviews: { state, overall: { label: "Very Positive", percent: 92, total: 100 } } }) as Enrichment;
  it("adds reviews and chart place the summary lacked", () => {
    const g = withEnrichment(game("k"), e("ok"));
    expect([g.reviewPercent, g.reviewTotal, g.popularRank]).toEqual([92, 100, 4]);
  });
  it("never invents reviews from an unavailable provider", () => {
    expect(withEnrichment(game("k"), e("unavailable")).reviewTotal).toBe(0);
  });
  it("keeps what the summary already says", () => {
    expect(withEnrichment(game("k", { reviewTotal: 5, reviewPercent: 50 }), e("ok")).reviewPercent).toBe(50);
  });
});
