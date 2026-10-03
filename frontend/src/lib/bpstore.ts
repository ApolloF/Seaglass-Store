// Big picture's Store, kept pure so it can be tested: which sections show,
// Home's shelves and moving between them, where focus goes back to, the
// Browse query and its filters, and what the install confirmation allows.
// Labels the desktop Store already has come from storefront.ts.
import { offerLine } from "./catalog";
import { bytes } from "./format";
import { countText, emptyQuery, hoursText, notReadyText, reviewText, sourceLabel, type SearchView } from "./storefront";
import type { BrowseQuery, CatalogEntry, DiscoveryStatus, Enrichment, GameSummary, PreparedRelease, Recommendation, RecommendationBasis, ReviewScore, StoreHome } from "./types";

// ---------------------------------------------------------------------------
// Sections. The Store is one only while the experimental Store is on.

export type Section = "home" | "library" | "search" | "store";

export function sectionsFor(storeOn: boolean): { id: Section; label: string }[] {
  return [
    { id: "home", label: "Home" },
    { id: "library", label: "Library" },
    { id: "search", label: "Search" },
    ...(storeOn ? [{ id: "store" as const, label: "Store" }] : []),
  ];
}

/** The Store's own tabs, switched with L2 / R2. */
export const STORE_TABS = [
  { id: "home", label: "Home" },
  { id: "browse", label: "Browse" },
  { id: "wishlist", label: "Wishlist" },
] as const;
export type StoreTab = (typeof STORE_TABS)[number]["id"];

// ---------------------------------------------------------------------------
// Home's shelves.

export interface Shelf {
  id: string;
  title: string;
  /** Said next to the title: where the shelf comes from, or that it's cached. */
  note: string;
  games: GameSummary[];
  /** Featured games show bigger. */
  big: boolean;
  /** Per game key: why it's recommended. */
  because: Record<string, string>;
}

const basisTexts: Record<RecommendationBasis, string> = {
  "": "",
  played: "Based on games you played",
  wishlist: "Based on your wishlist",
  "played+wishlist": "Based on games you played and your wishlist",
  popular: "Popular on Steam, until you've played or wishlisted games",
};
export const basisText = (b: RecommendationBasis) => basisTexts[b] ?? "";

/** "Like Ember Crown, Dune Lark", or the shared genres, or "". */
export function becauseText(r: Recommendation): string {
  if (r.because.length) return `Like ${r.because.join(", ")}`;
  return r.genres.slice(0, 2).join(" · ");
}

/** The shelves with games in them, featured first. Empty shelves are left out so focus never lands on nothing. */
export function homeShelves(h: StoreHome): Shelf[] {
  const shelf = (id: string, title: string, games: GameSummary[], note = "", big = false, because: Record<string, string> = {}): Shelf => ({ id, title, note, games, big, because });
  return [
    shelf("featured", "Featured", h.featured, "", true),
    shelf("new", "New", h.new),
    shelf("popular", "Popular", h.popular, h.popularState === "stale" ? "Cached chart" : "Steam's most played"),
    shelf("updated", "Recently updated", h.updated),
    shelf(
      "recommended",
      "Recommended",
      h.recommended.map((r) => r.game),
      basisText(h.recommendedBasis),
      false,
      Object.fromEntries(h.recommended.map((r) => [r.game.key, becauseText(r)])),
    ),
    shelf("wishlist", "Wishlist activity", h.wishlist),
  ].filter((s) => s.games.length > 0);
}

/** Why a shelf you'd expect isn't there. */
export function homeNotes(h: StoreHome): string[] {
  return h.popularState === "unavailable" ? ["Popular: Steam's most-played chart isn't available right now."] : [];
}

// ---------------------------------------------------------------------------
// Card lines.

/** Where a game's releases come from: "FitGirl · DODI". */
export const sourcesText = (g: Pick<GameSummary, "sources">) => g.sources.map(sourceLabel).join(" · ");

/** Whether the game can be got, in a few words. */
export function availabilityText(g: GameSummary): string {
  if (!g.sourceBacked) return "No known source release";
  if (g.installed?.update) return "Update available";
  if (g.installed) return `Installed${g.installed.version ? ` ${g.installed.version}` : ""}`;
  return g.installable ? "Installable" : "Needs checking";
}

/** Review and completion time, where known: "Very Positive · 92% · 59 h main story". */
export function factsText(g: GameSummary): string {
  const h = hoursText(g.completionMain);
  return [reviewText(g), h && `${h} main story`].filter(Boolean).join(" · ");
}

/** "Very Positive · 92% · 612,400 reviews"; "" without reviews. */
export const scoreLine = (s: ReviewScore) => [s.label, s.percent ? `${s.percent}%` : "", s.total ? countText(s.total, "review") : ""].filter(Boolean).join(" · ");

/** HowLongToBeat's three times that are known; none without a match. */
export function completionTimes(e: Enrichment | null): { label: string; time: string }[] {
  if (!e?.completion.hltbId) return [];
  return (
    [
      ["Main Story", e.completion.main],
      ["Main + Extras", e.completion.mainExtras],
      ["Completionist", e.completion.completionist],
    ] as [string, number][]
  )
    .filter(([, m]) => m > 0)
    .map(([label, m]) => ({ label, time: hoursText(m) }));
}

// ---------------------------------------------------------------------------
// Moving between shelves. Each shelf remembers its column, so going down and
// back up returns to the same game.

export interface ShelfFocus {
  row: number;
  /** The column per shelf. */
  cols: number[];
}

export const startFocus = (): ShelfFocus => ({ row: 0, cols: [] });

/** The next focus for a move, or null at an edge. `lens` are the shelves' lengths. */
export function shelfMove(f: ShelfFocus, lens: number[], intent: string): ShelfFocus | null {
  const col = Math.min(f.cols[f.row] ?? 0, Math.max(0, (lens[f.row] ?? 1) - 1));
  switch (intent) {
    case "left":
    case "right": {
      const next = col + (intent === "left" ? -1 : 1);
      if (next < 0 || next >= (lens[f.row] ?? 0)) return null;
      const cols = [...f.cols];
      cols[f.row] = next;
      return { row: f.row, cols };
    }
    case "up":
    case "down": {
      const row = f.row + (intent === "up" ? -1 : 1);
      if (row < 0 || row >= lens.length) return null;
      return { row, cols: [...f.cols] };
    }
  }
  return null;
}

/** The focused shelf column, kept inside the shelf. */
export const shelfCol = (f: ShelfFocus, len: number) => Math.max(0, Math.min(f.cols[f.row] ?? 0, len - 1));

/** What focus is on, as "shelf/game key": it survives the shelves being fetched again. */
export function focusKey(shelves: Pick<Shelf, "id" | "games">[], f: ShelfFocus): string {
  const s = shelves[f.row];
  const g = s?.games[shelfCol(f, s.games.length)];
  return s && g ? `${s.id}/${g.key}` : "";
}

/**
 * Focus for a key after the shelves changed: the same game on the same
 * shelf, else the same place on that shelf, else the same row, else the
 * start. Coming back from a game's page lands where it was opened.
 */
export function restoreFocus(shelves: Pick<Shelf, "id" | "games">[], key: string, prev: ShelfFocus): ShelfFocus {
  if (!shelves.length) return startFocus();
  const cut = key.indexOf("/");
  const id = cut < 0 ? "" : key.slice(0, cut);
  const gameKey = cut < 0 ? "" : key.slice(cut + 1);
  const keep = (row: number, col: number): ShelfFocus => {
    const cols = shelves.map((s, i) => Math.min(prev.cols[i] ?? 0, Math.max(0, s.games.length - 1)));
    cols[row] = col;
    return { row, cols };
  };
  const row = shelves.findIndex((s) => s.id === id);
  if (row >= 0) {
    const col = shelves[row].games.findIndex((g) => g.key === gameKey);
    return keep(row, col >= 0 ? col : Math.min(prev.cols[prev.row] ?? 0, shelves[row].games.length - 1));
  }
  const r = Math.min(prev.row, shelves.length - 1);
  return keep(r, Math.min(prev.cols[r] ?? 0, shelves[r].games.length - 1));
}

// ---------------------------------------------------------------------------
// Browse: the query, its two filters, and the results.

export interface Choice {
  id: string;
  label: string;
}

/** The sources to filter by: the chosen ones, from the registry's status. */
export function sourceChoices(status: DiscoveryStatus | null): Choice[] {
  return [{ id: "", label: "All sources" }, ...(status?.sources ?? []).filter((s) => s.enabled).map((s) => ({ id: s.id, label: s.name || sourceLabel(s.id) }))];
}

export const SHOW_CHOICES: Choice[] = [
  { id: "", label: "All games" },
  { id: "installable", label: "Installable" },
  { id: "installed", label: "Installed" },
  { id: "not-installed", label: "Not installed" },
];

/** The next choice in a list, round and round. */
export const cycle = (n: number, i: number, d: -1 | 1) => (n ? (i + d + n) % n : 0);

export function browseQuery(text: string, source: string, show: string): BrowseQuery {
  return {
    ...emptyQuery(),
    text,
    sources: source ? [source] : [],
    availability: show === "installable" ? "installable" : "",
    installed: show === "installed" || show === "not-installed" ? show : "",
  };
}

export interface Result {
  game: GameSummary;
  /** A Steam game with no known source release: wishlist only. */
  other: boolean;
}

/** Source releases first, then the other games Steam knows. */
export function browseResults(v: Pick<SearchView, "page" | "other">): Result[] {
  const games = v.page?.games ?? [];
  const seen = new Set(games.map((g) => g.key));
  return [...games.map((game) => ({ game, other: false })), ...v.other.filter((g) => !seen.has(g.key)).map((game) => ({ game, other: true }))];
}

// ---------------------------------------------------------------------------
// The install confirmation, as the desktop's install dialog does it.

export type InstallStep = "checking" | "confirm" | "browser" | "blocked";

/** Where the flow is: nothing can be downloaded without a validated transport. */
export function installStep(prepared: PreparedRelease | null, entry: CatalogEntry | null = null): InstallStep {
  if (entry) return entry.offers.length ? "confirm" : "blocked";
  if (!prepared) return "checking";
  if (prepared.state === "ready" && prepared.ready && prepared.offers.length > 0 && prepared.offers.every((o) => o.infoHash)) return "confirm";
  return prepared.state === "unresolved" || prepared.state === "browser" ? "browser" : "blocked";
}

/** Why a release can't be installed. */
export const blockedText = (p: PreparedRelease | null): string => notReadyText(p ?? { state: "unavailable" });

export interface InstallChoice {
  label: string;
  languages: string[];
  sizeBytes: number;
  installedSizeBytes?: number;
  line: string;
}

/** The versions to choose from: a feed entry's offers, or a prepared release's validated ones. */
export function installChoices(prepared: PreparedRelease | null, entry: CatalogEntry | null = null): InstallChoice[] {
  if (entry) {
    return entry.offers.map((x, i) => ({
      label: `${x.version || "Version not given"} · ${x.feedName}${i === (entry.recommended?.offer ?? 0) ? " (recommended)" : ""}`,
      languages: x.languages ?? [],
      sizeBytes: x.sizeBytes ?? 0,
      installedSizeBytes: x.installedSizeBytes,
      line: offerLine(x),
    }));
  }
  return (prepared?.offers ?? []).map((x) => ({
    label: `${x.version || prepared?.release.version || "Version not given"} · ${x.sourceName}${x.torrentName ? ` · ${x.torrentName}` : ""}`,
    languages: x.languages.length ? x.languages : (prepared?.release.languages ?? []),
    sizeBytes: x.sizeBytes,
    installedSizeBytes: x.installedSizeBytes,
    line: [x.sourceName, x.version, bytes(x.sizeBytes)].filter(Boolean).join(" · "),
  }));
}

/**
 * The language to install: the current one when the version has it, else
 * English, else the version's first. Without stated languages English is
 * asked for and checked once the metadata arrives.
 */
export function pickLanguage(languages: string[], current: string): string {
  if (!languages.length) return "English";
  const has = (l: string) => languages.find((x) => x.toLowerCase() === l.toLowerCase());
  return has(current) ?? has("English") ?? languages[0];
}

/** "Download 47.9 GB · 95.8 GB once installed". */
export function sizesText(c: Pick<InstallChoice, "sizeBytes" | "installedSizeBytes">): string {
  return [c.sizeBytes && `Download ${bytes(c.sizeBytes)}`, c.installedSizeBytes && `${bytes(c.installedSizeBytes)} once installed`].filter(Boolean).join(" · ");
}
