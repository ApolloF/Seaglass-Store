import { describe, expect, it } from "vitest";
import { keyMove, typeKey, typable } from "./bpstore-keyboard";
import {
  availabilityText,
  basisText,
  browseQuery,
  browseResults,
  completionTimes,
  cycle,
  factsText,
  focusKey,
  homeNotes,
  homeShelves,
  installChoices,
  installStep,
  pickLanguage,
  restoreFocus,
  scoreLine,
  sectionsFor,
  shelfMove,
  sourceChoices,
  startFocus,
} from "./bpstore";
import { createSearchController, type SearchView } from "./storefront";
import type { BrowseQuery, CatalogEntry, DiscoveryStatus, Enrichment, GameSummary, PreparedRelease, Release, SearchResult, StoreHome } from "./types";

const game = (key: string, p: Partial<GameSummary> = {}): GameSummary => ({
  key, title: key, sourceBacked: true, sources: ["fitgirl"], releases: 1, publishedAt: 0, updatedAt: 0, sizeBytes: 0, languages: [], genres: [],
  installable: true, popularRank: 0, reviewPercent: 0, reviewTotal: 0, completionMain: 0, wishlisted: false, activity: false, ...p,
});
const status = (p: Partial<DiscoveryStatus> = {}): DiscoveryStatus => ({ enabled: true, setupNeeded: false, sources: [], games: 0, releases: 0, refreshing: false, paused: false, playing: false, stale: false, ...p });
const home = (p: Partial<StoreHome> = {}): StoreHome => ({
  new: [], popular: [], popularState: "ok", updated: [], wishlist: [], featured: [], recommended: [], recommendedBasis: "", status: status(), ...p,
});
const release = (p: Partial<Release> = {}): Release => ({
  id: "r1", origin: "source", source: "fitgirl", sourceName: "FitGirl", title: "Ember Crown", rawTitle: "Ember Crown v1.2", version: "v1.2", publishedAt: 0, updatedAt: 0, sizeBytes: 4e10,
  languages: ["English", "German"], kind: "release", availability: "installable", transports: 1, browserOnly: false, unresolved: [], warnings: [], newer: false, feedOffer: -1, ...p,
});
const prepared = (p: Partial<PreparedRelease> = {}): PreparedRelease => ({
  gameKey: "steam:1", release: release(), ready: true, state: "ready", warnings: [],
  offers: [{ transport: 0, title: "Ember Crown", version: "v1.2", sizeBytes: 4e10, languages: ["German", "English"], infoHash: "a".repeat(40), sourceName: "FitGirl" }],
  ...p,
});

describe("Store section", () => {
  it("is hidden while the Store is off", () => {
    expect(sectionsFor(false).map((s) => s.id)).toEqual(["home", "library", "search"]);
    expect(sectionsFor(true).map((s) => s.id)).toEqual(["home", "library", "search", "store"]);
  });
});

describe("Home shelves", () => {
  const h = home({
    featured: [game("f1")],
    new: [game("a"), game("b"), game("c")],
    popular: [game("p")],
    recommended: [{ game: game("r"), because: ["Dune Lark", "Ember Crown"], genres: ["Action"] }, { game: game("s"), because: [], genres: ["RPG", "Indie", "Action"] }],
    recommendedBasis: "played",
  });
  const shelves = homeShelves(h);

  it("shows only shelves with games, featured first", () => {
    expect(shelves.map((s) => s.id)).toEqual(["featured", "new", "popular", "recommended"]);
    expect(shelves[0].big).toBe(true);
  });

  it("labels recommendations with their basis and each game's reason", () => {
    const rec = shelves.find((s) => s.id === "recommended")!;
    expect(rec.note).toBe("Based on games you played");
    expect(rec.because).toEqual({ r: "Like Dune Lark, Ember Crown", s: "RPG · Indie" });
    expect(basisText("popular")).toMatch(/^Popular on Steam/);
  });

  it("says when Steam's chart is missing", () => {
    expect(homeNotes(home({ popularState: "unavailable" }))).toEqual(["Popular: Steam's most-played chart isn't available right now."]);
    expect(homeNotes(h)).toEqual([]);
  });

  it("moves along a shelf and between shelves, remembering each shelf's column", () => {
    const lens = shelves.map((s) => s.games.length); // 1, 3, 1, 2
    let f = shelfMove(startFocus(), lens, "down")!;
    expect(f.row).toBe(1);
    f = shelfMove(f, lens, "right")!;
    f = shelfMove(f, lens, "right")!;
    expect(focusKey(shelves, f)).toBe("new/c");
    expect(shelfMove(f, lens, "right")).toBeNull();
    f = shelfMove(f, lens, "down")!;
    expect(focusKey(shelves, f)).toBe("popular/p");
    f = shelfMove(f, lens, "up")!;
    expect(focusKey(shelves, f)).toBe("new/c");
    expect(shelfMove(startFocus(), lens, "up")).toBeNull();
    expect(shelfMove(startFocus(), lens, "left")).toBeNull();
  });

  it("restores focus to the game it left from, after the shelves were fetched again", () => {
    const at = { row: 1, cols: [0, 2] };
    expect(focusKey(shelves, at)).toBe("new/c");
    // A new game arrived at the front of New: focus follows the game.
    const again = homeShelves({ ...h, new: [game("z"), ...h.new] });
    const f = restoreFocus(again, "new/c", at);
    expect(focusKey(again, f)).toBe("new/c");
    // The game left the shelf: the same place on it.
    const gone = homeShelves({ ...h, new: [game("a"), game("b")] });
    expect(focusKey(gone, restoreFocus(gone, "new/c", at))).toBe("new/b");
    // The shelf itself went away: the same row.
    const noNew = homeShelves({ ...h, new: [] });
    expect(restoreFocus(noNew, "new/c", at).row).toBe(1);
    expect(restoreFocus([], "new/c", at)).toEqual(startFocus());
  });
});

describe("cards", () => {
  it("say whether the game can be got, its review and completion time", () => {
    expect(availabilityText(game("a"))).toBe("Installable");
    expect(availabilityText(game("a", { installable: false }))).toBe("Needs checking");
    expect(availabilityText(game("a", { sourceBacked: false }))).toBe("No known source release");
    expect(availabilityText(game("a", { installed: { download: "", version: "v1", update: true } }))).toBe("Update available");
    expect(availabilityText(game("a", { installed: { download: "", version: "v1", update: false } }))).toBe("Installed v1");
    expect(factsText(game("a", { reviewPercent: 92, reviewTotal: 10, reviewLabel: "Very Positive", completionMain: 3540 }))).toBe("Very Positive · 92% · 59 h main story");
    expect(factsText(game("a"))).toBe("");
  });
});

describe("game page", () => {
  it("shows the review summary and the completion times that are known", () => {
    expect(scoreLine({ label: "Very Positive", percent: 92, total: 612400 })).toBe("Very Positive · 92% · 612,400 reviews");
    expect(scoreLine({ percent: 0, total: 0 })).toBe("");
    const e = { completion: { hltbId: 7, main: 3540, mainExtras: 0, completionist: 8880 } } as Enrichment;
    expect(completionTimes(e)).toEqual([
      { label: "Main Story", time: "59 h" },
      { label: "Completionist", time: "148 h" },
    ]);
    expect(completionTimes({ completion: { hltbId: 0, main: 60 } } as Enrichment)).toEqual([]);
    expect(completionTimes(null)).toEqual([]);
  });
});

describe("Browse", () => {
  it("filters by a chosen source and by what's installed or installable", () => {
    const st = status({ sources: [{ id: "fitgirl", name: "FitGirl", enabled: true } as DiscoveryStatus["sources"][number], { id: "dodi", name: "DODI", enabled: false } as DiscoveryStatus["sources"][number]] });
    expect(sourceChoices(st).map((c) => c.label)).toEqual(["All sources", "FitGirl"]);
    expect(browseQuery("ember", "fitgirl", "installable")).toMatchObject({ text: "ember", sources: ["fitgirl"], availability: "installable", installed: "" });
    expect(browseQuery("", "", "not-installed")).toMatchObject({ sources: [], availability: "", installed: "not-installed" });
    expect(cycle(4, 3, 1)).toBe(0);
    expect(cycle(4, 0, -1)).toBe(3);
  });

  it("lists other games from Steam after the source releases, marked as wishlist only", () => {
    const r = browseResults({ page: { games: [game("a")], total: 1, unknown: 0, languages: [], genres: [] }, other: [game("a"), game("steam:9", { sourceBacked: false })] });
    expect(r.map((x) => [x.game.key, x.other])).toEqual([["a", false], ["steam:9", true]]);
  });

  it("ignores answers to an older query", async () => {
    const pending: { q: BrowseQuery; resolve: (r: SearchResult) => void }[] = [];
    const answer = (q: BrowseQuery, key: string): SearchResult => ({ query: q, page: { games: [game(key)], total: 1, unknown: 0, languages: [], genres: [] }, other: [], remote: [], complete: true });
    const timers: (() => void)[] = [];
    let view: SearchView | null = null;
    const c = createSearchController({
      browse: (q) => new Promise((resolve) => pending.push({ q, resolve })),
      search: (q) => Promise.resolve(answer(q, `remote-${q.text}`)),
      onChange: (v) => (view = v),
      setTimer: (fn) => timers.push(fn),
      clearTimer: () => {},
    });
    c.update(browseQuery("em", "", ""));
    c.update(browseQuery("emb", "", ""));
    pending[1].resolve(answer(pending[1].q, "new"));
    pending[0].resolve(answer(pending[0].q, "old"));
    await Promise.resolve();
    await Promise.resolve();
    expect(browseResults(view!).map((r) => r.game.key)).toEqual(["new"]);
    timers[0](); // the older query's remote answer comes late and is dropped
    timers[1]();
    await Promise.resolve();
    await Promise.resolve();
    expect(browseResults(view!).map((r) => r.game.key)).toEqual(["remote-emb"]);
  });
});

describe("on-screen keyboard", () => {
  it("moves like big picture's Search keyboard", () => {
    expect(keyMove(11, "right")).toBe(12);
    expect(keyMove(9, "right")).toBe("out");
    expect(keyMove(0, "left")).toBeNull();
    expect(keyMove(33, "down")).toBe(41);
    expect(keyMove(41, "up")).toBe(32);
    expect(keyMove(43, "right")).toBe("out");
    expect(keyMove(40, "left")).toBeNull();
  });
  it("types, deletes and clears", () => {
    expect(typeKey("em", "b")).toBe("emb");
    expect(typeKey("emb", "del")).toBe("em");
    expect(typeKey("em", "space")).toBe("em ");
    expect(typeKey("em", "clear")).toBe("");
    expect(typeKey("em", "done")).toBe("em");
    expect(typable("a")).toBe(true);
    expect(typable("Enter")).toBe(false);
  });
});

describe("install confirmation", () => {
  it("offers a download only with a validated transport", () => {
    expect(installStep(null)).toBe("checking");
    expect(installStep(prepared())).toBe("confirm");
    expect(installStep(prepared({ offers: [] }))).toBe("blocked");
    expect(installStep(prepared({ ready: false, state: "unresolved", offers: [] }))).toBe("browser");
    expect(installStep(prepared({ offers: [{ ...prepared().offers[0], infoHash: "" }] }))).toBe("blocked");
    expect(installStep(prepared({ ready: false, state: "update-only", offers: [] }))).toBe("blocked");
    expect(installStep(prepared({ ready: false, state: "unavailable", offers: [] }))).toBe("blocked");
    expect(installStep(prepared({ ready: false, state: "preview", offers: [] }))).toBe("blocked");
    expect(installStep(prepared({ ready: false, state: "browser", offers: [] }))).toBe("browser");
    const entry = { key: "title:x", title: "X", offers: [], version: "", updated: "", size: 0, languages: [] } as CatalogEntry;
    expect(installStep(null, entry)).toBe("blocked");
  });

  it("starts on English when the version has it, else its first language", () => {
    const [c] = installChoices(prepared());
    expect(c.languages).toEqual(["German", "English"]);
    expect(pickLanguage(c.languages, "English")).toBe("English");
    expect(pickLanguage(c.languages, "german")).toBe("German");
    expect(pickLanguage(["French", "German"], "English")).toBe("French");
    expect(pickLanguage([], "German")).toBe("English");
  });

  it("falls back to the release's languages when the offer states none", () => {
    const p = prepared({ offers: [{ ...prepared().offers[0], languages: [] }] });
    expect(installChoices(p)[0].languages).toEqual(["English", "German"]);
  });
});
