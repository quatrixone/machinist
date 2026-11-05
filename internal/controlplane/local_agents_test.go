package controlplane

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/owainlewis/machinist/internal/config"
)

func TestLocalAgentSetup(t *testing.T) {
	_, setup := setupTestServer(t)
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	setup.settings.Selection.Repository = "owner/repo"
	for _, agent := range []string{"opencode", "hermes"} {
		if err := os.WriteFile(filepath.Join(dir, agent), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := setup.selectAgent(agent, "local"); err != nil {
			t.Fatal(err)
		}
		for _, host := range []string{"remote", "kubernetes"} {
			if err := setup.selectAgent(agent, host); err == nil {
				t.Fatalf("accepted %s on %s", agent, host)
			}
		}
		worker := config.Worker{Executors: setupExecutors(agent)}
		command, err := worker.ResolveCommandModel(config.ResolvedCommand{Executor: agent + "-native"}, "provider/model")
		if err != nil || command.Model != "provider/model" || command.Adapter == "" {
			t.Fatalf("bad model resolution: %+v %v", command, err)
		}
		for name, executor := range worker.Executors {
			if executor.ReadOnly {
				t.Fatalf("%s falsely advertises a readonly sandbox", name)
			}
		}
	}
	status, err := setup.status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(status["agents"].([]map[string]any)) != 4 {
		t.Fatalf("missing agent choices: %+v", status["agents"])
	}
}

func TestSetupProcessExecutorModels(t *testing.T) {
	for _, agent := range []string{"codex", "claude"} {
		worker := config.Worker{Executors: setupExecutors(agent)}
		for _, executor := range []string{agent, agent + "-review"} {
			selected, err := worker.ResolveCommandModel(config.ResolvedCommand{Executor: executor}, "selected-model")
			if err != nil || !strings.Contains(strings.Join(selected.Command, " "), "--model=selected-model") {
				t.Fatalf("%s: %+v %v", executor, selected, err)
			}
			defaults, err := worker.ResolveCommandModel(config.ResolvedCommand{Executor: executor}, "")
			if err != nil || strings.Contains(strings.Join(defaults.Command, " "), "--model") {
				t.Fatalf("%s: %+v %v", executor, defaults, err)
			}
		}
	}
}
