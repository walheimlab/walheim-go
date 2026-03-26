package v1alpha1

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/yamlutil"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

// appDescribeResult is the structured output for describe app, including runtime status.
type appDescribeResult struct {
	APIVersion string                 `json:"apiVersion" yaml:"apiVersion"`
	Kind       string                 `json:"kind" yaml:"kind"`
	Metadata   appDescribeMeta        `json:"metadata" yaml:"metadata"`
	Status     *apiv1alpha1.AppStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type appDescribeMeta struct {
	Name      string   `json:"name" yaml:"name"`
	Namespace string   `json:"namespace" yaml:"namespace"`
	Services  []string `json:"services,omitempty" yaml:"services,omitempty"`
	SSH       string   `json:"ssh" yaml:"ssh"`
}

func (a *App) runDescribe(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	namespace := opts.Namespace
	name := opts.Name

	_, m, err := a.getOne(namespace, name)
	if err != nil {
		output.Errorf(jsonMode, "NotFound",
			fmt.Sprintf("app %q not found in namespace %q", name, namespace), "", nil, false)

		return err
	}

	nsMeta, err := a.loadNamespaceManifest(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	target := nsMeta.Spec.SSHTarget()
	client := nsMeta.Spec.NewSSHClient()

	remoteAppDir := nsMeta.Spec.RemoteBaseDir() + "/apps/" + name
	remoteExists := false

	if _, checkErr := client.RunOutput("test -d " + remoteAppDir + " && echo yes"); checkErr == nil {
		remoteExists = true
	}

	composePS := ""
	if remoteExists {
		composePS, _ = client.RunOutput("cd " + remoteAppDir + " && docker compose ps 2>/dev/null")
	}

	statusMap := a.prefetchStatus([]string{namespace})
	state, ready := aggregateStatus(statusMap, namespace, name)

	status := &apiv1alpha1.AppStatus{
		State:     state,
		Ready:     ready,
		Deployed:  remoteExists,
		ComposePS: strings.TrimSpace(composePS),
	}

	if opts.Output == "json" || opts.Output == "yaml" {
		result := appDescribeResult{
			APIVersion: m.APIVersion,
			Kind:       m.Kind,
			Metadata: appDescribeMeta{
				Name:      name,
				Namespace: namespace,
				Services:  serviceNames(m),
				SSH:       target,
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

	fmt.Printf("Name:       %s\n", name)
	fmt.Printf("Namespace:  %s\n", namespace)
	fmt.Printf("SSH Target: %s\n", target)
	fmt.Println()

	fmt.Printf("Status:     %s\n", status.State)
	fmt.Printf("Ready:      %s\n", status.Ready)
	fmt.Printf("Remote:     ")

	if remoteExists {
		fmt.Println("deployed")
	} else {
		fmt.Println("not deployed")
	}

	fmt.Println()
	fmt.Println("Services:")

	for _, svcName := range serviceNames(m) {
		svc := m.Spec.Compose.Services[svcName]

		img := svc.Image
		if img == "" {
			img = "(no image)"
		}

		fmt.Printf("  %-20s %s\n", svcName, img)
	}

	if ps := strings.TrimSpace(composePS); ps != "" {
		fmt.Println()
		fmt.Println("docker compose ps:")

		for _, line := range strings.Split(ps, "\n") {
			fmt.Printf("  %s\n", line)
		}
	}

	return nil
}

// serviceNames returns sorted service names from an App.
func serviceNames(m *apiv1alpha1.App) []string {
	names := make([]string, 0, len(m.Spec.Compose.Services))
	for n := range m.Spec.Compose.Services {
		names = append(names, n)
	}

	sort.Strings(names)

	return names
}
