import { describe, expect, it, vi } from "vitest";
import { candidateTimes, completionLink, completionView, FETCH_DELAY_MS, hoursLabel, isFor, searchUrl, timeRows, watchCompletion } from "./completion";
import type { Completion, LibraryCompletion } from "./types";

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

const answerOf = (gameId: number, main: number, state: Completion["state"] = "ok"): LibraryCompletion => ({
  gameId,
  key: `title:${gameId}`,
  completion: { hltbId: main ? 1 : 0, main, mainExtras: 0, completionist: 0, url: "", corrected: false, fetchedAt: 0, state },
});

describe("isFor", () => {
  it("accepts only an answer for the game on screen", () => {
    expect(isFor(1, answerOf(1, 60))).toBe(true);
    expect(isFor(2, answerOf(1, 60))).toBe(false);
  });
});

describe("watchCompletion", () => {
  const setup = () => {
    vi.useFakeTimers();
    const got: LibraryCompletion[] = [];
    let failed = 0;
    const get = vi.fn(async (id: number, fetch: boolean) => answerOf(id, fetch ? 600 : 300));
    return { got, get, on: { answer: (a: LibraryCompletion) => got.push(a), failed: () => failed++ }, failedCount: () => failed };
  };

  it("shows the cached answer at once and fetches only after the game stayed selected", async () => {
    const w = setup();
    watchCompletion(1, w.get, w.on);
    await vi.advanceTimersByTimeAsync(0);
    expect(w.got.map((a) => a.completion.main)).toEqual([300]);
    expect(w.get).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(FETCH_DELAY_MS);
    expect(w.get).toHaveBeenLastCalledWith(1, true);
    expect(w.got.map((a) => a.completion.main)).toEqual([300, 600]);
    vi.useRealTimers();
  });

  it("never fetches for a game that was left before the delay", async () => {
    const w = setup();
    const stop = watchCompletion(1, w.get, w.on);
    await vi.advanceTimersByTimeAsync(FETCH_DELAY_MS - 1);
    stop();
    await vi.advanceTimersByTimeAsync(FETCH_DELAY_MS);
    expect(w.get).not.toHaveBeenCalledWith(1, true);
    vi.useRealTimers();
  });

  it("drops a late answer once the game is left or it is for another game", async () => {
    const w = setup();
    let release: (a: LibraryCompletion) => void = () => {};
    w.get.mockImplementation((id, fetch) => (fetch ? new Promise<LibraryCompletion>((r) => (release = r)) : Promise.resolve(answerOf(id, 0, "loading"))));
    const stop = watchCompletion(1, w.get, w.on);
    await vi.advanceTimersByTimeAsync(FETCH_DELAY_MS);
    stop();
    release(answerOf(1, 600));
    await vi.advanceTimersByTimeAsync(0);
    expect(w.got.filter((a) => a.completion.main > 0)).toEqual([]);

    const other = setup();
    other.get.mockImplementation(async (_id, fetch) => answerOf(2, fetch ? 600 : 300));
    watchCompletion(1, other.get, other.on);
    await vi.advanceTimersByTimeAsync(FETCH_DELAY_MS);
    expect(other.got).toEqual([]);
    vi.useRealTimers();
  });

  it("keeps the cached times when the refresh fails, and reports a failure only without them", async () => {
    const w = setup();
    w.get.mockImplementation(async (id, fetch) => {
      if (fetch) throw new Error("offline");
      return answerOf(id, 300);
    });
    watchCompletion(1, w.get, w.on);
    await vi.advanceTimersByTimeAsync(FETCH_DELAY_MS);
    expect(w.failedCount()).toBe(0);

    const none = setup();
    none.get.mockRejectedValue(new Error("offline"));
    watchCompletion(1, none.get, none.on);
    await vi.advanceTimersByTimeAsync(FETCH_DELAY_MS);
    expect(none.failedCount()).toBe(1);
    vi.useRealTimers();
  });
});
