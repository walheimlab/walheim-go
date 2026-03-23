package resource_test

import (
	"testing"

	"github.com/walheimlab/walheim-go/internal/resource"
)

func TestKindInfo_Singular(t *testing.T) {
	cases := []struct {
		kind string
		want string
	}{
		{"Namespace", "namespace"},
		{"App", "app"},
		{"ConfigMap", "configmap"},
		{"DaemonSet", "daemonset"},
		{"Job", "job"},
		{"Secret", "secret"},
	}
	for _, tc := range cases {
		k := resource.KindInfo{Kind: tc.kind}
		if got := k.Singular(); got != tc.want {
			t.Errorf("KindInfo{Kind:%q}.Singular() = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

func TestKindInfo_APIVersion(t *testing.T) {
	cases := []struct {
		group, version string
		want           string
	}{
		{"walheim", "v1alpha1", "walheim/v1alpha1"},
		{"apps", "v1", "apps/v1"},
		{"", "v1", "v1"},
	}
	for _, tc := range cases {
		k := resource.KindInfo{Group: tc.group, Version: tc.version}
		if got := k.APIVersion(); got != tc.want {
			t.Errorf("APIVersion() = %q, want %q", got, tc.want)
		}
	}
}
