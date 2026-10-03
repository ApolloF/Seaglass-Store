import { describe, expect, it } from "vitest";
import { clockText, importLine, indexingSummary, providerLine, providerProgress, providerState, providerTraits } from "./indexing";
import type { DiscoverySourceStatus, DiscoveryStatus } from "./types";

const now = 1_800_000_000;
const src = (o: Partial<DiscoverySourceStatus> = {}): DiscoverySourceStatus => ({
  id: "dodi",
  name: "DODI",
  enabled: true,
  state: "idle",
  releases: 1234,
  recentAt: now - 600,
  backfillPage: 14,
  backfillDone: false,
  retryAt: 0,
  host: "dodi-repacks.site",
  search: true,
  paged: true,
  torrents: true,
  defaultOn: true,
  notes: [],
  ...o,
});
const status = (o: Partial<DiscoveryStatus> = {}): DiscoveryStatus => ({
  enabled: true,
  setupNeeded: false,
  sources: [src()],
  games: 10,
  releases: 1234,
  refreshing: false,
  paused: false,
  playing: false,
  stale: false,
  ...o,
});
const idle = { paused: false, playing: false };

describe("indexingSummary", () => {
  it("says indexing is paused before anything else", () => {
    expect(indexingSummary(status({ paused: true, refreshing: true }))).toEqual({ text: "Indexing is paused", tone: "idle" });
  });
  it("says indexing waits while a game runs", () => {
    expect(indexingSummary(status({ playing: true }))).toEqual({ text: "Indexing waits for your game to end", tone: "idle" });
  });
  it("falls back to the Store's summary", () => {
    expect(indexingSummary(status()).text).toBe("Up to date");
    expect(indexingSummary(null).text).toBe("");
  });
});

describe("providerState", () => {
  it("names pausing, playing, refreshing, backing off and indexing", () => {
    expect(providerState(src(), { paused: true, playing: true }, now)).toBe("Paused");
    expect(providerState(src(), { paused: false, playing: true }, now)).toBe("Waiting for your game to end");
    expect(providerState(src({ state: "recent" }), idle, now)).toBe("Checking for new releases");
    expect(providerState(src({ state: "backoff", retryAt: now + 600 }), idle, now)).toBe(`Backing off until ${clockText(now + 600)}`);
    expect(providerState(src({ state: "backfill" }), idle, now)).toBe("Indexing");
    expect(providerState(src({ backfillDone: true }), idle, now)).toBe("Up to date");
    expect(providerState(src({ enabled: false }), idle, now)).toBe("Off");
  });
  it("treats a passed backoff as indexing again", () => {
    expect(providerState(src({ retryAt: now - 5 }), idle, now)).toBe("Indexing");
  });
});

describe("providerProgress and providerLine", () => {
  it("shows the page or that the catalog is complete", () => {
    expect(providerProgress(src())).toBe("Page 14");
    expect(providerProgress(src({ backfillDone: true }))).toBe("Complete");
    expect(providerProgress(src({ backfillPage: 0 }))).toBe("Not started");
  });
  it("joins releases, progress and state", () => {
    expect(providerLine(src({ state: "backfill" }), idle, now)).toBe("1,234 releases · Page 14 · Indexing");
    expect(providerLine(src({ releases: 1, backfillDone: true }), idle, now)).toBe("1 release · Complete · Up to date");
  });
});

describe("providerTraits", () => {
  it("lists what a provider can't do, then its notes", () => {
    expect(providerTraits({ search: true, torrents: true, notes: [] })).toEqual([]);
    expect(providerTraits({ search: false, torrents: false, notes: ["Lists one page."] })).toEqual(["No site search", "Releases open in your browser", "Lists one page."]);
  });
});

describe("importLine", () => {
  const r = { steamId: "76561198000000042", fetched: 12, added: 9, existing: 3, available: 4, searching: 5, items: [] };
  it("counts what the import did", () => {
    expect(importLine(r)).toBe("Imported 12 games: 9 new, 3 already saved. 4 have releases now; 5 are being searched.");
    expect(importLine({ ...r, fetched: 1, added: 0, existing: 1, available: 1, searching: 0 })).toBe("Imported 1 game: 0 new, 1 already saved. 1 has releases now.");
    expect(importLine({ ...r, available: 0, searching: 0 })).toBe("Imported 12 games: 9 new, 3 already saved.");
  });
  it("says when the wishlist is empty", () => {
    expect(importLine({ ...r, fetched: 0 })).toBe("The Steam wishlist is empty.");
  });
});
