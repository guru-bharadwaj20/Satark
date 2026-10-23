# Evaluation dataset and harness

`dataset.jsonl` holds 300 labelled messages (one JSON object per line):

| Field | Meaning |
|---|---|
| `id` | Stable ID (`syn-NNN`) |
| `text` | Message text, already PII-masked |
| `lang` | `en`, `hi` (Devanagari or romanised Hindi) or `kn` |
| `label` | One category from `app.llm.schema.Category` |
| `is_scam` | `false` only for `legitimate` |
| `source` | `synthetic` for every current row |

Composition: 180 scam messages (18 per scam category) and 120 legitimate ones,
including hard negatives such as genuine OTP-delivery, debit-alert and bill-generated
messages. 172 English, 80 Hindi, 48 Kannada.

## Privacy

Every message is **synthetic and clearly fake**. Phone numbers are masked
(`98XXXXXX21`), UPI IDs are masked (`xy***@examplebank`), links use `.test`/`example`
domains, and banks and couriers are fictional (`ExampleBank`, `DemoPost`). Never add
real messages without masking them to this standard first.

## Labelling guide

- Pick the category describing the main ask of the message (money, code, app install, call).
- Messages that *mention* OTP/KYC/bills but only inform and say "do not share" are `legitimate`.
- Use `other_scam` only when no specific category fits.

## Running the evaluation

```bash
python eval/run_eval.py --provider baseline            # offline keyword baseline
python eval/run_eval.py --provider ollama --lang hi    # production LLM classifier
python eval/run_eval.py --predictions preds.jsonl      # score external predictions
```

The report gives precision, recall and F1 per category, macro averages, binary
scam recall/precision and the legitimate false-positive rate. `--out` writes JSON,
`--markdown` writes a table for PR comments, and the `--min-*`/`--max-*` flags turn it
into a gate (exit code 2 on failure). LLM fallbacks are scored with the keyword
baseline, mirroring the production rules-only path.

The baseline exists so the gate can run without an LLM. Its patterns were written
against this synthetic set, so treat its scores as a regression signal, not a
measure of real-world accuracy.
