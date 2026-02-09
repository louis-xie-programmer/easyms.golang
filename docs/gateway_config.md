# Gateway Configuration

This document summarizes key gateway config fields relevant to performance and observability.

## Metrics

```yaml
gateway:
  metrics:
    sample_rate: 0.1  # 0-1, latency sampling rate. 0 disables sampling (5xx still recorded).
    upstream_sample_rate: 1.0  # 0-1, retry metrics sampling rate.
```

Notes:
- `gateway_request_duration_seconds` is latency-sampled; 5xx responses are always recorded.
- `gateway_requests_total` is full-fidelity (no sampling).
- Labels include `service`, `route`, `instance`, `method`, `status`.
- Upstream retry/failure metrics include `service`, `route`, and `instance` labels.
- Retry metrics respect `upstream_sample_rate`; failures are always recorded.
- If `upstream_sample_rate` is omitted, the default is `1.0`.

## Proxy / Upstream

```yaml
gateway:
  proxy:
    connect_timeout: 5s
    response_header_timeout: 30s
    max_idle_conns: 100
    max_idle_conns_per_host: 20
    max_conns_per_host: 0 # 0 = unlimited
    idle_conn_timeout: 90s
    tls_handshake_timeout: 10s
    expect_continue_timeout: 1s
    max_retries: 2       # idempotent requests only
    retry_backoff: 100ms
```

Notes:
- `max_conns_per_host` limits total in-flight connections per upstream host.
- `response_header_timeout` protects against slow upstreams.
- retries apply only to `GET/HEAD/OPTIONS` and 502/503/504 responses.

## Circuit Breaker Metrics

- `gateway_circuitbreaker_state{service,state}` tracks open/closed state.
- `gateway_circuitbreaker_state_changes_total{service,from,to}` tracks transitions.

## Monitoring Assets

- Grafana dashboard: `deploy/monitoring/grafana/dashboards/gateway.json`
- Prometheus alerts: `deploy/monitoring/prometheus/alerts/gateway.yml`
- Dashboard includes: circuit breaker open state, rate limit blocked, rate limit Redis errors.
