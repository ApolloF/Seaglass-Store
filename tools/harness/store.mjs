// The experimental store end to end in the real app: a feed served from
// here, a .torrent whose only source is this script (a web seed, so
// nothing comes from or goes to strangers), and then Get, download,
// safety checks, install into the games folder, and uninstall.
//
//   node store.mjs
//
// Needs qBittorrent 5 installed and a dev build (wails3 build).
import crypto from "node:crypto";
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { OUT, backupAppData, fakeGameExe, makeDevData, restoreAppData, sleep, startApp } from "./lib.mjs";

const NAME = "Tiny Store Game";
const out = path.join(OUT, "store");

// ---- bencode and the torrent ----

function bencode(v) {
  if (typeof v === "number") return Buffer.from(`i${v}e`);
  if (typeof v === "string" || Buffer.isBuffer(v)) {
    const b = Buffer.isBuffer(v) ? v : Buffer.from(v);
    return Buffer.concat([Buffer.from(`${b.length}:`), b]);
  }
  if (Array.isArray(v)) return Buffer.concat([Buffer.from("l"), ...v.map(bencode), Buffer.from("e")]);
  const keys = Object.keys(v).sort();
  return Buffer.concat([Buffer.from("d"), ...keys.flatMap((k) => [bencode(k), bencode(v[k])]), Buffer.from("e")]);
}

/** A multi-file torrent of dir, with a web seed at seedURL. */
function makeTorrent(dir, seedURL) {
  const files = fs.readdirSync(dir).sort();
  const data = Buffer.concat(files.map((f) => fs.readFileSync(path.join(dir, f))));
  const pieceLength = 256 * 1024;
  const pieces = [];
  for (let at = 0; at < data.length; at += pieceLength) pieces.push(crypto.createHash("sha1").update(data.subarray(at, at + pieceLength)).digest());
  const info = {
    name: path.basename(dir),
    "piece length": pieceLength,
    pieces: Buffer.concat(pieces),
    files: files.map((f) => ({ length: fs.statSync(path.join(dir, f)).size, path: [f] })),
  };
  return bencode({ info, "url-list": seedURL, "created by": "Seaglass harness" });
}

// ---- the server: feed, torrent and web seed (with ranges) ----

function serve(seedDir) {
  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      const url = decodeURIComponent(new URL(req.url, "http://x").pathname);
      if (url === "/feed.json") return send(res, 200, server.feed, "application/json");
      if (url === "/tiny.torrent") return send(res, 200, server.torrent, "application/x-bittorrent");
      if (url.startsWith("/seed/")) {
        const p = path.join(seedDir, ...url.slice("/seed/".length).split("/"));
        if (!p.startsWith(seedDir) || !fs.existsSync(p)) return send(res, 404, "not found");
        const all = fs.readFileSync(p);
        const m = /bytes=(\d+)-(\d*)/.exec(req.headers.range ?? "");
        if (!m) return send(res, 200, all, "application/octet-stream");
        const start = Number(m[1]);
        const end = m[2] ? Number(m[2]) : all.length - 1;
        res.writeHead(206, { "Content-Range": `bytes ${start}-${end}/${all.length}`, "Content-Length": end - start + 1, "Content-Type": "application/octet-stream" });
        return res.end(all.subarray(start, end + 1));
      }
      send(res, 404, "not found");
    });
    server.listen(0, "127.0.0.1", () => resolve(server));
  });
}

function send(res, code, body, type = "text/plain") {
  res.writeHead(code, { "Content-Type": type });
  res.end(body);
}

// ---- the run ----

fs.rmSync(out, { recursive: true, force: true });
const seedDir = path.join(out, "seed");
const gameDir = path.join(seedDir, NAME);
fs.mkdirSync(gameDir, { recursive: true });
fs.copyFileSync(fakeGameExe(), path.join(gameDir, "TinyStoreGame.exe"));
fs.writeFileSync(path.join(gameDir, "fakegame.txt"), "--run=3 --title=Tiny Store Game");
const exeSHA = crypto.createHash("sha256").update(fs.readFileSync(path.join(gameDir, "TinyStoreGame.exe"))).digest("hex");

const server = await serve(seedDir);
const base = `http://127.0.0.1:${server.address().port}`;
server.torrent = makeTorrent(gameDir, `${base}/seed/`);
server.feed = JSON.stringify({
  schema: 1,
  name: "Harness feed",
  items: [{ title: NAME, version: "v1.0", buildDate: "2026-10-02", sizeBytes: fs.statSync(path.join(gameDir, "TinyStoreGame.exe")).size, torrentUrl: `${base}/tiny.torrent`, installerType: "portable", sha256: exeSHA, languages: ["English"] }],
});

const downloads = path.join(out, "downloads");
const games = path.join(out, "games");
backupAppData();
let app;
let ok = false;
try {
  const data = makeDevData("store", {
    fake: false,
    settings: {
      experimentalStore: true,
      // Feed only: source setup answered with no sources, so nothing is indexed.
      store: { feeds: [{ url: `${base}/feed.json`, enabled: true, trust: 0 }], sources: [], sourceSetup: "done", downloads, games, keepDownloads: false, pauseWhilePlaying: false, blockDetections: true },
    },
  });
  app = await startApp({ data, pad: null });
  const page = app.page;
  await app.viewport(1600, 1000);
  const shot = (name) => app.shot(path.join(out, `${name}.png`));

  await page.locator("aside button", { hasText: /^\s*Store\s*$/ }).click();
  // Home's shelves go by source publication dates, which feed offers don't
  // have, so the feed game is found on Browse.
  await page.getByRole("tab", { name: /Browse/ }).click();
  const card = page.locator("#sf-panel-browse .card", { hasText: NAME });
  await card.waitFor({ timeout: 30000 });
  await shot("1-store");
  await card.click();
  await page.locator("main .get").click();
  await page.locator("[role=dialog] button", { hasText: /^\s*Download\s*$/ }).click();
  await shot("2-started");

  await page.locator("aside button", { hasText: "Downloads" }).click();
  const row = page.locator("main .item", { hasText: NAME });
  const t0 = Date.now();
  let status = "";
  while (Date.now() - t0 < 4 * 60_000) {
    status = (await row.locator(".status").textContent().catch(() => "")) ?? "";
    if (/^Installed/.test(status) || (await row.evaluate((e) => e.classList.contains("failed")).catch(() => false))) break;
    await sleep(1000);
  }
  await shot("3-downloads");
  console.log(`after ${Math.round((Date.now() - t0) / 1000)} s: ${status}`);
  if (!/^Installed/.test(status)) throw new Error(`not installed: ${status}`);

  const installed = path.join(games, NAME, "TinyStoreGame.exe");
  if (!fs.existsSync(installed)) throw new Error(`no ${installed}`);
  const settings = JSON.parse(fs.readFileSync(path.join(data, "settings.json"), "utf8"));
  if (!settings.folders?.some((f) => f.toLowerCase() === games.toLowerCase())) throw new Error(`games folder not in the library's folders: ${settings.folders}`);
  console.log("installed into", path.dirname(installed), "and the games folder is a library folder");

  await row.locator("button", { hasText: /^Uninstall$/ }).click();
  await row.locator(".confirm button", { hasText: /^Uninstall$/ }).click();
  for (let k = 0; k < 60 && fs.existsSync(installed); k++) await sleep(500);
  if (fs.existsSync(installed)) throw new Error("still installed after uninstalling");
  console.log("uninstalled");
  await shot("4-uninstalled");
  ok = true;
} finally {
  await app?.quit();
  server.close();
  restoreAppData();
}
console.log(ok ? "store: ok" : "store: failed", "· screenshots in", out);
process.exit(ok ? 0 : 1);
