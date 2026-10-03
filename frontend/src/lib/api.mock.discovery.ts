// Store discovery, enrichment and wishlist for `npm run dev:mock`: a
// made-up index of source releases with Steam reviews, a chart and
// HowLongToBeat times. With `?store=1` (the Store on), add `&discovery=`
// to the URL to see other states (comma-separated): setup (the one-time
// source choice), empty (nothing indexed yet), offline (remote searches
// and providers fail; cached answers come back stale), nochart (Steam's
// chart is unavailable), slow (remote answers take longer).
import type { Api } from "./api";
import type {
  BrowsePage,
  BrowseQuery,
  CompletionCandidate,
  DiscoveryChange,
  DiscoveryStatus,
  Download,
  Enrichment,
  GameDetails,
  GameSummary,
  Meta,
  PreparedRelease,
  ProviderProgress,
  Release,
  Review,
  ReviewPage,
  SearchProgress,
  SearchResult,
  Settings,
  WishlistItem,
} from "./types";

const flags = new Set(
  (typeof location === "undefined" ? "" : (new URLSearchParams(location.search).get("discovery") ?? ""))
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean),
);
/** Turns a scenario on or off (tests use it; the URL sets it in the browser). */
export function mockDiscoveryFlag(name: string, on: boolean) {
  if (on) flags.add(name);
  else flags.delete(name);
}

const copy = <T>(v: T): T => structuredClone(v);
const now = () => Math.floor(Date.now() / 1000);
const day = 86400;
const gb = 1 << 30;
const wait = (ms: number) => new Promise((r) => setTimeout(r, flags.has("slow") ? ms * 3 : ms));
const squash = (t: string) => t.toLowerCase().replace(/&/g, "and").replace(/[^a-z0-9]/g, "");
const SOURCE_NAMES: Record<string, string> = { fitgirl: "FitGirl", dodi: "DODI", feeds: "Feeds", steam: "Steam" };

/** The providers the mock registry declares, as sources.Providers does. */
const PROVIDERS = [
  { id: "fitgirl", host: "fitgirl-repacks.site", search: true, paged: true, torrents: true, defaultOn: true, notes: [] as string[] },
  { id: "dodi", host: "dodi-repacks.site", search: true, paged: true, torrents: true, defaultOn: true, notes: [] as string[] },
];

// A release as the fixtures write it.
interface R {
  source: "fitgirl" | "dodi" | "feeds";
  version?: string;
  ago: number; // days since publication
  changedAgo?: number; // days since the article changed
  size?: number; // GiB
  languages?: string[];
  claim?: string;
  avail?: Release["availability"];
  unresolved?: string[];
  kind?: "release" | "update";
  // What preparing it does: resolve (works after a moment), captcha (needs the browser).
  prepare?: "resolve" | "captcha";
  backfill?: boolean;
}
interface G {
  title: string;
  appId?: number;
  releases: R[];
  genres?: string[];
  rank?: number;
  review?: [number, number, string];
  recent?: [number, number, string];
  critic?: number;
  hltb?: [number, number, number] | "none";
  dev?: string;
  year?: number;
  blurb?: string;
}

const EN = ["English"];
const MULTI = ["English", "French", "German", "Spanish", "Russian", "Japanese"];

const fixtures: G[] = [
  { title: "Ember Crown", appId: 1245620, rank: 4, genres: ["Action", "RPG"], review: [92, 612_400, "Very Positive"], recent: [88, 4_120, "Very Positive"], critic: 94, hltb: [3540, 6060, 8880], dev: "Kiln & Anvil", year: 2026,
    blurb: "A fallen kingdom, a crown that remembers every bearer, and a long road through the ash.",
    releases: [
      { source: "fitgirl", version: "v1.2.0", ago: 2, size: 47.9, languages: MULTI, claim: "MULTi6", avail: "installable" },
      { source: "dodi", version: "v1.1.4", ago: 30, size: 52.3, languages: ["English", "Russian"], claim: "ENG/RUS", avail: "unresolved", unresolved: ["File-Me: not resolved yet"], prepare: "resolve" },
      { source: "fitgirl", version: "v1.0.6", ago: 120, changedAgo: 1, size: 38.1, languages: MULTI, avail: "installable", backfill: true },
    ] },
  { title: "Hollow Tide", appId: 2210110, rank: 12, genres: ["Adventure", "Indie"], review: [81, 23_800, "Very Positive"], recent: [74, 310, "Mostly Positive"], hltb: [540, 720, 1100], dev: "Tidewater", year: 2025,
    blurb: "Dive a drowned city at low tide and get back before the sea returns.",
    releases: [{ source: "dodi", version: "v2.0.3", ago: 1, size: 6.2, languages: EN, avail: "manual", unresolved: ["Up-4ever needs a browser challenge (CAPTCHA)"], prepare: "captcha" }] },
  { title: "Brass Orchard", genres: ["Simulation"], review: [67, 1_204, "Mostly Positive"], hltb: [1200, 2400, 4800], dev: "Gearwright", year: 2026,
    blurb: "Tend clockwork trees and sell their brass fruit before the frost.",
    releases: [
      { source: "fitgirl", version: "Build 15302", ago: 4, size: 2.1, languages: ["English", "Spanish"], avail: "installable" },
      { source: "feeds", version: "Build 15302", ago: 33, size: 2.1, languages: ["English", "Spanish"], avail: "installable" },
    ] },
  { title: "Glass Meridian", appId: 1873000, rank: 31, genres: ["Strategy"], review: [58, 9_870, "Mixed"], recent: [41, 512, "Mixed"], critic: 71, hltb: [2100, 3300, 6900], dev: "Prism Works", year: 2026,
    releases: [{ source: "fitgirl", version: "v0.9 beta", ago: 6, size: 3.4, languages: ["English", "Japanese"], avail: "installable" }] },
  { title: "Ashen Lanterns", appId: 1938400, genres: ["Action"], review: [88, 45_000, "Very Positive"], hltb: [960, 1500, 2280], dev: "Low Ember", year: 2025,
    releases: [
      { source: "fitgirl", version: "v1.4.2", ago: 9, size: 11.8, languages: ["English", "German"], claim: "ENG/GER", avail: "summary" },
      { source: "dodi", version: "v1.4.2 + 3 DLCs", ago: 10, size: 13.0, languages: [], claim: "MULTi9", avail: "unresolved", unresolved: ["File-Me: rate-limited, try again in a minute"], prepare: "resolve" },
    ] },
  { title: "Ashen Lanterns 2", genres: ["Action"], hltb: "none", dev: "Low Ember", year: 2026,
    releases: [{ source: "dodi", version: "v0.3 Early Access", ago: 3, size: 9.0, languages: [], avail: "unresolved", unresolved: ["File-Me: not resolved yet"], prepare: "resolve" }] },
  { title: "Cinder Drift", genres: ["Racing"], review: [74, 3_300, "Mostly Positive"], dev: "Sidecar", year: 2025,
    releases: [{ source: "feeds", version: "1.0", ago: 280, size: 0.4, languages: EN, avail: "installable" }] },
  { title: "Dune Lark", appId: 1678010, rank: 58, genres: ["Adventure"], review: [95, 18_200, "Overwhelmingly Positive"], critic: 89, hltb: [300, 420, 600], dev: "Sandglass", year: 2024,
    releases: [{ source: "fitgirl", version: "v1.0.12", ago: 14, changedAgo: 2, size: 1.7, languages: MULTI, avail: "installable" }] },
  { title: "Iron Vigil: Remastered", appId: 1555420, rank: 77, genres: ["Strategy", "Simulation"], review: [79, 7_600, "Mostly Positive"], hltb: [2700, 4200, 9000], year: 2025,
    releases: [{ source: "dodi", version: "v1.0.5", ago: 21, size: 24.0, languages: ["English", "French"], avail: "installable" }] },
  { title: "Iron Vigil", appId: 412020, genres: ["Strategy"], review: [90, 52_000, "Very Positive"], hltb: [2400, 3900, 8400], year: 2016,
    releases: [{ source: "fitgirl", version: "v2.31", ago: 900, size: 8.0, languages: EN, avail: "installable", backfill: true }] },
  { title: "Quiet Orbit", genres: ["Puzzle"], hltb: [180, 240, 300], year: 2026,
    releases: [{ source: "fitgirl", ago: 1, size: 0.8, languages: EN, avail: "installable" }] },
  { title: "Rust Psalm", appId: 2301900, rank: 19, genres: ["RPG"], review: [86, 31_700, "Very Positive"], recent: [91, 2_050, "Very Positive"], critic: 84, hltb: [4200, 7200, 12000], year: 2026,
    releases: [
      { source: "dodi", version: "v1.03", ago: 5, size: 61.5, languages: MULTI, claim: "MULTi12", avail: "installable" },
      { source: "dodi", version: "Update v1.02 to v1.03", ago: 5, size: 1.2, kind: "update", avail: "update-only" },
    ] },
  { title: "Saltmarsh Saga", genres: ["Strategy"], year: 2023,
    releases: [{ source: "fitgirl", version: "v3.0.1", ago: 400, size: 5.5, languages: ["English", "Polish"], avail: "unavailable", backfill: true }] },
  { title: "Twelve Bells", appId: 1999000, genres: ["Horror"], review: [62, 2_100, "Mostly Positive"], year: 2026,
    releases: [{ source: "fitgirl", version: "v1.1", ago: 8, size: 14.2, languages: [], claim: "Text: ENG; Audio: JAP", avail: "installable" }] },
  ...["Amber Atlas", "Bramble Keep", "Copper Choir", "Drowned Bell", "Echo Harvest", "Fern & Flint", "Gale Runner", "Hearth Tactics", "Ivory Lantern", "Juniper Line"].map(
    (title, i): G => ({ title, genres: [["Indie"], ["RPG"], ["Strategy"], ["Adventure"]][i % 4], year: 2020 + (i % 6), review: i % 3 ? undefined : [70 + i, 1000 * (i + 1), "Mostly Positive"],
      releases: [{ source: i % 2 ? "dodi" : "fitgirl", version: `v1.${i}`, ago: 40 + i * 37, size: 1 + i * 2.3, languages: i % 3 === 1 ? [] : EN, avail: i % 4 === 3 ? "unresolved" : "installable", backfill: i > 4, prepare: "resolve" }] }),
  ),
];

// Steam games no source has released: they show under "Other games".
const steamOnly: { appId: number; title: string; genres: string[]; review: [number, number, string] }[] = [
  { appId: 3100100, title: "Hollow Tide II", genres: ["Adventure"], review: [0, 0, "No user reviews"] },
  { appId: 3100200, title: "Ember Crown: Ashes of the Old Kings", genres: ["Action", "RPG"], review: [90, 1_200, "Very Positive"] },
  { appId: 3100300, title: "Lantern Season", genres: ["Simulation"], review: [77, 5_400, "Mostly Positive"] },
];

// What a remote search on a source finds that the index doesn't know yet.
const remoteOnly: Record<string, G> = {
  lantern: { title: "Lantern Season", appId: 3100300, genres: ["Simulation"], review: [77, 5_400, "Mostly Positive"], hltb: [600, 900, 1500],
    releases: [{ source: "dodi", version: "v1.0.2", ago: 700, size: 3.3, languages: EN, avail: "unresolved", unresolved: ["File-Me: not resolved yet"], prepare: "resolve" }] },
};

// ---------------------------------------------------------------------------
// The index.

interface Rec {
  release: Release;
  backfill: boolean;
  prepare?: "resolve" | "captcha";
}
interface Game {
  g: G;
  key: string;
  title: string;
  appId?: number;
  corrected?: boolean;
  recs: Rec[];
}

const keyOf = (g: { title: string; appId?: number }) => (g.appId ? `steam:${g.appId}` : `title:${squash(g.title)}`);
let seq = 0;
function makeGame(g: G): Game {
  const key = keyOf(g);
  const recs = g.releases.map((r, i): Rec => {
    const id = r.source === "feeds" ? `feed:${i}` : `${r.source}-${squash(g.title)}-${i}-${++seq}`;
    const sourceName = r.source === "feeds" ? "Indie Showcase" : SOURCE_NAMES[r.source];
    const avail = r.avail ?? "installable";
    return {
      backfill: !!r.backfill,
      prepare: r.prepare,
      release: {
        id,
        origin: r.source === "feeds" ? "feed" : "source",
        source: r.source === "feeds" ? "https://feeds.example/indie.json" : r.source,
        sourceName,
        title: g.title,
        rawTitle: `${g.title}${r.version ? ` – ${r.version}` : ""}${r.source === "dodi" ? " [DODI Repack]" : ""}`,
        version: r.version,
        pageUrl: r.source === "feeds" ? undefined : `https://${r.source === "dodi" ? "dodi-repacks.site" : "fitgirl-repacks.site"}/${squash(g.title)}/`,
        publishedAt: now() - r.ago * day,
        updatedAt: now() - (r.changedAgo ?? r.ago) * day,
        sizeBytes: Math.round((r.size ?? 0) * gb),
        sizeClaim: r.size ? `${r.size} GB` : undefined,
        languages: r.languages ?? [],
        languageClaim: r.claim ?? (r.languages ?? []).join(", "),
        kind: r.kind ?? "release",
        availability: avail,
        transports: avail === "installable" ? 1 : 0,
        unresolved: r.unresolved ?? [],
        warnings:
          r.source === "feeds"
            ? []
            : ["Source metadata is unverified; payload safety has not been assessed", ...(avail === "summary" ? ["Search summary; fetch the release page for complete metadata"] : [])],
        newer: false,
        feedKey: r.source === "feeds" ? key : undefined,
        feedOffer: 0,
      },
    };
  });
  return { g, key, title: g.title, appId: g.appId, recs };
}

let games: Game[] = fixtures.map(makeGame);

type Wish = { key: string; title: string; steamAppId?: number; addedAt: number; activity: WishlistItem["activity"]; origin?: "" | "steam" };
let wishes: Wish[] = [
  { key: "title:ashenlanterns2", title: "Ashen Lanterns 2", addedAt: now() - 20 * day,
    activity: [{ id: "a1", kind: "available", releaseId: "", source: "dodi", sourceName: "DODI", version: "v0.3 Early Access", at: now() - 3 * day, read: false }] },
  { key: "steam:1245620", title: "Ember Crown", steamAppId: 1245620, addedAt: now() - 60 * day,
    activity: [{ id: "a2", kind: "newer", releaseId: "", source: "fitgirl", sourceName: "FitGirl", version: "v1.2.0", at: now() - 2 * day, read: false }] },
  { key: "steam:3100100", title: "Hollow Tide II", steamAppId: 3100100, addedAt: now() - 5 * day, activity: [] },
];
for (const w of wishes) for (const a of w.activity) a.releaseId = games.find((g) => g.key === w.key)?.recs[0]?.release.id ?? "";

// What the Store installed, by key (the mock's Downloads agree loosely).
const installed: Record<string, { download: string; version: string }> = {
  "title:brassorchard": { download: "sg-5", version: "Build 15302" },
  "steam:1678010": { download: "sg-d1", version: "v1.0.9" },
};

let refreshing = false;
let lastRefresh = now() - 2 * 3600;

function status(settings: Settings): DiscoveryStatus {
  const st = settings.store;
  const setup = settings.experimentalStore && (flags.has("setup") || st.sourceSetup === "ask");
  const on = (id: string) => settings.experimentalStore && st.privateSources && st.sourceSetup === "done" && st.sources.includes(id) && !flags.has("setup");
  const empty = flags.has("empty");
  const sources = PROVIDERS.map(({ id, ...caps }) => {
    const releases = empty ? 0 : games.reduce((n, g) => n + g.recs.filter((r) => r.release.source === id).length, 0);
    const offline = flags.has("offline");
    return {
      id,
      name: SOURCE_NAMES[id],
      enabled: on(id),
      state: !on(id) ? "disabled" : refreshing ? "recent" : offline ? "backoff" : id === "dodi" ? "backfill" : "idle",
      releases,
      recentAt: empty ? 0 : lastRefresh,
      backfillPage: empty ? 0 : id === "dodi" ? 14 : 112,
      backfillDone: id === "fitgirl" && !empty,
      retryAt: offline ? now() + 600 : 0,
      error: offline && on(id) ? "couldn't reach the source: no such host" : undefined,
      ...caps,
    } as DiscoveryStatus["sources"][number];
  });
  return {
    enabled: sources.some((s) => s.enabled),
    setupNeeded: setup,
    sources,
    games: empty ? 0 : games.filter((g) => g.recs.length).length,
    releases: sources.reduce((n, s) => n + s.releases, 0),
    refreshing,
    paused: settings.store.indexingPaused,
    playing: false,
    stale: flags.has("offline") || now() - lastRefresh > 6 * 3600,
  };
}

function visible(settings: Settings): Game[] {
  if (flags.has("empty") || flags.has("setup")) return games.filter((g) => g.recs.some((r) => r.release.origin === "feed"));
  const st = settings.store;
  return games.filter((g) => g.recs.some((r) => r.release.origin === "feed" || (st.privateSources && st.sources.includes(r.release.source))));
}

function summary(game: Game, settings: Settings): GameSummary {
  const recs = game.recs.filter((r) => r.release.origin === "feed" || settings.store.sources.includes(r.release.source));
  const newest = [...recs].sort((a, b) => b.release.publishedAt - a.release.publishedAt)[0]?.release;
  const g = game.g;
  const inst = installed[game.key];
  const wish = wishes.find((w) => w.key === game.key);
  const enriched = enrichedKeys.has(game.key) || !!g.rank;
  return {
    key: game.key,
    title: game.title,
    steamAppId: game.appId,
    sourceBacked: recs.length > 0,
    sources: [...new Set(recs.map((r) => (r.release.origin === "feed" ? "feeds" : r.release.source)))],
    releases: recs.length,
    version: newest?.version,
    publishedAt: newest?.publishedAt ?? 0,
    updatedAt: Math.max(0, ...recs.map((r) => r.release.updatedAt)),
    sizeBytes: newest?.sizeBytes ?? 0,
    languages: [...new Set(recs.flatMap((r) => r.release.languages))],
    genres: g.genres ?? [],
    installable: recs.some((r) => r.release.availability === "installable"),
    popularRank: flags.has("nochart") ? 0 : (g.rank ?? 0),
    reviewPercent: enriched && g.review ? g.review[0] : 0,
    reviewTotal: enriched && g.review ? g.review[1] : 0,
    reviewLabel: enriched && g.review ? g.review[2] : undefined,
    completionMain: enriched && Array.isArray(g.hltb) ? g.hltb[0] : 0,
    installed: inst ? { ...inst, update: game.key === "steam:1678010" } : undefined,
    wishlisted: !!wish,
    activity: !!wish?.activity.some((a) => !a.read),
  };
}

function steamOnlySummary(s: (typeof steamOnly)[number]): GameSummary {
  const wish = wishes.find((w) => w.key === `steam:${s.appId}`);
  return {
    key: `steam:${s.appId}`, title: s.title, steamAppId: s.appId, sourceBacked: false, sources: [], releases: 0, publishedAt: 0, updatedAt: 0, sizeBytes: 0,
    languages: [], genres: s.genres, installable: false, popularRank: 0, reviewPercent: s.review[0], reviewTotal: s.review[1], reviewLabel: s.review[2],
    completionMain: 0, wishlisted: !!wish, activity: false,
  };
}

const sortTitle = (t: string) => t.toLowerCase().replace(/^(the|a|an) /, "");

function query(settings: Settings, q: BrowseQuery): BrowsePage {
  const text = squash(q.text);
  let hits = visible(settings).map((g) => summary(g, settings)).filter((s) => s.sourceBacked);
  const languages = [...new Set(hits.flatMap((s) => s.languages))].sort();
  const genres = [...new Set(hits.flatMap((s) => s.genres))].sort();
  if (text) hits = hits.filter((s) => squash(s.title).includes(text));
  if (q.sources.length) hits = hits.filter((s) => s.sources.some((x) => q.sources.includes(x)));
  if (q.availability === "installable") hits = hits.filter((s) => s.installable);
  if (q.availability === "unresolved") hits = hits.filter((s) => !s.installable);
  if (q.installed === "installed") hits = hits.filter((s) => s.installed);
  if (q.installed === "not-installed") hits = hits.filter((s) => !s.installed);
  let unknown = 0;
  if (q.language) {
    unknown += hits.filter((s) => !s.languages.length).length;
    hits = hits.filter((s) => s.languages.some((l) => l.toLowerCase() === q.language.toLowerCase()));
  }
  if (q.genre) {
    unknown += hits.filter((s) => !s.genres.length).length;
    hits = hits.filter((s) => s.genres.includes(q.genre));
  }
  const byTitle = (a: GameSummary, b: GameSummary) => sortTitle(a.title).localeCompare(sortTitle(b.title));
  switch (q.sort) {
    case "published":
      hits.sort((a, b) => b.publishedAt - a.publishedAt || byTitle(a, b));
      break;
    case "popular":
      hits.sort((a, b) => (a.popularRank || 1e9) - (b.popularRank || 1e9) || byTitle(a, b));
      break;
    case "reviews":
      hits.sort((a, b) => (b.reviewTotal ? b.reviewPercent : -1) - (a.reviewTotal ? a.reviewPercent : -1) || byTitle(a, b));
      break;
    default:
      hits.sort(byTitle);
  }
  const limit = q.limit > 0 ? Math.min(q.limit, 200) : 60;
  const off = Math.min(Math.max(q.offset, 0), hits.length);
  return { games: hits.slice(off, off + limit), total: hits.length, unknown, languages, genres };
}

function others(q: BrowseQuery): GameSummary[] {
  const text = squash(q.text);
  if (text.length < 2) return [];
  return steamOnly.filter((s) => squash(s.title).includes(text) && !games.some((g) => g.key === `steam:${s.appId}`)).map(steamOnlySummary);
}

// ---------------------------------------------------------------------------
// Enrichment.

const enrichedKeys = new Set<string>();
const hltbOverride: Record<string, number> = {};
const candidates: CompletionCandidate[] = [
  { hltbId: 90001, title: "Ember Crown", year: 2026, type: "game", main: 3540, mainExtras: 6060, completionist: 8880, url: "https://howlongtobeat.com/game/90001" },
  { hltbId: 90002, title: "Ember Crown", year: 2009, type: "game", main: 600, mainExtras: 900, completionist: 1400, url: "https://howlongtobeat.com/game/90002" },
  { hltbId: 90003, title: "Ashen Lanterns", year: 2025, type: "game", main: 960, mainExtras: 1500, completionist: 2280, url: "https://howlongtobeat.com/game/90003" },
  { hltbId: 90004, title: "Ashen Lanterns 2", year: 2026, type: "game", main: 0, mainExtras: 0, completionist: 0, url: "https://howlongtobeat.com/game/90004" },
  { hltbId: 90005, title: "Ember Crown: Tides of Ash", year: 2026, type: "dlc", main: 540, mainExtras: 780, completionist: 1020, url: "https://howlongtobeat.com/game/90005" },
  { hltbId: 90006, title: "Ember Crown Remixed", year: 2026, type: "mod", main: 0, mainExtras: 0, completionist: 0, url: "https://howlongtobeat.com/game/90006" },
];

function findAny(key: string): { g: G; title: string; appId?: number } | undefined {
  const game = games.find((x) => x.key === key);
  if (game) return { g: game.g, title: game.title, appId: game.appId };
  const s = steamOnly.find((x) => `steam:${x.appId}` === key);
  if (s) return { g: { title: s.title, appId: s.appId, releases: [], genres: s.genres, review: s.review }, title: s.title, appId: s.appId };
  return undefined;
}

function enrichment(key: string, fresh: boolean): Enrichment {
  const found = findAny(key);
  const off = flags.has("offline");
  const g = found?.g;
  const appId = found?.appId ?? 0;
  const fetched = off ? now() - 3 * day : now() - (fresh ? 0 : 1800);
  const st = (has: boolean) => (!has ? "unavailable" : off ? "stale" : "ok") as Enrichment["reviews"]["state"];
  const steamUrl = `https://store.steampowered.com/app/${appId}/#app_reviews_hash`;
  const reviews: Enrichment["reviews"] = appId
    ? { appId, overall: g?.review ? { label: g.review[2], percent: g.review[0], total: g.review[1] } : { label: "No user reviews", percent: 0, total: 0 },
        recent: g?.recent ? { label: g.recent[2], percent: g.recent[0], total: g.recent[1] } : { percent: 0, total: 0 }, url: steamUrl, fetchedAt: fetched, state: st(true) }
    : { appId: 0, overall: { percent: 0, total: 0 }, recent: { percent: 0, total: 0 }, url: "", fetchedAt: 0, state: "unavailable", error: "No Steam match: reviews need one" };
  const critic: Enrichment["critic"] = appId
    ? { appId, score: g?.critic ?? 0, url: g?.critic ? `https://www.metacritic.com/game/${squash(found!.title)}/` : undefined, fetchedAt: fetched, state: st(true) }
    : { appId: 0, score: 0, fetchedAt: 0, state: "unavailable" };
  const chosen = hltbOverride[key] ? candidates.find((c) => c.hltbId === hltbOverride[key]) : undefined;
  const times = chosen ? [chosen.main, chosen.mainExtras, chosen.completionist] : g?.hltb && g.hltb !== "none" ? g.hltb : undefined;
  const search = `https://howlongtobeat.com/?q=${encodeURIComponent(found?.title ?? "")}`;
  const completion: Enrichment["completion"] = times
    ? { hltbId: chosen?.hltbId ?? 90000 + (squash(found!.title).length % 1000), title: chosen?.title ?? found!.title, main: times[0], mainExtras: times[1], completionist: times[2],
        url: chosen?.url ?? `https://howlongtobeat.com/game/${90000 + (squash(found!.title).length % 1000)}`, corrected: !!chosen, fetchedAt: fetched, state: st(true) }
    : { hltbId: 0, main: 0, mainExtras: 0, completionist: 0, url: search, corrected: false, fetchedAt: off ? 0 : now(), state: off ? "error" : "unavailable",
        error: off ? "HowLongToBeat couldn't be reached" : "No confident HowLongToBeat match" };
  return { key, steamAppId: appId, reviews, critic, completion, popularRank: flags.has("nochart") ? 0 : (g?.rank ?? 0) };
}

const reviewTexts = [
  "The combat finally clicked around the second region. Parry windows are generous, but the bosses punish greed.",
  "Runs well on a Steam Deck at 40 fps with medium settings. Loading times are short.",
  "Gorgeous art, but the late game drags. The last two chapters are mostly backtracking.",
  "Bought it on a whim, stayed for the soundtrack. Ten out of ten fishing minigame.",
  "Crashed twice on launch until I updated my drivers; fine since then.",
  "Not for me: too much inventory management, too little exploring.",
];
function reviewPage(appId: number, cursor: string, filter: string): ReviewPage {
  if (flags.has("offline")) return { appId, reviews: [], cursor: "", more: false, state: "error", error: "Steam couldn't be reached" };
  const page = cursor ? Number(cursor) : 0;
  const reviews: Review[] = Array.from({ length: 5 }, (_, i) => {
    const n = page * 5 + i;
    return {
      id: `${appId}-${filter}-${n}`,
      author: ["lanternfish", "Mira", "76561198000000042", "brass_owl", "Tidecaller", "quietbell"][n % 6],
      recommended: n % 4 !== 2,
      text: reviewTexts[n % reviewTexts.length] + (n % 3 === 0 ? "\n\nUpdate after 40 hours: still playing." : ""),
      language: "english",
      helpful: 400 - n * 13,
      funny: n % 5,
      playtimeAtReview: 60 * (n + 3),
      playtimeForever: 60 * (n + 9),
      posted: now() - (filter === "recent" ? n : n * 9) * day,
      url: `https://steamcommunity.com/profiles/76561198000000042/recommended/${appId}/`,
    };
  });
  const more = page < 2;
  return { appId, reviews, cursor: more ? String(page + 1) : "", more, state: "ok" };
}

// ---------------------------------------------------------------------------

const statusListeners = new Set<(s: DiscoveryStatus) => void>();
const gameListeners = new Set<(c: DiscoveryChange) => void>();
const searchListeners = new Set<(p: SearchProgress) => void>();
const enrichListeners = new Set<(e: Enrichment) => void>();
const wishListeners = new Set<(w: WishlistItem[]) => void>();
const on = <T>(set: Set<(v: T) => void>, cb: (v: T) => void) => {
  set.add(cb);
  return () => set.delete(cb);
};

/** Art and descriptions for discovery games, for the mock's store.art. */
export function mockDiscoveryArt(key: string): Meta | undefined {
  const f = findAny(key);
  if (!f) return undefined;
  return { description: f.g.blurb, developers: f.g.dev ? [f.g.dev] : undefined, genres: f.g.genres, releaseYear: f.g.year };
}

/** Hooks the mock's Downloads: queues a download the way downloadOffer does. */
export type QueueDownload = (d: Partial<Download> & { title: string; gameKey: string }) => Download;

export function mockDiscovery(getSettings: () => Settings, setSettings: (s: Settings) => void, queue: QueueDownload): Pick<Api["store"], "discovery" | "enrich" | "wishlist"> {
  const emitStatus = () => statusListeners.forEach((cb) => cb(status(getSettings())));
  const emitGames = (c: DiscoveryChange) => gameListeners.forEach((cb) => cb(copy(c)));
  const wishItems = (): WishlistItem[] => {
    const settings = getSettings();
    return [...wishes]
      .sort((a, b) => b.addedAt - a.addedAt)
      .map((w) => {
        const game = games.find((g) => g.key === w.key);
        const s = steamOnly.find((x) => `steam:${x.appId}` === w.key);
        const summaryOf = game ? summary(game, settings) : s ? steamOnlySummary(s) : steamOnlySummary({ appId: w.steamAppId ?? 0, title: w.title, genres: [], review: [0, 0, ""] });
        return { key: w.key, title: w.title, steamAppId: w.steamAppId, addedAt: w.addedAt, game: summaryOf, activity: copy(w.activity), unread: w.activity.filter((a) => !a.read).length, origin: w.origin ?? "" };
      });
  };
  const emitWish = () => wishListeners.forEach((cb) => cb(wishItems()));
  const findGame = (key: string) => {
    const g = games.find((x) => x.key === key);
    if (!g) throw new Error("that game isn't in the Store anymore");
    return g;
  };
  const findRec = (key: string, id: string) => {
    const r = findGame(key).recs.find((x) => x.release.id === id);
    if (!r) throw new Error("that release isn't listed anymore");
    return r;
  };
  const needOn = () => {
    const s = getSettings();
    if (!s.experimentalStore) throw new Error("the store is turned off (Settings, Experimental)");
    if (!s.store.privateSources) throw new Error("private catalog sources are disabled");
  };
  const prepared = (key: string, rec: Rec, reason?: string): PreparedRelease => {
    const r = rec.release;
    const ready = r.availability === "installable";
    return {
      gameKey: key,
      release: copy(r),
      ready,
      offers: ready
        ? [{ transport: 0, title: r.title, version: r.version, sizeBytes: r.sizeBytes, installedSizeBytes: r.sizeBytes * 2, languages: r.languages, torrentName: r.rawTitle, infoHash: "a".repeat(40), sourceName: r.sourceName }]
        : [],
      state: ready ? "ready" : r.availability === "update-only" ? "update-only" : r.availability === "unavailable" ? "unavailable" : "unresolved",
      reason: ready ? undefined : (reason ?? r.unresolved[0]),
      warnings: copy(r.warnings),
      installed: installed[key] ? { version: installed[key].version, dir: `C:\\Users\\you\\Games\\${r.title}` } : undefined,
    };
  };
  let searchSeq = 0;

  return {
    discovery: {
      async status() {
        return status(getSettings());
      },
      async setupSources(sources) {
        const s = getSettings();
        flags.delete("setup");
        setSettings({ ...s, store: { ...s.store, sources: [...sources], sourceSetup: "done", privateSources: sources.length > 0 } });
        emitStatus();
        emitGames({ keys: [], all: true });
        return copy(getSettings());
      },
      async pauseIndexing(paused) {
        const s = getSettings();
        setSettings({ ...s, store: { ...s.store, indexingPaused: paused } });
        emitStatus();
        return status(getSettings());
      },
      async refresh() {
        needOn();
        refreshing = true;
        emitStatus();
        await wait(1500);
        refreshing = false;
        if (flags.has("offline")) {
          emitStatus();
          throw new Error("couldn't reach FitGirl or DODI: showing what was indexed before");
        }
        lastRefresh = now();
        emitStatus();
        emitGames({ keys: [], all: true });
        return status(getSettings());
      },
      async home() {
        const settings = getSettings();
        await wait(150);
        const all = visible(settings).map((g) => summary(g, settings)).filter((s) => s.sourceBacked);
        const chart = flags.has("nochart") || flags.has("empty");
        return {
          new: [...all].sort((a, b) => b.publishedAt - a.publishedAt).slice(0, 12),
          popular: chart ? [] : all.filter((s) => s.popularRank > 0).sort((a, b) => a.popularRank - b.popularRank).slice(0, 12),
          popularState: chart ? "unavailable" : flags.has("offline") ? "stale" : "ok",
          updated: all.filter((s) => s.updatedAt - s.publishedAt > day).sort((a, b) => b.updatedAt - a.updatedAt).slice(0, 12),
          wishlist: wishItems().filter((w) => w.unread > 0).map((w) => w.game),
          featured: [],
          recommended: [],
          recommendedBasis: "",
          status: status(settings),
        };
      },
      async browse(q) {
        const settings = getSettings();
        await wait(60);
        const remote: ProviderProgress[] = squash(q.text).length >= 2 ? [] : ["fitgirl", "dodi", "steam"].map((id) => ({ id, name: SOURCE_NAMES[id], state: "skipped", found: 0, cached: false }));
        return { query: copy(q), page: query(settings, q), other: [], remote, complete: true } satisfies SearchResult;
      },
      async search(q) {
        const mine = ++searchSeq;
        const settings = getSettings();
        const text = squash(q.text);
        const ids = ["fitgirl", "dodi", "steam"].filter((id) => id === "steam" || (settings.store.privateSources && settings.store.sources.includes(id)));
        const remote: ProviderProgress[] = ids.map((id) => ({ id, name: SOURCE_NAMES[id], state: text.length < 2 ? "skipped" : "loading", found: 0, cached: false }));
        const result = (): SearchResult => ({ query: copy(q), page: query(getSettings(), q), other: others(q), remote: copy(remote), complete: remote.every((r) => r.state !== "loading") });
        if (text.length < 2) return result();
        searchListeners.forEach((cb) => cb({ text: q.text, remote: copy(remote) }));
        for (const p of remote) {
          await wait(p.id === "steam" ? 500 : 900);
          if (mine !== searchSeq) return result(); // superseded: the partial answer
          if (flags.has("offline") && p.id !== "steam") {
            p.state = "error";
            p.error = `couldn't reach ${p.name}`;
          } else if (flags.has("offline")) {
            p.state = "stale";
            p.cached = true;
          } else {
            p.state = "ok";
            const extra = Object.entries(remoteOnly).find(([k]) => text.includes(k) || k.includes(text));
            if (extra && p.id === extra[1].releases[0].source && !games.some((g) => g.title === extra[1].title)) {
              games = [...games, makeGame(extra[1])];
              emitGames({ keys: [keyOf(extra[1])], all: false });
            }
            p.found = p.id === "steam" ? others(q).length : games.filter((g) => squash(g.title).includes(text) && g.recs.some((r) => r.release.source === p.id)).length;
          }
          searchListeners.forEach((cb) => cb({ text: q.text, remote: copy(remote) }));
        }
        return result();
      },
      async game(key) {
        const settings = getSettings();
        await wait(120);
        const lone = steamOnly.find((x) => `steam:${x.appId}` === key);
        if (lone && !games.some((g) => g.key === key)) {
          return { summary: steamOnlySummary(lone), releases: [], recommended: -1, why: [], identity: { steamAppId: lone.appId, name: lone.title, how: "", corrected: false }, loading: false };
        }
        const game = findGame(key);
        const releases = game.recs.filter((r) => r.release.origin === "feed" || settings.store.sources.includes(r.release.source)).map((r) => copy(r.release)).sort((a, b) => b.publishedAt - a.publishedAt);
        const inst = installed[key];
        for (const r of releases) r.newer = !!inst && key === "steam:1678010" && r.version === "v1.0.12";
        const rec = releases.findIndex((r) => r.availability === "installable" && r.kind === "release");
        const loading = releases.some((r) => r.availability === "summary");
        if (loading) {
          setTimeout(() => {
            for (const r of game.recs) if (r.release.availability === "summary") Object.assign(r.release, { availability: "installable", transports: 1, warnings: r.release.warnings.slice(0, 1) });
            emitGames({ keys: [key], all: false });
          }, 1800);
        }
        return {
          summary: summary(game, settings),
          releases,
          recommended: rec,
          why: rec < 0 ? [] : releases.length > 1 ? ["The newest version with a validated torrent", releases[rec].languages.includes("English") ? "Has English" : "Languages not stated"] : ["The only release"],
          identity: { steamAppId: game.appId ?? 0, name: game.appId ? game.title : undefined, how: game.corrected ? "correction" : game.appId ? "game-database" : "", corrected: !!game.corrected },
          loading,
        } satisfies GameDetails;
      },
      async setSteamMatch(key, appId, name) {
        needOn();
        const game = findGame(key);
        game.appId = appId || undefined;
        game.corrected = true;
        if (appId && name) game.title = name;
        const old = game.key;
        game.key = appId ? `steam:${appId}` : `title:${squash(game.g.title)}`;
        emitGames({ keys: [old, game.key], all: false });
        return this.game(game.key);
      },
      async prepareRelease(key, id) {
        needOn();
        const rec = findRec(key, id);
        await wait(rec.release.availability === "installable" ? 300 : 2000);
        if (rec.release.availability === "unresolved" && rec.prepare === "resolve" && !flags.has("offline")) {
          Object.assign(rec.release, { availability: "installable", transports: 1, unresolved: [], warnings: [...rec.release.warnings, "Torrent metadata was validated; game identity and payload safety remain unverified"] });
          emitGames({ keys: [key], all: false });
        } else if (rec.release.availability === "unresolved" || rec.release.availability === "manual") {
          return prepared(key, rec, rec.prepare === "captcha" || flags.has("offline") ? "The file host needs a browser challenge. Open the release page, get the .torrent file there, then attach it." : undefined);
        }
        return prepared(key, rec);
      },
      async attachTorrent(key, id) {
        needOn();
        const rec = findRec(key, id);
        Object.assign(rec.release, { availability: "installable", transports: 1, unresolved: [], warnings: [...rec.release.warnings, "Manual torrent attached: confirm that its name matches the selected game."] });
        emitGames({ keys: [key], all: false });
        return prepared(key, rec);
      },
      async openRelease(key, id) {
        findRec(key, id);
      },
      async downloadRelease(key, id, transport, opts) {
        needOn();
        const rec = findRec(key, id);
        if (rec.release.availability !== "installable" || transport !== 0) throw new Error("this release has no validated torrent: prepare it again");
        const r = rec.release;
        return queue({ title: r.version ? `${r.title} ${r.version}` : r.title, gameKey: key, version: r.version, feedName: r.sourceName, size: r.sizeBytes, installDir: opts.dir, language: opts.language, autoInstall: opts.install });
      },
      onStatus: (cb) => on(statusListeners, cb),
      onGames: (cb) => on(gameListeners, cb),
      onSearch: (cb) => on(searchListeners, cb),
    },

    enrich: {
      async games(keys) {
        const out: Enrichment[] = [];
        const later: string[] = [];
        for (const k of keys) {
          if (!findAny(k)) continue;
          if (enrichedKeys.has(k)) out.push(enrichment(k, false));
          else later.push(k);
        }
        if (later.length) {
          setTimeout(() => {
            later.forEach((k) => enrichedKeys.add(k));
            later.forEach((k) => enrichListeners.forEach((cb) => cb(enrichment(k, true))));
          }, 700);
        }
        return out;
      },
      async game(key) {
        if (!findAny(key)) throw new Error("that game isn't in the Store anymore");
        await wait(enrichedKeys.has(key) ? 100 : 1200);
        enrichedKeys.add(key);
        return enrichment(key, true);
      },
      async reviews(q) {
        await wait(500);
        return reviewPage(q.appId, q.cursor, q.filter);
      },
      async completionCandidates(_key, title) {
        await wait(600);
        if (flags.has("offline")) throw new Error("HowLongToBeat couldn't be reached");
        const t = squash(title);
        return copy(candidates.filter((c) => squash(c.title).includes(t.slice(0, 5))));
      },
      async setCompletionMatch(key, id) {
        if (id) hltbOverride[key] = id;
        else delete hltbOverride[key];
        const e = enrichment(key, true);
        enrichListeners.forEach((cb) => cb(e));
        return e;
      },
      async openLink(url) {
        if (!/^https:\/\/(store\.steampowered\.com|steamcommunity\.com|(www\.)?metacritic\.com|(www\.)?howlongtobeat\.com)\//.test(url)) throw new Error("that link doesn't go to Steam, Metacritic or HowLongToBeat");
        window.open(url, "_blank", "noopener");
      },
      onEnrichment: (cb) => on(enrichListeners, cb),
    },

    wishlist: {
      async list() {
        return wishItems();
      },
      async add(key, title, steamAppId) {
        if (!wishes.some((w) => w.key === key)) wishes = [...wishes, { key, title, steamAppId: steamAppId || undefined, addedAt: now(), activity: [] }];
        emitWish();
        emitGames({ keys: [key], all: false });
        return wishItems();
      },
      async remove(key) {
        wishes = wishes.filter((w) => w.key !== key);
        emitWish();
        emitGames({ keys: [key], all: false });
        return wishItems();
      },
      async acknowledge(key) {
        for (const w of wishes) if (!key || w.key === key) w.activity = w.activity.map((a) => ({ ...a, read: true }));
        emitWish();
        emitGames({ keys: key ? [key] : wishes.map((w) => w.key), all: false });
        return wishItems();
      },
      async steamAccount() {
        if (flags.has("nosteam")) return { steamId: "", detected: false, error: "Steam isn't installed" };
        return { steamId: "76561198000000042", detected: true };
      },
      async importSteam(steamId) {
        if (!/^7656119\d{10}$/.test(steamId.trim())) throw new Error("That isn't a SteamID64 (17 digits starting with 7656119).");
        await wait(900);
        if (flags.has("private")) throw new Error("Steam didn't share that wishlist. In Steam, set the profile and its game details to Public.");
        const steamWish = [
          { appId: 1245620, title: "Ember Crown" },
          { appId: 2210110, title: "Hollow Tide" },
          { appId: 9990001, title: "Northwind Saga" },
        ];
        let added = 0;
        let existing = 0;
        for (const w of steamWish) {
          if (wishes.some((x) => x.steamAppId === w.appId || x.key === `steam:${w.appId}`)) {
            existing++;
            continue;
          }
          wishes = [...wishes, { key: `steam:${w.appId}`, title: w.title, steamAppId: w.appId, addedAt: now(), activity: [], origin: "steam" }];
          added++;
        }
        emitWish();
        emitGames({ keys: steamWish.map((w) => `steam:${w.appId}`), all: false });
        const items = wishItems();
        const available = items.filter((i) => steamWish.some((w) => i.steamAppId === w.appId) && i.game.sourceBacked).length;
        return { steamId: steamId.trim(), fetched: steamWish.length, added, existing, available, searching: steamWish.length - available, items };
      },
      onChange: (cb) => on(wishListeners, cb),
    },
  };
}

/** For tests: the mock page of a query against settings. */
export const mockBrowse = (settings: Settings, q: BrowseQuery): BrowsePage => query(settings, q);
