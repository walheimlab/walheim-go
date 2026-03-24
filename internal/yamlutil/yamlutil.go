package yamlutil

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

// Marshal serialises v to YAML with 2-space indentation.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)

	if err := enc.Encode(v); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
