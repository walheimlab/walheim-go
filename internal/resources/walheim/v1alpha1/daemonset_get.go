package v1alpha1

import (
	"fmt"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

func (d *DaemonSet) runGet(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"

	if opts.Name != "" {
		meta, m, err := d.getOne(opts.Name)
		if err != nil {
			output.Errorf(jsonMode, "NotFound",
				fmt.Sprintf("daemonset %q not found", opts.Name), "", nil, false)

			return err
		}

		nsMetas, nsNames, _ := matchingNamespaces(m.Spec.NamespaceSelector, d.FS, d.DataDir)
		m.Status = &apiv1alpha1.DaemonSetStatus{Namespaces: d.fetchDaemonSetStatus(opts.Name, nsMetas, nsNames)}

		return output.PrintOne(meta, opts.Output)
	}

	items, err := d.listAll()
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if len(items) == 0 {
		output.PrintEmpty("", daemonSetKind, opts.Output, opts.Quiet)
		return nil
	}

	return output.PrintList(items, []string{"NAME", "IMAGE", "SELECTOR", "NAMESPACES"}, daemonSetKind, opts.Output, opts.Quiet)
}
