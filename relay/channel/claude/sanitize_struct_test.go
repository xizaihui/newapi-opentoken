package claude

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

func ptrFloat(v float64) *float64 { return &v }
func ptrInt(v int) *int           { return &v }

func TestSanitize_Opus47_StripsTempTopPTopK(t *testing.T) {
	req := &dto.ClaudeRequest{
		Model:       "claude-opus-4-7-20260101",
		Temperature: ptrFloat(0.7),
		TopP:        ptrFloat(0.9),
		TopK:        ptrInt(40),
	}
	changed := SanitizeClaudeStruct(req)
	if !changed {
		t.Fatal("expected changed=true")
	}
	if req.Temperature != nil || req.TopP != nil || req.TopK != nil {
		t.Errorf("expected all sampling params nil, got temp=%v top_p=%v top_k=%v",
			req.Temperature, req.TopP, req.TopK)
	}
}

func TestSanitize_Opus47_StripsContextManagement(t *testing.T) {
	req := &dto.ClaudeRequest{
		Model:             "claude-opus-4-7",
		ContextManagement: []byte(`{"compaction":"enabled"}`),
	}
	if !SanitizeClaudeStruct(req) {
		t.Fatal("expected changed=true")
	}
	if req.ContextManagement != nil {
		t.Errorf("expected context_management nil, got %s", string(req.ContextManagement))
	}
}

func TestSanitize_Opus47_ThinkingEnabledToAdaptive(t *testing.T) {
	req := &dto.ClaudeRequest{
		Model: "claude-opus-4-7",
		Thinking: &dto.Thinking{
			Type:         "enabled",
			BudgetTokens: ptrInt(2000),
		},
	}
	if !SanitizeClaudeStruct(req) {
		t.Fatal("expected changed=true")
	}
	if req.Thinking.Type != "adaptive" {
		t.Errorf("expected adaptive, got %s", req.Thinking.Type)
	}
	if req.Thinking.BudgetTokens != nil {
		t.Errorf("expected budget_tokens nil, got %d", *req.Thinking.BudgetTokens)
	}
	if req.Thinking.Display != "summarized" {
		t.Errorf("expected display=summarized, got %s", req.Thinking.Display)
	}
}

func TestSanitize_Sonnet46_DropsTopPWhenTempPresent(t *testing.T) {
	req := &dto.ClaudeRequest{
		Model:       "claude-sonnet-4-6-20251015",
		Temperature: ptrFloat(0.5),
		TopP:        ptrFloat(0.9),
	}
	if !SanitizeClaudeStruct(req) {
		t.Fatal("expected changed=true")
	}
	if req.Temperature == nil {
		t.Error("expected temperature preserved")
	}
	if req.TopP != nil {
		t.Errorf("expected top_p nil, got %v", *req.TopP)
	}
}

func TestSanitize_Sonnet46_KeepsTopPWhenAlone(t *testing.T) {
	req := &dto.ClaudeRequest{
		Model: "claude-sonnet-4-6",
		TopP:  ptrFloat(0.9),
	}
	if SanitizeClaudeStruct(req) {
		t.Error("expected no change (top_p alone is fine)")
	}
	if req.TopP == nil {
		t.Error("expected top_p preserved")
	}
}

func TestSanitize_HaikuStripsAdaptiveThinking(t *testing.T) {
	req := &dto.ClaudeRequest{
		Model: "claude-haiku-4-5-20251001",
		Thinking: &dto.Thinking{
			Type: "adaptive",
		},
		OutputConfig: []byte(`{"effort":"high"}`),
	}
	if !SanitizeClaudeStruct(req) {
		t.Fatal("expected changed=true")
	}
	if req.Thinking != nil {
		t.Error("expected thinking stripped on haiku")
	}
	if req.OutputConfig != nil {
		t.Error("expected output_config stripped on haiku")
	}
}

func TestSanitize_OtherModelsUntouched(t *testing.T) {
	req := &dto.ClaudeRequest{
		Model:       "claude-sonnet-4-5",
		Temperature: ptrFloat(0.5),
		TopP:        ptrFloat(0.9),
	}
	if SanitizeClaudeStruct(req) {
		t.Error("expected no change for sonnet-4-5")
	}
}

func TestSanitize_NilSafe(t *testing.T) {
	if SanitizeClaudeStruct(nil) {
		t.Error("nil should return false")
	}
}
