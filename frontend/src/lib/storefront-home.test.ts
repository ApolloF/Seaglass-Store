import { describe, expect, it } from "vitest";
import { normalizeHome, availabilityText, featuredWhy, homeTabs, mainStoryText, moveIndex, previewKey, recommendationTitle, recommendationWhy, rowFacts, tabEmpty, tabGames, wrapIndex } from "./storefront-home";
import type { GameSummary } from "./types";

const game = (key: string, extra: Partial<GameSummary> = {}): GameSummary => ({
  key, title: key, sourceBacked: true, sources: ["fitgirl"], releases: 1, publishedAt: 0, updatedAt: 0, sizeBytes: 0, languages: [], genres: [],
  installable: true, browserOnly: false, announced: false, popularRank: 0, completionMain: 0, reviewPercent: 0, reviewTotal: 0, wishlisted: false, activity: false, ...extra,
});

describe("tabs", () => {
  const home = { new: [game("n")], popular: [game("p1"), game("p2")], updated: [] };
  it("lists new releases, popular games and recently updated ones", () => {
    expect(homeTabs.map((t) => t.id)).toEqual(["new", "popular", "updated"]);
    expect(tabGames(home, "popular").map((g) => g.key)).toEqual(["p1", "p2"]);
    expect(tabGames(home, "updated")).toEqual([]);
  });
  it("says why the popular tab is empty without inventing another ranking", () => {
    expect(tabEmpty("popular", "unavailable")).toMatch(/isn't available/);
    expect(tabEmpty("popular", "ok")).toMatch(/most-played chart have a release/);
  });
});

describe("preview selection", () => {
  const rows = [game("a"), game("b")];
  it("keeps the chosen game while it is listed and otherwise shows the first", () => {
    expect(previewKey(rows, "b")).toBe("b");
    expect(previewKey(rows, "gone")).toBe("a");
    expect(previewKey([], "a")).toBe("");
  });
  it("moves with arrows, Home and End and stops at the ends", () => {
    expect(moveIndex(0, "ArrowDown", 3)).toBe(1);
    expect(moveIndex(2, "ArrowDown", 3)).toBe(2);
    expect(moveIndex(0, "ArrowUp", 3)).toBe(0);
    expect(moveIndex(1, "End", 3)).toBe(2);
    expect(moveIndex(2, "Home", 3)).toBe(0);
    expect(moveIndex(1, "x", 3)).toBe(-1);
    expect(moveIndex(0, "ArrowDown", 0)).toBe(-1);
  });
  it("wraps through the featured games", () => {
    expect(wrapIndex(4, 1, 5)).toBe(0);
    expect(wrapIndex(0, -1, 5)).toBe(4);
    expect(wrapIndex(0, 1, 0)).toBe(0);
  });
});

describe("recommendation labels", () => {
  it("names what the played basis rests on, most named first", () => {
    const recs = [{ because: ["Hollow Tide", "Ember Crown"] }, { because: ["Hollow Tide", "Grimwald"] }, { because: ["Hollow Tide"] }];
    expect(recommendationTitle("played", recs)).toBe("Because you played Hollow Tide and 2 more");
    expect(recommendationTitle("played", [{ because: ["Hollow Tide"] }])).toBe("Because you played Hollow Tide");
  });
  it("labels the wishlist, both and the popular fallback", () => {
    expect(recommendationTitle("wishlist", [])).toBe("Based on your wishlist");
    expect(recommendationTitle("played+wishlist", [])).toBe("Based on what you play and wishlist");
    expect(recommendationTitle("popular", [])).toBe("Popular now");
    expect(recommendationTitle("", [])).toBe("");
  });
  it("explains a recommendation with shared genres and leaves popular ones unexplained", () => {
    expect(recommendationWhy({ because: ["Hollow Tide", "Ember Crown"], genres: ["Action", "RPG", "Indie"] })).toBe("Action · RPG, like Hollow Tide, Ember Crown");
    expect(recommendationWhy({ because: ["Hollow Tide"], genres: [] })).toBe("Like Hollow Tide");
    expect(recommendationWhy({ because: [], genres: [] })).toBe("");
  });
});

describe("featured line", () => {
  const now = 1_000_000_000;
  it("uses only what is known", () => {
    expect(featuredWhy(game("a", { popularRank: 4, publishedAt: now - 3 * 86400 }), now)).toBe("#4 on Steam's most-played chart · New: published 3 days ago");
    expect(featuredWhy(game("b", { publishedAt: now - 90 * 86400 }), now)).toBe("Ready to install");
    expect(featuredWhy(game("c", { installable: false }), now)).toBe("Release found");
  });
});

describe("facts", () => {
  it("leaves unknown facts empty", () => {
    const f = rowFacts(game("a"));
    expect([f.version, f.review, f.mainStory, f.published]).toEqual(["", "", "", ""]);
    expect(f.availability).toBe("Installable");
  });
  it("keeps the game's release date apart from the source's publication date", () => {
    const g = game("a", { sources: ["fitgirl", "dodi"], version: "v1.2", reviewTotal: 10, reviewPercent: 90, reviewLabel: "Very Positive", completionMain: 750, publishedAt: 1_700_000_000, releaseDate: "12 Mar, 2026" });
    expect(rowFacts(g)).toMatchObject({ sources: "FitGirl · DODI", version: "v1.2", review: "Very Positive · 90%", mainStory: "12½ h main story" });
    expect(rowFacts(g).published).toMatch(/^Published /);
  });
  it("says why a browser-only or announced game has no install", () => {
    expect(availabilityText({ installable: false, browserOnly: true, announced: false })).toBe("Opens in your browser");
    expect(availabilityText({ installable: false, browserOnly: false, announced: true })).toBe("Announced");
    expect(featuredWhy(game("a", { installable: false, browserOnly: true }), 1_800_000_000)).toBe("Opens in your browser");
    expect(featuredWhy(game("a", { installable: false, announced: true }), 1_800_000_000)).toBe("Announced");
  });
  it("calls a game without a validated torrent not installable yet", () => {
    expect(availabilityText({ installable: false, browserOnly: false, announced: false })).toBe("Not installable yet");
    expect(mainStoryText({ completionMain: 0 })).toBe("");
  });
});

describe("normalizeHome", () => {
  it("turns shelves the backend sent as null into empty lists", () => {
    const h = normalizeHome({ featured: null, recommended: null, new: null } as never);
    expect([h.featured, h.recommended, h.new, h.popular, h.updated, h.wishlist]).toEqual([[], [], [], [], [], []]);
  });
  it("keeps shelves that have games", () => {
    const g = game("a");
    expect(normalizeHome({ featured: [g] } as never).featured).toEqual([g]);
  });
});
