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

// PrintList renders a list of resources as a table or JSON array.
//
// columns: ordered column names. "NAMESPACE" and "NAME" are magic tokens;
// any other name is looked up case-insensitively in ResourceMeta.Summary.
// For cross-namespace listings pass "NAMESPACE" as the first column.
//
// In JSON mode: array written to stdout, warnings to stderr.
// In quiet mode: one NAME per line, no headers.
func PrintList(items []resource.ResourceMeta, columns []string, jsonMode, quiet bool) error {
	if jsonMode {
		return printListJSON(items, columns)
	}
	if quiet {
		for _, item := range items {
			fmt.Println(item.Name)
		}
		return nil
	}
	return printListTable(items, columns)
}

// PrintOne renders a single resource as raw YAML (human) or a JSON object.
// This is what "get <kind> <name>" uses — not a one-row table.
func PrintOne(item resource.ResourceMeta, jsonMode bool) error {
	if jsonMode {
		obj := flattenMeta(item)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(obj)
	}

	// Human mode: print raw YAML of the manifest
	data, err := yaml.Marshal(item.Raw)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}
	fmt.Print(string(data))
	return nil
}

// PrintEmpty prints the appropriate "nothing found" message.
// In JSON mode: writes [] to stdout.
// In quiet mode: prints nothing.
// In human mode: prints "No <kind> found" or "No <kind> found in namespace '<ns>'".
func PrintEmpty(kind, namespace string, jsonMode, quiet bool) {
	if jsonMode {
		fmt.Println("[]")
		return
	}
	if quiet {
		return
	}
	if namespace != "" {
		fmt.Fprintf(os.Stderr, "No %s found in namespace %q\n", kind, namespace)
	} else {
		fmt.Fprintf(os.Stderr, "No %s found\n", kind)
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

// printListJSON renders items as a JSON array of flat objects.
func printListJSON(items []resource.ResourceMeta, columns []string) error {
	result := make([]map[string]any, len(items))
	for i, item := range items {
		result[i] = flattenMeta(item)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// flattenMeta builds a flat map from a ResourceMeta for JSON output.
func flattenMeta(item resource.ResourceMeta) map[string]any {
	obj := make(map[string]any)
	if item.Namespace != "" {
		obj["namespace"] = item.Namespace
	}
	obj["name"] = item.Name
	for k, v := range item.Summary {
		obj[strings.ToLower(k)] = v
	}
	if len(item.Labels) > 0 {
		obj["labels"] = item.Labels
	}
	return obj
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
