package v1alpha1

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/ssh"
	"github.com/walheimlab/walheim-go/internal/yamlutil"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

type daemonSetDescribeResult struct {
	APIVersion string                       `json:"apiVersion" yaml:"apiVersion"`
	Kind       string                       `json:"kind" yaml:"kind"`
	Metadata   daemonSetDescribeMeta        `json:"metadata" yaml:"metadata"`
	Status     *apiv1alpha1.DaemonSetStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type daemonSetDescribeMeta struct {
	Name     string   `json:"name" yaml:"name"`
	Selector string   `json:"selector" yaml:"selector"`
	Services []string `json:"services,omitempty" yaml:"services,omitempty"`
}

// fetchDaemonSetStatus queries each matching namespace concurrently and returns
// the per-namespace container status for the given daemonset.
func (d *DaemonSet) fetchDaemonSetStatus(dsName string, nsMetas []*apiv1alpha1.Namespace, nsNames []string) []apiv1alpha1.DaemonSetNamespaceStatus {
	results := make([]apiv1alpha1.DaemonSetNamespaceStatus, len(nsNames))

	var wg sync.WaitGroup

	for i, ns := range nsNames {
		wg.Add(1)

		go func(i int, ns string, nsMeta *apiv1alpha1.Namespace) {
			defer wg.Done()

			target := nsMeta.Spec.SSHTarget()
			client := ssh.NewClient(target)

			nsStatus := apiv1alpha1.DaemonSetNamespaceStatus{Namespace: ns}

			remoteDir := nsMeta.Spec.RemoteBaseDir() + "/daemonsets/" + dsName
			if _, err := client.RunOutput("test -d " + remoteDir + " && echo yes"); err == nil {
				nsStatus.Deployed = true
			}

			out, err := client.RunOutput(
				`docker ps -a --filter label=walheim.managed=true` +
					` --filter label=walheim.namespace=` + ns +
					` --filter label=walheim.daemonset=` + dsName +
					` --format '{{.State}}'`)
			if err != nil {
				nsStatus.State = "Unknown"
				nsStatus.Ready = "-"
				results[i] = nsStatus

				return
			}

			var states []string

			for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
				if line != "" {
					states = append(states, line)
				}
			}

			if len(states) == 0 {
				nsStatus.State = "NotFound"
				nsStatus.Ready = "-"
				results[i] = nsStatus

				return
			}

			total := len(states)
			counts := map[string]int{}

			for _, s := range states {
				counts[s]++
			}

			running := counts["running"]
			nsStatus.Ready = fmt.Sprintf("%d/%d", running, total)

			switch {
			case running == total:
				nsStatus.State = "Running"
			case counts["exited"] == total:
				nsStatus.State = "Stopped"
			case running > 0:
				nsStatus.State = "Degraded"
			case counts["paused"] > 0:
				nsStatus.State = "Paused"
			case counts["restarting"] > 0:
				nsStatus.State = "Restarting"
			default:
				nsStatus.State = "Unknown"
			}

			results[i] = nsStatus
		}(i, ns, nsMetas[i])
	}

	wg.Wait()

	return results
}

func (d *DaemonSet) runDescribe(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	name := opts.Name

	_, m, err := d.getOne(name)
	if err != nil {
		output.Errorf(jsonMode, "NotFound",
			fmt.Sprintf("daemonset %q not found", name), "", nil, false)

		return err
	}

	nsMetas, nsNames, err := matchingNamespaces(m.Spec.NamespaceSelector, d.FS, d.DataDir)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	nsStatuses := d.fetchDaemonSetStatus(name, nsMetas, nsNames)

	status := &apiv1alpha1.DaemonSetStatus{Namespaces: nsStatuses}

	selector := "(all)"

	if m.Spec.NamespaceSelector != nil && len(m.Spec.NamespaceSelector.MatchLabels) != 0 {
		parts := make([]string, 0, len(m.Spec.NamespaceSelector.MatchLabels))
		for k, v := range m.Spec.NamespaceSelector.MatchLabels {
			parts = append(parts, k+"="+v)
		}

		sort.Strings(parts)
		selector = strings.Join(parts, ",")
	}

	svcNames := make([]string, 0, len(m.Spec.Compose.Services))
	for svcName := range m.Spec.Compose.Services {
		svcNames = append(svcNames, svcName)
	}

	sort.Strings(svcNames)

	if opts.Output == "json" || opts.Output == "yaml" {
		result := daemonSetDescribeResult{
			APIVersion: m.APIVersion,
			Kind:       m.Kind,
			Metadata: daemonSetDescribeMeta{
				Name:     name,
				Selector: selector,
				Services: svcNames,
			},
			Status: status,
		}

		if opts.Output == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")

			return enc.Encode(result)
		}

		data, err := yamlutil.Marshal(result)
		if err != nil {
			return err
		}

		fmt.Print(string(data))

		return nil
	}

	fmt.Printf("Name:      %s\n", name)
	fmt.Printf("Selector:  %s\n", selector)
	fmt.Println()

	fmt.Println("Services:")

	for _, svcName := range svcNames {
		svc := m.Spec.Compose.Services[svcName]

		img := svc.Image
		if img == "" {
			img = "(no image)"
		}

		fmt.Printf("  %-20s %s\n", svcName, img)
	}

	fmt.Println()

	if len(nsStatuses) == 0 {
		fmt.Println("Namespaces: (none matched)")
	} else {
		fmt.Println("Namespaces:")
		fmt.Printf("  %-20s %-12s %-8s %s\n", "NAMESPACE", "STATE", "READY", "DEPLOYED")

		for _, ns := range nsStatuses {
			deployed := "no"
			if ns.Deployed {
				deployed = "yes"
			}

			fmt.Printf("  %-20s %-12s %-8s %s\n", ns.Namespace, ns.State, ns.Ready, deployed)
		}
	}

	return nil
}
