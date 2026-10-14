import type { CheckResult } from "./api";

// User-facing wording. Never claim something is "100% safe".
const HEADLINES: Record<CheckResult["risk"], string> = {
  high: "High risk: this looks like a scam",
  medium: "Be careful: some warning signs",
  low: "No warning signs found",
  unknown: "We couldn't assess this",
};

const DEFAULT_ADVICE: Record<CheckResult["risk"], string> = {
  high: "Don't click, pay or share any OTP, PIN or password. Contact the organisation through its official app or website.",
  medium: "Verify through an official channel before you act.",
  low: "That doesn't guarantee it's safe. If anything feels off, verify through an official channel.",
  unknown: "Treat it with caution and verify through an official channel.",
};

function el<K extends keyof HTMLElementTagNameMap>(tag: K, text?: string, cls?: string): HTMLElementTagNameMap[K] {
  const e = document.createElement(tag);
  if (text) e.textContent = text;
  if (cls) e.className = cls;
  return e;
}

/** Builds the result card. Uses textContent only, so API text can't inject HTML. */
export function renderResult(r: CheckResult): HTMLElement {
  const card = el("div", undefined, `result ${r.risk}`);
  card.append(el("h2", HEADLINES[r.risk]));
  if (r.reasons.length) {
    const ul = el("ul");
    for (const reason of r.reasons.slice(0, 8)) ul.append(el("li", reason));
    card.append(ul);
  }
  card.append(el("p", r.advice ?? DEFAULT_ADVICE[r.risk]));
  if (r.risk === "high" || r.risk === "medium") {
    const p = el("p", "Lost money? Call ");
    const a = el("a", "1930");
    a.href = "tel:1930";
    p.append(a, " immediately.");
    card.append(p);
  }
  return card;
}

export function renderError(message: string): HTMLElement {
  return el("p", message, "error");
}
