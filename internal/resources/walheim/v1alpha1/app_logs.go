package v1alpha1

import (
	"fmt"
	"strings"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/registry"
)

func (a *App) runLogs(opts registry.OperationOpts) error {
	namespace := opts.Namespace
	name := opts.Name

	nsMeta, err := a.loadNamespaceManifest(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	target := nsMeta.Spec.SSHTarget()

	follow := opts.Bool("follow")
	tail := opts.Int("tail")
	timestamps := opts.Bool("timestamps")
	service := opts.String("service")

	var cmdParts []string

	cmdParts = append(cmdParts, "cd "+nsMeta.Spec.RemoteBaseDir()+"/apps/"+name+" && docker compose logs")
	if follow {
		cmdParts = append(cmdParts, "--follow")
	}

	if tail != -1 {
		cmdParts = append(cmdParts, fmt.Sprintf("--tail %d", tail))
	}

	if timestamps {
		cmdParts = append(cmdParts, "--timestamps")
	}

	if service != "" {
		cmdParts = append(cmdParts, service)
	}

	cmd := strings.Join(cmdParts, " ")

	if opts.DryRun {
		fmt.Printf("Would run: ssh %s %q\n", target, cmd)
		return nil
	}

	sshClient := nsMeta.Spec.NewSSHClient()
	if follow {
		// Replace process via syscall.Exec for proper Ctrl+C handling
		return sshClient.Exec(cmd, false)
	}

	return sshClient.Run(cmd)
}
