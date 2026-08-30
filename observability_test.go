package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

const (
	dashboardPath = "grafana/soju-overview.json"
	scrapePath    = "grafana/prometheus-scrape.yml.example"
)

func TestGrafanaDashboardContract(t *testing.T) {
	raw, err := os.ReadFile(dashboardPath)
	if err != nil {
		t.Fatal(err)
	}

	var dashboard map[string]any
	if err := json.Unmarshal(raw, &dashboard); err != nil {
		t.Fatalf("invalid Grafana dashboard JSON: %v", err)
	}

	if got := dashboard["uid"]; got != "soju-irc-bouncer" {
		t.Fatalf("unexpected dashboard uid: %v", got)
	}
	if got := dashboard["title"]; got != "Soju IRC Bouncer" {
		t.Fatalf("unexpected dashboard title: %v", got)
	}
	if got := dashboard["schemaVersion"]; got != float64(39) {
		t.Fatalf("unexpected dashboard schema version: %v", got)
	}
	inputs, ok := dashboard["__inputs"].([]any)
	if !ok || len(inputs) != 1 {
		t.Fatalf("dashboard must declare exactly one Prometheus data-source input, got %T %#v", dashboard["__inputs"], dashboard["__inputs"])
	}
	input, ok := inputs[0].(map[string]any)
	if !ok || input["name"] != "DS_PROMETHEUS" || input["pluginId"] != "prometheus" {
		t.Fatalf("dashboard has an invalid Prometheus data-source input: %#v", inputs[0])
	}

	panels, ok := dashboard["panels"].([]any)
	if !ok || len(panels) < 10 {
		t.Fatalf("dashboard must contain its operational panels, got %T with length %d", dashboard["panels"], len(panels))
	}
	panelIDs := make(map[float64]bool)
	queryCount := 0
	for i, rawPanel := range panels {
		panel, ok := rawPanel.(map[string]any)
		if !ok {
			t.Fatalf("panel %d is not an object", i)
		}
		id, ok := panel["id"].(float64)
		if !ok || id <= 0 || panelIDs[id] {
			t.Fatalf("panel %d has an invalid or duplicate id: %v", i, panel["id"])
		}
		panelIDs[id] = true
		assertPortableDatasource(t, fmt.Sprintf("panel %d", int(id)), panel["datasource"])

		grid, ok := panel["gridPos"].(map[string]any)
		if !ok {
			t.Fatalf("panel %d has no grid position", int(id))
		}
		x, xOK := grid["x"].(float64)
		width, widthOK := grid["w"].(float64)
		height, heightOK := grid["h"].(float64)
		if !xOK || !widthOK || !heightOK || x < 0 || width <= 0 || height <= 0 || x+width > 24 {
			t.Fatalf("panel %d has an invalid 24-column grid position: %#v", int(id), grid)
		}

		targets, ok := panel["targets"].([]any)
		if !ok || len(targets) == 0 {
			t.Fatalf("panel %d has no Prometheus targets", int(id))
		}
		for j, rawTarget := range targets {
			target, ok := rawTarget.(map[string]any)
			if !ok {
				t.Fatalf("panel %d target %d is not an object", int(id), j)
			}
			assertPortableDatasource(t, fmt.Sprintf("panel %d target %d", int(id), j), target["datasource"])
			expr, ok := target["expr"].(string)
			if !ok || strings.TrimSpace(expr) == "" {
				t.Fatalf("panel %d target %d has no PromQL expression", int(id), j)
			}
			if !strings.Contains(expr, `job=~"$job"`) || !strings.Contains(expr, `instance=~"$instance"`) {
				t.Errorf("panel %d target %d does not honor both dashboard selectors: %s", int(id), j, expr)
			}
			queryCount++
		}
	}
	if queryCount < 15 {
		t.Fatalf("dashboard contains too few Prometheus queries: %d", queryCount)
	}

	encoded := string(raw)
	requiredMetrics := []string{
		"soju_users_active",
		"soju_downstreams_active",
		"soju_upstreams_active",
		"soju_upstream_out_messages_total",
		"soju_upstream_in_messages_total",
		"soju_downstream_out_messages_total",
		"soju_downstream_in_messages_total",
		"soju_upstream_connect_errors_total",
		"soju_worker_panics_total",
		"process_start_time_seconds",
		"process_cpu_seconds_total",
		"process_resident_memory_bytes",
		"go_goroutines",
		"go_memstats_heap_alloc_bytes",
	}
	for _, metric := range requiredMetrics {
		if !strings.Contains(encoded, metric) {
			t.Errorf("dashboard does not query required metric %q", metric)
		}
	}

	for _, requirement := range []string{"${DS_PROMETHEUS}", "$job", "$instance", "$__rate_interval", "$__range"} {
		if !strings.Contains(encoded, requirement) {
			t.Errorf("dashboard does not contain portable selector %q", requirement)
		}
	}
	if strings.Contains(encoded, "10.69.0.22") || strings.Contains(encoded, "localhost:8081") {
		t.Error("dashboard JSON must not hard-code an operator host or exporter address")
	}
}

func assertPortableDatasource(t *testing.T, location string, raw any) {
	t.Helper()
	datasource, ok := raw.(map[string]any)
	if !ok || datasource["type"] != "prometheus" || datasource["uid"] != "${DS_PROMETHEUS}" {
		t.Errorf("%s does not use the imported Prometheus data source: %#v", location, raw)
	}
}

func TestPrometheusScrapeExampleContract(t *testing.T) {
	raw, err := os.ReadFile(scrapePath)
	if err != nil {
		t.Fatal(err)
	}
	config := string(raw)

	for _, required := range []string{
		"job_name: 'soju'",
		"scrape_interval: 30s",
		"scrape_timeout: 10s",
		"metrics_path: /metrics",
		"targets: ['127.0.0.1:8081']",
		"role: 'irc-bouncer'",
		"use: 'soju'",
		"environment: 'production'",
	} {
		if !strings.Contains(config, required) {
			t.Errorf("Prometheus example is missing %q", required)
		}
	}
	for _, forbidden := range []string{"10.69.0.22", "0.0.0.0", "http+insecure"} {
		if strings.Contains(config, forbidden) {
			t.Errorf("Prometheus example contains unsafe or operator-specific value %q", forbidden)
		}
	}
}
