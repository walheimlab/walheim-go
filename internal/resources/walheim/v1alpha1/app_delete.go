package v1alpha1

import (
	"fmt"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
)

func (a *App) runDelete(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	namespace := opts.Namespace
	name := opts.Name

	exists, err := a.Exists(namespace, name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if !exists {
		msg := fmt.Sprintf("app %q not found in namespace %q", name, namespace)
		output.Errorf(jsonMode, "NotFound", msg, "", nil, false)

		return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
	}

	if opts.DryRun {
		fmt.Printf("Would stop and delete app %q in namespace %q\n", name, namespace)
		return nil
	}

	if err := promptConfirm(opts.Yes,
		fmt.Sprintf("Delete app %q in namespace %q (stops containers and removes remote files)?", name, namespace)); err != nil {
		return err
	}

	// Pre-delete hook: stop (pause + remote rm)
	if err := a.runStop(opts); err != nil {
		return err
	}

	if err := a.RemoveDir(namespace, name); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	fmt.Printf("Deleted app %q\n", name)

	return nil
}
