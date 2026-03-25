package v1alpha1

import (
	"fmt"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
)

func (d *DaemonSet) runDelete(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	name := opts.Name

	exists, err := d.Exists(name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if !exists {
		msg := fmt.Sprintf("daemonset %q not found", name)
		output.Errorf(jsonMode, "NotFound", msg, "", nil, false)

		return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
	}

	if opts.DryRun {
		fmt.Printf("Would stop and delete daemonset %q\n", name)
		return nil
	}

	if err := promptConfirm(opts.Yes,
		fmt.Sprintf("Delete daemonset %q (stops containers on all matching namespaces)?", name)); err != nil {
		return err
	}

	if err := d.runStop(opts); err != nil {
		return err
	}

	if err := d.RemoveDir(name); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	fmt.Printf("Deleted daemonset %q\n", name)

	return nil
}
