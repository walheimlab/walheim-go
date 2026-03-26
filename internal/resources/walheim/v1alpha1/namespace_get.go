package v1alpha1

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/yamlutil"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

func (n *Namespace) runGet(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"

	if opts.Name == "" {
		items, err := n.listAll()
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if len(items) == 0 {
			output.PrintEmpty("", namespaceKind, opts.Output, opts.Quiet)
			return nil
		}

		return output.PrintList(items, []string{"NAME", "HOSTNAME", "USERNAME"}, namespaceKind, opts.Output, opts.Quiet)
	}

	meta, m, err := n.getOne(opts.Name)
	if err != nil {
		output.Errorf(jsonMode, "NotFound",
			fmt.Sprintf("namespace %q not found", opts.Name), "", nil, false)

		return err
	}

	// For structured output, populate runtime status via SSH and emit a full view.
	if opts.Output == "json" || opts.Output == "yaml" {
		return n.getWithStatus(opts.Name, m, opts.Output)
	}

	return output.PrintOne(meta, opts.Output)
}

func (n *Namespace) getWithStatus(name string, m *apiv1alpha1.Namespace, format string) error {
	result := namespaceDescribeResult{
		APIVersion: m.APIVersion,
		Kind:       m.Kind,
		Metadata:   namespaceDescribeMeta{Name: name},
		Spec: namespaceDescribeSpec{
			Hostname: m.Spec.Hostname,
			Username: m.Spec.Username,
			BaseDir:  m.Spec.RemoteBaseDir(),
		},
		Status: n.buildDescribeStatus(m),
	}

	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")

		return enc.Encode(result)
	}

	data, err := yamlutil.Marshal(result)
	if err != nil {
		return err
	}

	fmt.Print(string(data))

	return nil
}
