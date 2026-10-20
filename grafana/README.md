# Grafana dashboard for Soju

This directory contains an optional Grafana dashboard for the Soju bouncer
administered by soju-tui. The metrics come directly from the long-running Soju
process. soju-tui remains an interactive command and does not need to stay open
or expose a network service.

The dashboard is based on the metric names exported by Soju 0.10.1:

- active users, upstreams, and downstreams;
- upstream and downstream IRC message rates;
- upstream connection failures and worker panics, with rates, selected-range
  estimates, and process-lifetime counters shown separately;
- process CPU, resident memory, uptime, and Go runtime health;
- Prometheus scrape duration and sample counts.

These native panels work with SQLite and PostgreSQL. Optional aggregate panels
add configured networks, paused networks, unexpectedly disconnected networks,
saved channels, and currently joined channels through the read-only collector
below. They do not query the PostgreSQL-only `soju_networks_total` metric.

## Enable Soju's exporter

Add this listener to `/etc/soju/config`, choosing an unused local port:

```text
listen http+prometheus://localhost:18080
```

Listener changes require a Soju restart; reloading with HUP does not apply them.
A restart disconnects and reconnects upstream networks, which also performs
fresh TLS certificate checks. Plan for that interruption, then verify locally:

```sh
curl --fail --silent --show-error http://127.0.0.1:18080/metrics |
  grep '^soju_'
```

For Soju 0.10.1, this listener requires the hostname `localhost`. The exporter
has no application-level authentication, so do not publish it directly on a LAN
or the Internet. Run Prometheus (or a Prometheus agent) on the Soju host, or use
an authenticated TLS reverse proxy with a strict source-address policy.

An authenticated encrypted tunnel, such as WireGuard between trusted peers, is
another option for remote scraping. Keep Soju on loopback and bind the forwarding
listener only to the tunnel address and interface. Scrape that private forwarding
endpoint from the remote Prometheus instance. Do not bind the forwarding listener
to all interfaces or make it reachable from an untrusted network.

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
or a reviewed private forwarding path; do not weaken Soju's loopback restriction.

## Import the dashboard

1. In Grafana, open **Dashboards > New > Import**.
2. Upload `soju-overview.json`.
3. Select the Prometheus data source when prompted.
4. Choose the **Services** folder.
5. Choose an instance, or **All**, at the top of the dashboard.

Every query is scoped to the `soju` job. Use that job name in Prometheus, as in
the supplied example. The Instance selector reads the `up` metric so a failing
target remains selectable. Selecting **All** cannot bring unrelated services'
process metrics into this dashboard.

The dashboard uses `$__rate_interval` for counter rates and `$__range` for
selected-range event estimates. Each query has a 30-second minimum step, matching
the example's scrape interval and allowing `$__rate_interval` to use at least
two minutes. Change that minimum step if your scrape interval differs.

If a panel has no data, confirm the target is `UP` in Prometheus and the selected
instance matches its scrape labels. Historical gaps cannot be reconstructed.

## Optional network and channel totals

Soju 0.10.1's native exporter does not provide joined-channel counts. The optional
Python 3 helper [soju-textfile.py](soju-textfile.py) reads `server status`,
`user status`, and each user's `network status` through the local `sojuctl`
admin socket. It never reads chat messages, modifies Soju, or opens an HTTP
listener. The interactive TUI does not need to stay running.

The helper emits aggregate `soju_admin_*` gauges for node_exporter's
[textfile collector](https://github.com/prometheus/node_exporter#textfile-collector).
These are implementation-specific supplementary metrics, not native Soju
metrics. Only the `soju_instance` label is exported: no account names, upstream
addresses, channel names, or error messages. Keep this endpoint private too.

- **Networks total** includes enabled and disabled user/network configurations.
- **Networks up** uses Soju's native active-upstream gauge.
- **Networks paused** counts explicitly disabled configurations.
- **Networks down** counts enabled configurations currently disconnected.
- **Channels joined** sums the live joined-channel count of connected upstreams.
  A downstream's detached channel still counts if the bouncer remains joined.
- **Channels saved** counts persisted channel records, including parted channels
  and channels on disabled networks. It can exceed the joined count.

Counts cover user/network configurations, so two users connected to the same IRC
network or channel count separately. This is not a unique-network or unique-name
inventory. Status commands run sequentially, so counts are a short operational
snapshot rather than an atomic server-wide observation. Uptime remains native:
the card shows the shortest selected process uptime, and the chart shows each
instance separately.

For an opt-in systemd deployment, review the supplied service and timer examples,
then install them explicitly. Debian packages include these as documentation
only; they do not enable a service or change Soju permissions. The service example
runs as root to access the existing admin socket. Use a trusted dedicated account
instead if it already has the necessary socket and config access.

```sh
sudo install -d -m 0755 /usr/local/libexec /var/lib/prometheus/soju-textfile
sudo install -m 0644 soju-textfile.py /usr/local/libexec/soju-textfile.py
sudo install -m 0644 soju-textfile.service.example /etc/systemd/system/soju-textfile.service
sudo install -m 0644 soju-textfile.timer.example /etc/systemd/system/soju-textfile.timer
```

Edit the installed service's `--instance` to match the native `job="soju"`
target's exact Prometheus `instance` label. Adjust the absolute `--sojuctl` and
`--config` paths if needed. The output directory and its ancestors must be
administrator-owned and not writable by untrusted users. Then run:

```sh
sudo systemctl daemon-reload
sudo systemctl start soju-textfile.service
sudo systemctl enable --now soju-textfile.timer
```

Expose that directory with a private node_exporter listener, using
`--collector.textfile.directory=/var/lib/prometheus/soju-textfile`. For a
container, mount the directory read-only and use the path inside the container.
Use a dedicated private listener when the existing node_exporter is publicly
reachable. Do not add these metrics to a public endpoint. Add its address to
Prometheus under the job name `soju-admin`, with a 30-second scrape interval.
The dashboard associates `soju_instance` with the selected native `soju` target;
the exporter itself can have a different scrape address. Configure exactly one
supplementary exporter per Soju instance to avoid double counting.

The helper writes atomically, bounds the complete collection to 20 seconds and
each command's output to 1 MiB, and fails closed on unknown, ambiguous, truncated,
or changing configuration listings. Soju 0.10.1 limits `user status` to 50 users;
larger instances need a different enumeration mechanism and show failed
collection rather than partial totals. On failure, old counts are replaced by
`collection_success=0`. Grafana excludes failed collections, collections older
than 120 seconds, and unreachable supplementary exporters. Missing counts show
N/A, not a fabricated zero; **Counts collector** shows freshness separately.

Test the optional helper with:

```sh
python3 -m unittest discover -s grafana -p 'test_*.py' -v
```

## Understand connection failures

**Scrape status: UP** means Prometheus can reach Soju's exporter. It does not
mean every upstream IRC connection is healthy. Message rates count IRC protocol
traffic, including protocol messages, rather than chat messages alone.

`soju_upstream_connect_errors_total` counts failed upstream connection attempts
since each Soju process started. Each failed retry of the same network increments
it again. It is not the number of networks currently disconnected.

- **Reliability event rate** shows failures and panics per second. Its legend
  reports the latest and maximum rates, not a misleading sum of rates.
- **Events in selected range** uses Prometheus `increase()` over the time range
  selected in Grafana. For example, one failure in the last five minutes can
  coexist with four failures since process start. Prometheus extrapolates the
  increase at range boundaries, so these are estimates rounded for display.
- **Failed attempts since start** and **Worker panics since start** show the raw
  historical counters, summed across selected processes. They do not become zero
  just because an upstream recovers; process restarts reset their own counters.

The **Networks down** card is the current disconnected-network count. Historical
failures use an amber indication; a failed retry or a cancelled in-flight
connection can leave a historical event after a network has been paused.

See the [Prometheus `increase()` documentation](https://prometheus.io/docs/prometheus/latest/querying/functions/#increase)
for range extrapolation and counter-reset handling.

Use the live network status to find which connection is failing:

```sh
sojuctl user run example-user network status
```

Repair the reported cause. An expired server certificate requires the upstream
operator to renew the certificate and reload the IRC service. Restarting Soju to
clear a counter does not repair that problem, and disabling TLS verification is
not a certificate-renewal fix.

To stop retries temporarily for one unavailable network, explicitly disable
only that network. This keeps it disconnected until you re-enable it:

```sh
sojuctl user run example-user network update example-network -enabled false
# After the upstream problem is fixed:
sojuctl user run example-user network update example-network -enabled true
```

Pausing retries preserves historical error counts. With no new failures, recent
event rates and selected-range estimates become zero once the failures leave
those time windows.

## Files

- `soju-overview.json`: importable Grafana dashboard.
- `prometheus-scrape.yml.example`: production-safe same-host scrape job.
- `soju-textfile.py`: optional read-only aggregate collector (Python 3).
- `soju-textfile.service.example` and `soju-textfile.timer.example`: opt-in units.
- `test_soju_textfile.py`: synthetic parser, failure, timeout, and atomic-write tests.
