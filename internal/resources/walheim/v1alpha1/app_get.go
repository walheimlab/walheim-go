package v1alpha1

import (
	"fmt"
	"sync"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/resource"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

func (a *App) runGet(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"

	if opts.Name != "" {
		namespace := opts.Namespace

		meta, m, err := a.getOne(namespace, opts.Name)
		if err != nil {
			output.Errorf(jsonMode, "NotFound",
				fmt.Sprintf("app %q not found in namespace %q", opts.Name, namespace), "", nil, false)

			return err
		}

		statusMap := a.prefetchStatus([]string{namespace})
		state, ready := aggregateStatus(statusMap, namespace, opts.Name)
		m.Status = &apiv1alpha1.AppStatus{State: state, Ready: ready}

		return output.PrintOne(meta, opts.Output)
	}

	if opts.AllNamespaces {
		namespaces, err := a.ValidNamespaces()
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		statusMap := a.prefetchStatus(namespaces)

		type nsAppResult struct {
			manifests []*apiv1alpha1.App
			names     []string
			err       error
		}

		nsResults := make([]nsAppResult, len(namespaces))

		var wg sync.WaitGroup

		for i, ns := range namespaces {
			wg.Add(1)

			go func(i int, ns string) {
				defer wg.Done()

				manifests, names, err := a.listNamespace(ns)
				nsResults[i] = nsAppResult{manifests, names, err}
			}(i, ns)
		}

		wg.Wait()

		var items []resource.ResourceMeta

		for i, r := range nsResults {
			if r.err != nil {
				return exitErr(exitcode.Failure, r.err)
			}

			ns := namespaces[i]

			for j, m := range r.manifests {
				status, ready := aggregateStatus(statusMap, ns, r.names[j])
				items = append(items, appToMeta(ns, r.names[j], m, status, ready))
			}
		}

		if len(items) == 0 {
			output.PrintEmpty("", appKind, opts.Output, opts.Quiet)
			return nil
		}

		return output.PrintList(items, []string{"NAMESPACE", "NAME", "IMAGE", "READY", "STATUS"}, appKind, opts.Output, opts.Quiet)
	}

	namespace := opts.Namespace
	statusMap := a.prefetchStatus([]string{namespace})

	manifests, names, err := a.listNamespace(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if len(manifests) == 0 {
		output.PrintEmpty(namespace, appKind, opts.Output, opts.Quiet)
		return nil
	}

	items := make([]resource.ResourceMeta, len(manifests))
	for i, m := range manifests {
		status, ready := aggregateStatus(statusMap, namespace, names[i])
		items[i] = appToMeta(namespace, names[i], m, status, ready)
	}

	return output.PrintList(items, []string{"NAME", "IMAGE", "READY", "STATUS"}, appKind, opts.Output, opts.Quiet)
}
