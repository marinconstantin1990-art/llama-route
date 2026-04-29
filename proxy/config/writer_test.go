package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempYAML(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestConfigWriter_SetModelAddsAndReplaces(t *testing.T) {
	p := writeTempYAML(t, `# top comment
healthCheckTimeout: 30
models:
  existing:
    cmd: llama-server -m existing.gguf
`)

	if err := SetModel(p, "newone", ModelConfig{Cmd: "llama-server -m new.gguf"}); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, p)
	if !strings.Contains(got, "# top comment") {
		t.Errorf("top-level comment lost:\n%s", got)
	}
	if !strings.Contains(got, "newone:") || !strings.Contains(got, "new.gguf") {
		t.Errorf("new model not added:\n%s", got)
	}
	if !strings.Contains(got, "existing:") {
		t.Errorf("existing model dropped:\n%s", got)
	}

	if err := SetModel(p, "newone", ModelConfig{Cmd: "llama-server -m updated.gguf"}); err != nil {
		t.Fatal(err)
	}
	got = readFile(t, p)
	if !strings.Contains(got, "updated.gguf") || strings.Contains(got, "new.gguf") {
		t.Errorf("update did not replace:\n%s", got)
	}
}

func TestConfigWriter_DeleteModel(t *testing.T) {
	p := writeTempYAML(t, `models:
  a: {cmd: llama-server -m a}
  b: {cmd: llama-server -m b}
`)
	if err := DeleteModel(p, "a"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, p)
	if strings.Contains(got, "a:") {
		t.Errorf("model a not deleted:\n%s", got)
	}
	if !strings.Contains(got, "b:") {
		t.Errorf("model b dropped:\n%s", got)
	}
}

func TestConfigWriter_SetGPUEnabled(t *testing.T) {
	p := writeTempYAML(t, `models: {}
`)
	if err := SetGPUEnabled(p, "nvidia:0", false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, p)
	if !strings.Contains(got, "nvidia:0") || !strings.Contains(got, "enabled: false") {
		t.Errorf("gpu entry not written:\n%s", got)
	}
	if err := SetGPUEnabled(p, "nvidia:0", true); err != nil {
		t.Fatal(err)
	}
	got = readFile(t, p)
	if !strings.Contains(got, "enabled: true") {
		t.Errorf("gpu entry not updated:\n%s", got)
	}
}

func TestConfigWriter_EmptyFile(t *testing.T) {
	p := writeTempYAML(t, "")
	if err := SetModel(p, "first", ModelConfig{Cmd: "llama-server -m first"}); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, p)
	if !strings.Contains(got, "first:") {
		t.Errorf("model not added to empty file:\n%s", got)
	}
}
