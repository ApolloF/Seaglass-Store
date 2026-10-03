// What the desktop Store says and does, kept pure so it can be tested:
// the search controller, labels and the choice of which empty state to show.
import { ago, bytes } from "./format";
import type {
  BrowsePage,
  BrowseQuery,
  DiscoverySourceStatus,
  DiscoveryStatus,
  Enrichment,
  GameSummary,
  PreparedRelease,
  ProviderProgress,
  ProviderState,
  Release,
  ReleaseAvailability,
  SearchProgress,
  SearchResult,
  WishlistActivity,
} from "./types";

export const PAGE_SIZE = 60;
export const SEARCH_DEBOUNCE_MS = 600;
export const SEARCH_MIN_CHARS = 2;

export const emptyQuery = (): BrowseQuery => ({
  text: "",
  sources: [],
  language: "",
  genre: "",
  availability: "",
  installed: "",
  sort: "published",
  offset: 0,
  limit: PAGE_SIZE,
});

// ---------------------------------------------------------------------------
// Search. Every change asks the local index at once; the sites and Steam are
// asked after typing stops. A newer change makes every older answer moot.

/** What the Browse tab shows for the current query. */
export interface SearchView {
  query: BrowseQuery;
  /** null until the first answer. */
  page: BrowsePage | null;
  /** Steam games with no known source release. */
  other: GameSummary[];
  /** Per provider; empty while the text is too short to search remotely. */
  remote: ProviderProgress[];
  /** The remote search is on its way. */
  searching: boolean;
  /** The whole remote search failed (a single provider failing is in `remote`). */
  remoteError: string;
}

export interface SearchDeps {
  browse(q: BrowseQuery): Promise<SearchResult>;
  search(q: BrowseQuery): Promise<SearchResult>;
  onChange(view: SearchView): void;
  onError?(message: string): void;
  debounceMs?: number;
  minChars?: number;
  setTimer?(fn: () => void, ms: number): unknown;
  clearTimer?(handle: unknown): void;
}

export interface SearchController {
  /** The query changed (text, a filter or the sort). */
  update(q: BrowseQuery): void;
  /** Remote progress from the store:search event. */
  progress(p: SearchProgress): void;
  /** The index changed: asks again for the current query's local matches. */
  refreshLocal(): void;
  /** Stops waiting; late answers are dropped. */
  dispose(): void;
}

const messageOf = (e: unknown) => (e instanceof Error ? e.message : String(e || "something went wrong"));

export function createSearchController(deps: SearchDeps): SearchController {
  const wait = deps.debounceMs ?? SEARCH_DEBOUNCE_MS;
  const minChars = deps.minChars ?? SEARCH_MIN_CHARS;
  const setTimer = deps.setTimer ?? ((fn, ms) => setTimeout(fn, ms));
  const clearTimer = deps.clearTimer ?? ((h) => clearTimeout(h as ReturnType<typeof setTimeout>));
  let seq = 0;
  let timer: unknown = null;
  let view: SearchView = { query: emptyQuery(), page: null, other: [], remote: [], searching: false, remoteError: "" };

  const emit = (next: Partial<SearchView>) => {
    view = { ...view, ...next };
    deps.onChange(view);
  };
  const stopTimer = () => {
    if (timer !== null) clearTimer(timer);
    timer = null;
  };

  return {
    update(q) {
      const mine = ++seq;
      stopTimer();
      const text = q.text.trim();
      const remote = text.length >= minChars;
      const sameText = view.query.text.trim() === text;
      // Remote answers belong to the text, not to the filters: keep them while
      // only a filter changes, drop them when the text does.
      emit({ query: q, searching: remote, remoteError: "", ...(sameText && remote ? {} : { other: [], remote: [] }) });
      deps.browse(q).then(
        (r) => {
          if (mine !== seq) return;
          emit({ page: r.page });
        },
        (e) => {
          if (mine !== seq) return;
          deps.onError?.(messageOf(e));
        },
      );
      if (!remote) {
        emit({ searching: false });
        return;
      }
      timer = setTimer(() => {
        timer = null;
        deps.search(q).then(
          (r) => {
            if (mine !== seq) return;
            // A failed provider is in `remote`; the local matches stay.
            emit({ page: r.page, other: r.other, remote: r.remote, searching: !r.complete });
          },
          (e) => {
            if (mine !== seq) return;
            emit({ searching: false, remoteError: messageOf(e) });
          },
        );
      }, wait);
    },
    refreshLocal() {
      const mine = seq;
      if (mine === 0) return;
      deps.browse(view.query).then(
        (r) => mine === seq && emit({ page: r.page }),
        () => {},
      );
    },
    progress(p) {
      if (p.text.trim() !== view.query.text.trim() || !view.searching) return;
      emit({ remote: p.remote });
    },
    dispose() {
      seq++;
      stopTimer();
    },
  };
}

/** A second page of local results, or the first page again when the list is a prefix of it. */
export function adoptPage(current: GameSummary[], incoming: BrowsePage): GameSummary[] {
  if (current.length <= incoming.games.length) return incoming.games;
  const same = incoming.games.every((g, i) => current[i]?.key === g.key);
  return same ? [...incoming.games, ...current.slice(incoming.games.length)] : incoming.games;
}

/** The same query without paging, to tell old answers from new ones. */
export const queryKey = (q: BrowseQuery) => JSON.stringify({ ...q, offset: 0 });

// ---------------------------------------------------------------------------
// Which state the Store is in.

export type StoreMode = "loading" | "setup" | "ready" | "finding" | "unreachable" | "off";

export interface ModeInput {
  loaded: boolean;
  status: DiscoveryStatus | null;
  /** Games the front page or the catalog show right now (also from feeds). */
  shown: number;
}

/** Empty states follow what is actually there, not whether a feed is configured. */
export function storeMode({ loaded, status, shown }: ModeInput): StoreMode {
  if (!loaded || !status) return "loading";
  if (status.setupNeeded) return "setup";
  if (shown > 0) return "ready";
  if (!status.enabled) return "off";
  const on = status.sources.filter((s) => s.enabled);
  if (status.refreshing || on.some((s) => s.recentAt === 0 && !s.error)) return "finding";
  if (on.length && on.every((s) => s.error)) return "unreachable";
  return "finding";
}

/** "in 10 min" for a time ahead. */
export function inText(unix: number, now = Date.now() / 1000): string {
  const s = Math.max(0, Math.round(unix - now));
  if (s < 90) return "in a minute";
  if (s < 3600) return `in ${Math.round(s / 60)} min`;
  return `in ${Math.round(s / 3600)} h`;
}

/** The short status under the toolbar. */
export function statusSummary(status: DiscoveryStatus | null): { text: string; tone: "ok" | "busy" | "warn" } {
  if (!status || !status.enabled) return { text: "", tone: "ok" };
  const on = status.sources.filter((s) => s.enabled);
  if (status.refreshing) return { text: "Checking for new releases…", tone: "busy" };
  if (on.some((s) => s.error)) return { text: status.stale ? "Showing cached results. A source couldn't be reached." : "A source couldn't be reached.", tone: "warn" };
  if (status.stale) return { text: "Showing cached results", tone: "warn" };
  if (on.some((s) => s.state === "backfill")) return { text: "Indexing older releases", tone: "busy" };
  return { text: "Up to date", tone: "ok" };
}

// ---------------------------------------------------------------------------
// Cards and labels.

/** Reviews and chart place from the enrichment, where the summary lacks them. */
export function withEnrichment(g: GameSummary, e?: Enrichment): GameSummary {
  if (!e) return g;
  const o = e.reviews?.overall;
  const useReviews = !g.reviewTotal && o && o.total > 0 && (e.reviews.state === "ok" || e.reviews.state === "stale");
  return {
    ...g,
    popularRank: g.popularRank || e.popularRank || 0,
    ...(useReviews ? { reviewPercent: o.percent, reviewTotal: o.total, reviewLabel: o.label } : {}),
  };
}

/** The line under a card's title. Unknown parts are left out. */
export function cardLine(g: GameSummary): string {
  if (!g.sourceBacked) return "No known source release";
  return [g.version, bytes(g.sizeBytes)].filter(Boolean).join(" · ");
}

const dateFmt = (unix: number) => new Date(unix * 1000).toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" });

/** "12 Sep 2026", or "" when unknown. */
export const dateText = (unix: number) => (unix > 0 ? dateFmt(unix) : "");

/** "Published 12 Sep 2026": the source's date, never the game's release date. */
export const publishedText = (unix: number) => (unix > 0 ? `Published ${dateFmt(unix)}` : "");

const builtinNames: Record<string, string> = { fitgirl: "FitGirl", dodi: "DODI", feeds: "Feeds", steam: "Steam" };
const providerNames = new Map<string, string>();

/** Remembers the names the source registry gave, so a new provider needs no change here. */
export function learnSources(list: Pick<DiscoverySourceStatus, "id" | "name">[]) {
  for (const s of list) if (s.name) providerNames.set(s.id, s.name);
}

/** Why a game with source releases can't be installed from its card, when the sources say so; null otherwise. */
export const notInstallableText = (g: Pick<GameSummary, "browserOnly" | "announced">): string | null => (g.announced ? "Announced" : g.browserOnly ? "Opens in your browser" : null);

export const sourceLabel = (id: string): string => providerNames.get(id) ?? builtinNames[id] ?? id;

/** The source filters: the registry's enabled providers, then the games from feeds. */
export function sourceFilters(status: Pick<DiscoveryStatus, "sources"> | null): { id: string; label: string }[] {
  const on = (status?.sources ?? []).filter((s) => s.enabled).map((s) => ({ id: s.id, label: s.name || sourceLabel(s.id) }));
  return [...on, { id: "feeds", label: "Feeds" }];
}

/** "Steam: Very Positive (92%)", or "" without reviews. */
export function reviewText(g: Pick<GameSummary, "reviewLabel" | "reviewPercent" | "reviewTotal">): string {
  if (!g.reviewTotal) return "";
  return [g.reviewLabel, `${g.reviewPercent}%`].filter(Boolean).join(" · ");
}

/** "612,400 reviews". */
export const countText = (n: number, one: string, many = `${one}s`) => `${n.toLocaleString("en")} ${n === 1 ? one : many}`;

/** Minutes as hours: "45 min", "12 h", "12½ h". "" when unknown. */
export function hoursText(minutes: number): string {
  if (!(minutes > 0)) return "";
  if (minutes < 60) return `${Math.round(minutes)} min`;
  const h = Math.round((minutes / 60) * 2) / 2;
  const whole = Math.floor(h);
  return `${whole || ""}${h % 1 ? "½" : ""} h`;
}

// HowLongToBeat lists DLC and mods next to the game they belong to; a plain
// game needs no label.
const completionKinds: Record<string, string> = { dlc: "DLC", mod: "Mod", hack: "ROM hack", multi: "Multiplayer", compilation: "Collection" };

/** The label for a HowLongToBeat entry's kind, or "" for a game. */
export function completionKind(type: string): string {
  const t = type.trim().toLowerCase();
  if (!t || t === "game") return "";
  return completionKinds[t] ?? t.charAt(0).toUpperCase() + t.slice(1);
}

const availabilityLabels: Record<ReleaseAvailability, string> = {
  installable: "Installable",
  unresolved: "Needs resolving",
  manual: "Browser only",
  "update-only": "Update only",
  preview: "Announced",
  summary: "Details loading",
  unavailable: "No longer listed",
};
export const availabilityLabel = (a: ReleaseAvailability) => availabilityLabels[a] ?? a;

/** A release is a game someone can install: not an update patch or an announcement, not gone, from a source with torrents. */
export const canPrepare = (r: Release) =>
  r.kind === "release" && !r.browserOnly && r.availability !== "update-only" && r.availability !== "preview" && r.availability !== "unavailable" && r.availability !== "summary";

/** Why a prepared release can't be installed, when the Go side gave no reason. */
export function notReadyText(p: Pick<PreparedRelease, "state" | "reason">): string {
  if (p.reason) return p.reason;
  switch (p.state) {
    case "update-only":
      return "This is a patch for a game you need to have already. It can't be installed on its own.";
    case "preview":
      return "The source only announces this release. There is nothing to download yet.";
    case "browser":
      return "This source offers its files through file hosts in your browser only. Seaglass can't download or install them.";
    case "unresolved":
      return "Seaglass couldn't get this release's torrent file yet.";
    default:
      return "This release isn't listed by its source anymore.";
  }
}

/** The languages a release states, or what the source claims, or that it doesn't say. */
export function languagesLine(r: Pick<Release, "languages" | "languageClaim">): string {
  if (r.languages.length) return r.languages.join(", ");
  return r.languageClaim ? `Claimed: ${r.languageClaim}` : "Languages not stated";
}

/** The size, marked when the source only gives a minimum. */
export function sizeLine(r: Pick<Release, "sizeBytes" | "sizeIsMinimum" | "sizeClaim">): string {
  if (!r.sizeBytes) return r.sizeClaim ? `Size claimed: ${r.sizeClaim}` : "Size not stated";
  return `${r.sizeIsMinimum ? "At least " : ""}${bytes(r.sizeBytes)}`;
}

/** What a provider is doing, for the search progress. */
export function providerText(p: ProviderProgress): string {
  switch (p.state) {
    case "loading":
      return "searching…";
    case "ok":
      return p.cached ? `cached (${p.found})` : `done (${p.found})`;
    case "stale":
      return p.cached ? `cached (${p.found})` : "cached";
    case "skipped":
      return "skipped";
    case "unavailable":
    case "error":
      return `failed${p.error ? `: ${p.error}` : ""}`;
  }
}

/** The provider list for the current text; none when the text is too short to search. */
export const remoteShown = (text: string, remote: ProviderProgress[]) => (text.trim().length >= SEARCH_MIN_CHARS ? remote : []);

/** How many wishlist entries have unread activity. */
export const unreadCount = (items: { unread: number }[]) => items.reduce((n, i) => n + i.unread, 0);

/** One line about a wishlist event. */
export function activityText(a: WishlistActivity): string {
  const what = a.kind === "available" ? "First release" : "Newer version";
  return [`${what} on ${a.sourceName || sourceLabel(a.source)}`, a.version, dateText(a.at)].filter(Boolean).join(" · ");
}

/** A state label for provider data that may be old: "Cached 3 h ago". */
export function freshness(state: ProviderState, fetchedAt: number, now = Date.now() / 1000): string {
  if (state === "loading") return "Loading…";
  if (state === "stale") return fetchedAt ? `Cached ${ago(fetchedAt, now).toLowerCase()}` : "Cached";
  return "";
}
