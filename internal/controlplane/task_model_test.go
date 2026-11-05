package controlplane

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/owainlewis/machinist/internal/protocol"
)

func TestTaskModelPersistsAndControlsAdmission(t *testing.T) {
	for _, recipe := range []string{"single", "factory", "native-v1"} {
		t.Run(recipe, func(t *testing.T) {
			client := newMemoryFactoryGitHub()
			s := githubFactoryTestServer(t, client)
			file, err := os.OpenFile(s.definitionPath, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			_, err = file.WriteString("\n[commands.primary]\nexecutor='codex'\ntimeout='1m'\n")
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
			input := map[string]string{"repository": "repo", "recipe": recipe, "spec": "Use the selected model", "model": "chosen-model", "client_request_id": "model-request"}
			raw := factoryHTTP(t, s, "POST", "/api/v1/jobs", input, 201)
			var created map[string]string
			if err := json.Unmarshal(raw, &created); err != nil {
				t.Fatal(err)
			}
			s = restartFactoryTestServer(t, s, client)
			if got := factoryJob(t, s, created["id"]).Model; got != "chosen-model" {
				t.Fatalf("lost model after restart: %q", got)
			}
			factoryHTTP(t, s, "POST", "/api/v1/jobs", input, 200)
			input["model"] = "different-model"
			factoryHTTP(t, s, "POST", "/api/v1/jobs", input, 409)
			poll := protocol.PollRequest{NativeAgents: true, SingleAgent: true, Factory: true, Workflows: true, Reviews: true, Executors: []string{"codex"}, ReadOnlyExecutors: []string{"codex"}, Repositories: []string{"repo"}, Adapters: map[string]string{"codex": "codex-app-server"}, Models: map[string][]string{"codex": {"other-model"}}, InstanceID: "worker-instance", Name: "worker"}
			raw = factoryHTTP(t, s, "POST", "/api/v1/workers/poll", poll, 200)
			var response protocol.PollResponse
			if err := json.Unmarshal(raw, &response); err != nil {
				t.Fatal(err)
			}
			if response.Run != nil {
				t.Fatal("worker claimed unsupported model")
			}
			poll.Models["codex"] = []string{"chosen-model"}
			raw = factoryHTTP(t, s, "POST", "/api/v1/workers/poll", poll, 200)
			if err := json.Unmarshal(raw, &response); err != nil {
				t.Fatal(err)
			}
			if response.Run == nil || response.Run.Model != "chosen-model" {
				t.Fatalf("missing model in dispatch: %s", raw)
			}
			task := s.tasks[created["id"]]
			for _, stage := range task.state.Job.Workflow.Steps {
				command, err := s.jobCommand(t.Context(), task, stage)
				if err != nil || command.Model != "chosen-model" {
					t.Fatalf("stage %s lost model: %+v %v", stage, command, err)
				}
			}
		})
	}
}

func TestTaskModelValidation(t *testing.T) {
	s := githubFactoryTestServer(t, newMemoryFactoryGitHub())
	for _, model := range []string{strings.Repeat("m", 129), " leading", "trailing ", "line\nbreak", "carriage\rreturn", "null\x00byte"} {
		factoryHTTP(t, s, "POST", "/api/v1/jobs", map[string]string{"repository": "repo", "recipe": "single", "spec": "Task", "model": model}, 400)
	}
	if len(s.tasks) != 0 {
		t.Fatal("invalid models created tasks")
	}
	factoryHTTP(t, s, "POST", "/api/v1/jobs", map[string]string{"repository": "repo", "recipe": "single", "spec": "Task", "model": ""}, 201)
}

func TestLocalAgentsRejectLegacyPipeline(t *testing.T) {
	for _, agent := range []string{"opencode", "hermes"} {
		for _, recipe := range []string{"", "factory", "single", "native-v1"} {
			err := validateProjectDefaults(projectDefaults{Recipe: recipe}, setupSelection{Agent: agent, Host: "local"})
			if (err != nil) != (recipe == "factory") {
				t.Fatalf("%s %s: %v", agent, recipe, err)
			}
		}
	}
}

func TestLocalAgentDefaultSubmissionAndDispatch(t *testing.T) {
	for agent, adapter := range map[string]string{"opencode": "opencode-json", "hermes": "hermes-stream"} {
		t.Run(agent, func(t *testing.T) {
			s := githubFactoryTestServer(t, newMemoryFactoryGitHub())
			file, err := os.OpenFile(s.definitionPath, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			_, err = file.WriteString("\n[commands.primary]\nexecutor='codex-native'\ntimeout='1m'\n")
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
			s.setup = &onboarding{server: s, settings: setupSettings{Finished: true, Selection: setupSelection{Agent: agent, Host: "local"}}}
			factoryHTTP(t, s, "POST", "/api/v1/jobs", map[string]string{"repository": "repo", "spec": "Answer", "model": "provider/model"}, 201)
			poll := protocol.PollRequest{NativeAgents: true, Factory: true, Workflows: true, Reviews: true, Executors: []string{agent + "-native"}, Repositories: []string{"repo"}, Adapters: map[string]string{agent + "-native": adapter}, Models: map[string][]string{agent + "-native": {}}, InstanceID: "worker-instance", Name: "worker"}
			raw := factoryHTTP(t, s, "POST", "/api/v1/workers/poll", poll, 200)
			var response protocol.PollResponse
			if err := json.Unmarshal(raw, &response); err != nil {
				t.Fatal(err)
			}
			if response.Run == nil || response.Run.Executor != agent+"-native" || response.Run.Model != "provider/model" || response.Run.Factory.Recipe != "native-v1" {
				t.Fatalf("bad local dispatch: %s", raw)
			}
			factoryFinish(t, s, response.Run, protocol.FactoryResult{Report: "Answered"})
		})
	}
}
