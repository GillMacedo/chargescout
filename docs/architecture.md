# Architecture

    statement.csv ─> load ─> merchant names ─> amount groups ─> cadence ─> alerts ─> CLI and MCP tools

**Loading:** finds the header in the first 20 rows. Amounts become integer cents. The date format comes from the whole column, and a column that could be either format stops with an error. Unreadable rows are reported.

**Merchant names:** a description loses card words and anything containing a digit, then keeps up to three words. That makes `SQ *SAMPLE STREAM 1004` and `SQ *SAMPLE STREAM 1005` one merchant.

**Amount groups:** a merchant's charges are split wherever an amount is more than 25% above the group's smallest, so two plans stay separate. Comparing with the smallest stops drifting amounts from chaining together.

**Cadence:** repeats within 3 days at the same amount become double charge alerts and don't count as intervals. The median interval must match a known cycle, and 75% of intervals must fit it. Yearly billing needs 2 charges, and anything else needs 3.

**Alerts:** a price rise is reported only when the price mostly holds. A bill that changes in most cycles is treated as usage-based and marked likely.

**Packaging:** one npm package holds all five binaries, and `npm/bin.js` picks the right one. An earlier project used a package per platform, and npm's spam check rejected one of them.

See `docs/decisions/`.
