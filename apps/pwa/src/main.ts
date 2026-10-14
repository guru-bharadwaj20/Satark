import { checkInput, checkQr, type CheckResult } from "./api";
import { renderError, renderResult } from "./render";
import { combineShared, takeShared } from "./share";

const form = document.querySelector<HTMLFormElement>("#check-form")!;
const input = document.querySelector<HTMLTextAreaElement>("#input")!;
const qr = document.querySelector<HTMLInputElement>("#qr")!;
const btn = document.querySelector<HTMLButtonElement>("#check-btn")!;
const out = document.querySelector<HTMLElement>("#result")!;

export async function run(task: () => Promise<CheckResult>): Promise<void> {
  btn.disabled = true;
  out.replaceChildren(document.createTextNode("Checking..."));
  try {
    out.replaceChildren(renderResult(await task()));
  } catch (e) {
    out.replaceChildren(renderError(e instanceof Error ? e.message : "The check failed."));
  } finally {
    btn.disabled = false;
  }
}

form.addEventListener("submit", (ev) => {
  ev.preventDefault();
  const value = input.value.trim();
  if (!value) {
    out.replaceChildren(renderError("Paste a message or link first."));
    return;
  }
  void run(() => checkInput(value));
});

qr.addEventListener("change", () => {
  const file = qr.files?.[0];
  if (file) void run(() => checkQr(file));
  qr.value = "";
});

if ("serviceWorker" in navigator && import.meta.env.PROD) {
  void navigator.serviceWorker.register("/sw.js");
}

// Opened from the Android share menu (see share.ts and sw.js).
async function handleShared(): Promise<void> {
  const params = new URLSearchParams(location.search);
  if (!params.has("shared") || !("caches" in window)) return;
  history.replaceState(null, "", "/");
  const shared = await takeShared();
  if (shared?.file) {
    await run(() => checkQr(shared.file!));
  } else if (shared?.text) {
    input.value = shared.text;
    await run(() => checkInput(shared.text!));
  }
}
void handleShared();

// Fallback for browsers that deliver a GET share (?text=...).
const q = new URLSearchParams(location.search);
if (!q.has("shared") && (q.has("text") || q.has("url"))) {
  const value = combineShared(q.get("title"), q.get("text"), q.get("url"));
  history.replaceState(null, "", "/");
  if (value) {
    input.value = value;
    void run(() => checkInput(value));
  }
}
