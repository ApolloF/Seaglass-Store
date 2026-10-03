// The Store's front page, kept pure so it can be tested: which games a tab
// lists, which one the preview shows, and the labels around recommendations.
import { ago } from "./format";
import { hoursText, notInstallableText, publishedText, reviewText, sourceLabel } from "./storefront";
import type { GameSummary, Recommendation, RecommendationBasis, StoreHome } from "./types";

export type HomeTab = "new" | "popular" | "updated";

export const homeTabs: { id: HomeTab; label: string }[] = [
  { id: "new", label: "New releases" },
  { id: "popular", label: "Popular" },
  { id: "updated", label: "Recently updated" },
];

/** The games a tab lists, in the order the backend gave them. */
export function tabGames(home: Pick<StoreHome, "new" | "popular" | "updated">, tab: HomeTab): GameSummary[] {
  return home[tab];
}

/** What a tab says when it has no games. */
export function tabEmpty(tab: HomeTab, popularState: StoreHome["popularState"]): string {
  if (tab === "new") return "No releases yet.";
  if (tab === "updated") return "No releases were changed recently.";
  return popularState === "unavailable" ? "Steam's most-played chart isn't available right now." : "None of the games on Steam's most-played chart have a release here yet.";
}

/** The key the preview shows: the chosen game while it is still listed, else the first. */
export function previewKey(rows: GameSummary[], chosen: string): string {
  return rows.some((g) => g.key === chosen) ? chosen : (rows[0]?.key ?? "");
}

/** The index a list key moves to; -1 when the key is not a list key. */
export function moveIndex(current: number, key: string, length: number): number {
  if (length <= 0) return -1;
  switch (key) {
    case "ArrowDown":
      return Math.min(current + 1, length - 1);
    case "ArrowUp":
      return Math.max(current - 1, 0);
    case "Home":
      return 0;
    case "End":
      return length - 1;
  }
  return -1;
}

/** Steps through the featured games and wraps around. */
export const wrapIndex = (current: number, step: number, length: number) => (length > 0 ? (((current + step) % length) + length) % length : 0);

/** The shelf title: what the recommendations are based on. */
export function recommendationTitle(basis: RecommendationBasis, recs: Pick<Recommendation, "because">[]): string {
  switch (basis) {
    case "popular":
      return "Popular now";
    case "wishlist":
      return "Based on your wishlist";
    case "played+wishlist":
      return "Based on what you play and wishlist";
    case "played": {
      const [first, ...rest] = namedMost(recs);
      if (!first) return "Based on what you play";
      return `Because you played ${first}${rest.length ? ` and ${rest.length} more` : ""}`;
    }
  }
  return "";
}

/** The titles recommendations name, most often named first. */
function namedMost(recs: Pick<Recommendation, "because">[]): string[] {
  const count = new Map<string, number>();
  for (const r of recs) for (const t of r.because) count.set(t, (count.get(t) ?? 0) + 1);
  return [...count.keys()].sort((a, b) => (count.get(b) ?? 0) - (count.get(a) ?? 0));
}

/** The line under a recommended game: the genres it shares, and with what. */
export function recommendationWhy(r: Pick<Recommendation, "because" | "genres">): string {
  if (!r.because.length) return "";
  const genres = r.genres.slice(0, 2).join(" · ");
  const names = r.because.join(", ");
  return genres ? `${genres}, like ${names}` : `Like ${names}`;
}

const NEW_DAYS = 14;

/** A short, true reason to look at a featured game: chart place, how new it is, how it installs. */
export function featuredWhy(g: GameSummary, now = Date.now() / 1000): string {
  const parts: string[] = [];
  if (g.popularRank > 0) parts.push(`#${g.popularRank} on Steam's most-played chart`);
  if (g.publishedAt > 0 && now - g.publishedAt < NEW_DAYS * 86400) parts.push(`New: published ${ago(g.publishedAt, now).toLowerCase()}`);
  if (!parts.length) parts.push(g.installable ? "Ready to install" : (notInstallableText(g) ?? "Release found"));
  return parts.join(" · ");
}

/** How a game can be had: "Installable", or why it is not. */
export const availabilityText = (g: Pick<GameSummary, "installable" | "browserOnly" | "announced">) => (g.installable ? "Installable" : (notInstallableText(g) ?? "Not installable yet"));

/** "12 h main story", or "" when HowLongToBeat's time is not cached. */
export const mainStoryText = (g: Pick<GameSummary, "completionMain">) => (g.completionMain > 0 ? `${hoursText(g.completionMain)} main story` : "");

export interface RowFacts {
  sources: string;
  availability: string;
  version: string;
  review: string;
  mainStory: string;
  /** The source's publication date, labelled as the source's. */
  published: string;
}

/** What a compact row shows. Unknown facts are empty strings, never made up. */
export function rowFacts(g: GameSummary): RowFacts {
  return {
    sources: g.sources.map(sourceLabel).join(" · "),
    availability: availabilityText(g),
    version: g.version ?? "",
    review: reviewText(g),
    mainStory: mainStoryText(g),
    published: publishedText(g.publishedAt),
  };
}

/** Every shelf is a list, even when the backend had nothing to put in it. */
export function normalizeHome(h: StoreHome): StoreHome {
  return { ...h, new: h.new ?? [], popular: h.popular ?? [], updated: h.updated ?? [], wishlist: h.wishlist ?? [], featured: h.featured ?? [], recommended: h.recommended ?? [] };
}
