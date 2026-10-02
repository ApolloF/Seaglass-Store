// What the store's catalog and feed list say: pure, so it can be tested.
import { ago, bytes } from "./format";
import type { CatalogEntry, CatalogOffer, FeedInfo } from "./types";

/** A feed's line under its name. */
export function feedLine(f: FeedInfo, now = Date.now() / 1000): string {
  if (f.error && !f.fetched) return f.error;
  const parts = [`${f.items} ${f.items === 1 ? "game" : "games"}`];
  if (f.skipped) parts.push(`${f.skipped} left out (they didn't pass the checks)`);
  parts.push(`fetched ${ago(f.fetched, now).toLowerCase()}`);
  if (f.error) parts.push(`last try failed: ${f.error}`);
  return parts.join(" · ");
}

/** A short list of languages: "English, German +3". */
export function languagesText(langs: string[], show = 2): string {
  if (langs.length <= show + 1) return langs.join(", ");
  return `${langs.slice(0, show).join(", ")} +${langs.length - show}`;
}

/** One line about a catalog entry. */
export function entryLine(e: CatalogEntry): string {
  const feeds = new Set(e.offers.map((o) => o.feedName)).size;
  return [e.version, bytes(e.size), languagesText(e.languages), feeds > 1 ? `${feeds} feeds` : e.offers[0]?.feedName].filter(Boolean).join(" · ");
}

const installers: Record<string, string> = { inno: "Inno Setup installer", nsis: "NSIS installer", msi: "Windows Installer", archive: "Archive", portable: "No install needed" };

/** One line about an offer. */
export function offerLine(o: CatalogOffer): string {
  return [o.buildDate, bytes(o.sizeBytes), o.installerType && installers[o.installerType], o.feedName].filter(Boolean).join(" · ");
}
