import { describe, expect, it } from "vitest";
import { mockApi } from "./api.mock";

describe("mock completion", () => {
  it("keeps a chosen match so a later get agrees, and forgets it when set back to automatic", async () => {
    const id = (await mockApi.games())[0].id;
    const chosen = await mockApi.completion.setMatch(id, 4242);
    expect(chosen.completion).toMatchObject({ hltbId: 4242, corrected: true });
    expect((await mockApi.completion.get(id, false)).completion.hltbId).toBe(4242);
    expect((await mockApi.completion.get(id, true)).completion.hltbId).toBe(4242);
    const auto = await mockApi.completion.setMatch(id, 0);
    expect(auto.completion.corrected).toBe(false);
    expect((await mockApi.completion.get(id, true)).completion.hltbId).not.toBe(4242);
  });
});
