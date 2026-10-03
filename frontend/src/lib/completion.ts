// How a game's HowLongToBeat times are written and which state to show.
// Pure, so the rules are tested apart from the components.
import type { Completion, CompletionCandidate } from "./types";

/** Minutes as the hours people say: "45 min", "8 h", "12½ h". "" when unknown. */
export function hoursLabel(minutes: number): string {
  if (!(minutes > 0)) return "";
  if (minutes < 60) return `${Math.round(minutes)} min`;
  const h = Math.round((minutes / 60) * 2) / 2;
  return `${Math.floor(h)}${h % 1 ? "½" : ""} h`;
}

export interface TimeRow {
  label: string;
  text: string;
}

export type CompletionMode =
  /** Not asked yet, or being fetched. */
  | "loading"
  /** Times to show, possibly old. */
  | "times"
  /** Nothing known: say so and link to a search. */
  | "none";

export interface CompletionView {
  mode: CompletionMode;
  rows: TimeRow[];
  /** The times are cached and refreshing failed. */
  stale: boolean;
  /** The HowLongToBeat game the times belong to; "" when it is the obvious one. */
  matched: string;
}

/** The three times the person asked for, in order, leaving out unknown ones. */
export function timeRows(c: Pick<Completion, "main" | "mainExtras" | "completionist">): TimeRow[] {
  return [
    { label: "Main story", text: hoursLabel(c.main) },
    { label: "Main + extras", text: hoursLabel(c.mainExtras) },
    { label: "Completionist", text: hoursLabel(c.completionist) },
  ].filter((r) => r.text);
}

/** What to show for an answer; null means nothing has been asked yet. */
export function completionView(c: Completion | null): CompletionView {
  if (!c || c.state === "loading") return { mode: "loading", rows: [], stale: false, matched: "" };
  const rows = c.hltbId ? timeRows(c) : [];
  if (!rows.length) return { mode: "none", rows, stale: false, matched: "" };
  return { mode: "times", rows, stale: c.state === "stale", matched: c.corrected && c.title ? c.title : "" };
}

/** HowLongToBeat's own search, for when its answer is missing. */
export function searchUrl(title: string): string {
  return `https://howlongtobeat.com/?q=${encodeURIComponent(title.trim())}`;
}

/** The link to follow for an answer: its game page, else a search. */
export function completionLink(c: Completion | null, title: string): string {
  return c?.url || searchUrl(title);
}

/** One line of times for a candidate in the picker. */
export function candidateTimes(c: Pick<CompletionCandidate, "main" | "mainExtras" | "completionist">): string {
  const parts = [
    c.main > 0 ? `Main ${hoursLabel(c.main)}` : "",
    c.mainExtras > 0 ? `Extras ${hoursLabel(c.mainExtras)}` : "",
    c.completionist > 0 ? `Full ${hoursLabel(c.completionist)}` : "",
  ].filter(Boolean);
  return parts.join(" · ") || "No times given";
}
