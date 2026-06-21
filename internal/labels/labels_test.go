package labels_test

import (
	"errors"
	"testing"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/labels"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/resource"
	"github.com/walheimlab/walheim-go/internal/testutil"
)

type errorFS struct {
	*testutil.MemFS
	readErr  error
	writeErr error
}

func (e *errorFS) ReadFile(path string) ([]byte, error) {
	if e.readErr != nil {
		return nil, e.readErr
	}

	return e.MemFS.ReadFile(path)
}

func (e *errorFS) WriteFile(path string, data []byte) error {
	if e.writeErr != nil {
		return e.writeErr
	}

	return e.MemFS.WriteFile(path, data)
}

func init() {
	registry.Register(registry.Registration{
		Info: resource.KindInfo{
			Group:   "test",
			Version: "v1alpha1",
			Kind:    "ClusterWidget",
			Plural:  "clusterwidgets",
		},
		Scope: registry.ClusterScoped,
	})
	registry.Register(registry.Registration{
		Info: resource.KindInfo{
			Group:   "test",
			Version: "v1alpha1",
			Kind:    "NamespacedWidget",
			Plural:  "namespacedwidgets",
		},
		Scope: registry.NamespaceScoped,
	})
}

func TestResolvePath_ClusterScoped(t *testing.T) {
	fs := testutil.NewMemFS()

	path, err := labels.ResolvePath(fs, "/data", "clusterwidgets", "foo", "")
	if err != nil {
		t.Fatalf("ResolvePath failed: %v", err)
	}

	expected := "/data/clusterwidgets/foo/.clusterwidget.yaml"
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}

	// Cluster-scoped with namespace should fail
	_, err = labels.ResolvePath(fs, "/data", "clusterwidgets", "foo", "ns")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitcode.Error
	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.UsageError {
		t.Errorf("expected exitcode.UsageError, got %v", err)
	}
}

func TestResolvePath_Namespaced(t *testing.T) {
	fs := testutil.NewMemFS()

	path, err := labels.ResolvePath(fs, "/data", "namespacedwidgets", "foo", "ns1")
	if err != nil {
		t.Fatalf("ResolvePath failed: %v", err)
	}

	expected := "/data/namespaces/ns1/namespacedwidgets/foo/.namespacedwidget.yaml"
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}

	// Namespaced without namespace should fail
	_, err = labels.ResolvePath(fs, "/data", "namespacedwidgets", "foo", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitcode.Error
	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.UsageError {
		t.Errorf("expected exitcode.UsageError, got %v", err)
	}
}

func TestResolvePath_UnknownKind(t *testing.T) {
	fs := testutil.NewMemFS()

	_, err := labels.ResolvePath(fs, "/data", "unknownkind", "foo", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitcode.Error
	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.UsageError {
		t.Errorf("expected exitcode.UsageError, got %v", err)
	}
}

func TestGet_NonexistentFile(t *testing.T) {
	fs := testutil.NewMemFS()

	_, err := labels.Get(fs, "/data", "clusterwidgets", "foo", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitcode.Error
	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.NotFound {
		t.Errorf("expected exitcode.NotFound, got %v", err)
	}
}

func TestGet_InvalidYAML(t *testing.T) {
	fs := testutil.NewMemFS()
	path := "/data/clusterwidgets/foo/.clusterwidget.yaml"
	_ = fs.WriteFile(path, []byte(`{invalid: yaml: `))

	_, err := labels.Get(fs, "/data", "clusterwidgets", "foo", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitcode.Error
	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.Failure {
		t.Errorf("expected exitcode.Failure, got %v", err)
	}
}

func TestGet_NoLabels(t *testing.T) {
	fs := testutil.NewMemFS()
	path := "/data/clusterwidgets/foo/.clusterwidget.yaml"
	_ = fs.WriteFile(path, []byte(`
metadata:
  name: foo
`))

	m, err := labels.Get(fs, "/data", "clusterwidgets", "foo", "")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestGet_WithLabels(t *testing.T) {
	fs := testutil.NewMemFS()
	path := "/data/clusterwidgets/foo/.clusterwidget.yaml"
	_ = fs.WriteFile(path, []byte(`
metadata:
  name: foo
  labels:
    app: walheim
    env: prod
`))

	m, err := labels.Get(fs, "/data", "clusterwidgets", "foo", "")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(m) != 2 || m["app"] != "walheim" || m["env"] != "prod" {
		t.Errorf("unexpected labels map: %v", m)
	}
}

func TestSet_SuccessAndFailure(t *testing.T) {
	fs := testutil.NewMemFS()
	path := "/data/clusterwidgets/foo/.clusterwidget.yaml"
	_ = fs.WriteFile(path, []byte(`
metadata:
  name: foo
  labels:
    app: walheim
`))

	// Write without overwrite of existing key should fail
	err := labels.Set(fs, "/data", "clusterwidgets", "foo", "", []string{"app=newval"}, false)
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}

	var exitErr *exitcode.Error
	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.Conflict {
		t.Errorf("expected exitcode.Conflict, got %v", err)
	}

	// Write without overwrite of new key should succeed
	changed, removed, err := labels.SetTracked(fs, "/data", "clusterwidgets", "foo", "", []string{"env=prod"}, false)
	if err != nil {
		t.Fatalf("SetTracked failed: %v", err)
	}

	if len(changed) != 1 || changed[0] != "env" || len(removed) != 0 {
		t.Errorf("unexpected changed/removed: %v, %v", changed, removed)
	}

	// With overwrite should succeed on existing key
	changed, removed, err = labels.SetTracked(fs, "/data", "clusterwidgets", "foo", "", []string{"app=newval"}, true)
	if err != nil {
		t.Fatalf("SetTracked failed: %v", err)
	}

	if len(changed) != 1 || changed[0] != "app" || len(removed) != 0 {
		t.Errorf("unexpected changed/removed: %v, %v", changed, removed)
	}

	// Remove key
	changed, removed, err = labels.SetTracked(fs, "/data", "clusterwidgets", "foo", "", []string{"app-"}, false)
	if err != nil {
		t.Fatalf("SetTracked failed: %v", err)
	}

	if len(changed) != 0 || len(removed) != 1 || removed[0] != "app" {
		t.Errorf("unexpected changed/removed: %v, %v", changed, removed)
	}

	// Verification of final document content
	m, err := labels.Get(fs, "/data", "clusterwidgets", "foo", "")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(m) != 1 || m["env"] != "prod" {
		t.Errorf("unexpected final labels: %v", m)
	}
}

func TestSet_Validation(t *testing.T) {
	fs := testutil.NewMemFS()
	path := "/data/clusterwidgets/foo/.clusterwidget.yaml"
	_ = fs.WriteFile(path, []byte(`
metadata:
  name: foo
`))

	// Invalid specs
	invalidSpecs := []string{
		"invalidSpecNoEqualsOrMinus",
		"=val",
		"invalid..key=val",
		"invalid@key=val",
	}

	for _, spec := range invalidSpecs {
		t.Run(spec, func(t *testing.T) {
			err := labels.Set(fs, "/data", "clusterwidgets", "foo", "", []string{spec}, true)
			if err == nil {
				t.Fatalf("expected error for spec %q, got nil", spec)
			}

			var exitErr *exitcode.Error
			if !errors.As(err, &exitErr) || (exitErr.Code != exitcode.UsageError) {
				t.Errorf("expected UsageError for spec %q, got %v", spec, err)
			}
		})
	}
}

func TestSet_InvalidRootNode(t *testing.T) {
	fs := testutil.NewMemFS()
	path := "/data/clusterwidgets/foo/.clusterwidget.yaml"
	_ = fs.WriteFile(path, []byte(`"hello"`))

	err := labels.Set(fs, "/data", "clusterwidgets", "foo", "", []string{"key=value"}, true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitcode.Error
	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.Failure {
		t.Errorf("expected exitcode.Failure, got %v", err)
	}
}

func TestGet_InvalidLabelsType(t *testing.T) {
	fs := testutil.NewMemFS()
	path := "/data/clusterwidgets/foo/.clusterwidget.yaml"
	_ = fs.WriteFile(path, []byte(`
metadata:
  labels: "not-a-map"
`))

	_, err := labels.Get(fs, "/data", "clusterwidgets", "foo", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitcode.Error
	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.Failure {
		t.Errorf("expected exitcode.Failure, got %v", err)
	}
}

func TestSet_InvalidLabelsType(t *testing.T) {
	fs := testutil.NewMemFS()
	path := "/data/clusterwidgets/foo/.clusterwidget.yaml"
	_ = fs.WriteFile(path, []byte(`
metadata:
  labels: "not-a-map"
`))

	_, _, err := labels.SetTracked(fs, "/data", "clusterwidgets", "foo", "", []string{"key=value"}, true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitcode.Error
	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.Failure {
		t.Errorf("expected exitcode.Failure, got %v", err)
	}
}

func TestSet_MissingMetadata(t *testing.T) {
	fs := testutil.NewMemFS()
	path := "/data/clusterwidgets/foo/.clusterwidget.yaml"
	_ = fs.WriteFile(path, []byte(`
name: foo
`))

	changed, removed, err := labels.SetTracked(fs, "/data", "clusterwidgets", "foo", "", []string{"key=value"}, true)
	if err != nil {
		t.Fatalf("SetTracked failed: %v", err)
	}

	if len(changed) != 1 || changed[0] != "key" || len(removed) != 0 {
		t.Errorf("unexpected changed/removed: %v, %v", changed, removed)
	}

	m, err := labels.Get(fs, "/data", "clusterwidgets", "foo", "")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if m["key"] != "value" {
		t.Errorf("expected label key=value, got: %v", m)
	}
}

func TestSet_EmptyDoc(t *testing.T) {
	fs := testutil.NewMemFS()
	path := "/data/clusterwidgets/foo/.clusterwidget.yaml"
	_ = fs.WriteFile(path, []byte(``))

	_, _, err := labels.SetTracked(fs, "/data", "clusterwidgets", "foo", "", []string{"key=value"}, true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitcode.Error
	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.Failure {
		t.Errorf("expected exitcode.Failure, got %v", err)
	}
}

func TestGet_ScalarRoot(t *testing.T) {
	fs := testutil.NewMemFS()
	path := "/data/clusterwidgets/foo/.clusterwidget.yaml"
	_ = fs.WriteFile(path, []byte(`"hello"`))

	m, err := labels.Get(fs, "/data", "clusterwidgets", "foo", "")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestGet_MissingMetadata(t *testing.T) {
	fs := testutil.NewMemFS()
	path := "/data/clusterwidgets/foo/.clusterwidget.yaml"
	_ = fs.WriteFile(path, []byte(`
name: foo
`))

	m, err := labels.Get(fs, "/data", "clusterwidgets", "foo", "")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestGet_ResolvePathError(t *testing.T) {
	fs := testutil.NewMemFS()

	_, err := labels.Get(fs, "/data", "unknownkind", "foo", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitcode.Error
	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.UsageError {
		t.Errorf("expected exitcode.UsageError, got %v", err)
	}
}

func TestSetTracked_ResolvePathError(t *testing.T) {
	fs := testutil.NewMemFS()

	_, _, err := labels.SetTracked(fs, "/data", "unknownkind", "foo", "", []string{"key=value"}, true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitcode.Error
	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.UsageError {
		t.Errorf("expected exitcode.UsageError, got %v", err)
	}
}

func TestSetTracked_ReadManifestError(t *testing.T) {
	memFS := testutil.NewMemFS()
	fs := &errorFS{
		MemFS:   memFS,
		readErr: errors.New("read error"),
	}

	_, _, err := labels.SetTracked(fs, "/data", "clusterwidgets", "foo", "", []string{"key=value"}, true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Note: in readManifestDoc, any read error is treated as exitcode.NotFound
	var exitErr *exitcode.Error
	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.NotFound {
		t.Errorf("expected exitcode.NotFound, got %v", err)
	}
}

func TestSetTracked_WriteManifestError(t *testing.T) {
	memFS := testutil.NewMemFS()
	path := "/data/clusterwidgets/foo/.clusterwidget.yaml"
	_ = memFS.WriteFile(path, []byte(`
metadata:
  name: foo
`))
	fs := &errorFS{
		MemFS:    memFS,
		writeErr: errors.New("write error"),
	}

	_, _, err := labels.SetTracked(fs, "/data", "clusterwidgets", "foo", "", []string{"key=value"}, true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitcode.Error
	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.Failure {
		t.Errorf("expected exitcode.Failure, got %v", err)
	}
}

func TestSetTracked_InvalidKeyForRemoval(t *testing.T) {
	fs := testutil.NewMemFS()
	path := "/data/clusterwidgets/foo/.clusterwidget.yaml"
	_ = fs.WriteFile(path, []byte(`
metadata:
  name: foo
`))

	_, _, err := labels.SetTracked(fs, "/data", "clusterwidgets", "foo", "", []string{"invalid..key-"}, true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitcode.Error
	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.UsageError {
		t.Errorf("expected exitcode.UsageError, got %v", err)
	}
}
