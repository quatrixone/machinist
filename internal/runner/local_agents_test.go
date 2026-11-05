package runner

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/owainlewis/machinist/internal/config"
	"github.com/owainlewis/machinist/internal/protocol"
)

func TestLocalAgentNativeDelivery(t *testing.T) {
	for _, adapter := range []string{"opencode-json", "hermes-stream"} {
		t.Run(adapter, func(t *testing.T) {
			dir := t.TempDir()
			promptPath := filepath.Join(dir, "prompt")
			script := filepath.Join(dir, "agent.sh")
			t.Setenv("TEST_API_KEY", "configured")
			t.Setenv("PROMPT_PATH", promptPath)
			output := `{"type":"text","part":{"messageID":"primary","text":"{\"outcome\":\"complete\",\"summary\":\"Answered\"}"}}
{"type":"step_finish","part":{"reason":"stop","tokens":{"input":5,"output":2,"reasoning":1,"cache":{"read":3,"write":0}}}}`
			if adapter == "hermes-stream" {
				output = `{"type":"text","text":"Interim"}
{"type":"result","exit_code":0,"text":"{\"outcome\":\"complete\",\"summary\":\"Answered\"}","tokens":{"total":11}}`
			}
			body := "test \"$1\" = --model && test \"$2\" = provider/model || exit 7\ntest \"$TEST_API_KEY\" = configured || exit 8\ncat >\"$PROMPT_PATH\"\ncat <<'RESULT'\n" + output + "\nRESULT\n"
			if err := os.WriteFile(script, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			result, err := Execute(t.Context(), Options{Repository: newGitRepository(t), DataDirectory: dir, Workflow: true,
				Factory: &protocol.FactoryContext{Recipe: "native-v1", Role: "primary"},
				Command: config.ResolvedCommand{Name: "primary", Adapter: adapter, Command: []string{"sh", script}, Model: "provider/model", Prompt: "literal $(echo unsafe) `echo unsafe`", Timeout: 5 * time.Second},
				Stdout:  io.Discard, Stderr: io.Discard})
			if err != nil {
				t.Fatal(err)
			}
			if result.StepResult == nil || result.StepResult.Summary != "Answered" || result.TokenUsage == nil || *result.TokenUsage != 11 {
				t.Fatalf("unexpected result: %+v", result)
			}
			prompt, err := os.ReadFile(promptPath)
			if err != nil || !strings.Contains(string(prompt), "literal $(echo unsafe) `echo unsafe`") {
				t.Fatalf("prompt altered: %q (%v)", prompt, err)
			}
		})
	}
}

func TestLocalAgentCompletionAndErrors(t *testing.T) {
	for _, test := range []struct {
		adapter, event  string
		fails, complete bool
	}{
		{"opencode-json", `{"type":"step_finish","part":{"reason":"tool-calls"}}`, false, false},
		{"opencode-json", `{"type":"error","error":{"message":"failed"}}`, true, false},
		{"opencode-json", `{"type":"future_event"}`, false, false},
		{"hermes-stream", `{"type":"result","exit_code":1,"text":"partial"}`, true, false},
		{"hermes-stream", `{"type":"result","text":"missing status"}`, true, false},
		{"hermes-stream", `{"type":"result","exit_code":0,"text":"answer"}`, false, true},
	} {
		n, _, _ := testNative(t, test.adapter)
		_, err := n.Write([]byte(test.event + "\n"))
		if (err != nil) != test.fails || n.completed != test.complete {
			t.Fatalf("%s: completed=%v err=%v", test.event, n.completed, err)
		}
	}
}
