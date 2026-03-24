package registry_test

import (
	"testing"

	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/resource"
)

// makeReg returns a minimal Registration for use in tests.
func makeReg(plural, singular string, aliases []string, scope registry.Scope, ops ...registry.OperationDef) registry.Registration {
	return registry.Registration{
		Info: resource.KindInfo{
			Group:   "test",
			Version: "v1",
			Kind:    singular,
			Plural:  plural,
			Aliases: aliases,
		},
		Scope:      scope,
		Operations: ops,
		Factory: func(dataDir string, filesystem fs.FS) resource.Handler {
			return nil
		},
	}
}

// ── Register / Get ─────────────────────────────────────────────────────────────

func TestRegister_lookupByPlural(t *testing.T) {
	registry.Register(makeReg("testwidgets", "Testwidget", nil, registry.ClusterScoped))

	e := registry.Get("testwidgets")
	if e == nil {
		t.Fatal("Get(plural) = nil, want entry")
	}

	if e.Registration.Info.Plural != "testwidgets" {
		t.Errorf("Plural = %q", e.Registration.Info.Plural)
	}
}

func TestRegister_lookupBySingular(t *testing.T) {
	registry.Register(makeReg("testgadgets", "Testgadget", nil, registry.ClusterScoped))

	e := registry.Get("testgadget")
	if e == nil {
		t.Fatal("Get(singular) = nil, want entry")
	}
}

func TestRegister_lookupByAlias(t *testing.T) {
	registry.Register(makeReg("testthings", "Testthing", []string{"tt"}, registry.ClusterScoped))

	e := registry.Get("tt")
	if e == nil {
		t.Fatal("Get(alias) = nil, want entry")
	}
}

func TestGet_unknownKind_returnsNil(t *testing.T) {
	e := registry.Get("completely-unknown-kind-xyz")
	if e != nil {
		t.Errorf("Get(unknown) = %v, want nil", e)
	}
}

// ── Visibility ────────────────────────────────────────────────────────────────

func TestRegister_pluralIsVisible(t *testing.T) {
	registry.Register(makeReg("testvis", "Testvi", nil, registry.ClusterScoped))

	e := registry.Get("testvis")
	if e == nil {
		t.Fatal("entry not found")
	}

	if !e.Visible {
		t.Error("plural entry should be Visible=true")
	}
}

func TestRegister_singularIsNotVisible(t *testing.T) {
	registry.Register(makeReg("testinvis", "Testinvi", nil, registry.ClusterScoped))

	e := registry.Get("testinvi")
	if e == nil {
		t.Fatal("entry not found")
	}

	if e.Visible {
		t.Error("singular entry should be Visible=false")
	}
}

// ── IsCluster ─────────────────────────────────────────────────────────────────

func TestEntry_IsCluster_true(t *testing.T) {
	registry.Register(makeReg("tcluster", "Tcluster", nil, registry.ClusterScoped))

	e := registry.Get("tcluster")
	if e == nil {
		t.Fatal("entry not found")
	}

	if !e.IsCluster() {
		t.Error("IsCluster() = false, want true")
	}
}

func TestEntry_IsCluster_false(t *testing.T) {
	registry.Register(makeReg("tnsres", "Tnsre", nil, registry.NamespaceScoped))

	e := registry.Get("tnsres")
	if e == nil {
		t.Fatal("entry not found")
	}

	if e.IsCluster() {
		t.Error("IsCluster() = true, want false")
	}
}

// ── FindOperation ─────────────────────────────────────────────────────────────

func TestEntry_FindOperation_found(t *testing.T) {
	op := registry.OperationDef{Verb: "get", Short: "Get resources"}
	registry.Register(makeReg("tfindop", "Tfindop", nil, registry.ClusterScoped, op))

	e := registry.Get("tfindop")
	if e == nil {
		t.Fatal("entry not found")
	}

	found := e.FindOperation("get")
	if found == nil {
		t.Fatal("FindOperation(get) = nil, want operation")
	}

	if found.Verb != "get" {
		t.Errorf("Verb = %q, want get", found.Verb)
	}
}

func TestEntry_FindOperation_notFound(t *testing.T) {
	registry.Register(makeReg("tfindop2", "Tfindop2", nil, registry.ClusterScoped))

	e := registry.Get("tfindop2")
	if e == nil {
		t.Fatal("entry not found")
	}

	found := e.FindOperation("delete")
	if found != nil {
		t.Errorf("FindOperation(delete) = %v, want nil", found)
	}
}

// ── AllEntries ─────────────────────────────────────────────────────────────────

func TestAllEntries_containsRegistered(t *testing.T) {
	registry.Register(makeReg("tallentries", "Tallentry", nil, registry.ClusterScoped))

	entries := registry.AllEntries()
	found := false

	for _, e := range entries {
		if e.Registration.Info.Plural == "tallentries" {
			found = true
			break
		}
	}

	if !found {
		t.Error("AllEntries() did not include registered kind")
	}
}

// ── AllOperations ─────────────────────────────────────────────────────────────

func TestAllOperations_includesRegisteredVerbs(t *testing.T) {
	op1 := registry.OperationDef{Verb: "tget", Short: "test get"}
	op2 := registry.OperationDef{Verb: "tapply", Short: "test apply"}
	registry.Register(makeReg("tallops", "Tallop", nil, registry.ClusterScoped, op1, op2))

	verbs := registry.AllOperations()

	verbSet := make(map[string]bool, len(verbs))
	for _, v := range verbs {
		verbSet[v] = true
	}

	if !verbSet["tget"] {
		t.Error("AllOperations() missing verb 'tget'")
	}

	if !verbSet["tapply"] {
		t.Error("AllOperations() missing verb 'tapply'")
	}
}

func TestAllOperations_isSorted(t *testing.T) {
	verbs := registry.AllOperations()

	for i := 1; i < len(verbs); i++ {
		if verbs[i] < verbs[i-1] {
			t.Errorf("AllOperations() not sorted: %v[%d]=%q < %v[%d]=%q",
				verbs, i, verbs[i], verbs, i-1, verbs[i-1])
		}
	}
}

// ── OperationOpts accessors ───────────────────────────────────────────────────

func TestOperationOpts_String(t *testing.T) {
	opts := registry.OperationOpts{
		Flags: map[string]any{"hostname": "myhost"},
	}

	if got := opts.String("hostname"); got != "myhost" {
		t.Errorf("String(hostname) = %q, want %q", got, "myhost")
	}
}

func TestOperationOpts_String_missing(t *testing.T) {
	opts := registry.OperationOpts{Flags: map[string]any{}}

	if got := opts.String("missing"); got != "" {
		t.Errorf("String(missing) = %q, want empty", got)
	}
}

func TestOperationOpts_Bool(t *testing.T) {
	opts := registry.OperationOpts{
		Flags: map[string]any{"verbose": true},
	}

	if !opts.Bool("verbose") {
		t.Error("Bool(verbose) = false, want true")
	}
}

func TestOperationOpts_Bool_missing(t *testing.T) {
	opts := registry.OperationOpts{Flags: map[string]any{}}

	if opts.Bool("missing") {
		t.Error("Bool(missing) = true, want false")
	}
}

func TestOperationOpts_Int(t *testing.T) {
	opts := registry.OperationOpts{
		Flags: map[string]any{"count": 42},
	}

	if got := opts.Int("count"); got != 42 {
		t.Errorf("Int(count) = %d, want 42", got)
	}
}

func TestOperationOpts_Int_missing(t *testing.T) {
	opts := registry.OperationOpts{Flags: map[string]any{}}

	if got := opts.Int("missing"); got != 0 {
		t.Errorf("Int(missing) = %d, want 0", got)
	}
}

// ── RunHook ───────────────────────────────────────────────────────────────────

func TestRunHook_emptyVerb_isNoop(t *testing.T) {
	registry.Register(makeReg("thook1", "Thook1", nil, registry.ClusterScoped))

	e := registry.Get("thook1")

	err := registry.RunHook(nil, e, "", registry.OperationOpts{})
	if err != nil {
		t.Errorf("RunHook(empty verb) = %v, want nil", err)
	}
}

func TestRunHook_unknownVerb_isNoop(t *testing.T) {
	registry.Register(makeReg("thook2", "Thook2", nil, registry.ClusterScoped))

	e := registry.Get("thook2")

	err := registry.RunHook(nil, e, "nonexistent-verb", registry.OperationOpts{})
	if err != nil {
		t.Errorf("RunHook(unknown verb) = %v, want nil", err)
	}
}

func TestRunHook_callsMatchedOperation(t *testing.T) {
	called := false
	op := registry.OperationDef{
		Verb: "tstart",
		Run: func(h resource.Handler, opts registry.OperationOpts) error {
			called = true
			return nil
		},
	}
	registry.Register(makeReg("thook3", "Thook3", nil, registry.ClusterScoped, op))

	e := registry.Get("thook3")

	err := registry.RunHook(nil, e, "tstart", registry.OperationOpts{})
	if err != nil {
		t.Fatalf("RunHook: %v", err)
	}

	if !called {
		t.Error("RunHook did not invoke the operation's Run function")
	}
}
