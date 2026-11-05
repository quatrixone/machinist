package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxAdvertisedModels = 1000
const maxModelIDLength = 128

type ModelChoice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type modelChoicesResponse struct {
	Agent  string        `json:"agent"`
	Models []ModelChoice `json:"models"`
}

func (s *GitHubFactoryServer) models(w http.ResponseWriter, r *http.Request) {
	if s.setup == nil {
		writeError(w, http.StatusNotFound, os.ErrNotExist)
		return
	}
	agent := s.setup.snapshot().Selection.Agent
	choices := []ModelChoice{}
	var err error
	switch agent {
	case "codex":
		choices, err = readCodexModels(filepath.Join(os.Getenv("HOME"), ".codex"))
	case "claude":
		choices = []ModelChoice{{ID: "sonnet", Label: "Claude Sonnet"}, {ID: "opus", Label: "Claude Opus"}, {ID: "haiku", Label: "Claude Haiku"}}
	case "opencode":
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		choices, err = listOpenCodeModels(ctx)
		cancel()
	case "hermes":
		choices, err = readHermesModels(filepath.Join(os.Getenv("HOME"), ".hermes"))
	}
	if err != nil {
		// Model listing is an enhancement, not a requirement for ordinary runs.
		choices = []ModelChoice{}
	}
	if choices == nil {
		choices = []ModelChoice{}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, modelChoicesResponse{Agent: agent, Models: choices})
}

func listOpenCodeModels(ctx context.Context) ([]ModelChoice, error) {
	command := exec.CommandContext(ctx, "opencode", "models")
	output, err := command.Output()
	if err != nil {
		return nil, err
	}
	return parseOpenCodeModelsOutput(string(output)), nil
}

func parseOpenCodeModelsOutput(output string) []ModelChoice {
	seen := map[string]bool{}
	choices := make([]ModelChoice, 0)
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		id := strings.TrimSpace(scanner.Text())
		if id == "" || !strings.Contains(id, "/") || len(id) > maxModelIDLength || strings.ContainsAny(id, "\x00\r\n") || seen[id] {
			continue
		}
		seen[id] = true
		choices = append(choices, ModelChoice{ID: id, Label: id})
		if len(choices) >= maxAdvertisedModels {
			break
		}
	}
	sort.Slice(choices, func(i, j int) bool { return choices[i].ID < choices[j].ID })
	return choices
}

func readCodexModels(home string) ([]ModelChoice, error) {
	body, err := os.ReadFile(filepath.Join(home, "models_cache.json"))
	if err != nil {
		return nil, err
	}
	var cache struct {
		Models []struct {
			Slug           string `json:"slug"`
			DisplayName    string `json:"display_name"`
			Visibility     string `json:"visibility"`
			SupportedInAPI bool   `json:"supported_in_api"`
		} `json:"models"`
	}
	if err = json.Unmarshal(body, &cache); err != nil {
		return nil, err
	}
	choices := make([]ModelChoice, 0, len(cache.Models))
	seen := map[string]bool{}
	for _, model := range cache.Models {
		if model.Slug == "" || len(model.Slug) > maxModelIDLength || model.Visibility != "list" || model.SupportedInAPI != true || seen[model.Slug] {
			continue
		}
		seen[model.Slug] = true
		label := model.DisplayName
		if label == "" {
			label = model.Slug
		}
		choices = append(choices, ModelChoice{ID: model.Slug, Label: label})
		if len(choices) >= maxAdvertisedModels {
			break
		}
	}
	sort.Slice(choices, func(i, j int) bool { return choices[i].ID < choices[j].ID })
	return choices, nil
}

func readHermesModels(home string) ([]ModelChoice, error) {
	configBody, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		return nil, err
	}
	// Read only provider and default-model scalar keys from Hermes config.
	provider, defaultModel := "", ""
	inModel := false
	scanner := bufio.NewScanner(strings.NewReader(string(configBody)))
	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "model:" {
			inModel = true
			continue
		}
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(raw, " ") && !strings.HasPrefix(raw, "\t") {
			inModel = false
		}
		if !inModel || line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.Trim(strings.TrimSpace(strings.SplitN(value, "#", 2)[0]), "\"'")
		switch strings.TrimSpace(key) {
		case "provider":
			provider = value
		case "default", "model":
			if defaultModel == "" {
				defaultModel = value
			}
		}
	}
	if err = scanner.Err(); err != nil {
		return nil, err
	}
	if provider == "" {
		provider = strings.SplitN(defaultModel, "/", 2)[0]
	}
	return readHermesProviderModels(home, provider, defaultModel)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func readHermesProviderModels(home, provider, defaultModel string) ([]ModelChoice, error) {
	choices := []ModelChoice{}
	seen := map[string]bool{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || len(id) > maxModelIDLength || strings.ContainsAny(id, "\x00\r\n") || seen[id] {
			return
		}
		seen[id] = true
		choices = append(choices, ModelChoice{ID: id, Label: id})
	}
	add(defaultModel)
	if provider == "" {
		provider = strings.SplitN(defaultModel, "/", 2)[0]
	}
	if provider != "" {
		body, err := os.ReadFile(filepath.Join(home, "provider_models_cache.json"))
		if err == nil {
			var cache map[string]struct {
				Models []string `json:"models"`
			}
			if json.Unmarshal(body, &cache) == nil {
				for _, model := range cache[provider].Models {
					if strings.Contains(model, "/") {
						add(model)
					} else {
						add(provider + "/" + model)
					}
				}
			}
		}
	}
	if len(choices) == 1 && defaultModel != "" {
		// Hermes deployments can use an OpenAI-compatible inference route whose
		// credential-specific model cache is separate from the configured CLI
		// provider. Offer that provider's installed models.dev catalog, while
		// retaining the configured default as the authoritative first choice.
		catalogProvider := strings.SplitN(defaultModel, "/", 2)[0]
		if strings.Contains(defaultModel, "/") {
			if models, err := readHermesPublicCatalog(home, catalogProvider); err == nil {
				for _, model := range models {
					add(model)
				}
			}
		}
	}
	sort.Slice(choices, func(i, j int) bool { return choices[i].ID < choices[j].ID })
	if len(choices) == 0 {
		return nil, os.ErrNotExist
	}
	return choices, nil
}
