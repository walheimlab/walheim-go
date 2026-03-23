package v1alpha1

import (
	"testing"
)

// ── aggregateStatus ───────────────────────────────────────────────────────────
//
// prefetchStatus is not tested here: it requires an SSH connection.
// See: internal/rsync and internal/ssh for similar untestable network code.

func TestAggregateStatus_unknown_hostNotQueried(t *testing.T) {
	results := map[string]containerStatus{}

	status, ready := aggregateStatus(results, "prod", "myapp")
	if status != "Unknown" {
		t.Errorf("status = %q, want %q", status, "Unknown")
	}

	if ready != "-" {
		t.Errorf("ready = %q, want %q", ready, "-")
	}
}

func TestAggregateStatus_notFound_noContainers(t *testing.T) {
	results := map[string]containerStatus{
		"_ns_prod": {queried: true},
	}

	status, ready := aggregateStatus(results, "prod", "myapp")
	if status != "NotFound" {
		t.Errorf("status = %q, want %q", status, "NotFound")
	}

	if ready != "-" {
		t.Errorf("ready = %q, want %q", ready, "-")
	}
}

func TestAggregateStatus_running(t *testing.T) {
	results := map[string]containerStatus{
		"_ns_prod":   {queried: true},
		"prod/myapp": {queried: true, states: []string{"running", "running"}},
	}

	status, ready := aggregateStatus(results, "prod", "myapp")
	if status != "Running" {
		t.Errorf("status = %q, want %q", status, "Running")
	}

	if ready != "2/2" {
		t.Errorf("ready = %q, want %q", ready, "2/2")
	}
}

func TestAggregateStatus_stopped(t *testing.T) {
	results := map[string]containerStatus{
		"_ns_prod":   {queried: true},
		"prod/myapp": {queried: true, states: []string{"exited", "exited"}},
	}

	status, ready := aggregateStatus(results, "prod", "myapp")
	if status != "Stopped" {
		t.Errorf("status = %q, want %q", status, "Stopped")
	}

	if ready != "0/2" {
		t.Errorf("ready = %q, want %q", ready, "0/2")
	}
}

func TestAggregateStatus_degraded(t *testing.T) {
	results := map[string]containerStatus{
		"_ns_prod":   {queried: true},
		"prod/myapp": {queried: true, states: []string{"running", "exited"}},
	}

	status, ready := aggregateStatus(results, "prod", "myapp")
	if status != "Degraded" {
		t.Errorf("status = %q, want %q", status, "Degraded")
	}

	if ready != "1/2" {
		t.Errorf("ready = %q, want %q", ready, "1/2")
	}
}

func TestAggregateStatus_paused(t *testing.T) {
	results := map[string]containerStatus{
		"_ns_prod":   {queried: true},
		"prod/myapp": {queried: true, states: []string{"paused"}},
	}

	status, _ := aggregateStatus(results, "prod", "myapp")
	if status != "Paused" {
		t.Errorf("status = %q, want %q", status, "Paused")
	}
}

func TestAggregateStatus_restarting(t *testing.T) {
	results := map[string]containerStatus{
		"_ns_prod":   {queried: true},
		"prod/myapp": {queried: true, states: []string{"restarting"}},
	}

	status, _ := aggregateStatus(results, "prod", "myapp")
	if status != "Restarting" {
		t.Errorf("status = %q, want %q", status, "Restarting")
	}
}

func TestAggregateStatus_singleRunningContainer(t *testing.T) {
	results := map[string]containerStatus{
		"_ns_ns1":  {queried: true},
		"ns1/app1": {queried: true, states: []string{"running"}},
	}

	status, ready := aggregateStatus(results, "ns1", "app1")
	if status != "Running" {
		t.Errorf("status = %q, want %q", status, "Running")
	}

	if ready != "1/1" {
		t.Errorf("ready = %q, want %q", ready, "1/1")
	}
}
