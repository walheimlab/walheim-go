package v1alpha1

import (
	"reflect"
	"sort"
	"testing"
)

// ── targetServices ────────────────────────────────────────────────────────────

func TestTargetServices_allWhenEmpty(t *testing.T) {
	services := map[string]ComposeService{
		"web": {}, "db": {}, "cache": {},
	}
	got := targetServices(services, nil)
	sort.Strings(got)

	want := []string{"cache", "db", "web"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("targetServices(nil) = %v, want %v", got, want)
	}
}

func TestTargetServices_explicitNames(t *testing.T) {
	services := map[string]ComposeService{
		"web": {}, "db": {}, "cache": {},
	}
	got := targetServices(services, []string{"web", "db"})
	// Order matches input, not sorted
	if len(got) != 2 {
		t.Fatalf("got %d services, want 2", len(got))
	}

	gotSet := map[string]bool{got[0]: true, got[1]: true}
	if !gotSet["web"] || !gotSet["db"] {
		t.Errorf("targetServices([web,db]) = %v, want web and db", got)
	}
}

func TestTargetServices_sorted(t *testing.T) {
	services := map[string]ComposeService{"z": {}, "a": {}, "m": {}}

	got := targetServices(services, nil)
	if !sort.StringsAreSorted(got) {
		t.Errorf("targetServices() result is not sorted: %v", got)
	}
}

// ── substituteVars ────────────────────────────────────────────────────────────

func TestSubstituteVars(t *testing.T) {
	cases := []struct {
		name  string
		input string
		env   map[string]string
		want  string
	}{
		{
			name:  "no vars",
			input: "hello world",
			env:   map[string]string{},
			want:  "hello world",
		},
		{
			name:  "single substitution",
			input: "prefix-${NAME}-suffix",
			env:   map[string]string{"NAME": "myapp"},
			want:  "prefix-myapp-suffix",
		},
		{
			name:  "unknown var preserved",
			input: "${MISSING}",
			env:   map[string]string{},
			want:  "${MISSING}",
		},
		{
			name:  "multiple vars",
			input: "${A}-${B}",
			env:   map[string]string{"A": "foo", "B": "bar"},
			want:  "foo-bar",
		},
		{
			name:  "partial unknown",
			input: "${KNOWN}-${UNKNOWN}",
			env:   map[string]string{"KNOWN": "yes"},
			want:  "yes-${UNKNOWN}",
		},
		{
			name:  "empty string",
			input: "",
			env:   map[string]string{"X": "y"},
			want:  "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := substituteVars(tc.input, tc.env)
			if got != tc.want {
				t.Errorf("substituteVars(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ── sortedUnique ──────────────────────────────────────────────────────────────

func TestSortedUnique(t *testing.T) {
	cases := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "empty",
			input: nil,
			want:  nil,
		},
		{
			name:  "already unique and sorted",
			input: []string{"a", "b", "c"},
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "duplicates removed",
			input: []string{"b", "a", "b", "c", "a"},
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "single element",
			input: []string{"x"},
			want:  []string{"x"},
		},
		{
			name:  "all same",
			input: []string{"z", "z", "z"},
			want:  []string{"z"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sortedUnique(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("sortedUnique(%v) = %v, want %v", tc.input, got, tc.want)
			}

			if got != nil && !sort.StringsAreSorted(got) {
				t.Errorf("result is not sorted: %v", got)
			}
		})
	}
}
