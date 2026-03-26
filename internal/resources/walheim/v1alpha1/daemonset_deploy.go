package v1alpha1

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/rsync"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

func (d *DaemonSet) runStart(opts registry.OperationOpts) error {
	name := opts.Name

	_, m, err := d.getOne(name)
	if err != nil {
		return err
	}

	nsMetas, nsNames, err := matchingNamespaces(m.Spec.NamespaceSelector, d.FS, d.DataDir)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	// Reconcile: remove from namespaces that no longer match the selector.
	deployed, err := d.deployedNamespaces(name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	desired := make(map[string]bool, len(nsNames))
	for _, ns := range nsNames {
		desired[ns] = true
	}

	var toRemove []string

	for _, ns := range deployed {
		if !desired[ns] {
			toRemove = append(toRemove, ns)
		}
	}

	if opts.DryRun {
		for _, ns := range toRemove {
			fmt.Printf("Would remove daemonset %q from namespace %q (no longer selected)\n", name, ns)
		}
	} else if len(toRemove) > 0 {
		removeErrs := make([]error, len(toRemove))

		var removeWg sync.WaitGroup

		for i, ns := range toRemove {
			removeWg.Add(1)

			go func(i int, ns string) {
				defer removeWg.Done()

				removeErrs[i] = d.stopInNamespace(name, ns)
			}(i, ns)
		}

		removeWg.Wait()

		for i, err := range removeErrs {
			if err != nil {
				return err
			}

			fmt.Printf("Removed daemonset %q from namespace %q (no longer selected)\n", name, toRemove[i])
		}
	}

	if len(nsNames) == 0 {
		fmt.Printf("Daemonset %q: no matching namespaces\n", name)
		return nil
	}

	if opts.DryRun {
		fmt.Printf("Would deploy daemonset %q to namespaces: %s\n", name, strings.Join(nsNames, ", "))
		return nil
	}

	deployErrs := make([]error, len(nsNames))

	var deployWg sync.WaitGroup

	for i, ns := range nsNames {
		deployWg.Add(1)

		go func(i int, ns string, nsMeta *apiv1alpha1.Namespace) {
			defer deployWg.Done()

			mc, err := copyDaemonSetManifest(m)
			if err != nil {
				deployErrs[i] = exitErr(exitcode.Failure, fmt.Errorf("copy manifest for namespace %q: %w", ns, err))
				return
			}

			if err := generateDaemonSetCompose(ns, name, mc, d.FS, d.DataDir); err != nil {
				deployErrs[i] = exitErr(exitcode.Failure, fmt.Errorf("generate compose for namespace %q: %w", ns, err))
				return
			}

			target := nsMeta.Spec.SSHTarget()
			localDir := filepath.Join(d.DataDir, "daemonsets", name, ns)
			remoteDir := nsMeta.Spec.RemoteBaseDir() + "/daemonsets/" + name

			if err := rsync.NewSyncer().Sync(d.FS, localDir, target, remoteDir); err != nil {
				deployErrs[i] = exitErr(exitcode.Failure, fmt.Errorf("rsync to %q: %w", ns, err))
				return
			}

			sshClient := nsMeta.Spec.NewSSHClient()

			cmd := "cd " + remoteDir + " && docker compose --progress plain up -d --remove-orphans"
			if err := sshClient.Run(cmd); err != nil {
				deployErrs[i] = exitErr(exitcode.Failure, fmt.Errorf("docker compose up in %q: %w", ns, err))
			}
		}(i, ns, nsMetas[i])
	}

	deployWg.Wait()

	for i, err := range deployErrs {
		if err != nil {
			return err
		}

		fmt.Printf("Started daemonset %q in namespace %q\n", name, nsNames[i])
	}

	return nil
}

func (d *DaemonSet) runStop(opts registry.OperationOpts) error {
	name := opts.Name

	deployed, err := d.deployedNamespaces(name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if len(deployed) == 0 {
		fmt.Printf("Daemonset %q is not deployed anywhere\n", name)
		return nil
	}

	if opts.DryRun {
		fmt.Printf("Would stop daemonset %q on namespaces: %s\n", name, strings.Join(deployed, ", "))
		return nil
	}

	stopErrs := make([]error, len(deployed))

	var wg sync.WaitGroup

	for i, ns := range deployed {
		wg.Add(1)

		go func(i int, ns string) {
			defer wg.Done()

			stopErrs[i] = d.stopInNamespace(name, ns)
		}(i, ns)
	}

	wg.Wait()

	for i, err := range stopErrs {
		if err != nil {
			return err
		}

		fmt.Printf("Stopped daemonset %q in namespace %q\n", name, deployed[i])
	}

	return nil
}
