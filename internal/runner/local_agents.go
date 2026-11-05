package runner

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Both CLIs read the rendered prompt from stdin and emit JSONL. Unknown events
// stay in the raw spool; only explicit primary completion establishes success.
func (n *nativeStream) localAgentLine(event map[string]json.RawMessage) error {
	var kind string
	_ = json.Unmarshal(event["type"], &kind)
	if n.adapter == "hermes-stream" {
		if kind != "result" {
			return nil
		}
		var result struct {
			ExitCode *int   `json:"exit_code"`
			Text     string `json:"text"`
			Error    string `json:"error"`
			Tokens   struct {
				Total int64 `json:"total"`
			} `json:"tokens"`
		}
		body, _ := json.Marshal(event)
		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Errorf("invalid Hermes result: %w", err)
		}
		if result.ExitCode == nil || *result.ExitCode != 0 || result.Error != "" {
			return errors.New("Hermes primary did not complete successfully")
		}
		n.final = truncateNative(result.Text, maxFinalMessageBytes)
		n.completed = true
		n.usage = &result.Tokens.Total
		return n.message(n.final, "", true)
	}
	if kind == "error" {
		return errors.New("OpenCode primary reported an error; see raw output")
	}
	var part struct {
		MessageID string `json:"messageID"`
		Text      string `json:"text"`
		Reason    string `json:"reason"`
		Tokens    struct {
			Input, Output, Reasoning int64
			Cache                    struct{ Read, Write int64 }
		} `json:"tokens"`
	}
	if err := json.Unmarshal(event["part"], &part); err != nil {
		return nil
	}
	switch kind {
	case "text":
		if n.turn != part.MessageID {
			n.turn, n.final = part.MessageID, ""
		}
		n.final = truncateNative(n.final+part.Text, maxFinalMessageBytes)
		return n.message(part.Text, "", true)
	case "step_start":
		n.completed = false
		if n.turn != part.MessageID {
			n.turn, n.final = part.MessageID, ""
		}
	case "step_finish":
		if n.usage == nil {
			n.usage = new(int64)
		}
		*n.usage += part.Tokens.Input + part.Tokens.Output + part.Tokens.Reasoning + part.Tokens.Cache.Read + part.Tokens.Cache.Write
		n.completed = part.Reason == "stop"
	}
	return nil
}
