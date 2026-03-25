package v1alpha1

import (
	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/resource"
)

func registerApp() {
	fileFlag := []registry.FlagDef{
		{Name: "file", Short: "f", Type: "string", Usage: "Manifest file path, or - for stdin"},
	}

	registry.Register(registry.Registration{
		Info:       appKind,
		Scope:      registry.NamespaceScoped,
		ApplyOrder: registry.ApplyOrderNamespaceWorkload,
		Operations: []registry.OperationDef{
			{
				Verb:         "get",
				Short:        "List apps in a namespace, or get a single app by name",
				NSHandling:   registry.NSOptionalAll,
				RequiresName: false,
				Examples: []string{
					"whctl get apps -n production",
					"whctl get app myapp -n production",
					"whctl get apps -A",
					"whctl get apps -n production -o json",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runGet(opts)
				},
			},
			{
				Verb:         "apply",
				Short:        "Create or update an app from a manifest file, then start it",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Flags:        fileFlag,
				Examples: []string{
					"whctl apply app myapp -n production -f myapp.yaml",
					"whctl apply app myapp -n production -f myapp.yaml --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runApply(opts)
				},
			},
			{
				Verb:         "delete",
				Short:        "Stop an app and delete its local manifest and remote files",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Examples: []string{
					"whctl delete app myapp -n production --yes",
					"whctl delete app myapp -n production --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runDelete(opts)
				},
			},
			{
				Verb:         "describe",
				Short:        "Show app details with live docker compose ps output",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Examples: []string{
					"whctl describe app myapp -n production",
					"whctl describe app myapp -n production -o json",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runDescribe(opts)
				},
			},
			{
				Verb:         "import",
				Short:        "Wrap an existing docker-compose.yml into an app manifest without deploying",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Flags:        fileFlag,
				Examples: []string{
					"whctl import app myapp -n production -f docker-compose.yml",
					"whctl import app myapp -n production -f docker-compose.yml --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runImport(opts)
				},
			},
			{
				Verb:         "start",
				Short:        "Generate compose file, sync to remote, and run docker compose up",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Examples: []string{
					"whctl start app myapp -n production",
					"whctl start app myapp -n production --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runStart(opts)
				},
			},
			{
				Verb:         "pause",
				Short:        "Run docker compose down on the remote, keeping remote files in place",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Examples: []string{
					"whctl pause app myapp -n production",
					"whctl pause app myapp -n production --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runPause(opts)
				},
			},
			{
				Verb:         "stop",
				Short:        "Run docker compose down and remove all remote files for an app",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Examples: []string{
					"whctl stop app myapp -n production",
					"whctl stop app myapp -n production --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runStop(opts)
				},
			},
			{
				Verb:         "pull",
				Short:        "Pull the latest images on the remote without restarting containers",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Examples: []string{
					"whctl pull app myapp -n production",
					"whctl pull app myapp -n production --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runPull(opts)
				},
			},
			{
				Verb:         "logs",
				Short:        "Print or stream logs from a running app's containers",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Flags: []registry.FlagDef{
					{Name: "follow", Short: "f", Type: "bool", Usage: "Follow log output"},
					{Name: "tail", Type: "int", Default: -1, Usage: "Number of lines from end (-1 = all)"},
					{Name: "timestamps", Short: "T", Type: "bool", Usage: "Show timestamps"},
					{Name: "service", Short: "s", Type: "string", Usage: "Limit to one service"},
				},
				Examples: []string{
					"whctl logs app myapp -n production",
					"whctl logs app myapp -n production --follow",
					"whctl logs app myapp -n production --tail 100 --timestamps",
					"whctl logs app myapp -n production --service worker",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runLogs(opts)
				},
			},
			{
				Verb:         "exec",
				Short:        "Open an interactive shell or run a command in a running container",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Flags: []registry.FlagDef{
					{Name: "service", Short: "s", Type: "string", Usage: "Target service (default: first service)"},
					{Name: "tty", Short: "t", Type: "bool", Default: true, Usage: "Allocate a TTY"},
					{Name: "cmd", Short: "c", Type: "string", Usage: "Command to run in the container"},
				},
				Examples: []string{
					"whctl exec app myapp -n production",
					"whctl exec app myapp -n production --service worker",
					"whctl exec app myapp -n production --cmd bash",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runExec(opts)
				},
			},
			{
				Verb:         "doctor",
				Short:        "Check app manifests and envFrom references for issues",
				NSHandling:   registry.NSOptionalAll,
				RequiresName: false,
				Examples: []string{
					"whctl doctor apps -n production",
					"whctl doctor app myapp -n production",
					"whctl doctor apps -A",
					"whctl doctor apps -A -o json",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runDoctor(opts)
				},
			},
		},
		SummaryColumns: []string{"NAMESPACE", "NAME", "IMAGE", "READY", "STATUS"},
		Factory: func(dataDir string, filesystem fs.FS) resource.Handler {
			return newApp(dataDir, filesystem)
		},
	})
}
