// Client for the public gateway-api (POST /v1/scan/text and /v1/scan/qr, see
// apps/gateway-api/internal/api/types.go). The base URL is configurable at
// build time with VITE_API_BASE (default "/api", reverse-proxied to gateway-api).

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

const VERDICT_RISK: Record<string, Risk> = {
  likely_scam: "high",
  suspicious: "medium",
  no_red_flags: "low",
};

/** Normalises a gateway response defensively; unknown shapes become "unknown". */
export function parseResult(body: unknown): CheckResult {
  const b = (body ?? {}) as Record<string, unknown>;
  const risk = VERDICT_RISK[b.verdict as string] ?? "unknown";
  const reasons = Array.isArray(b.reasons)
    ? b.reasons
        .map((r) => (r as Record<string, unknown> | null)?.message)
        .filter((m): m is string => typeof m === "string")
    : [];
  const steps = Array.isArray(b.next_steps) ? b.next_steps.filter((x): x is string => typeof x === "string") : [];
  return {
    risk,
    score: typeof b.risk_score === "number" ? b.risk_score : undefined,
    reasons,
    advice: steps.length ? steps.map((x) => (/[.!?)]$/.test(x.trim()) ? x.trim() : `${x.trim()}.`)).join(" ") : undefined,
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
  return post("/v1/scan/text", json({ text }), fetchFn);
}

export function checkQr(image: Blob, fetchFn: typeof fetch = fetch): Promise<CheckResult> {
  const form = new FormData();
  form.append("image", image, "qr");
  return post("/v1/scan/qr", { body: form }, fetchFn);
}

/**
 * Checks free-form input. Links go through the text scan too: it extracts
 * and scores URLs, and the gateway has no separate URL endpoint yet.
 */
export function checkInput(input: string, fetchFn: typeof fetch = fetch): Promise<CheckResult> {
  return checkText(input.trim(), fetchFn);
}
