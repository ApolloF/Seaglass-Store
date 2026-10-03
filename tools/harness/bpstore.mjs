// Big Picture Store with the virtual controller in the real app: index the
// sources once from desktop mode, switch to big picture, then Home, a game
// page and back (focus kept), Browse with the on-screen keyboard, Wishlist,
// and the install confirmation (closed, nothing downloaded). 720p and 1080p.
import fs from "node:fs";
import path from "node:path";
import { OUT, backupAppData, makeDevData, restoreAppData, sleep, startApp } from "./lib.mjs";

const out = path.join(OUT, "bpstore");
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
  const data = makeDevData("bpstore", {
    fake: false,
    settings: { experimentalStore: true, store: { privateSources: true, sources: ["fitgirl", "dodi"], sourceSetup: "done", pauseWhilePlaying: true, blockDetections: true, language: "English" } },
  });
  app = await startApp({ data, pad: "ps" });
  await app.viewport(1400, 900);
  let page = app.page;
  await page.locator("aside button", { hasText: /^\s*Store\s*$/ }).click();
  const refresh = page.getByRole("button", { name: "Check for new releases" });
  await refresh.waitFor({ timeout: 20000 });
  await refresh.click();
  await page.locator("#sf-panel-home [data-key]").first().waitFor({ timeout: 120000 });
  await page.getByRole("button", { name: "Check for new releases" }).and(page.locator(":not([disabled])")).waitFor({ timeout: 120000 }).catch(() => {});
  await sleep(3000);
  await page.locator("aside button.bp").click();
  await sleep(2500);
  page = await app.mainPage();
  await sleep(1500);
  const focusText = () => page.evaluate(() => document.querySelector("button.card.on")?.getAttribute("aria-label") ?? "");
  const press = async (b, wait = 350) => { await app.pad.press(b); await sleep(wait); };
  const shot = async (name) => {
    for (const [w, h] of [[1280, 720], [1920, 1080]]) {
      await app.viewport(w, h);
      await sleep(500);
      await app.shot(path.join(out, `${name}-${w}x${h}.png`));
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - innerWidth);
      if (overflow > 0) check(`${name} at ${w}x${h} has no horizontal scrolling`, false, `${overflow} px`);
    }
    await app.viewport(1920, 1080);
  };
  await app.viewport(1920, 1080);
  const isBP = await page.evaluate(() => location.search.includes("bigpicture") || !!document.querySelector("[class*=bp], .bigpicture"));
  check("Big picture opened from desktop mode", isBP, page.url());
  // Sections: Home → Library → Search → Store with RB.
  for (let i = 0; i < 3; i++) await press("rb", 600);
  await sleep(1500);
  const body = await page.locator("body").innerText();
  check("Store Home shows Featured", /Featured/.test(body));
  await shot("1-home");
  await press("down");
  await press("right");
  const before = await focusText();
  await press("south", 1500);
  const details = await page.locator("body").innerText();
  check("A game page opens with the controller", /Release|Install|Get|source/i.test(details), before);
  await shot("2-details");
  // Source choice and confirmation: south on the first action, then back out.
  await press("south", 1500);
  await shot("3-install");
  const inst = await page.locator("body").innerText();
  check("The install step shows a choice or a reason", /Install|Download|browser|Choose|release/i.test(inst));
  await press("east", 800);
  await press("east", 800);
  await sleep(600);
  const after = await focusText();
  check("Going back keeps focus on the same game", before !== "" && before === after, `${before} → ${after}`);
  await shot("3b-back");
  // Browse tab and the on-screen keyboard.
  await press("rt", 900);
  await shot("4-browse");
  await press("north", 900);
  await press("down"); await press("south"); await press("right"); await press("south");
  await sleep(1200);
  const typed = await page.evaluate(() => document.querySelector(".field")?.textContent?.trim() ?? "");
  check("The on-screen keyboard types into search", typed.length > 0, JSON.stringify(typed));
  await shot("5-search-keyboard");
  await press("rt", 900);
  await shot("6-wishlist");
  const tabOn = await page.evaluate(() => [...document.querySelectorAll(".tabs .on, .tabs [aria-selected=true]")].map((e) => e.textContent.trim()).join(","));
  check("R2 goes on to the Wishlist tab", /Wishlist/.test(tabOn), tabOn);
  await press("east", 900);
  const home = await page.evaluate(() => [...document.querySelectorAll(".tabs .on, .tabs [aria-selected=true]")].map((e) => e.textContent.trim()).join(","));
  check("Back from the Wishlist returns to the Store's Home tab", /Home/.test(home), home);
  const st = await app.state().catch(() => ({}));
  fs.writeFileSync(path.join(out, "state.json"), JSON.stringify(st, null, 1));
} catch (e) {
  check("run finished", false, e.message);
} finally {
  await app?.quit().catch(() => {});
  try { restoreAppData(); } catch (e) { console.log("restore:", e.message); }
  fs.writeFileSync(path.join(out, "report.json"), JSON.stringify(checks, null, 1));
}
