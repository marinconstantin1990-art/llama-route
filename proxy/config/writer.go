package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// EditFunc receives the parsed root document node and may mutate it in place.
// Returning an error aborts the write.
type EditFunc func(root *yaml.Node) error

// EditFile reads path, parses it as YAML preserving comments + key order, runs
// edit on the document node, then atomically rewrites the file.
//
// Existing comments and the order of unmodified keys are preserved because we
// operate on yaml.Node trees rather than going through a Go struct.
func EditFile(path string, edit EditFunc) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	var root yaml.Node
	if len(bytes.TrimSpace(data)) == 0 {
		// empty file: synthesize an empty mapping document
		root = yaml.Node{
			Kind: yaml.DocumentNode,
			Content: []*yaml.Node{
				{Kind: yaml.MappingNode, Tag: "!!map"},
			},
		}
	} else if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	if err := edit(&root); err != nil {
		return err
	}

	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	enc.Close()

	return atomicWrite(path, out.Bytes())
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".llama-swap-config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

// rootMap returns the top-level mapping node of a document, creating it if the
// document is empty.
func rootMap(root *yaml.Node) (*yaml.Node, error) {
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return nil, fmt.Errorf("yaml: not a document node")
	}
	m := root.Content[0]
	if m.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("yaml: top-level is not a mapping")
	}
	return m, nil
}

// findKey returns the value node for key in mapping, or nil if absent. The
// returned node may be mutated in place.
func findKey(mapping *yaml.Node, key string) *yaml.Node {
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// ensureKey returns the value node for key, creating it as an empty mapping if
// it does not yet exist.
func ensureKey(mapping *yaml.Node, key string) *yaml.Node {
	if v := findKey(mapping, key); v != nil {
		return v
	}
	k := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	v := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	mapping.Content = append(mapping.Content, k, v)
	return v
}

// deleteKey removes key from mapping if present and returns whether it did.
func deleteKey(mapping *yaml.Node, key string) bool {
	if mapping.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return true
		}
	}
	return false
}

// modelWriteShape is the on-disk representation used when persisting a model
// from the API. It deliberately uses omitempty everywhere so we never write
// zero-value fields that would (a) override sensible defaults applied during
// load or (b) trip up custom UnmarshalYAML logic — e.g. MacroList rejects an
// empty list because it expects a mapping.
type modelWriteShape struct {
	Cmd              string           `yaml:"cmd"`
	Name             string           `yaml:"name,omitempty"`
	Description      string           `yaml:"description,omitempty"`
	Aliases          []string         `yaml:"aliases,omitempty"`
	ConcurrencyLimit int              `yaml:"concurrencyLimit,omitempty"`
	UnloadAfter      int              `yaml:"ttl,omitempty"`
	AutoScale        *AutoScaleConfig `yaml:"autoScale,omitempty"`
}

func toModelWriteShape(m ModelConfig) modelWriteShape {
	w := modelWriteShape{
		Cmd:              m.Cmd,
		Name:             m.Name,
		Description:      m.Description,
		Aliases:          m.Aliases,
		ConcurrencyLimit: m.ConcurrencyLimit,
		UnloadAfter:      m.UnloadAfter,
	}
	// Only emit autoScale when the user actually enabled it or set non-default
	// fields, so the file stays clean for legacy single-instance models.
	if m.AutoScale.Enabled || m.AutoScale.MaxInstances != 0 || len(m.AutoScale.AllowedGPUs) > 0 {
		as := m.AutoScale
		w.AutoScale = &as
	}
	return w
}

// SetModel adds or replaces a model entry in config.yaml. Only fields the
// caller actually populated are written — zero-value fields fall back to the
// defaults applied during config load.
func SetModel(path, id string, m ModelConfig) error {
	return EditFile(path, func(root *yaml.Node) error {
		top, err := rootMap(root)
		if err != nil {
			return err
		}
		models := ensureKey(top, "models")
		if models.Kind != yaml.MappingNode {
			return fmt.Errorf("yaml: models is not a mapping")
		}

		var modelNode yaml.Node
		if err := modelNode.Encode(toModelWriteShape(m)); err != nil {
			return fmt.Errorf("encode model: %w", err)
		}

		// Replace if present, otherwise append.
		for i := 0; i+1 < len(models.Content); i += 2 {
			if models.Content[i].Value == id {
				models.Content[i+1] = &modelNode
				return nil
			}
		}
		models.Content = append(models.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: id},
			&modelNode,
		)
		return nil
	})
}

// DeleteModel removes a model entry by ID. Returns nil if the model was not
// present.
func DeleteModel(path, id string) error {
	return EditFile(path, func(root *yaml.Node) error {
		top, err := rootMap(root)
		if err != nil {
			return err
		}
		if models := findKey(top, "models"); models != nil {
			deleteKey(models, id)
		}
		return nil
	})
}

// SetGPUEnabled writes gpus.<id>.enabled in config.yaml.
func SetGPUEnabled(path, id string, enabled bool) error {
	return EditFile(path, func(root *yaml.Node) error {
		top, err := rootMap(root)
		if err != nil {
			return err
		}
		gpus := ensureKey(top, "gpus")
		entry := ensureKey(gpus, id)
		// Reset entry to a fresh mapping with just enabled to avoid stale keys.
		entry.Kind = yaml.MappingNode
		entry.Tag = "!!map"
		entry.Content = []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "enabled"},
			{Kind: yaml.ScalarNode, Tag: "!!bool", Value: fmt.Sprintf("%t", enabled)},
		}
		return nil
	})
}
