import { describe, expect, it } from "vitest";
import { entryLine, feedLine, languagesText, offerLine } from "./catalog";
import type { CatalogEntry, CatalogOffer, FeedInfo } from "./types";

const now = new Date(2026, 9, 2, 12).getTime() / 1000;
const feed = (p: Partial<FeedInfo>): FeedInfo => ({ url: "https://a/f.json", name: "A", items: 3, skipped: 0, fetched: now - 600, enabled: true, ...p });
const offer = (p: Partial<CatalogOffer>): CatalogOffer => ({ title: "T", feedUrl: "https://a/f.json", feedName: "A", ...p });

describe("catalog", () => {
  it("says how a feed's last fetch went", () => {
    expect(feedLine(feed({}), now)).toBe("3 games · fetched today");
    expect(feedLine(feed({ items: 1, skipped: 2 }), now)).toBe("1 game · 2 left out (they didn't pass the checks) · fetched today");
    expect(feedLine(feed({ fetched: 0, error: "the feed answered 404 Not Found" }), now)).toBe("the feed answered 404 Not Found");
    expect(feedLine(feed({ error: "down" }), now)).toBe("3 games · fetched today · last try failed: down");
  });

  it("shortens long language lists", () => {
    expect(languagesText(["English"])).toBe("English");
    expect(languagesText(["English", "German", "French"])).toBe("English, German, French");
    expect(languagesText(["English", "German", "French", "Polish"])).toBe("English, German +2");
    expect(languagesText([])).toBe("");
  });

  it("describes entries and offers", () => {
    const e: CatalogEntry = {
      key: "k", title: "T", version: "v1.2", updated: "2026-09-01", size: 2e9, languages: ["English"],
      offers: [offer({ feedName: "A" }), offer({ feedName: "B" })],
    };
    expect(entryLine(e)).toBe("v1.2 · 2.0 GB · English · 2 feeds");
    expect(entryLine({ ...e, version: "", offers: [offer({})] })).toBe("2.0 GB · English · A");
    expect(offerLine(offer({ buildDate: "2026-09-01", sizeBytes: 5e8, installerType: "inno" }))).toBe("2026-09-01 · 500 MB · Inno Setup installer · A");
  });
});
