import { describe, expect, it, vi } from "vitest";
import { checkInput, isLink, parseResult } from "./api";
import { renderResult } from "./render";

const ok = (body: unknown) =>
  vi.fn(async () => new Response(JSON.stringify(body), { status: 200 })) as unknown as typeof fetch;

describe("isLink", () => {
  it("detects bare links", () => {
    expect(isLink("https://kyc-update.example/login")).toBe(true);
    expect(isLink("kyc-update.example")).toBe(true);
    expect(isLink("Your account is blocked, visit kyc-update.example")).toBe(false);
  });
});

describe("checkInput", () => {
  it("sends links and text to the text scan", async () => {
    const f = ok({ verdict: "likely_scam", reasons: [{ code: "kyc.update", message: "New domain" }] });
    await checkInput("https://kyc-update.example", f);
    await checkInput(" Dear customer, update KYC ", f);
    const calls = (f as unknown as { mock: { calls: [string, RequestInit][] } }).mock.calls;
    expect(calls[0][0]).toBe("/api/v1/scan/text");
    expect(calls[1][0]).toBe("/api/v1/scan/text");
    expect(JSON.parse(calls[1][1].body as string)).toEqual({ text: "Dear customer, update KYC" });
  });

  it("explains rate limiting", async () => {
    const f = vi.fn(async () => new Response("", { status: 429 })) as unknown as typeof fetch;
    await expect(checkInput("hello there", f)).rejects.toThrow(/Too many checks/);
  });
});

describe("parseResult", () => {
  it("maps the gateway response", () => {
    expect(
      parseResult({
        verdict: "suspicious",
        risk_score: 44,
        reasons: [{ code: "kyc.update", message: "Banks do not ask for KYC by link", weight: 30 }],
        next_steps: ["Check directly with the bank.", "Never share your OTP."],
      }),
    ).toEqual({
      risk: "medium",
      score: 44,
      reasons: ["Banks do not ask for KYC by link"],
      advice: "Check directly with the bank. Never share your OTP.",
    });
  });

  it("tolerates unexpected shapes", () => {
    expect(parseResult({ verdict: "safe", reasons: [1, { message: "x" }, null] })).toEqual({ risk: "unknown", reasons: ["x"], score: undefined, advice: undefined });
  });
});

describe("renderResult", () => {
  it("never claims 100% safe and escapes API text", () => {
    const card = renderResult({ risk: "low", reasons: ["<img src=x onerror=alert(1)>"] });
    expect(card.textContent).not.toMatch(/100% safe/i);
    expect(card.textContent).toMatch(/doesn't guarantee/);
    expect(card.querySelector("img")).toBeNull();
  });

  it("shows 1930 for risky results", () => {
    expect(renderResult({ risk: "high", reasons: [] }).querySelector('a[href="tel:1930"]')).not.toBeNull();
  });
});
