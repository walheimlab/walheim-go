package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/resource"
)

// ErrorPayload is emitted to stdout when --output json is set and a command fails.
// In human mode, only the Message is printed to stderr.
type ErrorPayload struct {
	Error      string            `json:"error"`
	Message    string            `json:"message"`
	Input      map[string]string `json:"input,omitempty"`
	Suggestion string            `json:"suggestion,omitempty"`
	Retryable  bool              `json:"retryable"`
}

// Errorf prints a structured error. In JSON mode, writes to stdout; otherwise stderr.
func Errorf(jsonMode bool, code, message, suggestion string, input map[string]string, retryable bool) {
	if jsonMode {
		payload := ErrorPayload{
			Error:      code,
			Message:    message,
			Input:      input,
			Suggestion: suggestion,
			Retryable:  retryable,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(payload)
	} else {
		fmt.Fprintf(os.Stderr, "Error: %s\n", message)

		if suggestion != "" {
			fmt.Fprintf(os.Stderr, "Hint:  %s\n", suggestion)
		}
	}
}

// Warnf always writes to stderr regardless of output mode.
func Warnf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Warning: "+format+"\n", args...)
}

// PrintList renders a list of resources.
//
// columns: ordered column names for human/table output. "NAMESPACE" and "NAME"
// are magic tokens; any other name is looked up case-insensitively in ResourceMeta.Summary.
//
// format: "human" (default table), "yaml" (K8s List manifest), "json" (K8s List manifest).
// In quiet mode: one NAME per line, no headers (human only).
func PrintList(items []resource.ResourceMeta, columns []string, info resource.KindInfo, format string, quiet bool) error {
	if format == "yaml" || format == "json" {
		return printListManifest(items, info, format)
	}

	// human mode
	if quiet {
		for _, item := range items {
			fmt.Println(item.Name)
		}

		return nil
	}

	return printListTable(items, columns)
}

// PrintOne renders a single resource as its K8s manifest.
// format: "human"/"yaml" → YAML manifest; "json" → JSON manifest.
func PrintOne(item resource.ResourceMeta, format string) error {
	if format == "json" {
		obj, err := rawToAny(item.Raw)
		if err != nil {
			return fmt.Errorf("failed to marshal manifest: %w", err)
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")

		return enc.Encode(obj)
	}

	// human and yaml both output the YAML manifest
	data, err := yaml.Marshal(item.Raw)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	fmt.Print(string(data))

	return nil
}

// PrintEmpty prints the appropriate "nothing found" output.
// format: "human" → message to stderr; "yaml"/"json" → empty K8s List manifest to stdout.
// In quiet human mode: prints nothing.
func PrintEmpty(namespace string, info resource.KindInfo, format string, quiet bool) {
	switch format {
	case "yaml":
		list := buildEmptyList(info)
		data, _ := yaml.Marshal(list)
		fmt.Print(string(data))
	case "json":
		list := buildEmptyList(info)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(list)
	default: // human
		if quiet {
			return
		}

		if namespace != "" {
			fmt.Fprintf(os.Stderr, "No %s found in namespace %q\n", info.Plural, namespace)
		} else {
			fmt.Fprintf(os.Stderr, "No %s found\n", info.Plural)
		}
	}
}

// printListTable renders items as a tab-separated table with headers.
func printListTable(items []resource.ResourceMeta, columns []string) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)

	defer func() { _ = w.Flush() }()

	// Print header
	_, _ = fmt.Fprintln(w, strings.Join(columns, "\t"))

	// Print rows
	for _, item := range items {
		row := make([]string, len(columns))
		for i, col := range columns {
			switch col {
			case "NAMESPACE":
				row[i] = item.Namespace
			case "NAME":
				row[i] = item.Name
			default:
				// Case-insensitive lookup in Summary
				row[i] = lookupSummary(item.Summary, col)
			}
		}

		_, _ = fmt.Fprintln(w, strings.Join(row, "\t"))
	}

	return nil
}

// printListManifest renders items as a K8s List manifest (yaml or json).
func printListManifest(items []resource.ResourceMeta, info resource.KindInfo, format string) error {
	rawItems := make([]any, 0, len(items))

	for _, item := range items {
		obj, err := rawToAny(item.Raw)
		if err != nil {
			return fmt.Errorf("failed to marshal manifest: %w", err)
		}

		rawItems = append(rawItems, obj)
	}

	list := map[string]any{
		"apiVersion": info.APIVersion(),
		"kind":       info.Kind + "List",
		"metadata":   map[string]any{},
		"items":      rawItems,
	}

	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")

		return enc.Encode(list)
	}

	data, err := yaml.Marshal(list)
	if err != nil {
		return err
	}

	fmt.Print(string(data))

	return nil
}

// buildEmptyList builds an empty K8s List manifest map.
func buildEmptyList(info resource.KindInfo) map[string]any {
	return map[string]any{
		"apiVersion": info.APIVersion(),
		"kind":       info.Kind + "List",
		"metadata":   map[string]any{},
		"items":      []any{},
	}
}

// rawToAny converts a typed manifest struct to map[string]any via YAML round-trip,
// preserving yaml field names for correct JSON serialisation.
func rawToAny(raw any) (any, error) {
	data, err := yaml.Marshal(raw)
	if err != nil {
		return nil, err
	}

	var obj any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return nil, err
	}

	return obj, nil
}

// lookupSummary does a case-insensitive lookup in the summary map.
func lookupSummary(summary map[string]string, key string) string {
	if v, ok := summary[key]; ok {
		return v
	}
	// Try case-insensitive
	keyLower := strings.ToLower(key)
	for k, v := range summary {
		if strings.ToLower(k) == keyLower {
			return v
		}
	}

	return ""
}
