# ADR-005: Bounded LLM influence on the risk score

- **Status:** Accepted
- **Date:** 2026-10-14
- **Deciders:** Satark maintainers

## Context

`text-analyzer` produces a deterministic risk score from YAML pattern rules
(urgency, KYC, OTP/PIN, remote access, ...). An LLM adds a classification with a
confidence, reasons and a plain-language explanation. LLMs are good at reading
intent in messy, mixed-language text, but they:

- are non-deterministic and change when the provider updates a model;
- can be steered by text inside the message itself (prompt injection);
- can be unavailable, slow or expensive at exactly the wrong moment;
- can be confidently wrong, and a false "looks fine" costs a user real money.

We need the LLM's judgement without making the verdict depend on it.

## Decision

The rule score is the anchor. The LLM may only nudge it within fixed bounds
(`app/llm/scoring.py`):

1. **Bounded delta:** at most **+15** towards scam and **-10** towards legitimate.
   The asymmetry is deliberate: a missed scam is worse than an extra warning.
2. **Confidence-scaled:** the delta scales linearly from 0 at confidence 0.5 to the
   bound at 1.0. Below 0.5 the LLM has no effect on the score.
3. **No arguing down strong rules:** if a strong rule fired (OTP/PIN request,
   remote-access app, "digital arrest"), the LLM cannot lower the score.
4. **No arguing down suspected injections:** if the message contains
   prompt-injection phrasing, downward adjustments are ignored.
5. **Fail closed to rules:** timeouts, errors, invalid output or an open circuit
   breaker produce a rules-only answer (adjustment 0).
6. **Validated output only:** the LLM's JSON must pass the Pydantic schema; output
   that says "100% safe", calls someone a scammer, or echoes the prompt is rejected.
7. Clamped to 0..100. Verdict buckets stay owned by `gateway-api`.

Explanations may come from the LLM (when in the requested script) or from reviewed
templates; the explanation never changes the score.

## Alternatives considered

| Option | Why not |
|---|---|
| LLM decides the verdict | Injection, outages and model drift directly change user-facing verdicts; not testable. |
| Rules only | Misses new wording and mixed Hindi/Kannada/English phrasing; explanations are generic. |
| Weighted average (e.g. 50/50) | A single confident wrong LLM answer can flip a verdict; weight hard to justify. |
| LLM only for explanations | Loses a useful signal for messages the rules don't cover. |

## Consequences

- Positive: verdicts stay explainable and reproducible from the rules; LLM failure
  degrades quality, not availability; injection can at most fail to raise a score.
- Positive: the bound makes the eval gate meaningful: rule changes and
  prompt changes are measured separately.
- Negative: a scam the rules miss entirely can be raised by only 15 points, so it may
  stay in a lower bucket. We accept this and grow the rules from evaluation misses.
- Negative: the constants are a judgement call; revisit them using `eval/run_eval.py`
  results before changing them, and record changes in a new ADR.
