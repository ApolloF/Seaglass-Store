import { describe, expect, it } from "vitest";
import { candidateTimes, completionLink, completionView, hoursLabel, searchUrl, timeRows } from "./completion";
import type { Completion } from "./types";

const done = (extra: Partial<Completion> = {}): Completion => ({
  hltbId: 7, title: "Portal 2", main: 515, mainExtras: 826, completionist: 1376, url: "https://howlongtobeat.com/game/7", corrected: false, fetchedAt: 1, state: "ok", ...extra,
});

describe("hoursLabel", () => {
  it("writes minutes under an hour and half hours above it", () => {
    expect(hoursLabel(0)).toBe("");
    expect(hoursLabel(-5)).toBe("");
    expect(hoursLabel(45)).toBe("45 min");
    expect(hoursLabel(60)).toBe("1 h");
    expect(hoursLabel(515)).toBe("8½ h");
    expect(hoursLabel(750)).toBe("12½ h");
    expect(hoursLabel(1376)).toBe("23 h");
  });
});

describe("timeRows", () => {
  it("lists main story, main plus extras and completionist, leaving out unknown ones", () => {
    expect(timeRows(done()).map((r) => r.label)).toEqual(["Main story", "Main + extras", "Completionist"]);
    expect(timeRows(done({ mainExtras: 0 })).map((r) => r.label)).toEqual(["Main story", "Completionist"]);
  });
});

describe("completionView", () => {
  it("is loading until an answer arrives", () => {
    expect(completionView(null).mode).toBe("loading");
    expect(completionView(done({ state: "loading", hltbId: 0 })).mode).toBe("loading");
  });
  it("shows times for an answer and says old ones may be out of date", () => {
    expect(completionView(done())).toMatchObject({ mode: "times", stale: false });
    expect(completionView(done({ state: "stale" }))).toMatchObject({ mode: "times", stale: true });
  });
  it("names the game only when the person chose it", () => {
    expect(completionView(done()).matched).toBe("");
    expect(completionView(done({ corrected: true })).matched).toBe("Portal 2");
  });
  it("is unavailable without a match, without times, or on an error", () => {
    expect(completionView(done({ hltbId: 0, state: "unavailable" })).mode).toBe("none");
    expect(completionView(done({ hltbId: 0, state: "error" })).mode).toBe("none");
    expect(completionView(done({ main: 0, mainExtras: 0, completionist: 0 })).mode).toBe("none");
  });
});

describe("links", () => {
  it("searches HowLongToBeat for the title when there is no page", () => {
    expect(searchUrl(" Portal 2 ")).toBe("https://howlongtobeat.com/?q=Portal%202");
    expect(completionLink(done(), "Portal 2")).toBe("https://howlongtobeat.com/game/7");
    expect(completionLink(null, "Portal 2")).toBe("https://howlongtobeat.com/?q=Portal%202");
    expect(completionLink(done({ url: "" }), "Portal 2")).toBe("https://howlongtobeat.com/?q=Portal%202");
  });
});

describe("candidateTimes", () => {
  it("lists the times a candidate has", () => {
    expect(candidateTimes({ main: 600, mainExtras: 0, completionist: 1200 })).toBe("Main 10 h · Full 20 h");
    expect(candidateTimes({ main: 0, mainExtras: 0, completionist: 0 })).toBe("No times given");
  });
});
