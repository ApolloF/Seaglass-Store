import { describe, expect, it } from "vitest";
import { activeCount, eta, needsYou, progress, statusLine } from "./downloads";
import type { Download, EngineStatus } from "./types";

const d = (p: Partial<Download>): Download => ({
  id: "sg-1", title: "T", source: "magnet:?", savePath: "C:\D", state: "downloading",
  size: 2e9, done: 5e8, downSpeed: 0, upSpeed: 0, seeds: 0, peers: 0, eta: 0, seeding: false, created: 0, ...p,
});
const running: EngineStatus = { installed: true, exe: "q.exe", running: true, interfaceMissing: false, gameRunning: false, held: false };

describe("downloads", () => {
  it("counts what's still on its way", () => {
    expect(activeCount([d({}), d({ state: "queued" }), d({ state: "paused" }), d({ state: "downloaded" }), d({ state: "installing" }), d({ state: "installed" })])).toBe(3);
  });

  it("says what happens after downloading", () => {
    const report = (verdict: "clean" | "warn" | "block") => ({ verdict, findings: [], checked: 1 });
    expect(statusLine(d({ state: "scanning" }), running)).toBe("Downloaded · running the safety checks…");
    expect(statusLine(d({ state: "downloaded", safety: report("clean") }), running)).toBe("Downloaded and checked · ready to install");
    expect(statusLine(d({ state: "downloaded", safety: report("warn") }), running)).toBe("Downloaded · the safety checks have warnings");
    expect(statusLine(d({ state: "blocked", safety: report("block") }), running)).toMatch(/found a problem/);
    expect(statusLine(d({ state: "installing", installDone: 3e9 }), running)).toBe("Installing… · 3.0 GB so far");
    expect(statusLine(d({ state: "installing", stalled: true }), running)).toMatch(/no progress/);
    expect(statusLine(d({ state: "installed", installDir: "D:\\Games\\T" }), running)).toBe("Installed in D:\\Games\\T");
    expect(needsYou(d({ state: "downloaded", safety: report("warn"), autoInstall: true }))).toBe(true);
    expect(needsYou(d({ state: "downloaded", safety: report("clean"), autoInstall: true }))).toBe(false);
  });

  it("shows progress, done or not", () => {
    expect(progress(d({}))).toBe(0.25);
    expect(progress(d({ size: 0, state: "downloaded" }))).toBe(1);
    expect(progress(d({ size: 0 }))).toBe(0);
  });

  it("says how long is left", () => {
    expect(eta(0)).toBe("");
    expect(eta(30)).toBe("under a minute");
    expect(eta(600)).toBe("10 min");
    expect(eta(3900)).toBe("1 h 5 min");
    expect(eta(7200)).toBe("2 h");
  });

  it("says why a download stands still", () => {
    expect(statusLine(d({ downSpeed: 4e6, eta: 600 }), running)).toBe("500 MB of 2.0 GB · 4.0 MB/s · 10 min left");
    expect(statusLine(d({}), { ...running, interfaceMissing: true })).toMatch(/interface/);
    expect(statusLine(d({}), { ...running, gameRunning: true })).toBe("Waiting for your game to close");
    expect(statusLine(d({}), { ...running, held: true })).toBe("Paused until you resume all downloads · 500 MB of 2.0 GB");
    expect(statusLine(d({}), { ...running, running: false, error: "qBittorrent isn't installed" })).toBe("Waiting: qBittorrent isn't installed");
    expect(statusLine(d({ engine: "metadata", size: 0, done: 0 }), running)).toBe("Asking peers for the file list…");
    expect(statusLine(d({ seeds: 2 }), running)).toBe("500 MB of 2.0 GB · 2 peers, no data yet");
  });

  it("describes finished, paused and failed downloads whatever the engine does", () => {
    const off = { ...running, running: false };
    expect(statusLine(d({ state: "paused" }), off)).toBe("Paused · 500 MB of 2.0 GB");
    expect(statusLine(d({ state: "downloaded", done: 2e9, seeding: true, upSpeed: 2e5 }), running)).toBe("Downloaded · 2.0 GB · sharing at 200 KB/s");
    expect(statusLine(d({ state: "failed", error: "Disk full" }), off)).toBe("Disk full");
  });
});
