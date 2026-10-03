// The on-screen keyboard's layout and moves, as big picture's Search has
// them: four rows of ten keys, then four wide keys under columns 0-1, 2-4,
// 5-7 and 8-9.
import { gridStep } from "../bigpicture/nav";

export const KEYS = [..."1234567890", ..."qwertyuiop", ..."asdfghjkl'", ..."zxcvbnm-:.", "space", "del", "clear", "done"] as const;
const COLS = 10;
const WIDE_COLS = [0, 2, 5, 8];

export const isWide = (k: number) => k >= 40;
const colOf = (k: number) => (isWide(k) ? WIDE_COLS[k - 40] : k % COLS);
export const keyLabel = (id: string) => (id === "space" ? "Space" : id === "del" ? "⌫" : id === "clear" ? "Clear" : id === "done" ? "Done" : id);
/** Where a wide key sits in the grid. */
export const wideColumn = (k: number) => ["1 / span 2", "3 / span 3", "6 / span 3", "9 / span 2"][k - 40];

/** The key a move lands on; "out" past the right edge (to the results), null at another edge. */
export function keyMove(k: number, intent: string): number | "out" | null {
  if (isWide(k)) {
    if (intent === "left") return k > 40 ? k - 1 : null;
    if (intent === "right") return k < 43 ? k + 1 : "out";
    if (intent === "up") return 30 + colOf(k);
    return null;
  }
  if (intent === "down" && k >= 30) {
    const c = k % COLS;
    return 40 + (c < 2 ? 0 : c < 5 ? 1 : c < 8 ? 2 : 3);
  }
  if (intent === "right" && k % COLS === COLS - 1) return "out";
  return gridStep(k, 40, COLS, intent);
}

/** The text after pressing a key; "done" leaves it as it is. */
export function typeKey(text: string, id: string): string {
  if (id === "space") return text + " ";
  if (id === "del") return text.slice(0, -1);
  if (id === "clear") return "";
  if (id === "done") return text;
  return text + id;
}

/** A character typed on a real keyboard that goes into the text, not to the shortcuts. */
export const typable = (key: string) => key.length === 1 && /[\p{L}\p{N}'\-:.&! ]/u.test(key);
