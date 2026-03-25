package v1alpha1

import (
	"fmt"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
)

func (j *Job) runDelete(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	namespace := opts.Namespace
	name := opts.Name

	exists, err := j.Exists(namespace, name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if !exists {
		msg := fmt.Sprintf("job %q not found in namespace %q", name, namespace)
		output.Errorf(jsonMode, "NotFound", msg, "", nil, false)

		return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
	}

	if opts.DryRun {
		fmt.Printf("Would delete job %q in namespace %q\n", name, namespace)
		return nil
	}

	if err := promptConfirm(opts.Yes,
		fmt.Sprintf("Delete job %q in namespace %q? (remote containers are not removed)", name, namespace)); err != nil {
		return err
	}

	if err := j.RemoveDir(namespace, name); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	fmt.Printf("Deleted job %q\n", name)

	return nil
}
