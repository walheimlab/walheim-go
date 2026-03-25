package v1alpha1

import (
	"testing"
)

// prefetchJobStatus is not tested: it requires an SSH connection.

func TestAggregateJobStatus_unknown_hostNotQueried(t *testing.T) {
	results := map[string]jobRunInfo{}

	status, lastRun := aggregateJobStatus(results, "prod", "db-backup")
	if status != "Unknown" {
		t.Errorf("status = %q, want %q", status, "Unknown")
	}

	if lastRun != "-" {
		t.Errorf("lastRun = %q, want %q", lastRun, "-")
	}
}

func TestAggregateJobStatus_never_noContainers(t *testing.T) {
	results := map[string]jobRunInfo{
		"_ns_prod": {queried: true},
	}

	status, lastRun := aggregateJobStatus(results, "prod", "db-backup")
	if status != "Never" {
		t.Errorf("status = %q, want %q", status, "Never")
	}

	if lastRun != "-" {
		t.Errorf("lastRun = %q, want %q", lastRun, "-")
	}
}

func TestAggregateJobStatus_running(t *testing.T) {
	results := map[string]jobRunInfo{
		"_ns_prod":        {queried: true},
		"prod/db-backup":  {queried: true, state: "running", statusText: "Up 3 minutes", createdAt: "2024-01-15 10:30:00 +0000 UTC"},
	}

	status, _ := aggregateJobStatus(results, "prod", "db-backup")
	if status != "Running" {
		t.Errorf("status = %q, want %q", status, "Running")
	}
}

func TestAggregateJobStatus_succeeded(t *testing.T) {
	results := map[string]jobRunInfo{
		"_ns_prod":        {queried: true},
		"prod/db-backup":  {queried: true, state: "exited", statusText: "Exited (0) 2 hours ago", createdAt: "2024-01-15 08:00:00 +0000 UTC"},
	}

	status, _ := aggregateJobStatus(results, "prod", "db-backup")
	if status != "Succeeded" {
		t.Errorf("status = %q, want %q", status, "Succeeded")
	}
}

func TestAggregateJobStatus_failed(t *testing.T) {
	results := map[string]jobRunInfo{
		"_ns_prod":        {queried: true},
		"prod/db-backup":  {queried: true, state: "exited", statusText: "Exited (1) 1 hour ago", createdAt: "2024-01-15 09:00:00 +0000 UTC"},
	}

	status, _ := aggregateJobStatus(results, "prod", "db-backup")
	if status != "Failed" {
		t.Errorf("status = %q, want %q", status, "Failed")
	}
}

func TestAggregateJobStatus_pending(t *testing.T) {
	results := map[string]jobRunInfo{
		"_ns_prod":        {queried: true},
		"prod/db-backup":  {queried: true, state: "created", statusText: "Created", createdAt: "2024-01-15 10:00:00 +0000 UTC"},
	}

	status, _ := aggregateJobStatus(results, "prod", "db-backup")
	if status != "Pending" {
		t.Errorf("status = %q, want %q", status, "Pending")
	}
}

func TestAggregateJobStatus_lastRunFormatted(t *testing.T) {
	results := map[string]jobRunInfo{
		"_ns_prod":        {queried: true},
		"prod/db-backup":  {queried: true, state: "exited", statusText: "Exited (0) 1 hour ago", createdAt: "2024-01-15 10:30:00 +0000 UTC"},
	}

	_, lastRun := aggregateJobStatus(results, "prod", "db-backup")
	if lastRun != "2024-01-15 10:30:00" {
		t.Errorf("lastRun = %q, want %q", lastRun, "2024-01-15 10:30:00")
	}
}
