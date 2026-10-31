# chargescout

Finds subscriptions and recurring charges in bank statement CSVs. It's a Go MCP server and CLI, with a Claude Code plugin wrapper.

## Architecture
- `internal/subs/load.go`: reads files, columns, amounts in cents and dates.
- `internal/subs/find.go`: merchant names, cadence detection and alerts.
- `internal/server`: MCP tools. `main.go`: CLI.
- `npm/bin.js` and `scripts/build-npm.sh`: one npm package holding every platform's binary.
- `vendor/`: dependencies, committed so tests run with no network.

## Commands

    make test
    make lint    # plugin validation and MCP Inspector
    make demo    # the README example

## Rules
- Money is integer cents. Never use floats for amounts, except a yearly estimate from a known cycle count.
- Never guess day-first or month-first when a column could be either.
- Never drop a row silently. Skipped rows are reported.
- Never add network access, writes to input files, or a way to cancel anything.
- Sample data stays obviously made up.
- Never write to stdout in server code, because it corrupts JSON-RPC.

## Definition of done
Tests pass, the README example matches, and `CHANGELOG.md` and `ROADMAP.md` are updated if relevant.
