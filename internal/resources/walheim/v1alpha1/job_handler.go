package v1alpha1

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/resource"
	"github.com/walheimlab/walheim-go/internal/ssh"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

var jobKind = resource.KindInfo{
	Group:   "walheim",
	Version: "v1alpha1",
	Kind:    "Job",
	Plural:  "jobs",
	Aliases: []string{},
}

// Job is the handler for the Job resource kind.
type Job struct {
	resource.NamespacedBase
}

func newJob(dataDir string, filesystem fs.FS) *Job {
	return &Job{
		NamespacedBase: resource.NamespacedBase{
			DataDir:          dataDir,
			FS:               filesystem,
			Info:             jobKind,
			ManifestFilename: ".job.yaml",
		},
	}
}

func (j *Job) KindInfo() resource.KindInfo { return jobKind }

func (j *Job) loadNamespaceManifest(namespace string) (*apiv1alpha1.Namespace, error) {
	path := filepath.Join(j.DataDir, "namespaces", namespace, ".namespace.yaml")

	data, err := j.FS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("namespace %q not found", namespace)
	}

	var m apiv1alpha1.Namespace
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	return &m, nil
}

type jobRunInfo struct {
	queried    bool
	state      string
	statusText string
	createdAt  string
}

// prefetchJobStatus queries each unique SSH host once (concurrently) and
// returns a map keyed by "namespace/name" → jobRunInfo (most recent run).
func (j *Job) prefetchJobStatus(namespaces []string) map[string]jobRunInfo {
	type nsHost struct {
		ns     string
		client *ssh.Client
	}

	var pairs []nsHost

	seen := map[string]bool{}

	for _, ns := range namespaces {
		m, err := j.loadNamespaceManifest(ns)
		if err != nil {
			continue
		}

		host := m.Spec.SSHTarget()
		if !seen[host] {
			seen[host] = true

			pairs = append(pairs, nsHost{ns, m.Spec.NewSSHClient()})
		}
	}

	mu := sync.Mutex{}
	results := map[string]jobRunInfo{}

	var wg sync.WaitGroup

	for _, p := range pairs {
		wg.Add(1)

		go func() {
			defer wg.Done()

			client := p.client
			out, err := client.RunOutput(
				`docker ps -a` +
					` --filter "label=walheim.managed=true"` +
					` --filter "label=walheim.job"` +
					` --format '{{.Label "walheim.namespace"}}|{{.Label "walheim.job"}}|{{.State}}|{{.Status}}|{{.CreatedAt}}'`)

			mu.Lock()
			defer mu.Unlock()

			results["_ns_"+p.ns] = jobRunInfo{queried: err == nil}
			if err != nil {
				return
			}

			for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}

				parts := strings.SplitN(line, "|", 5)
				if len(parts) < 5 {
					continue
				}

				ns, name := parts[0], parts[1]
				key := ns + "/" + name

				if _, exists := results[key]; !exists {
					results[key] = jobRunInfo{
						queried:    true,
						state:      parts[2],
						statusText: parts[3],
						createdAt:  parts[4],
					}
				}
			}
		}()
	}

	wg.Wait()

	return results
}

// aggregateJobStatus derives STATUS and LAST RUN from the prefetched map.
func aggregateJobStatus(results map[string]jobRunInfo, namespace, name string) (status, lastRun string) {
	if !results["_ns_"+namespace].queried {
		return "Unknown", "-"
	}

	info, ok := results[namespace+"/"+name]
	if !ok || info.state == "" {
		return "Never", "-"
	}

	switch info.state {
	case "running":
		status = "Running"
	case "exited":
		if strings.Contains(info.statusText, "(0)") {
			status = "Succeeded"
		} else {
			status = "Failed"
		}
	case "created":
		status = "Pending"
	default:
		status = "Unknown"
	}

	parts := strings.Fields(info.createdAt)
	if len(parts) >= 2 {
		lastRun = parts[0] + " " + parts[1]
	} else {
		lastRun = info.createdAt
	}

	return
}
