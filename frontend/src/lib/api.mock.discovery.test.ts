import { describe, expect, it } from "vitest";
import { mockBrowse, mockDiscovery, mockDiscoveryFlag } from "./api.mock.discovery";
import { mockStoreSettings } from "./api.mock.store";
import type { BrowseQuery, Settings } from "./types";

// The mock is the contract the desktop Store is built against; these keep
// its answers honest about unknown values and states.
const settings = { experimentalStore: true, store: { ...mockStoreSettings } } as Settings;
const q = (p: Partial<BrowseQuery> = {}): BrowseQuery => ({ text: "", sources: [], language: "", genre: "", availability: "", installed: "", sort: "title", offset: 0, limit: 60, ...p });

describe("mock discovery", () => {
  it("pages source-backed games in groups of 60 at most", () => {
    const page = mockBrowse(settings, q({ limit: 5 }));
    expect(page.games).toHaveLength(5);
    expect(page.total).toBeGreaterThan(5);
    expect(page.games.every((g) => g.sourceBacked)).toBe(true);
  });

  it("counts games left out only because their languages are unknown", () => {
    const page = mockBrowse(settings, q({ language: "English" }));
    expect(page.unknown).toBeGreaterThan(0);
    expect(page.games.every((g) => g.languages.includes("English"))).toBe(true);
  });

  it("puts unranked games after the chart, never inventing a rank", () => {
    const page = mockBrowse(settings, q({ sort: "popular" }));
    const ranks = page.games.map((g) => g.popularRank);
    const firstUnranked = ranks.indexOf(0);
    expect(firstUnranked).toBeGreaterThan(0);
    expect(ranks.slice(firstUnranked).every((r) => r === 0)).toBe(true);
  });

  it("finds Steam-only games as other games that can't be installed", async () => {
    let s = settings;
    const api = mockDiscovery(() => s, (v) => (s = v), () => { throw new Error("no downloads here"); });
    const r = await api.discovery.search(q({ text: "hollow" }));
    expect(r.other.map((g) => g.title)).toContain("Hollow Tide II");
    expect(r.other.every((g) => !g.sourceBacked && !g.installable)).toBe(true);
    expect(r.complete).toBe(true);
  });

  it("keeps the chart's empty state when it is unavailable", async () => {
    let s = settings;
    const api = mockDiscovery(() => s, (v) => (s = v), () => { throw new Error("no downloads here"); });
    mockDiscoveryFlag("nochart", true);
    try {
      const home = await api.discovery.home();
      expect(home.popularState).toBe("unavailable");
      expect(home.popular).toHaveLength(0);
    } finally {
      mockDiscoveryFlag("nochart", false);
    }
  });
});
