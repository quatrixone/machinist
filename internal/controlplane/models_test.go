package controlplane

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseOpenCodeModelsOutput(t *testing.T) {
	got := parseOpenCodeModelsOutput("opencode/gpt-6-luna\nopencode/claude-sonnet-5\nopencode/gpt-6-luna\n\nnot a model\n" + string(make([]byte, 129)))
	want := []ModelChoice{
		{ID: "opencode/claude-sonnet-5", Label: "opencode/claude-sonnet-5"},
		{ID: "opencode/gpt-6-luna", Label: "opencode/gpt-6-luna"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %#v, want %#v", got, want)
	}
}

func TestReadCodexModelsUsesVisibleAccountModels(t *testing.T) {
	dir := t.TempDir()
	body := `{"models":[{"slug":"gpt-6-luna","display_name":"GPT-6 Luna","visibility":"list","supported_in_api":true},{"slug":"hidden","display_name":"Hidden","visibility":"hide","supported_in_api":true},{"slug":"unsupported","display_name":"Unsupported","visibility":"list","supported_in_api":false}]}`
	if err := os.WriteFile(filepath.Join(dir, "models_cache.json"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := readCodexModels(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []ModelChoice{{ID: "gpt-6-luna", Label: "GPT-6 Luna"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %#v, want %#v", got, want)
	}
}

func TestReadHermesModelsIncludesConfiguredDefaultAndCachedProviderModels(t *testing.T) {
	dir := t.TempDir()
	config := "model:\n  provider: anthropic\n  default: anthropic/claude-sonnet-5\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	cache := `{"anthropic":{"models":["claude-sonnet-5","claude-opus-5"]}}`
	if err := os.WriteFile(filepath.Join(dir, "provider_models_cache.json"), []byte(cache), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := readHermesModels(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []ModelChoice{
		{ID: "anthropic/claude-opus-5", Label: "anthropic/claude-opus-5"},
		{ID: "anthropic/claude-sonnet-5", Label: "anthropic/claude-sonnet-5"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %#v, want %#v", got, want)
	}
}

func TestReadHermesOpenAICompatibleModelsFromLocalCatalog(t *testing.T) {
	dir := t.TempDir()
	config := "model:\n  provider: nous\n  default: openai/gpt-6-luna\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	catalog := `{"openai":{"models":{"gpt-6-luna":{"name":"GPT-6 Luna"},"gpt-5.6-sol":{"name":"GPT-5.6 Sol"}}}}`
	if err := os.WriteFile(filepath.Join(dir, "models_dev_cache.json"), []byte(catalog), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := readHermesModels(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("models = %#v, want configured default and two local catalog models", got)
	}
	for _, model := range got {
		if !strings.HasPrefix(model.ID, "openai/") {
			t.Fatalf("unexpected model %q", model.ID)
		}
	}
}
