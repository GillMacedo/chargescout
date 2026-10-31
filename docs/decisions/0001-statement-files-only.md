# 0001: Read statement files, and never connect to a bank

**Decision:** the tool only reads a CSV the user gives it. It has no bank connections and no network access.

**Why:** people are rightly wary of sharing bank logins, and a local file needs no account or trust in a third party.

**Trade-off:** results are only as fresh as the export, and a person has to download it.

**Alternatives:** bank aggregator APIs, which need accounts and fees. They fit a possible paid version.
