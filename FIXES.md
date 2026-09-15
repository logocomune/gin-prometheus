# Fixes applied on top of v1.3.1

Audited and patched locally. No new dependencies were added (the crypto/subtle
fix uses the standard library only). No public API was removed — only
already-dead, unreachable, unexported code was deleted.

## 1. Unbounded request-body buffering (request_helper.go, middleware.go)

**Problem:** When `WithRecordRequestSize` was enabled (the default) and a
request arrived with unknown `Content-Length` (e.g. chunked
transfer-encoding), `calculateBodySizeStream()` read the *entire* remaining
body into an unbounded `bytes.Buffer` via `io.Copy`, with no size limit.
A request to any route that doesn't fully consume its own body could be used
to force large in-memory allocations — a memory-exhaustion / DoS vector.

Worse, this read happened *after* the handler had already run (`c.Next()`),
so for a body the handler *did* read, this code re-read whatever was left —
usually nothing — making the whole mechanism unreliable in addition to being
unsafe: it either measured close to 0 for a well-behaved handler, or
unbounded bytes for one that ignores its body.

**Fix:** Introduced `countingReadCloser`, a thin wrapper installed on
`c.Request.Body` *before* `c.Next()` runs whenever Content-Length is
unknown and request-size recording is enabled. It counts bytes as the
handler actually reads them — no buffering, no truncation, no re-reading —
exactly the same principle Gin's own `ResponseWriter` already uses to count
response bytes (`c.Writer.Size()`). `readRequestBody()` (dead code with its
own separate 10 MB cap that was never wired into the live path) was removed
as unnecessary now that the live path is safe by construction.

## 2. Non-constant-time Basic Auth comparison (metrics_handler.go)

**Problem:** `withBasicAuth` compared credentials with plain `!=`, which
short-circuits on the first differing byte — a timing side-channel against
the `/metrics` endpoint's Basic Auth.

**Fix:** Switched to `crypto/subtle.ConstantTimeCompare` for both username
and password.

## 3. Global registration panics on name collision (middleware.go)

**Problem:** `defaultMetricsCollection()` (run from `init()`, i.e. on every
import of the package) and `NewMetricsCollection()` used
`prometheus.MustRegister`, which panics if a collector with the same name is
already registered on the target registry — including the shared global
default registry. Any other package in the same process registering
`http_requests_total` etc. would crash the program at startup.

**Fix:** Added `registerCounterOrReuse` / `registerHistogramOrReuse`: on
`AlreadyRegisteredError`, the pre-existing collector is reused instead of
panicking. Genuine registration errors (e.g. invalid metric name) still
panic, matching the previous behaviour for real failures.

## 4. Dead code removed (middleware.go)

All unreachable/unused, unexported — removing them changes no public API:

- `getPathFromContext()` — verbatim duplicate of `getPathWithFallback()`,
  never called.
- `handleMetrics()` / `recordRequestMetrics()` — legacy wrappers commented
  as "kept for backward compatibility", but both are unexported, so no
  external caller could have depended on them anyway. Never called.
- Package-level vars `totalRequests`, `responseSize`, `requestSize`,
  `duration` — declared, never read or written.
- `computeResponseSize()` (request_helper.go) — unused duplicate of the
  inline `c.Writer.Size()` call already used in the live path.

## 5. Inaccurate doc comment on `WithPathAggregator` (options.go)

**Problem:** The doc comment implied the default aggregator's
`"path_4xx"`/`"path_5xx"`/`"missing_route"` fallbacks apply to unmatched
routes generally. In reality, `handleUnmatchedPath()` already rewrites an
empty route to `/unmatched/*` (or similar) *before* the aggregator runs
whenever `WithUnmatchedRouteHandling` is at its default (`true`) — so those
fallback branches are only reachable with unmatched-route handling
explicitly disabled.

**Fix:** Corrected the comment to state this precisely. No behavioural
change — changing the actual precedence would be a breaking change for
anyone already relying on the current (default) behaviour.

## Tests

- `request_helper_test.go` rewritten to test `countingReadCloser` directly
  (full read, partial read, empty body, nil receiver, and an errored read
  that still returns partial bytes — mirroring a dropped connection
  mid-body).
- `middleware_test.go`: updated `getRequestSize` tests for the new
  `(*http.Request, *countingReadCloser)` signature; added an end-to-end test
  sending a request with unknown Content-Length through the full router and
  asserting the handler receives the byte-for-byte original body; added a
  test proving a second `NewMetricsCollection` against an
  already-populated registry no longer panics and reuses the existing
  collectors.

## Not changed (flagged for awareness only)

- Query-parameter API keys, plain-text protocol quirks, etc. in the
  companion `maclookup-go` library were left as-is (dictated by the
  upstream API, not fixable client-side) — see that repo's README for a
  documentation update reflecting maclookup.app's 2026 API-key deprecation.

## Verification

Verified locally on the target platform (Termux/Android, Go 1.26.5):

go build ./...   # clean
go vet ./...     # clean
go test ./...    # ok  github.com/logocomune/gin-prometheus  0.011s
