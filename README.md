# chargescout: find subscriptions and recurring charges in your bank statement

[![npm](https://img.shields.io/npm/v/chargescout)](https://www.npmjs.com/package/chargescout) [![CI](https://github.com/GillMacedo/chargescout/actions/workflows/ci.yml/badge.svg)](https://github.com/GillMacedo/chargescout/actions/workflows/ci.yml) [![MIT license](https://img.shields.io/badge/license-MIT-blue)](LICENSE) [![MCP Registry](https://img.shields.io/badge/MCP_Registry-listed-green)](https://registry.modelcontextprotocol.io/v0/servers?search=io.github.GillMacedo/chargescout)

**See every subscription you're paying for, what it costs per year, which prices went up, and where you were charged twice, without linking your bank account or uploading anything. For small businesses, freelancers and anyone auditing their spending.**

Subscription trackers usually want your bank login and charge a monthly fee. `chargescout` reads a CSV export from your bank or card instead, and finds the charges that repeat weekly, monthly, quarterly or yearly. It's an MCP server and a single Go binary, with no account, no API key and no network access.

## What you can ask your agent

- "What subscriptions am I paying for?"
- "How much do my recurring charges cost per year?"
- "Which subscription prices went up?"
- "Was I charged twice for anything?"
- "What should my business cancel to save money?"

## Install

As a Claude Code plugin, which adds the MCP server and a skill that tells Claude when to use it. You need Node, which runs the server through `npx`:

    claude plugin marketplace add GillMacedo/chargescout
    claude plugin install chargescout@chargescout-marketplace

For other MCP clients:

```json
{
  "mcpServers": {
    "chargescout": {
      "command": "npx",
      "args": ["-y", "chargescout", "serve"]
    }
  }
}
```

Or build the binary with Go 1.27 or newer:

    go install github.com/GillMacedo/chargescout@latest

## Usage

Without installing anything, run `npx chargescout find statement.csv`. This example uses the made-up statement in `examples/`:

    $ go run . find examples/statement.csv
    SAMPLE POWER CO  83.30  monthly  999.60/year  last 2026-09-12  active  (likely)
      ! amount varies, so this may be a usage-based bill
    MOCK ACCOUNTING SOFTWARE  60.00  quarterly  240.00/year  last 2026-08-25  active  (likely)
    SAMPLE STREAM  18.99  monthly  227.88/year  last 2026-09-03  active
      ! price went up from 15.99 to 18.99 on 2026-06-03
    PLACEHOLDER DOMAINS ANNUAL  120.00  yearly  120.00/year  last 2026-09-14  active  (likely)
    EXAMPLE CLOUD STORAGE  9.99  monthly  119.88/year  last 2026-09-07  active
      ! possible double charge of 9.99 on 2026-06-08
    FICTIONAL GYM MEMBERSHIP  45.00  monthly  540.00/year  last 2026-05-21  may have stopped
    SKIPPED row 59: no valid date or amount
    6 recurring charges, 5 active, costing 1707.36 a year

Add `--positive-out` if your file shows charges as positive numbers, as many card statements do, and `--json` for machine-readable output. A longer statement gives better results, and a year of history is ideal.

## How it decides

| Finding | Rule |
|---|---|
| **Recurring** | 4 or more charges from one merchant at a steady interval and price |
| **Likely** | 3 charges, 2 charges a year apart, or a steady bill whose amount changes most cycles |
| **May have stopped** | The next charge is overdue by the end of the statement. It's left out of the total |

Most intervals must fit the cycle, and amounts must stay within 25%, so everyday shopping isn't reported. Two plans from one company stay separate.

## MCP tools

| Tool | What it does |
|---|---|
| `subscriptions_find` | Returns recurring charges, yearly cost and alerts, with a limit for long results |
| `subscriptions_inspect_csv` | Shows detected columns and row counts, without row contents |

Both are read-only. `subscriptions_find` returns merchant names and amounts to the MCP client, so they reach whichever AI model you use. The CLI keeps everything on your machine.

## FAQ

**How do I find forgotten subscriptions?** Export your bank or card transactions as CSV and run `npx chargescout find statement.csv`.

**Do I have to connect my bank account?** No. It only reads the file you give it, and it has no network code.

**Can it cancel subscriptions?** No. It shows you what's there, and you cancel with each provider.

**Does it work with my bank?** It reads CSV exports and recognizes common column names in several languages, and you can name others with `--columns date=When,amount=Sum,description=What`. It hasn't been tested on each bank's real exports yet.

## Limits

- This helps you review spending and isn't financial advice. Check results against your statement.
- It reads CSV only, so PDF statements need converting first.
- Merchant names come from bank descriptions, so one company can appear under two names, and those won't be connected.
- It can miss charges with irregular timing, and can report steady non-subscription spending, such as rent or a regular purchase.
- Yearly cost is the last amount times the cycles per year, so it's an estimate.
- It assumes two decimal places, and doesn't convert currencies.
- So far it has been tested on made-up data, not real exports. If yours fails, please open an issue with the column headers, not your data.

## Contributing

Contributions are welcome, from people and from people working with AI agents. See [CONTRIBUTING.md](CONTRIBUTING.md). Good first issues:

- Add column names from more banks and languages
- Add more card-processor prefixes to merchant name cleanup
- Connect the same merchant under different names
- Add `--out` to write results to a CSV
- Add a plugin icon

## AI disclosure

This project was largely generated with Claude Code and reviewed by its maintainer.

## About me

I'm Giulliano Macedo, and I build developer tools at GoTech Solutions. Follow along or get in touch:

- GitHub: [GillMacedo](https://github.com/GillMacedo)
- LinkedIn: [giulliano](https://www.linkedin.com/in/giulliano/)
- X: [@giumacedo](https://x.com/giumacedo)

If `chargescout` found a charge you forgot about, a star helps others find it.

---

MIT licensed.

Built by [Giulliano Macedo](https://github.com/GillMacedo), GoTech Solutions · [LinkedIn](https://www.linkedin.com/in/giulliano/) · [X](https://x.com/giumacedo)
