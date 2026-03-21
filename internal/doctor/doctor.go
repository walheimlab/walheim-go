// Package doctor provides shared types and common filesystem checks
// for the "doctor" verb, usable by any resource kind.
package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

// Severity classifies how serious a finding is.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Finding is one diagnostic result for a single resource.
type Finding struct {
	Severity Severity `json:"severity"`
	// Resource is the human-readable identifier: "namespace/baymax", "app/freya/memos"
	Resource string `json:"resource"`
	// Check is a short machine-readable name for the check that produced this finding.
	Check string `json:"check"`
	// Message is the human-readable description of the problem.
	Message string `json:"message"`
}

// Report collects all findings from a doctor run.
type Report struct {
	findings []Finding
}

// Add appends a finding to the report.
func (r *Report) Add(severity Severity, resource, check, message string) {
	r.findings = append(r.findings, Finding{
		Severity: severity,
		Resource: resource,
		Check:    check,
		Message:  message,
	})
}

// Errorf adds an error-severity finding with a formatted message.
func (r *Report) Errorf(resource, check, format string, args ...any) {
	r.Add(SeverityError, resource, check, fmt.Sprintf(format, args...))
}

// Warnf adds a warning-severity finding with a formatted message.
func (r *Report) Warnf(resource, check, format string, args ...any) {
	r.Add(SeverityWarning, resource, check, fmt.Sprintf(format, args...))
}

// Infof adds an info-severity finding with a formatted message.
func (r *Report) Infof(resource, check, format string, args ...any) {
	r.Add(SeverityInfo, resource, check, fmt.Sprintf(format, args...))
}

// Findings returns a copy of all findings.
func (r *Report) Findings() []Finding {
	out := make([]Finding, len(r.findings))
	copy(out, r.findings)
	return out
}

// HasErrors reports whether any error-severity findings exist.
func (r *Report) HasErrors() bool {
	for _, f := range r.findings {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Counts returns error, warning, and info counts.
func (r *Report) Counts() (errors, warnings, infos int) {
	for _, f := range r.findings {
		switch f.Severity {
		case SeverityError:
			errors++
		case SeverityWarning:
			warnings++
		case SeverityInfo:
			infos++
		}
	}
	return
}

// PrintHuman writes findings and a summary to stdout in human-readable form.
func (r *Report) PrintHuman(quiet bool) {
	if len(r.findings) == 0 {
		if !quiet {
			fmt.Println("No issues found.")
		}
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	if !quiet {
		fmt.Fprintln(w, "SEVERITY\tRESOURCE\tCHECK\tMESSAGE")
	}
	for _, f := range r.findings {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			strings.ToUpper(string(f.Severity)), f.Resource, f.Check, f.Message)
	}
	w.Flush()

	if !quiet {
		errors, warnings, infos := r.Counts()
		fmt.Println()
		fmt.Printf("Summary: %d error(s), %d warning(s), %d info(s)\n", errors, warnings, infos)
	}
}

// PrintJSON writes findings as a JSON object with a findings array and summary.
func (r *Report) PrintJSON() error {
	errors, warnings, infos := r.Counts()
	out := map[string]any{
		"findings": r.findings,
		"summary": map[string]int{
			"errors":   errors,
			"warnings": warnings,
			"infos":    infos,
		},
	}
	if r.findings == nil {
		out["findings"] = []Finding{}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// ── Common checks ─────────────────────────────────────────────────────────────

// CheckDirNameMatchesMetadataName verifies that the resource's directory name
// equals metadata.name from the parsed manifest.
// dirName is the filesystem directory name; metadataName is from the manifest.
func CheckDirNameMatchesMetadataName(r *Report, resourceID, dirName, metadataName string) {
	if dirName != metadataName {
		r.Errorf(resourceID, "metadata-name-mismatch",
			"directory name %q does not match metadata.name %q", dirName, metadataName)
	}
}

// CheckNamespaceFieldMatchesDir verifies that metadata.namespace in a namespaced
// resource's manifest matches the actual parent namespace directory name.
func CheckNamespaceFieldMatchesDir(r *Report, resourceID, manifestNamespace, dirNamespace string) {
	if manifestNamespace != dirNamespace {
		r.Errorf(resourceID, "metadata-namespace-mismatch",
			"metadata.namespace %q does not match parent namespace directory %q",
			manifestNamespace, dirNamespace)
	}
}

// CheckAPIVersion verifies that the manifest's apiVersion matches the expected value.
func CheckAPIVersion(r *Report, resourceID, got, want string) {
	if got != want {
		r.Errorf(resourceID, "apiversion-mismatch",
			"apiVersion is %q, expected %q", got, want)
	}
}

// CheckKind verifies that the manifest's kind matches the expected value.
func CheckKind(r *Report, resourceID, got, want string) {
	if got != want {
		r.Errorf(resourceID, "kind-mismatch",
			"kind is %q, expected %q", got, want)
	}
}
