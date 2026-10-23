# Time to diagnose with and without opsbot

## Question

Does `opsbot` (the read-only ops assistant that summarises alerts, runs
`kubectl satark doctor --json` and cites runbooks) shorten the time from an
alert firing to a correct root-cause statement, without lowering accuracy?

## Design

- **Scenarios:** GD-03 (DNS), GD-04 (NetworkPolicy), GD-05 (bad deploy),
  GD-06 (runtime), GD-07 (disk), GD-09 (image pull). These have a single
  clear root cause and are safe to repeat.
- **Conditions:**
  - *manual:* alerts, Grafana, Hubble, `kubectl`, runbooks.
  - *opsbot:* the same tools plus opsbot in the incident channel.
- **Counterbalancing:** each responder does every scenario once, half under
  each condition, in an order fixed before the session (table below), so no
  one sees the same scenario twice and learning effects are spread across
  both conditions.
- **Measure:** seconds from the first alert to the responder stating the
  correct root cause. Wrong or partial answers are recorded with
  `correct=false` and excluded from time, included in accuracy.
- **Privacy:** responders are recorded by alias only.

| Responder | manual | opsbot |
| --- | --- | --- |
| R1 | GD-03, GD-05, GD-07 | GD-04, GD-06, GD-09 |
| R2 | GD-04, GD-06, GD-09 | GD-03, GD-05, GD-07 |

## Running a trial

```bash
tests/chaos/gd-04-bad-networkpolicy.sh observe
tests/chaos/ttd/ttd-timer.sh gd-04 opsbot R1   # facilitator, in another terminal
tests/chaos/gd-04-bad-networkpolicy.sh inject
# ... responder works; facilitator presses Enter at each milestone ...
tests/chaos/gd-04-bad-networkpolicy.sh revert
```

Analyse:

```bash
python3 tests/chaos/ttd/analyze.py
```

## Results

Paste the output of `analyze.py` here.

| Condition | Trials | Median TTD (s) | IQR (s) | Accuracy |
| --- | --- | --- | --- | --- |
| manual | | | | |
| opsbot | | | | |

## Interpretation

With two responders and six scenarios the sample is small; treat the result
as directional. Note scenarios where opsbot misled the responder: those
matter more than the median.
