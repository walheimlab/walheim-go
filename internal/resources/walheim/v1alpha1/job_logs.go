package v1alpha1

import (
	"fmt"
	"strings"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/registry"
)

func (j *Job) runLogs(opts registry.OperationOpts) error {
	namespace := opts.Namespace
	name := opts.Name

	nsMeta, err := j.loadNamespaceManifest(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	follow := opts.Bool("follow")
	tail := opts.Int("tail")

	remoteResourceDir := nsMeta.Spec.RemoteBaseDir() + "/jobs/" + name

	var cmdParts []string

	cmdParts = append(cmdParts, "cd "+remoteResourceDir+" && docker compose logs")
	if follow {
		cmdParts = append(cmdParts, "--follow")
	}

	if tail != -1 {
		cmdParts = append(cmdParts, fmt.Sprintf("--tail %d", tail))
	}

	cmdParts = append(cmdParts, "job")
	cmd := strings.Join(cmdParts, " ")

	if opts.DryRun {
		fmt.Printf("Would run on %s: %s\n", nsMeta.Spec.SSHTarget(), cmd)
		return nil
	}

	sshClient := nsMeta.Spec.NewSSHClient()
	if follow {
		return sshClient.Exec(cmd, false)
	}

	return sshClient.Run(cmd)
}
