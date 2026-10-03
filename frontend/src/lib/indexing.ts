// How background indexing and the Steam wishlist import are put into words
// for the Store's status bar, source setup, Settings and the wishlist.
import { countText, statusSummary } from "./storefront";
import type { DiscoverySourceStatus, DiscoveryStatus, WishlistImport } from "./types";

export type Tone = "ok" | "busy" | "warn" | "idle";

/** "14:05" for a unix time. */
export const clockText = (unix: number) => new Date(unix * 1000).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });

/** The short status under the Store's toolbar; pausing and playing come first. */
export function indexingSummary(status: DiscoveryStatus | null): { text: string; tone: Tone } {
  if (status?.enabled && status.paused) return { text: "Indexing is paused", tone: "idle" };
  if (status?.enabled && status.playing) return { text: "Indexing waits for your game to end", tone: "idle" };
  return statusSummary(status);
}

/** What a chosen provider is doing now. */
export function providerState(s: DiscoverySourceStatus, status: Pick<DiscoveryStatus, "paused" | "playing">, now = Date.now() / 1000): string {
  if (!s.enabled) return "Off";
  if (status.paused) return "Paused";
  if (status.playing) return "Waiting for your game to end";
  if (s.state === "recent") return "Checking for new releases";
  if (s.retryAt > now) return `Backing off until ${clockText(s.retryAt)}`;
  if (s.state === "backfill" || !s.backfillDone) return "Indexing";
  return "Up to date";
}

/** How far indexing of older releases got. */
export function providerProgress(s: DiscoverySourceStatus): string {
  if (s.backfillDone) return "Complete";
  return s.backfillPage ? `Page ${s.backfillPage}` : "Not started";
}

/** "1,234 releases · Page 14 · Indexing". */
export function providerLine(s: DiscoverySourceStatus, status: Pick<DiscoveryStatus, "paused" | "playing">, now = Date.now() / 1000): string {
  if (!s.enabled) return "Off";
  return [countText(s.releases, "release"), providerProgress(s), providerState(s, status, now)].join(" · ");
}

/** What a provider can't do, then its notes. */
export function providerTraits(s: Pick<DiscoverySourceStatus, "search" | "torrents" | "notes">): string[] {
  return [...(s.search ? [] : ["No site search"]), ...(s.torrents ? [] : ["Releases open in your browser"]), ...s.notes];
}

/** "Imported 12 games: 9 new, 3 already saved. 4 have releases now; 5 are being searched." */
export function importLine(r: WishlistImport): string {
  if (!r.fetched) return "The Steam wishlist is empty.";
  const head = `Imported ${countText(r.fetched, "game")}: ${r.added} new, ${r.existing} already saved.`;
  const tail = [r.available ? `${r.available} ${r.available === 1 ? "has" : "have"} releases now` : "", r.searching ? `${r.searching} ${r.searching === 1 ? "is" : "are"} being searched` : ""].filter(Boolean);
  return tail.length ? `${head} ${tail.join("; ")}.` : head;
}
