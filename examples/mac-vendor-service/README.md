# mac-vendor-service

An example HTTP service that combines **gin-prometheus** with
[**maclookup-go**](https://github.com/logocomune/maclookup-go): it resolves MAC
addresses to their IEEE-registered vendor via the
[maclookup.app API](https://maclookup.app/api-v2/documentation) and exposes
Prometheus metrics for both its own HTTP traffic and its use of the upstream API.

It is a separate Go module (with a `replace` pointing at this repository), so
the gin-prometheus library itself does not gain a dependency on maclookup-go.

## Run

Requires Go 1.25.7+ (inherited from maclookup-go; with `GOTOOLCHAIN=auto`
older Go versions download it automatically).

```bash
cd examples/mac-vendor-service
METRICS_USER=prometheus METRICS_PASSWORD=change-me go run .
```

```bash
curl localhost:8080/api/v1/vendors/00:00:0c:12:34:56       # full vendor record
curl localhost:8080/api/v1/vendors/00-00-0C/name           # vendor name only
curl -u prometheus:change-me localhost:8080/metrics
```

## API

| Route | Response |
|---|---|
| `GET /api/v1/vendors/:mac` | Full record (`company`, `address`, `country`, `block_*`, `is_random`, `is_private`, `cached`) |
| `GET /api/v1/vendors/:mac/name` | `company`, `is_private`, `cached` |
| `GET /healthz` | `{"status":"ok"}` |
| `GET /metrics` | Prometheus metrics (Basic Auth if configured) |

`:mac` accepts 6–12 hex digits, optionally separated by `:`, `-`, `.` or space.

| Status | Meaning |
|---|---|
| 200 | Vendor found |
| 404 | Prefix not registered (`"found": false`) |
| 400 | Invalid MAC (checked locally, never sent upstream) |
| 429 | Upstream quota exhausted; `Retry-After` is set |
| 502 | Upstream error (details are logged, not returned) |
| 504 | Upstream timeout |

## Configuration

| Variable | Default | |
|---|---|---|
| `ADDR` | `:8080` | Listen address |
| `MACLOOKUP_API_KEY` | – | Legacy API key (new keys are no longer issued) |
| `MACLOOKUP_BASE_URL` | – | Alternative API base URL (proxy, mock) |
| `MACLOOKUP_TIMEOUT` | `5s` | Per-request upstream timeout |
| `CACHE_TTL` | `24h` | How long results are cached; `0` disables caching |
| `CACHE_MAX_ENTRIES` | `10000` | Cache size limit |
| `METRICS_USER` / `METRICS_PASSWORD` | – | Basic Auth for `/metrics` (both required) |

## Design notes

- **Quota protection.** Results, including "not found", are cached. Concurrent
  requests for the same prefix share one upstream call (`singleflight`). After
  the API answers 429, no upstream calls are made until its reset time.
- **Input validation.** maclookup-go copies every non-separator byte into the
  request path, so the service validates MACs itself before calling it.
- **No API-key leaks.** maclookup-go sends the key as a query parameter, and
  network errors contain the request URL. The service strips the URL before
  logging an error.
- **Bounded memory and cardinality.** The cache has a size limit, and the HTTP
  metrics use the route pattern (`/api/v1/vendors/:mac`) as the `path` label,
  so arbitrary MACs never become label values.

## Metrics

HTTP metrics from gin-prometheus (`/healthz` and `/metrics` are excluded):
`http_requests_total`, `http_request_duration_seconds`,
`http_request_size_bytes`, `http_response_size_bytes`.

Service metrics:

| Metric | Labels |
|---|---|
| `maclookup_upstream_requests_total` | `endpoint`, `result` (`found`, `not_found`, `rate_limited`, `bad_request`, `bad_api_key`, `timeout`, `error`) |
| `maclookup_upstream_duration_seconds` | `endpoint` |
| `maclookup_cache_requests_total` | `endpoint`, `result` (`hit`, `miss`) |
| `maclookup_cache_entries` | – |
| `maclookup_rate_limit_remaining` | – (`-1` until the API has reported it) |

Example queries:

```promql
# Cache hit ratio
sum(rate(maclookup_cache_requests_total{result="hit"}[5m]))
  / sum(rate(maclookup_cache_requests_total[5m]))

# p95 upstream latency
histogram_quantile(0.95, sum by (le) (rate(maclookup_upstream_duration_seconds_bucket[5m])))

# Share of failing API requests
sum(rate(http_requests_total{path=~"/api/.*", status_code=~"5.."}[5m]))
  / sum(rate(http_requests_total{path=~"/api/.*"}[5m]))
```

Example alerts:

```yaml
groups:
  - name: mac-vendor-service
    rules:
      - alert: MaclookupQuotaLow
        expr: maclookup_rate_limit_remaining >= 0 and maclookup_rate_limit_remaining < 100
        for: 5m
      - alert: MaclookupUpstreamErrors
        expr: sum(rate(maclookup_upstream_requests_total{result=~"error|timeout|bad_api_key"}[5m])) > 0.1
        for: 10m
```

## Tests

```bash
go test -race ./...
```

The tests run against an in-process fake of the maclookup.app API, so they need
no network access.
