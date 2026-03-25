package v1alpha1

import (
	"fmt"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
)

func (n *Namespace) runDelete(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	name := opts.Name

	exists, err := n.Exists(name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if !exists {
		msg := fmt.Sprintf("namespace %q not found", name)
		output.Errorf(jsonMode, "NotFound", msg, "", nil, false)

		return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
	}

	if opts.DryRun {
		fmt.Printf("Would delete namespace %q and all its contents\n", name)
		return nil
	}

	if err := promptConfirm(opts.Yes,
		fmt.Sprintf("Delete namespace %q and all its local contents?", name)); err != nil {
		return err
	}

	if err := n.RemoveDir(name); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	fmt.Printf("Deleted namespace %q\n", name)

	return nil
}
