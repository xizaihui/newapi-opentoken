package claude

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func TestSanitize_DropsTopPWhenTemperatureSetOnSonnet46(t *testing.T) {
	temp := 0.7
	topP := 0.9
	req := &dto.ClaudeRequest{
		Model:       "claude-sonnet-4-6",
		Temperature: &temp,
		TopP:        &topP,
	}
	SanitizeRequestForAnthropic(req)
	if req.Temperature == nil {
		t.Fatal("temperature should be preserved")
	}
	if req.TopP != nil {
		t.Fatal("top_p should be dropped when temperature is also set on sonnet-4-6")
	}
}

func TestSanitize_KeepsTopPWhenTemperatureMissing(t *testing.T) {
	topP := 0.9
	req := &dto.ClaudeRequest{
		Model: "claude-sonnet-4-6",
		TopP:  &topP,
	}
	SanitizeRequestForAnthropic(req)
	if req.TopP == nil {
		t.Fatal("top_p should be preserved when temperature is absent")
	}
}

func TestSanitize_KeepsBothOnUnaffectedModel(t *testing.T) {
	temp := 0.7
	topP := 0.9
	req := &dto.ClaudeRequest{
		Model:       "claude-haiku-4-5-20251001",
		Temperature: &temp,
		TopP:        &topP,
	}
	SanitizeRequestForAnthropic(req)
	if req.Temperature == nil || req.TopP == nil {
		t.Fatal("haiku-4-5 should accept both temperature and top_p")
	}
}

func TestSanitize_RewritesEnabledThinkingOnOpus47(t *testing.T) {
	budget := 4096
	req := &dto.ClaudeRequest{
		Model: "claude-opus-4-7",
		Thinking: &dto.Thinking{
			Type:         "enabled",
			BudgetTokens: &budget,
		},
	}
	SanitizeRequestForAnthropic(req)
	if req.Thinking == nil {
		t.Fatal("thinking should not be removed")
	}
	if req.Thinking.Type != "adaptive" {
		t.Fatalf("thinking.type should be adaptive, got %q", req.Thinking.Type)
	}
	if req.Thinking.Display != "summarized" {
		t.Fatalf("thinking.display should be summarized, got %q", req.Thinking.Display)
	}
	if len(req.OutputConfig) == 0 {
		t.Fatal("output_config should be populated")
	}
	var oc map[string]string
	if err := json.Unmarshal(req.OutputConfig, &oc); err != nil {
		t.Fatalf("output_config should be valid JSON: %v", err)
	}
	if oc["effort"] != "medium" {
		t.Fatalf("expected effort=medium for budget 4096, got %q", oc["effort"])
	}
}

func TestSanitize_LeavesEnabledThinkingOnOpus46(t *testing.T) {
	budget := 4096
	req := &dto.ClaudeRequest{
		Model: "claude-opus-4-6",
		Thinking: &dto.Thinking{
			Type:         "enabled",
			BudgetTokens: &budget,
		},
	}
	SanitizeRequestForAnthropic(req)
	if req.Thinking == nil || req.Thinking.Type != "enabled" {
		t.Fatal("opus-4-6 still accepts thinking.type=enabled")
	}
}

func TestSanitize_DropsAssistantThinkingWithMissingSignature(t *testing.T) {
	req := &dto.ClaudeRequest{
		Model: "claude-opus-4-7",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: []any{
				map[string]any{
					"type":     "thinking",
					"thinking": "let me think",
					// no signature
				},
				map[string]any{
					"type": "text",
					"text": "hello",
				},
			}},
		},
	}
	SanitizeRequestForAnthropic(req)

	arr, ok := req.Messages[1].Content.([]any)
	if !ok {
		t.Fatalf("expected content to remain []any, got %T", req.Messages[1].Content)
	}
	if len(arr) != 1 {
		t.Fatalf("expected 1 block remaining, got %d", len(arr))
	}
	block := arr[0].(map[string]any)
	if block["type"] != "text" {
		t.Fatalf("expected text block remaining, got %v", block)
	}
}

func TestSanitize_DropsAssistantThinkingWithNullSignature(t *testing.T) {
	req := &dto.ClaudeRequest{
		Messages: []dto.ClaudeMessage{
			{Role: "assistant", Content: []any{
				map[string]any{
					"type":      "thinking",
					"thinking":  "...",
					"signature": nil,
				},
				map[string]any{"type": "text", "text": "answer"},
			}},
		},
	}
	SanitizeRequestForAnthropic(req)
	arr := req.Messages[0].Content.([]any)
	if len(arr) != 1 || arr[0].(map[string]any)["type"] != "text" {
		t.Fatalf("expected only text block, got %#v", arr)
	}
}

func TestSanitize_KeepsAssistantThinkingWithValidSignature(t *testing.T) {
	req := &dto.ClaudeRequest{
		Messages: []dto.ClaudeMessage{
			{Role: "assistant", Content: []any{
				map[string]any{
					"type":      "thinking",
					"thinking":  "reasoned",
					"signature": "ErcBCkgIBxgC...",
				},
				map[string]any{"type": "text", "text": "answer"},
			}},
		},
	}
	SanitizeRequestForAnthropic(req)
	arr := req.Messages[0].Content.([]any)
	if len(arr) != 2 {
		t.Fatalf("expected both blocks preserved, got %d", len(arr))
	}
}

func TestSanitize_DoesNotTouchUserMessages(t *testing.T) {
	req := &dto.ClaudeRequest{
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: []any{
				map[string]any{
					"type":     "thinking",
					"thinking": "x",
				},
			}},
		},
	}
	SanitizeRequestForAnthropic(req)
	arr := req.Messages[0].Content.([]any)
	if len(arr) != 1 {
		t.Fatalf("user content should be untouched: %#v", arr)
	}
}

func TestSanitize_ReplacesEmptiedAssistantContentWithPlaceholder(t *testing.T) {
	req := &dto.ClaudeRequest{
		Messages: []dto.ClaudeMessage{
			{Role: "assistant", Content: []any{
				map[string]any{
					"type":     "thinking",
					"thinking": "x",
				},
			}},
		},
	}
	SanitizeRequestForAnthropic(req)
	arr := req.Messages[0].Content.([]any)
	if len(arr) != 1 {
		t.Fatalf("expected placeholder, got %d blocks", len(arr))
	}
	if arr[0].(map[string]any)["type"] != "text" {
		t.Fatalf("expected text placeholder, got %#v", arr[0])
	}
}

func TestSanitize_RoundTripsViaJsonAfterDeepCopy(t *testing.T) {
	temp := 0.7
	topP := 0.9
	src := &dto.ClaudeRequest{
		Model:       "claude-sonnet-4-6",
		Temperature: &temp,
		TopP:        &topP,
	}
	dst, err := common.DeepCopy(src)
	if err != nil {
		t.Fatalf("DeepCopy failed: %v", err)
	}
	SanitizeRequestForAnthropic(dst)
	if dst.TopP != nil {
		t.Fatal("expected sanitized copy to drop top_p")
	}
	// Source must not be mutated.
	if src.TopP == nil {
		t.Fatal("source request must remain untouched")
	}
}
