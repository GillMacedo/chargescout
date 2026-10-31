# 0002: One npm package holding every binary

**Decision:** written in Go, and published as one npm package containing all five platform binaries.

**Why:** the maintainer prefers Go. One package means one name to publish and one to trust, and avoids npm's spam check, which rejected a platform package name on an earlier project.

**Trade-off:** each install downloads all five binaries, about 15 MB compressed, and `npx` itself needs Node.

**Alternatives:** one optional package per platform, which downloads less but publishes six packages per release.
