// Package manifest provides helpers for loading resource manifests from
// files, directories, HTTP/HTTPS URLs, or stdin. It splits multi-document
// YAML and expands kind:List documents so callers receive one Envelope per
// resource, regardless of source format.
package manifest

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/fs"
)

// Envelope holds the GVK and metadata of a single resource document plus the
// raw YAML bytes ready for yaml.Unmarshal into the concrete manifest type.
type Envelope struct {
	APIVersion string
	Kind       string
	Name       string
	Namespace  string
	Raw        []byte
}

// LoadSources loads all resource Envelopes from the given sources.
// Each source can be a file path, directory path, HTTP/HTTPS URL, or "-"
// for stdin. Directory sources are walked one level deep; only .yaml/.yml
// files are processed.
func LoadSources(sources []string, filesystem fs.FS) ([]Envelope, error) {
	var result []Envelope

	for _, src := range sources {
		envs, err := loadSource(src, filesystem)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", src, err)
		}

		result = append(result, envs...)
	}

	return result, nil
}

func loadSource(source string, filesystem fs.FS) ([]Envelope, error) {
	switch {
	case source == "-":
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}

		return parseDocuments(data)

	case strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://"):
		data, err := fetchURL(source)
		if err != nil {
			return nil, err
		}

		return parseDocuments(data)

	default:
		isDir, err := filesystem.IsDir(source)
		if err == nil && isDir {
			return loadDir(source, filesystem)
		}

		data, err := filesystem.ReadFile(source)
		if err != nil {
			return nil, err
		}

		return parseDocuments(data)
	}
}

func loadDir(dir string, filesystem fs.FS) ([]Envelope, error) {
	names, err := filesystem.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %q: %w", dir, err)
	}

	var result []Envelope

	for _, name := range names {
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".yaml") && !strings.HasSuffix(lower, ".yml") && !strings.HasSuffix(lower, ".json") {
			continue
		}

		envs, err := loadSource(filepath.Join(dir, name), filesystem)
		if err != nil {
			return nil, err
		}

		result = append(result, envs...)
	}

	return result, nil
}

func fetchURL(url string) ([]byte, error) {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: HTTP %d", url, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response from %s: %w", url, err)
	}

	return data, nil
}

// envelopeHeader decodes only the identifying fields of a manifest document.
type envelopeHeader struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
}

// parseDocuments splits a multi-document YAML byte slice and returns one
// Envelope per non-empty resource document. kind:List documents are expanded
// into their individual items.
func parseDocuments(data []byte) ([]Envelope, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))

	var result []Envelope

	for {
		var node yaml.Node

		if err := dec.Decode(&node); err != nil {
			if err == io.EOF {
				break
			}

			return nil, fmt.Errorf("parse YAML: %w", err)
		}

		var hdr envelopeHeader

		if err := node.Decode(&hdr); err != nil {
			return nil, fmt.Errorf("decode manifest header: %w", err)
		}

		if hdr.Kind == "" {
			continue // skip empty or comment-only documents
		}

		if strings.EqualFold(hdr.Kind, "list") {
			items, err := expandList(&node)
			if err != nil {
				return nil, err
			}

			result = append(result, items...)

			continue
		}

		raw, err := yaml.Marshal(&node)
		if err != nil {
			return nil, err
		}

		result = append(result, Envelope{
			APIVersion: hdr.APIVersion,
			Kind:       hdr.Kind,
			Name:       hdr.Metadata.Name,
			Namespace:  hdr.Metadata.Namespace,
			Raw:        raw,
		})
	}

	return result, nil
}

// expandList extracts individual Envelopes from the items of a kind:List document.
func expandList(node *yaml.Node) ([]Envelope, error) {
	type listDoc struct {
		Items []*yaml.Node `yaml:"items"`
	}

	var l listDoc

	if err := node.Decode(&l); err != nil {
		return nil, fmt.Errorf("decode list items: %w", err)
	}

	var result []Envelope

	for _, item := range l.Items {
		var hdr envelopeHeader

		if err := item.Decode(&hdr); err != nil {
			return nil, fmt.Errorf("decode list item header: %w", err)
		}

		if hdr.Kind == "" {
			continue
		}

		raw, err := yaml.Marshal(item)
		if err != nil {
			return nil, err
		}

		result = append(result, Envelope{
			APIVersion: hdr.APIVersion,
			Kind:       hdr.Kind,
			Name:       hdr.Metadata.Name,
			Namespace:  hdr.Metadata.Namespace,
			Raw:        raw,
		})
	}

	return result, nil
}
