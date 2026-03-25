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

func (j *Job) runGet(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"

	if opts.Name != "" {
		namespace := opts.Namespace

		_, m, err := j.getOne(namespace, opts.Name)
		if err != nil {
			output.Errorf(jsonMode, "NotFound",
				fmt.Sprintf("job %q not found in namespace %q", opts.Name, namespace), "", nil, false)

			return err
		}

		statusMap := j.prefetchJobStatus([]string{namespace})
		status, lastRun := aggregateJobStatus(statusMap, namespace, opts.Name)

		return output.PrintOne(jobToMeta(namespace, opts.Name, m, status, lastRun), opts.Output)
	}

	if opts.AllNamespaces {
		namespaces, err := j.ValidNamespaces()
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		statusMap := j.prefetchJobStatus(namespaces)

		type nsJobResult struct {
			manifests []*apiv1alpha1.Job
			names     []string
			err       error
		}

		nsResults := make([]nsJobResult, len(namespaces))

		var wg sync.WaitGroup

		for i, ns := range namespaces {
			wg.Add(1)

			go func(i int, ns string) {
				defer wg.Done()

				manifests, names, err := j.listNamespace(ns)
				nsResults[i] = nsJobResult{manifests, names, err}
			}(i, ns)
		}

		wg.Wait()

		var items []resource.ResourceMeta

		for i, r := range nsResults {
			if r.err != nil {
				return exitErr(exitcode.Failure, r.err)
			}

			ns := namespaces[i]

			for k, m := range r.manifests {
				status, lastRun := aggregateJobStatus(statusMap, ns, r.names[k])
				items = append(items, jobToMeta(ns, r.names[k], m, status, lastRun))
			}
		}

		if len(items) == 0 {
			output.PrintEmpty("", jobKind, opts.Output, opts.Quiet)
			return nil
		}

		return output.PrintList(items, []string{"NAMESPACE", "NAME", "IMAGE", "STATUS", "LAST RUN"}, jobKind, opts.Output, opts.Quiet)
	}

	namespace := opts.Namespace
	statusMap := j.prefetchJobStatus([]string{namespace})

	manifests, names, err := j.listNamespace(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if len(manifests) == 0 {
		output.PrintEmpty(namespace, jobKind, opts.Output, opts.Quiet)
		return nil
	}

	items := make([]resource.ResourceMeta, len(manifests))
	for i, m := range manifests {
		status, lastRun := aggregateJobStatus(statusMap, namespace, names[i])
		items[i] = jobToMeta(namespace, names[i], m, status, lastRun)
	}

	return output.PrintList(items, []string{"NAME", "IMAGE", "STATUS", "LAST RUN"}, jobKind, opts.Output, opts.Quiet)
}
