---
name: finding-subscriptions
description: Finds subscriptions and recurring charges in a bank or card statement CSV, with yearly cost, price increases and possible double charges. Use when the user asks what subscriptions they pay for, where recurring money goes, what to cancel, or wants to audit business or personal spending.
---

# Finding subscriptions

Call `subscriptions_find` instead of reading the statement yourself, because cycles and amounts are easy to get wrong in a long file. If it warns that no row is money out, ask whether charges are shown as positive numbers, then use `positive_out`. If dates are ambiguous, ask whether they're day-first or month-first.

## Reporting results
- Lead with the active yearly total, then the most expensive charges.
- Call out `alerts`: price increases and possible double charges.
- `likely` findings have less evidence, so say so.
- `may have stopped` charges aren't in the total. Say that they may simply be missing from this statement.
- Report `warnings` and `skipped_rows`.

A longer statement gives better results, so suggest a year of history if there's little data. The tool can't cancel anything, and you shouldn't try to cancel a service without the user's approval. This isn't financial advice.
