package v1alpha1

import (
	"fmt"
	"strings"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/registry"
)

func (j *Job) runRun(opts registry.OperationOpts) error {
	namespace := opts.Namespace
	name := opts.Name

	_, m, err := j.getOne(namespace, name)
	if err != nil {
		return err
	}

	nsMeta, err := j.loadNamespaceManifest(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	target := nsMeta.Spec.SSHTarget()
	localResourceDir := j.ResourceDir(namespace, name)
	remoteResourceDir := nsMeta.Spec.RemoteBaseDir() + "/jobs/" + name

	if err := generateJobCompose(localResourceDir, namespace, name, m.Spec, j.FS, j.DataDir); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("generate docker-compose: %w", err))
	}

	detach := opts.Bool("detach")

	var cmdParts []string

	cmdParts = append(cmdParts, "cd "+remoteResourceDir+" && docker compose --progress plain run --rm")
	if detach {
		cmdParts = append(cmdParts, "--detach")
	}

	cmdParts = append(cmdParts, "job")
	cmd := strings.Join(cmdParts, " ")

	if opts.DryRun {
		fmt.Printf("Would rsync and run on %s: %s\n", target, cmd)
		return nil
	}

	if err := nsMeta.Spec.NewSyncer().Sync(j.FS, localResourceDir, target, remoteResourceDir); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("rsync: %w", err))
	}

	sshClient := nsMeta.Spec.NewSSHClient()
	if err := sshClient.Run(cmd); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("job %q failed: %w", name, err))
	}

	return nil
}
