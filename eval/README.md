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
real messages without masking them to this standard first (see CONTRIBUTING.md).

## Labelling guide

- Pick the category describing the main ask of the message (money, code, app install, call).
- Messages that *mention* OTP/KYC/bills but only inform and say "do not share" are `legitimate`.
- Use `other_scam` only when no specific category fits.

## Running the evaluation

See `run_eval.py --help`.
