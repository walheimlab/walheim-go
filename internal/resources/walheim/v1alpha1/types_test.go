package v1alpha1

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ── LabelSelector.Matches ─────────────────────────────────────────────────────

func TestLabelSelector_Matches(t *testing.T) {
	cases := []struct {
		name     string
		selector *LabelSelector
		labels   map[string]string
		want     bool
	}{
		{
			name:     "nil selector matches everything",
			selector: nil,
			labels:   map[string]string{"env": "prod"},
			want:     true,
		},
		{
			name:     "empty matchLabels matches everything",
			selector: &LabelSelector{MatchLabels: map[string]string{}},
			labels:   map[string]string{"env": "prod"},
			want:     true,
		},
		{
			name:     "exact match",
			selector: &LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
			labels:   map[string]string{"env": "prod"},
			want:     true,
		},
		{
			name:     "partial match on superset",
			selector: &LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
			labels:   map[string]string{"env": "prod", "region": "eu"},
			want:     true,
		},
		{
			name:     "value mismatch",
			selector: &LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
			labels:   map[string]string{"env": "dev"},
			want:     false,
		},
		{
			name:     "missing key",
			selector: &LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
			labels:   map[string]string{"region": "eu"},
			want:     false,
		},
		{
			name:     "empty labels, non-empty selector",
			selector: &LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
			labels:   map[string]string{},
			want:     false,
		},
		{
			name: "multi-key selector, all match",
			selector: &LabelSelector{MatchLabels: map[string]string{
				"env": "prod", "region": "eu",
			}},
			labels: map[string]string{"env": "prod", "region": "eu", "team": "infra"},
			want:   true,
		},
		{
			name: "multi-key selector, one missing",
			selector: &LabelSelector{MatchLabels: map[string]string{
				"env": "prod", "region": "eu",
			}},
			labels: map[string]string{"env": "prod"},
			want:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.selector.Matches(tc.labels)
			if got != tc.want {
				t.Errorf("Matches() = %v, want %v", got, tc.want)
			}
		})
	}
}

// ── NamespaceSpec helpers ─────────────────────────────────────────────────────

func TestNamespaceSpec_remoteBaseDir(t *testing.T) {
	cases := []struct {
		baseDir string
		want    string
	}{
		{"", DefaultRemoteBaseDir},
		{"/custom/path", "/custom/path"},
	}
	for _, tc := range cases {
		s := NamespaceSpec{BaseDir: tc.baseDir}
		if got := s.remoteBaseDir(); got != tc.want {
			t.Errorf("remoteBaseDir(%q) = %q, want %q", tc.baseDir, got, tc.want)
		}
	}
}

func TestNamespaceSpec_sshTarget(t *testing.T) {
	cases := []struct {
		username, hostname, want string
	}{
		{"", "myhost.local", "myhost.local"},
		{"admin", "myhost.local", "admin@myhost.local"},
	}
	for _, tc := range cases {
		s := NamespaceSpec{Username: tc.username, Hostname: tc.hostname}
		if got := s.sshTarget(); got != tc.want {
			t.Errorf("sshTarget() = %q, want %q", got, tc.want)
		}
	}
}

func TestNamespaceSpec_usernameDisplay(t *testing.T) {
	s := NamespaceSpec{Username: ""}
	if got := s.usernameDisplay(); !strings.Contains(got, "SSH config") {
		t.Errorf("usernameDisplay() = %q, expected mention of SSH config", got)
	}

	s.Username = "admin"
	if got := s.usernameDisplay(); got != "admin" {
		t.Errorf("usernameDisplay() = %q, want %q", got, "admin")
	}
}

func TestNamespaceSpec_baseDirDisplay(t *testing.T) {
	s := NamespaceSpec{BaseDir: ""}
	if !strings.Contains(s.baseDirDisplay(), "default") {
		t.Errorf("baseDirDisplay() = %q, expected mention of 'default'", s.baseDirDisplay())
	}

	s.BaseDir = "/custom"
	if got := s.baseDirDisplay(); got != "/custom" {
		t.Errorf("baseDirDisplay() = %q, want %q", got, "/custom")
	}
}

// ── ServiceEnv YAML round-trip ────────────────────────────────────────────────

func TestServiceEnv_UnmarshalYAML_mapForm(t *testing.T) {
	input := "KEY1: val1\nKEY2: val2\n"

	var env ServiceEnv
	if err := yaml.Unmarshal([]byte(input), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if env.Values["KEY1"] != "val1" {
		t.Errorf("KEY1 = %q, want %q", env.Values["KEY1"], "val1")
	}

	if env.Values["KEY2"] != "val2" {
		t.Errorf("KEY2 = %q, want %q", env.Values["KEY2"], "val2")
	}
}

func TestServiceEnv_UnmarshalYAML_listForm(t *testing.T) {
	input := "- KEY1=val1\n- KEY2=val2\n- BARE\n"

	var env ServiceEnv
	if err := yaml.Unmarshal([]byte(input), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if env.Values["KEY1"] != "val1" {
		t.Errorf("KEY1 = %q, want %q", env.Values["KEY1"], "val1")
	}

	if env.Values["BARE"] != "" {
		t.Errorf("BARE = %q, want empty", env.Values["BARE"])
	}
}

func TestServiceEnv_MarshalYAML_alwaysMap(t *testing.T) {
	env := ServiceEnv{Values: map[string]string{"A": "1"}}

	out, err := yaml.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Should marshal back as map (key: value), not list form.
	if !strings.Contains(string(out), "A: \"1\"") && !strings.Contains(string(out), "A: '1'") && !strings.Contains(string(out), "A: 1") {
		t.Errorf("marshal output %q does not look like map form", string(out))
	}

	if strings.Contains(string(out), "- ") {
		t.Errorf("marshal output %q looks like list form, want map form", string(out))
	}
}

// ── ServiceLabels YAML round-trip ─────────────────────────────────────────────

func TestServiceLabels_UnmarshalYAML_mapForm(t *testing.T) {
	input := "app: myapp\nenv: prod\n"

	var labels ServiceLabels
	if err := yaml.Unmarshal([]byte(input), &labels); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if labels.Values["app"] != "myapp" {
		t.Errorf("app = %q, want %q", labels.Values["app"], "myapp")
	}
}

func TestServiceLabels_UnmarshalYAML_listForm(t *testing.T) {
	input := "- app=myapp\n- env=prod\n- bare\n"

	var labels ServiceLabels
	if err := yaml.Unmarshal([]byte(input), &labels); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if labels.Values["app"] != "myapp" {
		t.Errorf("app = %q, want %q", labels.Values["app"], "myapp")
	}

	if labels.Values["bare"] != "" {
		t.Errorf("bare = %q, want empty", labels.Values["bare"])
	}
}
