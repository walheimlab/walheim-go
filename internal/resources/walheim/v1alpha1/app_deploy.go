package v1alpha1

import (
	"fmt"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/rsync"
	"github.com/walheimlab/walheim-go/internal/ssh"
)

func (a *App) runStart(opts registry.OperationOpts) error {
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

	if err := generateCompose(namespace, name, m, a.FS, a.DataDir); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("generate compose: %w", err))
	}

	if opts.DryRun {
		fmt.Printf("Would rsync and docker compose up for app %q in namespace %q\n", name, namespace)
		return nil
	}

	localDir := a.ResourceDir(namespace, name)
	remoteDir := nsMeta.Spec.RemoteBaseDir() + "/apps/" + name

	if err := rsync.NewSyncer().Sync(a.FS, localDir, target, remoteDir); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("rsync: %w", err))
	}

	sshClient := ssh.NewClient(target)

	cmd := "cd " + remoteDir + " && docker compose --progress plain up -d --remove-orphans"
	if err := sshClient.Run(cmd); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("docker compose up: %w", err))
	}

	fmt.Printf("Started app %q\n", name)

	return nil
}

func (a *App) runPause(opts registry.OperationOpts) error {
	namespace := opts.Namespace
	name := opts.Name

	nsMeta, err := a.loadNamespaceManifest(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	target := nsMeta.Spec.SSHTarget()

	if opts.DryRun {
		fmt.Printf("Would run docker compose down for app %q in namespace %q\n", name, namespace)
		return nil
	}

	remoteAppDir := nsMeta.Spec.RemoteBaseDir() + "/apps/" + name
	sshClient := ssh.NewClient(target)

	_, checkErr := sshClient.RunOutput("test -d " + remoteAppDir)
	if checkErr != nil {
		fmt.Printf("App %q is not deployed\n", name)
		return nil
	}

	// Only run compose down if the compose file is present; the dir may exist
	// after a partial deploy that never wrote docker-compose.yml.
	_, composeErr := sshClient.RunOutput("test -f " + remoteAppDir + "/docker-compose.yml")
	if composeErr == nil {
		if err := sshClient.Run("cd " + remoteAppDir + " && docker compose --progress plain down"); err != nil {
			return exitErr(exitcode.Failure, fmt.Errorf("docker compose down: %w", err))
		}
	}

	fmt.Printf("Paused app %q\n", name)

	return nil
}

func (a *App) runStop(opts registry.OperationOpts) error {
	namespace := opts.Namespace
	name := opts.Name

	if err := a.runPause(opts); err != nil {
		return err
	}

	if opts.DryRun {
		fmt.Printf("Would remove remote files for app %q\n", name)
		return nil
	}

	nsMeta, err := a.loadNamespaceManifest(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	target := nsMeta.Spec.SSHTarget()

	sshClient := ssh.NewClient(target)
	if err := sshClient.Run("rm -rf " + nsMeta.Spec.RemoteBaseDir() + "/apps/" + name); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("remove remote files: %w", err))
	}

	fmt.Printf("Stopped app %q\n", name)

	return nil
}

func (a *App) runPull(opts registry.OperationOpts) error {
	namespace := opts.Namespace
	name := opts.Name

	nsMeta, err := a.loadNamespaceManifest(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	target := nsMeta.Spec.SSHTarget()

	if opts.DryRun {
		fmt.Printf("Would run docker compose pull for app %q in namespace %q\n", name, namespace)
		return nil
	}

	remoteAppDir := nsMeta.Spec.RemoteBaseDir() + "/apps/" + name
	sshClient := ssh.NewClient(target)

	_, checkErr := sshClient.RunOutput("test -d " + remoteAppDir)
	if checkErr != nil {
		msg := fmt.Sprintf("app %q is not deployed in namespace %q", name, namespace)
		output.Errorf(opts.Output == "json", "NotFound", msg, "Run 'whctl start app "+name+" -n "+namespace+"' to deploy it first.", nil, false)

		return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
	}

	if err := sshClient.Run("cd " + remoteAppDir + " && docker compose --progress plain pull"); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("docker compose pull: %w", err))
	}

	fmt.Printf("Run 'whctl start app %s -n %s' to apply pulled images\n", name, namespace)

	return nil
}
