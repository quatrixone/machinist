package controlplane

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestModelsEndpointUsesSelectedLocalAgent(t *testing.T) {
	s, setup := setupTestServer(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	codexHome := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexHome, 0700); err != nil {
		t.Fatal(err)
	}
	codexCache := `{"models":[{"slug":"gpt-6-luna","display_name":"GPT-6 Luna","visibility":"list","supported_in_api":true}]}`
	if err := os.WriteFile(filepath.Join(codexHome, "models_cache.json"), []byte(codexCache), 0600); err != nil {
		t.Fatal(err)
	}

	hermesHome := filepath.Join(home, ".hermes")
	if err := os.MkdirAll(hermesHome, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hermesHome, "config.yaml"), []byte("model:\n  provider: nous\n  default: openai/gpt-6-luna\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hermesHome, "models_dev_cache.json"), []byte(`{"openai":{"models":{"gpt-6-luna":{"name":"GPT-6 Luna"},"gpt-5.6-sol":{"name":"GPT-5.6 Sol"}}}}`), 0600); err != nil {
		t.Fatal(err)
	}

	bin := t.TempDir()
	openCode := filepath.Join(bin, "opencode")
	if err := os.WriteFile(openCode, []byte("#!/bin/sh\nprintf 'opencode/gpt-6-luna\\nopencode/claude-sonnet-5\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	cases := []struct {
		agent string
		want  string
	}{
		{agent: "codex", want: "gpt-6-luna"},
		{agent: "opencode", want: "opencode/gpt-6-luna"},
		{agent: "hermes", want: "openai/gpt-6-luna"},
	}
	for _, tc := range cases {
		t.Run(tc.agent, func(t *testing.T) {
			setup.settings.Selection.Agent = tc.agent
			var result modelChoicesResponse
			if err := json.Unmarshal(factoryHTTP(t, s, "GET", "/api/v1/models", nil, 200), &result); err != nil {
				t.Fatal(err)
			}
			if result.Agent != tc.agent {
				t.Fatalf("response agent = %q, want %q", result.Agent, tc.agent)
			}
			if tc.agent != "claude" && len(result.Models) == 0 {
				t.Fatalf("no models returned for %s", tc.agent)
			}

		})
	}
}
