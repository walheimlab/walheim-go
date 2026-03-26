package v1alpha1

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/ssh"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

type namespaceDescribeResult struct {
	APIVersion string                       `json:"apiVersion" yaml:"apiVersion"`
	Kind       string                       `json:"kind" yaml:"kind"`
	Metadata   namespaceDescribeMeta        `json:"metadata" yaml:"metadata"`
	Spec       namespaceDescribeSpec        `json:"spec" yaml:"spec"`
	Status     *apiv1alpha1.NamespaceStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type namespaceDescribeMeta struct {
	Name string `json:"name" yaml:"name"`
}

type namespaceDescribeSpec struct {
	Hostname string `json:"hostname" yaml:"hostname"`
	Username string `json:"username,omitempty" yaml:"username,omitempty"`
	BaseDir  string `json:"baseDir" yaml:"baseDir"`
}

func (n *Namespace) runDescribe(opts registry.OperationOpts) error {
	name := opts.Name

	_, m, err := n.getOne(name)
	if err != nil {
		output.Errorf(false, "NotFound",
			fmt.Sprintf("namespace %q not found", name), "", nil, false)

		return err
	}

	return n.describeHuman(m)
}

// buildDescribeStatus connects to the namespace host and collects runtime status.
func (n *Namespace) buildDescribeStatus(ns *apiv1alpha1.Namespace) *apiv1alpha1.NamespaceStatus {
	name := ns.Name
	status := &apiv1alpha1.NamespaceStatus{
		Resources: n.countLocalResources(name),
	}

	client := ns.Spec.NewSSHClient()
	if client.TestConnection() {
		status.Connection = "Connected"
		status.Docker = namespaceDockerStatus(client)
		info := namespaceCollectStatus(client, name, n.localAppNames(name), n.localDaemonSetNames(name))
		status.DeployedApps = info.DeployedApps
		status.Containers = info.Containers
		status.Usage = namespaceUsageInfo(client)
	} else {
		status.Connection = "Failed"
	}

	return status
}

func (n *Namespace) describeHuman(m *apiv1alpha1.Namespace) error {
	fmt.Printf("Name:      %s\n", m.Name)
	fmt.Printf("Hostname:  %s\n", m.Spec.Hostname)
	fmt.Printf("Username:  %s\n", m.Spec.UsernameDisplay())
	fmt.Printf("Base Dir:  %s\n", m.Spec.BaseDirDisplay())
	fmt.Printf("SSH:       %s\n", m.Spec.SSHTarget())
	fmt.Println()

	fmt.Println("Status:")

	status := n.buildDescribeStatus(m)

	fmt.Printf("  Connection:  %s\n", status.Connection)

	if status.Connection == "Failed" {
		fmt.Println()
		fmt.Println("  Unable to connect. Check SSH configuration.")

		return nil
	}

	if d := status.Docker; d != nil {
		if d.Available {
			fmt.Printf("  Docker:      Available (v%s)\n", d.Version)
		} else {
			fmt.Println("  Docker:      Not available")
		}
	}

	if len(status.DeployedApps) > 0 {
		fmt.Println()
		fmt.Println("  Deployed Apps:")

		for _, a := range status.DeployedApps {
			fmt.Printf("    %-20s %-12s %d/%d\n", a.Name, a.State, a.Running, a.Total)
		}
	}

	if len(status.Containers) > 0 {
		fmt.Println()
		fmt.Println("  Containers:")
		fmt.Printf("    %-30s %-30s %-10s %-25s %s\n", "NAME", "OWNER", "STATE", "STATUS", "WALHEIM")

		for _, c := range status.Containers {
			owner := c.OwnerName
			if c.OwnerKind != "" && c.OwnerName != "" {
				owner = c.OwnerKind + "/" + c.OwnerName
			}

			fmt.Printf("    %-30s %-30s %-10s %-25s %s\n", c.Name, owner, c.State, c.DockerStatus, c.Management)
		}
	}

	fmt.Println()
	fmt.Println("  Resources:")
	fmt.Printf("    Apps:       %d\n", status.Resources.Apps)
	fmt.Printf("    Secrets:    %d\n", status.Resources.Secrets)
	fmt.Printf("    ConfigMaps: %d\n", status.Resources.ConfigMaps)

	if u := status.Usage; u != nil {
		fmt.Println()
		fmt.Println("  Usage:")

		if d := u.Disk; d != nil {
			fmt.Printf("    Disk:        %s used of %s\n", d.Used, d.Total)
		}

		if c := u.Containers; c != nil {
			fmt.Printf("    Containers:  %d running, %d stopped\n", c.Running, c.Stopped)
		}
	}

	return nil
}

func namespaceDockerStatus(client *ssh.Client) *apiv1alpha1.NamespaceDockerStatus {
	out, err := client.RunOutput("docker --version 2>/dev/null")
	if err != nil || strings.TrimSpace(out) == "" {
		return &apiv1alpha1.NamespaceDockerStatus{Available: false}
	}

	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) >= 3 {
		return &apiv1alpha1.NamespaceDockerStatus{
			Available: true,
			Version:   strings.TrimSuffix(parts[2], ","),
		}
	}

	return &apiv1alpha1.NamespaceDockerStatus{Available: true}
}

type namespaceStatusInfo struct {
	DeployedApps []apiv1alpha1.NamespaceDeployedApp
	Containers   []apiv1alpha1.NamespaceContainerStatus
}

// namespaceCollectStatus fetches all containers on the host in a single
// docker ps call and returns both the deployed-app summary (walheim-managed,
// aggregated by app) and the full per-container list with management status.
func namespaceCollectStatus(client *ssh.Client, nsName string, localApps, localDaemonSets map[string]struct{}) namespaceStatusInfo {
	cmd := `docker ps -a --format '{{.Names}}|{{.Label "walheim.namespace"}}|{{.Label "walheim.kind"}}|{{.Label "walheim.owner"}}|{{.State}}|{{.Status}}' 2>/dev/null`

	out, err := client.RunOutput(cmd)
	if err != nil || strings.TrimSpace(out) == "" {
		return namespaceStatusInfo{}
	}

	type appAgg struct {
		state   string
		running int
		total   int
	}

	appMap := make(map[string]*appAgg)

	var appOrder []string

	var containers []apiv1alpha1.NamespaceContainerStatus

	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 6)
		if len(parts) < 6 {
			continue
		}

		containerName, labelNs, labelKind, labelOwner, state, dockerStatus := parts[0], parts[1], parts[2], parts[3], parts[4], parts[5]

		management := containerManagement(labelNs, labelKind, labelOwner, nsName, localApps, localDaemonSets)

		containers = append(containers, apiv1alpha1.NamespaceContainerStatus{
			Name:         containerName,
			OwnerName:    labelOwner,
			OwnerKind:    labelKind,
			State:        state,
			DockerStatus: dockerStatus,
			Management:   management,
		})

		// Aggregate deployed-app summary for App-kind containers in this namespace.
		if labelNs == nsName && labelKind == "App" && labelOwner != "" {
			if _, ok := localApps[labelOwner]; ok {
				if _, ok := appMap[labelOwner]; !ok {
					appMap[labelOwner] = &appAgg{}
					appOrder = append(appOrder, labelOwner)
				}

				a := appMap[labelOwner]
				a.total++

				if strings.ToLower(state) == "running" {
					a.running++
				}

				if a.state == "" || strings.ToLower(state) != "running" {
					a.state = state
				}
			}
		}
	}

	apps := make([]apiv1alpha1.NamespaceDeployedApp, 0, len(appOrder))
	for _, appName := range appOrder {
		a := appMap[appName]
		apps = append(apps, apiv1alpha1.NamespaceDeployedApp{
			Name:    appName,
			State:   namespaceAppState(a.state, a.running, a.total),
			Running: a.running,
			Total:   a.total,
		})
	}

	return namespaceStatusInfo{DeployedApps: apps, Containers: containers}
}

func containerManagement(labelNs, labelKind, labelOwner, nsName string, localApps, localDaemonSets map[string]struct{}) string {
	if labelNs != nsName || labelOwner == "" {
		return "unmanaged"
	}

	switch labelKind {
	case "App":
		if _, ok := localApps[labelOwner]; ok {
			return "managed"
		}
	case "DaemonSet":
		if _, ok := localDaemonSets[labelOwner]; ok {
			return "managed"
		}
	}

	return "orphan"
}

func namespaceAppState(rawState string, running, total int) string {
	l := strings.ToLower(rawState)
	switch {
	case running == total && total > 0:
		return "Running"
	case running > 0:
		return "Degraded"
	case l == "paused":
		return "Paused"
	case l == "exited", l == "dead", l == "stopped":
		return "Stopped"
	default:
		return "Unknown"
	}
}

func namespaceUsageInfo(client *ssh.Client) *apiv1alpha1.NamespaceUsage {
	usage := &apiv1alpha1.NamespaceUsage{}
	hasData := false

	diskOut, _ := client.RunOutput("df -h /data 2>/dev/null | tail -1")
	if line := strings.TrimSpace(diskOut); line != "" {
		if parts := strings.Fields(line); len(parts) >= 3 {
			usage.Disk = &apiv1alpha1.NamespaceDiskUsage{Used: parts[2], Total: parts[1]}
			hasData = true
		}
	}

	ctOut, _ := client.RunOutput("docker ps -q 2>/dev/null | wc -l; docker ps -aq 2>/dev/null | wc -l")
	if lines := strings.Split(strings.TrimSpace(ctOut), "\n"); len(lines) >= 2 {
		run, err1 := strconv.Atoi(strings.TrimSpace(lines[0]))
		total, err2 := strconv.Atoi(strings.TrimSpace(lines[1]))

		if err1 == nil && err2 == nil {
			usage.Containers = &apiv1alpha1.NamespaceContainerCounts{Running: run, Stopped: total - run}
			hasData = true
		}
	}

	if !hasData {
		return nil
	}

	return usage
}
