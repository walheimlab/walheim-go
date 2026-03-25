package v1alpha1

import (
	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/resource"
)

func registerJob() {
	registry.Register(registry.Registration{
		Info:       jobKind,
		Scope:      registry.NamespaceScoped,
		ApplyOrder: registry.ApplyOrderNamespaceWorkload,
		Operations: []registry.OperationDef{
			{
				Verb:         "get",
				Short:        "List jobs or get a single job with its last run status",
				NSHandling:   registry.NSOptionalAll,
				RequiresName: false,
				Examples: []string{
					"whctl get jobs -n production",
					"whctl get job db-backup -n production",
					"whctl get jobs -A",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Job).runGet(opts)
				},
			},
			{
				Verb:         "apply",
				Short:        "Create or update a job manifest (does not run it)",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Flags: []registry.FlagDef{
					{Name: "file", Short: "f", Type: "string", Usage: "Manifest file path, or - for stdin"},
				},
				Examples: []string{
					"whctl apply job db-backup -n production -f job.yaml",
					"cat job.yaml | whctl apply job db-backup -n production -f -",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Job).runApply(opts)
				},
			},
			{
				Verb:         "run",
				Short:        "Execute a job on its namespace's host and stream output",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Flags: []registry.FlagDef{
					{Name: "detach", Short: "d", Type: "bool", Usage: "Run in the background; return immediately"},
				},
				Examples: []string{
					"whctl run job db-backup -n production",
					"whctl run job db-backup -n production --detach",
					"whctl run job db-backup -n production --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Job).runRun(opts)
				},
			},
			{
				Verb:         "delete",
				Short:        "Delete a job manifest (remote containers are not removed)",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Examples: []string{
					"whctl delete job db-backup -n production",
					"whctl delete job db-backup -n production --yes",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Job).runDelete(opts)
				},
			},
			{
				Verb:         "logs",
				Short:        "Fetch logs from the most recent container run of a job",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Flags: []registry.FlagDef{
					{Name: "follow", Short: "f", Type: "bool", Usage: "Follow log output"},
					{Name: "tail", Type: "int", Default: -1, Usage: "Number of lines to show from the end (-1 for all)"},
				},
				Examples: []string{
					"whctl logs job db-backup -n production",
					"whctl logs job db-backup -n production --follow",
					"whctl logs job db-backup -n production --tail 50",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Job).runLogs(opts)
				},
			},
			{
				Verb:         "doctor",
				Short:        "Check job manifests for issues",
				NSHandling:   registry.NSOptionalAll,
				RequiresName: false,
				Examples: []string{
					"whctl doctor jobs -n production",
					"whctl doctor job db-backup -n production",
					"whctl doctor jobs -A",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Job).runDoctor(opts)
				},
			},
		},
		SummaryColumns: []string{"NAME", "IMAGE", "STATUS", "LAST RUN"},
		Factory: func(dataDir string, filesystem fs.FS) resource.Handler {
			return newJob(dataDir, filesystem)
		},
	})
}
