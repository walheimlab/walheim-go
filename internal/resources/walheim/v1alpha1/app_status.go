package v1alpha1

import (
	"fmt"
	"strings"
	"sync"

	"github.com/walheimlab/walheim-go/internal/ssh"
)

// containerStatus holds the raw container states reported by docker ps for one app.
type containerStatus struct {
	queried bool     // true if the SSH call to this app's host succeeded
	states  []string // one entry per container: "running", "exited", "paused", etc.
}

// prefetchStatus queries each unique SSH host once, concurrently, and returns
// a map keyed by "namespace/name" → containerStatus.
// A sentinel key "_ns_<namespace>" records whether the host was reachable.
func (a *App) prefetchStatus(namespaces []string) map[string]containerStatus {
	// 1. Resolve unique hosts for each namespace.
	type nsHost struct {
		ns     string
		client *ssh.Client
	}

	var pairs []nsHost

	seen := map[string]bool{}

	for _, ns := range namespaces {
		m, err := a.loadNamespaceManifest(ns)
		if err != nil {
			continue
		}

		host := m.Spec.SSHTarget()
		if !seen[host] {
			seen[host] = true

			pairs = append(pairs, nsHost{ns, m.Spec.NewSSHClient()})
		}
	}

	// 2. One goroutine per unique host.
	mu := sync.Mutex{}
	results := map[string]containerStatus{}

	var wg sync.WaitGroup

	for _, p := range pairs {
		wg.Add(1)

		go func() {
			defer wg.Done()

			client := p.client
			out, err := client.RunOutput(
				`docker ps -a --filter label=walheim.managed=true` +
					` --format '{{.Label "walheim.namespace"}}|{{.Label "walheim.app"}}|{{.State}}'`)

			mu.Lock()
			defer mu.Unlock()
			// Record reachability for every namespace on this host.
			sentinel := containerStatus{queried: err == nil}
			results["_ns_"+p.ns] = sentinel

			if err != nil {
				return
			}

			for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
				parts := strings.SplitN(line, "|", 3)
				if len(parts) < 3 {
					continue
				}

				key := parts[0] + "/" + parts[1]
				cs := results[key]
				cs.queried = true
				cs.states = append(cs.states, parts[2])
				results[key] = cs
			}
		}()
	}

	wg.Wait()

	return results
}

// aggregateStatus derives human-readable STATUS and READY from raw states.
func aggregateStatus(results map[string]containerStatus, namespace, name string) (status, ready string) {
	nsKey := "_ns_" + namespace
	appKey := namespace + "/" + name

	if !results[nsKey].queried {
		return "Unknown", "-"
	}

	cs := results[appKey]
	if len(cs.states) == 0 {
		return "NotFound", "-"
	}

	total := len(cs.states)

	counts := map[string]int{}
	for _, s := range cs.states {
		counts[s]++
	}

	running := counts["running"]
	ready = fmt.Sprintf("%d/%d", running, total)

	switch {
	case running == total:
		status = "Running"
	case counts["exited"] == total:
		status = "Stopped"
	case running > 0:
		status = "Degraded"
	case counts["paused"] > 0:
		status = "Paused"
	case counts["restarting"] > 0:
		status = "Restarting"
	default:
		status = "Unknown"
	}

	return
}
