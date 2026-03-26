package v1alpha1

import (
	"fmt"
	"strings"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/registry"
)

func (a *App) runExec(opts registry.OperationOpts) error {
	namespace := opts.Namespace
	name := opts.Name

	_, m, err := a.getOne(namespace, name)
	if err != nil {
		return err
	}

	nsMeta, err := a.loadNamespaceManifest(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	target := nsMeta.Spec.SSHTarget()

	service := opts.String("service")
	if service == "" {
		for svcName := range m.Spec.Compose.Services {
			service = svcName
			break
		}
	}

	if service == "" {
		return exitErr(exitcode.UsageError, fmt.Errorf("no services defined in app %q", name))
	}

	tty := opts.Bool("tty")

	execCmd := opts.String("cmd")
	if execCmd == "" {
		execCmd = "sh"
	}

	var cmdParts []string

	cmdParts = append(cmdParts, "cd "+nsMeta.Spec.RemoteBaseDir()+"/apps/"+name+" && docker compose exec")
	if !tty {
		cmdParts = append(cmdParts, "--no-TTY")
	}

	cmdParts = append(cmdParts, service, execCmd)
	cmd := strings.Join(cmdParts, " ")

	if opts.DryRun {
		fmt.Printf("Would run: ssh %s %q\n", target, cmd)
		return nil
	}

	sshClient := nsMeta.Spec.NewSSHClient()

	return sshClient.Exec(cmd, tty)
}
