// Store discovery in the real app: the sources are indexed (one bounded
// pass each, metadata only), then search, a game's page, the wishlist and
// the install confirmation, which is closed without downloading. Each
// screen is shot in both themes at desktop size and at 390 px.
//
//   node discovery.mjs
//
// Needs a dev build (wails3 build) and the network: it reads FitGirl,
// DODI, Steam and HowLongToBeat like the Store does.
import fs from "node:fs";
import path from "node:path";
import { OUT, backupAppData, makeDevData, restoreAppData, sleep, startApp } from "./lib.mjs";

const out = path.join(OUT, "discovery");
fs.rmSync(out, { recursive: true, force: true });
fs.mkdirSync(out, { recursive: true });

const checks = [];
const check = (name, ok, detail = "") => {
  checks.push({ name, ok, detail });
  console.log(`${ok ? "ok  " : "FAIL"} ${name}${detail ? ` · ${detail}` : ""}`);
};

backupAppData();
let app;
try {
  const data = makeDevData("discovery", {
    fake: false,
    settings: { experimentalStore: true, store: { privateSources: true, sources: ["fitgirl", "dodi"], sourceSetup: "done", pauseWhilePlaying: true, blockDetections: true, language: "English" } },
  });
  app = await startApp({ data, pad: null });
  const page = app.page;
  await app.viewport(1400, 900);
  const theme = (t) => page.evaluate((t) => document.documentElement.setAttribute("data-theme", t), t);
  // Both themes, desktop and phone width (the app's sidebar has no phone
  // layout, so it is hidden for the 390 px shots, as the Store is what is checked).
  const shots = async (name) => {
    for (const t of ["dark", "light"]) {
      await theme(t);
      await app.viewport(1400, 900);
      await sleep(300);
      await app.shot(path.join(out, `${name}-${t}-1400.png`));
      await page.addStyleTag({ content: "aside.side{display:none!important}" }).catch(() => {});
      await app.viewport(390, 844);
      await sleep(300);
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - innerWidth);
      check(`${name} ${t} at 390 px has no horizontal scrolling`, overflow <= 0, `${overflow} px`);
      await app.shot(path.join(out, `${name}-${t}-390.png`));
      await page.evaluate(() => document.querySelectorAll("style").forEach((s) => s.textContent.includes("aside.side{display:none") && s.remove()));
    }
    await theme("dark");
    await app.viewport(1400, 900);
  };

  await page.locator("aside button", { hasText: /^\s*Store\s*$/ }).click();
  const refresh = page.getByRole("button", { name: "Check for new releases" });
  await refresh.waitFor({ timeout: 20000 });
  const t0 = Date.now();
  await refresh.click();
  // The frozen test data indexes only when asked: one pass per source.
  // Home's featured area, shelves and rows all mark a game with data-key.
  const game = page.locator("#sf-panel-home [data-key]").first();
  await game.waitFor({ timeout: 120000 });
  await page.getByRole("button", { name: "Check for new releases" }).and(page.locator(":not([disabled])")).waitFor({ timeout: 120000 }).catch(() => {});
  const games = await page.locator("#sf-panel-home [data-key]").count();
  check("Home fills from the sources without a feed", games > 0, `${games} games after ${Math.round((Date.now() - t0) / 1000)} s`);
  check("Home has a featured game", await page.locator("#sf-panel-home").getByText("Featured").first().isVisible());
  await sleep(4000); // the chart and art arrive
  await shots("1-home");

  // Search: cached matches at once, then the sources and Steam.
  await page.getByRole("tab", { name: /Browse/ }).click();
  await page.locator("input[type=search]").fill("witcher");
  await sleep(600 + 15000);
  const found = await page.locator("#sf-panel-browse .card").count();
  check("Search finds releases for 'witcher'", found > 0, `${found} cards`);
  await shots("2-search");

  // Keyboard: Tab reaches a card, Enter opens it, Escape goes back.
  await page.locator("input[type=search]").fill("");
  await sleep(800);
  const first = page.locator("#sf-panel-browse .card").first();
  await first.focus();
  await page.keyboard.press("Enter");
  const back = page.locator("main button.back");
  await back.waitFor({ timeout: 20000 });
  check("Enter opens a game from the keyboard", true);
  await page.keyboard.press("Escape");
  await sleep(500);
  check("Escape goes back to the Store", !(await back.isVisible().catch(() => false)));

  // A FitGirl game's page: releases, reviews, times, wishlist.
  await page.locator("input[type=search]").fill("");
  const fg = page.locator("#sf-panel-browse .card", { has: page.locator(".line", { hasText: "FitGirl" }) }).first();
  await (await fg.count() ? fg : first).click();
  await back.waitFor({ timeout: 20000 });
  const opened = Date.now();
  await page.locator("ul.releases > li").first().waitFor({ timeout: 60000 }).catch(() => {});
  const releases = await page.locator("ul.releases > li").count();
  check("The game page lists source releases", releases > 0, `${releases} releases after ${((Date.now() - opened) / 1000).toFixed(1)} s`);
  // Reviews, Metacritic and HowLongToBeat arrive within the enrichment timeout.
  await page.locator("main", { hasText: /Main Story|Times unavailable|No confident|HowLongToBeat couldn/ }).waitFor({ timeout: 45000 }).catch(() => {});
  const text = await page.locator("main").innerText();
  check("The page shows Steam reviews or says they're unavailable", /review|Steam/i.test(text));
  check("The page shows HowLongToBeat times or says they're unavailable", /Main Story|HowLongToBeat|Times unavailable/i.test(text));
  await shots("3-game");
  const wish = page.locator("main button.wish:visible").first();
  await wish.click();
  await sleep(800);
  check("Saving to the wishlist", (await wish.getAttribute("aria-pressed")) === "true");

  // The install confirmation, closed without downloading.
  const get = page.locator("ul.releases > li button", { hasText: /^(Download|Check release|Install over it)$/ }).first();
  await get.click();
  const dialog = page.locator("[role=dialog]");
  await dialog.first().waitFor({ timeout: 120000 });
  await sleep(1500);
  const dialogText = await dialog.last().innerText();
  check("Preparing a release opens the confirmation or explains why not", /Get|Update|torrent|release/i.test(dialogText), dialogText.split("\n")[0]);
  await shots("4-confirm");
  await page.keyboard.press("Escape");
  await sleep(500);

  await back.click();
  await page.getByRole("tab", { name: /Wishlist/ }).click();
  await sleep(800);
  check("The wishlist shows the saved game", (await page.locator("#sf-panel-wishlist").innerText()).length > 20);
  await shots("5-wishlist");

  const downloads = JSON.parse(fs.existsSync(path.join(data, "downloads.json")) ? fs.readFileSync(path.join(data, "downloads.json"), "utf8") : "[]");
  const jobs = Array.isArray(downloads) ? downloads : (downloads.jobs ?? []);
  check("Nothing was downloaded", jobs.length === 0, `${jobs.length} downloads`);
  const wl = path.join(data, "wishlist.json");
  check("The wishlist is saved", fs.existsSync(wl) && JSON.parse(fs.readFileSync(wl, "utf8")).entries.length === 1);
} finally {
  await app?.quit();
  restoreAppData();
}
fs.writeFileSync(path.join(out, "report.json"), JSON.stringify(checks, null, 1));
const failed = checks.filter((c) => !c.ok);
console.log(failed.length ? `discovery: ${failed.length} failed` : "discovery: ok", "· screenshots in", out);
process.exit(failed.length ? 1 : 0);
