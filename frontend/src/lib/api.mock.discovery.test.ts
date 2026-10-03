import { describe, expect, it } from "vitest";
import { mockBrowse, mockDiscovery, mockDiscoveryFlag, validSteamID64 } from "./api.mock.discovery";
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

describe("mock indexing and Steam import", () => {
  const make = () => {
    let s = structuredClone(settings);
    const api = mockDiscovery(() => s, (v) => (s = v), () => { throw new Error("no downloads here"); });
    return { api, get: () => s };
  };

  it("pauses and resumes indexing, refusing a refresh while paused", async () => {
    const { api } = make();
    const st = await api.discovery.pauseIndexing(true);
    expect(st.paused).toBe(true);
    expect(st.sources.filter((x) => x.enabled).every((x) => x.state === "paused")).toBe(true);
    await expect(api.discovery.refresh()).rejects.toThrow("Indexing is paused");
    expect((await api.discovery.pauseIndexing(false)).paused).toBe(false);
  });

  it("lists ElAmigos off, unsearched and browser only, as the registry does", async () => {
    const { api } = make();
    const elamigos = (await api.discovery.status()).sources.find((x) => x.id === "elamigos");
    expect(elamigos).toMatchObject({ name: "ElAmigos", enabled: false, defaultOn: false, search: false, paged: false, torrents: false });
    expect(elamigos?.notes.length).toBeGreaterThan(0);
  });

  it("accepts exactly the individual SteamID64 range", () => {
    expect(validSteamID64("76561197960265729")).toBe(true);
    expect(validSteamID64("76561202255233023")).toBe(true);
    expect(validSteamID64("76561202000000000")).toBe(true);
    expect(validSteamID64("76561197960265728")).toBe(false);
    expect(validSteamID64("76561202255233024")).toBe(false);
    expect(validSteamID64("76561190000000000")).toBe(false);
    expect(validSteamID64("7656119800000004x")).toBe(false);
    expect(validSteamID64("")).toBe(false);
  });

  it("imports by AppID, keeps saved games and reports private profiles", async () => {
    const { api } = make();
    await expect(api.wishlist.importSteam("123")).rejects.toThrow("SteamID64");
    const r = await api.wishlist.importSteam("76561198000000042");
    expect(r.fetched).toBe(4);
    expect(r.added + r.existing).toBe(4);
    expect(r.items.filter((i) => i.origin === "steam").length).toBeGreaterThanOrEqual(r.added);
    expect(r.items.some((i) => i.key === "steam:3100100")).toBe(true);
    const again = await api.wishlist.importSteam("76561198000000042");
    expect(again.added).toBe(0);
    expect(again.existing).toBe(4);
    mockDiscoveryFlag("private", true);
    try {
      await expect(api.wishlist.importSteam("76561198000000042")).rejects.toThrow("Public");
    } finally {
      mockDiscoveryFlag("private", false);
    }
  });
});
