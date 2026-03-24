package registry

import (
	"sort"
	"sync"

	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/resource"
)

// Scope indicates whether a resource is cluster-scoped or namespace-scoped.
type Scope int

const (
	ClusterScoped   Scope = iota
	NamespaceScoped Scope = iota
)

// ApplyOrder controls the order in which resources are applied when loading
// from a multi-resource manifest. Lower values are applied first.
type ApplyOrder int

const (
	// ApplyOrderNamespace — namespace definitions must exist before anything else.
	ApplyOrderNamespace ApplyOrder = 1
	// ApplyOrderClusterMetadata — cluster-scoped metadata resources (e.g. ClusterSecret).
	ApplyOrderClusterMetadata ApplyOrder = 2
	// ApplyOrderClusterWorkload — cluster-scoped workloads (e.g. DaemonSet).
	ApplyOrderClusterWorkload ApplyOrder = 3
	// ApplyOrderNamespaceMetadata — namespace-scoped metadata resources (e.g. ConfigMap, Secret).
	ApplyOrderNamespaceMetadata ApplyOrder = 4
	// ApplyOrderNamespaceWorkload — namespace-scoped workloads (e.g. App, Job).
	ApplyOrderNamespaceWorkload ApplyOrder = 5
)

// NSHandling controls how -n and -A flags behave for a given operation.
type NSHandling int

const (
	// NSNone: no namespace flags (automatically applied to all cluster-scoped operations).
	NSNone NSHandling = iota
	// NSRequired: -n <namespace> is required.
	NSRequired
	// NSOptionalAll: either -n <namespace> or -A/--all-namespaces is required.
	NSOptionalAll
)

// FlagDef declares a single operation-specific flag.
type FlagDef struct {
	Name    string // long flag name, e.g. "hostname"
	Short   string // single-char shorthand, e.g. "H" — empty means none
	Type    string // "string" | "bool" | "int"
	Default any    // zero value for the type if omitted
	Usage   string // help text
}

// OperationDef declares one operation that a resource supports.
type OperationDef struct {
	// Verb is the top-level cobra command name: "get", "apply", "start", "logs", etc.
	Verb string
	// Short is the one-line help description.
	Short string
	// Examples are shown in --help output.
	Examples []string
	// Flags are operation-specific flags (beyond the universal ones).
	// The framework merges flags from all resources that share the same verb.
	Flags []FlagDef
	// NSHandling controls namespace flag behaviour for namespaced resources.
	// Ignored for cluster-scoped resources (always NSNone).
	// Defaults to NSRequired if zero value.
	NSHandling NSHandling
	// RequiresName indicates that a resource name positional arg is mandatory.
	RequiresName bool
	// Run is called when the command executes.
	// h is the handler created by the resource's Factory; cast to the concrete type.
	Run func(h resource.Handler, opts OperationOpts) error
}

// Hooks declares lifecycle callbacks triggered by the framework during
// apply (create/update) and delete.
type Hooks struct {
	PostCreate string // verb to call after a successful create (e.g. "start")
	PostUpdate string // verb to call after a successful update (e.g. "start")
	PreDelete  string // verb to call before delete (e.g. "stop")
}

// Factory creates a resource handler for a given data directory and filesystem.
type Factory func(dataDir string, filesystem fs.FS) resource.Handler

// Registration is what a resource package passes to Register in init().
type Registration struct {
	Info       resource.KindInfo
	Scope      Scope
	ApplyOrder ApplyOrder
	Operations []OperationDef
	Hooks      Hooks
	// SummaryColumns is the ordered list of summary field names for table output.
	// Must match the keys returned by SummaryField functions in the resource.
	SummaryColumns []string
	Factory        Factory
}

// OperationOpts carries all parsed inputs to an operation's Run function.
type OperationOpts struct {
	// Resolved from global flags + config
	DataDir string
	FS      fs.FS

	// Parsed from positional args
	Kind string
	Name string // "" if not provided / not required

	// Namespace flags (only populated for namespaced resources)
	Namespace     string
	AllNamespaces bool

	// Universal flags (available on every command)
	Output string // "human" | "yaml" | "json"
	Quiet  bool
	DryRun bool
	Yes    bool

	// Operation-specific flags, keyed by FlagDef.Name.
	Flags map[string]any

	// RawManifest, if non-nil, is the pre-loaded document bytes for this
	// operation. Apply handlers use this instead of reading from the "file"
	// flag when set. Populated by the verb-level -f/--filename dispatch path.
	RawManifest []byte
}

// String returns an operation-specific string flag value.
func (o OperationOpts) String(key string) string {
	v, _ := o.Flags[key].(string)
	return v
}

// Bool returns an operation-specific bool flag value.
func (o OperationOpts) Bool(key string) bool {
	v, _ := o.Flags[key].(bool)
	return v
}

// Int returns an operation-specific int flag value.
func (o OperationOpts) Int(key string) int {
	v, _ := o.Flags[key].(int)
	return v
}

// Entry holds a registration and whether it is the visible (plural) name.
type Entry struct {
	Registration Registration // the full registration
	Visible      bool         // true for plural, false for singular/aliases
}

// FindOperation returns the OperationDef for verb, or nil if not declared.
func (e *Entry) FindOperation(verb string) *OperationDef {
	for i := range e.Registration.Operations {
		if e.Registration.Operations[i].Verb == verb {
			return &e.Registration.Operations[i]
		}
	}

	return nil
}

// IsCluster reports whether this resource is cluster-scoped.
func (e *Entry) IsCluster() bool {
	return e.Registration.Scope == ClusterScoped
}

// registry holds all registered resource kinds.
var (
	mu        sync.RWMutex
	byName    = make(map[string]*Entry) // keyed by plural, singular, and aliases
	allPlural []string                  // sorted list of plural names (visible entries)
)

// Register adds a resource kind. Called from resource package init() functions.
// Registers the plural name as visible, singular and aliases as invisible.
func Register(r Registration) {
	mu.Lock()
	defer mu.Unlock()

	pluralEntry := &Entry{Registration: r, Visible: true}
	byName[r.Info.Plural] = pluralEntry
	allPlural = append(allPlural, r.Info.Plural)
	sort.Strings(allPlural)

	singularEntry := &Entry{Registration: r, Visible: false}
	byName[r.Info.Singular()] = singularEntry

	for _, alias := range r.Info.Aliases {
		aliasEntry := &Entry{Registration: r, Visible: false}
		byName[alias] = aliasEntry
	}
}

// Get looks up an entry by any registered name (plural, singular, or alias).
// Returns nil if not found.
func Get(kind string) *Entry {
	mu.RLock()
	defer mu.RUnlock()

	return byName[kind]
}

// AllEntries returns one Entry per visible kind, sorted by plural name.
func AllEntries() []*Entry {
	mu.RLock()
	defer mu.RUnlock()

	result := make([]*Entry, 0, len(allPlural))
	for _, plural := range allPlural {
		if e, ok := byName[plural]; ok {
			result = append(result, e)
		}
	}

	return result
}

// AllOperations returns the deduplicated, sorted list of all verb names
// declared across all registered resources.
// This is what the CLI uses to build the cobra command tree.
func AllOperations() []string {
	mu.RLock()
	defer mu.RUnlock()

	seen := make(map[string]struct{})

	for _, plural := range allPlural {
		e, ok := byName[plural]
		if !ok {
			continue
		}

		for _, op := range e.Registration.Operations {
			seen[op.Verb] = struct{}{}
		}
	}

	verbs := make([]string, 0, len(seen))
	for v := range seen {
		verbs = append(verbs, v)
	}

	sort.Strings(verbs)

	return verbs
}

// RunHook looks up the named hook verb on the entry, finds its Run function,
// and calls it with the same handler and opts. If the hook verb is not declared
// or hookVerb is empty, it is a no-op.
func RunHook(h resource.Handler, entry *Entry, hookVerb string, opts OperationOpts) error {
	if hookVerb == "" {
		return nil
	}

	op := entry.FindOperation(hookVerb)
	if op == nil {
		return nil
	}

	return op.Run(h, opts)
}
