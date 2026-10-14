// Client for the public gateway-api. Endpoint paths follow the published
// spec (docs/api/openapi.yaml); the base URL is configurable at build time
// with VITE_API_BASE (default "/api", reverse-proxied to gateway-api).

export type Risk = "low" | "medium" | "high" | "unknown";

export interface CheckResult {
  risk: Risk;
  score?: number;
  reasons: string[];
  advice?: string;
}

const BASE: string = (import.meta.env?.VITE_API_BASE as string | undefined) ?? "/api";

const URL_ONLY = /^(https?:\/\/)?[\w.-]+\.[a-z]{2,}(\/\S*)?$/i;

/** True when the input is just a link (so we use the URL check). */
export function isLink(input: string): boolean {
  const s = input.trim();
  return !/\s/.test(s) && URL_ONLY.test(s);
}

/** Normalises a gateway response defensively; unknown shapes become "unknown". */
export function parseResult(body: unknown): CheckResult {
  const b = (body ?? {}) as Record<string, unknown>;
  const risk = ["low", "medium", "high"].includes(b.risk as string) ? (b.risk as Risk) : "unknown";
  const reasons = Array.isArray(b.reasons) ? b.reasons.filter((r): r is string => typeof r === "string") : [];
  return {
    risk,
    score: typeof b.score === "number" ? b.score : undefined,
    reasons,
    advice: typeof b.advice === "string" ? b.advice : undefined,
  };
}

async function post(path: string, init: RequestInit, fetchFn: typeof fetch): Promise<CheckResult> {
  const res = await fetchFn(BASE + path, { method: "POST", ...init });
  if (res.status === 429) throw new Error("Too many checks right now. Please wait a minute and try again.");
  if (!res.ok) throw new Error("The check failed. Please try again.");
  return parseResult(await res.json());
}

const json = (body: unknown): RequestInit => ({
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify(body),
});

export function checkText(text: string, fetchFn: typeof fetch = fetch): Promise<CheckResult> {
  return post("/v1/check/text", json({ text }), fetchFn);
}

export function checkUrl(url: string, fetchFn: typeof fetch = fetch): Promise<CheckResult> {
  return post("/v1/check/url", json({ url }), fetchFn);
}

export function checkQr(image: Blob, fetchFn: typeof fetch = fetch): Promise<CheckResult> {
  const form = new FormData();
  form.append("image", image, "qr");
  return post("/v1/check/qr", { body: form }, fetchFn);
}

/** Routes free-form input to the right check. */
export function checkInput(input: string, fetchFn: typeof fetch = fetch): Promise<CheckResult> {
  const s = input.trim();
  return isLink(s) ? checkUrl(s, fetchFn) : checkText(s, fetchFn);
}
