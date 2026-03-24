package output_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/resource"
)

// captureStdout redirects os.Stdout during f(), returns captured output.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	orig := os.Stdout
	os.Stdout = w

	f()

	_ = w.Close()

	os.Stdout = orig

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}

	return buf.String()
}

func makeKindInfo() resource.KindInfo {
	return resource.KindInfo{
		Group:   "walheim",
		Version: "v1alpha1",
		Kind:    "App",
		Plural:  "apps",
	}
}

func makeItems() []resource.ResourceMeta {
	return []resource.ResourceMeta{
		{
			Namespace: "prod",
			Name:      "memos",
			Summary:   map[string]string{"STATUS": "running"},
			Raw:       map[string]string{"kind": "App", "name": "memos"},
		},
		{
			Namespace: "prod",
			Name:      "tasks",
			Summary:   map[string]string{"STATUS": "stopped"},
			Raw:       map[string]string{"kind": "App", "name": "tasks"},
		},
	}
}

// ── PrintList human mode ───────────────────────────────────────────────────────

func TestPrintList_human_showsHeader(t *testing.T) {
	items := makeItems()
	info := makeKindInfo()

	out := captureStdout(t, func() {
		err := output.PrintList(items, []string{"NAMESPACE", "NAME", "STATUS"}, info, "human", false)
		if err != nil {
			t.Errorf("PrintList: %v", err)
		}
	})

	if !strings.Contains(out, "NAMESPACE") {
		t.Errorf("expected NAMESPACE header, got: %q", out)
	}

	if !strings.Contains(out, "NAME") {
		t.Errorf("expected NAME header, got: %q", out)
	}

	if !strings.Contains(out, "memos") {
		t.Errorf("expected 'memos' in output, got: %q", out)
	}

	if !strings.Contains(out, "prod") {
		t.Errorf("expected 'prod' namespace, got: %q", out)
	}
}

func TestPrintList_human_quiet_oneNamePerLine(t *testing.T) {
	items := makeItems()
	info := makeKindInfo()

	out := captureStdout(t, func() {
		err := output.PrintList(items, []string{"NAMESPACE", "NAME"}, info, "human", true)
		if err != nil {
			t.Errorf("PrintList quiet: %v", err)
		}
	})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines in quiet mode, got %d: %q", len(lines), out)
	}

	if lines[0] != "memos" && lines[1] != "memos" {
		t.Errorf("expected 'memos' in quiet output, got: %q", out)
	}
}

// ── PrintList yaml/json mode ──────────────────────────────────────────────────

func TestPrintList_yaml_isValidYAML(t *testing.T) {
	items := makeItems()
	info := makeKindInfo()

	out := captureStdout(t, func() {
		err := output.PrintList(items, []string{"NAME"}, info, "yaml", false)
		if err != nil {
			t.Errorf("PrintList yaml: %v", err)
		}
	})

	if !strings.Contains(out, "kind: List") {
		t.Errorf("expected 'kind: List', got: %q", out)
	}

	if !strings.Contains(out, "apiVersion: v1") {
		t.Errorf("expected 'apiVersion: v1', got: %q", out)
	}
}

func TestPrintList_json_isValidJSON(t *testing.T) {
	items := makeItems()
	info := makeKindInfo()

	out := captureStdout(t, func() {
		err := output.PrintList(items, []string{"NAME"}, info, "json", false)
		if err != nil {
			t.Errorf("PrintList json: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal JSON output: %v\noutput: %q", err, out)
	}

	if result["kind"] != "List" {
		t.Errorf("kind = %q, want List", result["kind"])
	}
}

// ── PrintOne ──────────────────────────────────────────────────────────────────

func TestPrintOne_yaml(t *testing.T) {
	item := resource.ResourceMeta{
		Name: "memos",
		Raw: map[string]string{
			"apiVersion": "walheim/v1alpha1",
			"kind":       "App",
			"name":       "memos",
		},
	}

	out := captureStdout(t, func() {
		err := output.PrintOne(item, "yaml")
		if err != nil {
			t.Errorf("PrintOne yaml: %v", err)
		}
	})

	if !strings.Contains(out, "memos") {
		t.Errorf("expected 'memos' in yaml output, got: %q", out)
	}
}

func TestPrintOne_json(t *testing.T) {
	type appManifest struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
		Name       string `yaml:"name"`
	}

	item := resource.ResourceMeta{
		Name: "memos",
		Raw:  appManifest{APIVersion: "walheim/v1alpha1", Kind: "App", Name: "memos"},
	}

	out := captureStdout(t, func() {
		err := output.PrintOne(item, "json")
		if err != nil {
			t.Errorf("PrintOne json: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal JSON output: %v\noutput: %q", err, out)
	}

	if result["name"] != "memos" {
		t.Errorf("name = %q, want memos", result["name"])
	}
}

// ── PrintEmpty ────────────────────────────────────────────────────────────────

func TestPrintEmpty_yaml(t *testing.T) {
	info := makeKindInfo()

	out := captureStdout(t, func() {
		output.PrintEmpty("prod", info, "yaml", false)
	})

	if !strings.Contains(out, "kind: List") {
		t.Errorf("expected 'kind: List', got: %q", out)
	}

	if !strings.Contains(out, "items: []") {
		t.Errorf("expected 'items: []', got: %q", out)
	}
}

func TestPrintEmpty_json(t *testing.T) {
	info := makeKindInfo()

	out := captureStdout(t, func() {
		output.PrintEmpty("prod", info, "json", false)
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal JSON output: %v\noutput: %q", err, out)
	}

	items, ok := result["items"].([]any)
	if !ok {
		t.Fatalf("items not []any: %T", result["items"])
	}

	if len(items) != 0 {
		t.Errorf("expected empty items, got %v", items)
	}
}

func TestPrintEmpty_human_quiet_printsNothing(t *testing.T) {
	info := makeKindInfo()

	out := captureStdout(t, func() {
		output.PrintEmpty("prod", info, "human", true)
	})

	if out != "" {
		t.Errorf("expected empty output in quiet mode, got: %q", out)
	}
}
