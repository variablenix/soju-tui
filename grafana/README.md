# Grafana dashboard for Soju

This directory contains an optional Grafana dashboard for the Soju bouncer
administered by soju-tui. The metrics come directly from the long-running Soju
process. soju-tui remains an interactive command and does not need to stay open
or expose a network service.

The dashboard is based on the metric names exported by Soju 0.10.1:

- active users, upstreams, and downstreams;
- upstream and downstream IRC message rates;
- upstream connection errors and worker panics;
- process CPU, resident memory, uptime, and Go runtime health;
- optional network totals when Soju uses PostgreSQL.

`soju_networks_total` is not exported by the SQLite backend. Its dashboard
panel therefore shows no data instead of incorrectly reporting zero networks.

## Enable Soju's exporter

Add this listener to `/etc/soju/config`, choosing an unused local port:

```text
listen http+prometheus://localhost:8081
```

Restart Soju, then verify the endpoint locally:

```sh
curl --fail --silent --show-error http://127.0.0.1:8081/metrics |
  grep '^soju_'
```

Soju intentionally requires the Prometheus listener hostname to be
`localhost`. The exporter has no application-level authentication, so do not
publish it directly on a LAN or the Internet. Run Prometheus (or a Prometheus
agent) on the Soju host, or place an authenticated TLS reverse proxy with a
strict source-address policy in front of the loopback listener.

The example in [prometheus-scrape.yml.example](prometheus-scrape.yml.example)
is for Prometheus running in the same network namespace as Soju. Merge it under
the existing top-level `scrape_configs:` key. Change the descriptive labels as
needed; `use: soju` keeps the target consistent with the accompanying service
inventory example.

Treat the endpoint as operationally sensitive. In addition to process and
connection counts, PostgreSQL-backed Soju instances may export upstream IRC
hostnames when Soju's privacy threshold is met.

Prometheus running in a container cannot reach the host's loopback through the
container's own `127.0.0.1`. Use host networking, a host-local Prometheus agent,
or a reviewed authenticated proxy; do not weaken Soju's loopback restriction.

## Import the dashboard

1. In Grafana, open **Dashboards > New > Import**.
2. Upload `soju-overview.json`.
3. Select the Prometheus data source when prompted.
4. Choose the `soju` job and instance at the top of the dashboard.

The dashboard uses `$__rate_interval` for counter rates and `$__range` for
selected-range event totals. A 30-second scrape interval works with the default
dashboard refresh. If a panel has no data, first confirm the target is `UP` in
Prometheus and that the selected job and instance match the scrape labels.

## Files

- `soju-overview.json`: importable Grafana dashboard.
- `prometheus-scrape.yml.example`: production-safe same-host scrape job.
