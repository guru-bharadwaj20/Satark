import { describe, expect, it } from "vitest";
import { combineShared, SHARE_FILE_KEY, SHARE_TEXT_KEY, takeShared } from "./share";

class FakeCache {
  store = new Map<string, Response>();
  async match(k: string) {
    return this.store.get(k)?.clone();
  }
  async delete(k: string) {
    return this.store.delete(k);
  }
}

describe("combineShared", () => {
  it("joins fields and drops duplicates", () => {
    expect(combineShared(null, "Your KYC expires https://kyc-update.example", "https://kyc-update.example")).toBe(
      "Your KYC expires https://kyc-update.example",
    );
    expect(combineShared("Alert", "Pay now", "https://x.example")).toBe("Alert\nPay now\nhttps://x.example");
    expect(combineShared(undefined, "  ", null)).toBe("");
  });
});

describe("takeShared", () => {
  it("returns and clears a pending share", async () => {
    const cache = new FakeCache();
    cache.store.set(SHARE_TEXT_KEY, new Response("hello"));
    cache.store.set(SHARE_FILE_KEY, new Response("png"));
    const storage = { open: async () => cache } as unknown as CacheStorage;
    const got = await takeShared(storage);
    expect(got?.text).toBe("hello");
    expect(got?.file?.size).toBe(3);
    expect(await takeShared(storage)).toBeNull();
  });
});
