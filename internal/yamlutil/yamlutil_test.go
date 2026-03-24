package yamlutil_test

import (
	"strings"
	"testing"

	"github.com/walheimlab/walheim-go/internal/yamlutil"
)

func TestMarshal_simpleStruct(t *testing.T) {
	type manifest struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
	}

	data, err := yamlutil.Marshal(manifest{APIVersion: "walheim/v1alpha1", Kind: "Namespace"})
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	s := string(data)
	if !strings.Contains(s, "apiVersion: walheim/v1alpha1") {
		t.Errorf("output missing apiVersion: %q", s)
	}

	if !strings.Contains(s, "kind: Namespace") {
		t.Errorf("output missing kind: %q", s)
	}
}

func TestMarshal_twoSpaceIndentation(t *testing.T) {
	type nested struct {
		Outer struct {
			Inner string `yaml:"inner"`
		} `yaml:"outer"`
	}

	var v nested

	v.Outer.Inner = "value"

	data, err := yamlutil.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	// 2-space indentation means "  inner:" not "    inner:"
	if !strings.Contains(string(data), "  inner: value") {
		t.Errorf("expected 2-space indent, got:\n%s", data)
	}
}

func TestMarshal_map(t *testing.T) {
	m := map[string]string{"key": "val"}

	data, err := yamlutil.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	if !strings.Contains(string(data), "key: val") {
		t.Errorf("expected key: val in output, got: %q", string(data))
	}
}

func TestMarshal_nil(t *testing.T) {
	data, err := yamlutil.Marshal(nil)
	if err != nil {
		t.Fatalf("Marshal(nil) error: %v", err)
	}
	// nil marshals to "null\n"
	if strings.TrimSpace(string(data)) != "null" {
		t.Errorf("Marshal(nil) = %q, want null", string(data))
	}
}
