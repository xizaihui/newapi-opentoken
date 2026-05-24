package claude

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

func ptr(s string) *string { return &s }
func iptr(i int) *int      { return &i }

func TestValidate_Opus47Enabled_Rejected(t *testing.T) {
	cases := []struct {
		name  string
		model string
	}{
		{"plain", "claude-opus-4-7"},
		{"versioned", "claude-opus-4-7-20251114"},
		{"effort-high", "claude-opus-4-7-effort-high"},
		{"effort-medium", "claude-opus-4-7-effort-medium"},
		{"thinking-suffix", "claude-opus-4-7-thinking"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &dto.ClaudeRequest{
				Model: tc.model,
				Thinking: &dto.Thinking{
					Type:         "enabled",
					BudgetTokens: iptr(1500),
				},
			}
			err := ValidateOpus47Thinking(req)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.model)
			}
			if !strings.Contains(err.Error(), "not supported") {
				t.Errorf("error message missing 'not supported': %v", err)
			}
			if !strings.Contains(err.Error(), tc.model) {
				t.Errorf("error should include model name, got: %v", err)
			}
		})
	}
}

func TestValidate_Opus47Adaptive_Passes(t *testing.T) {
	req := &dto.ClaudeRequest{
		Model: "claude-opus-4-7",
		Thinking: &dto.Thinking{
			Type: "adaptive",
		},
	}
	if err := ValidateOpus47Thinking(req); err != nil {
		t.Errorf("adaptive should pass, got: %v", err)
	}
}

func TestValidate_Opus47NoThinking_Passes(t *testing.T) {
	req := &dto.ClaudeRequest{Model: "claude-opus-4-7"}
	if err := ValidateOpus47Thinking(req); err != nil {
		t.Errorf("no thinking should pass, got: %v", err)
	}
}

func TestValidate_OtherModels_Enabled_Passes(t *testing.T) {
	// 这些模型在上游真实支持 enabled，不能误拦
	models := []string{
		"claude-opus-4-6",
		"claude-opus-4-5",
		"claude-opus-4",
		"claude-sonnet-4-7",
		"claude-sonnet-4-6",
		"claude-sonnet-4-5",
		"claude-haiku-4-5", // haiku 由 adaptor 单独处理，validate 这里放行
	}
	for _, m := range models {
		t.Run(m, func(t *testing.T) {
			req := &dto.ClaudeRequest{
				Model: m,
				Thinking: &dto.Thinking{
					Type:         "enabled",
					BudgetTokens: iptr(1500),
				},
			}
			if err := ValidateOpus47Thinking(req); err != nil {
				t.Errorf("model %s + enabled should pass, got: %v", m, err)
			}
		})
	}
}

func TestValidate_NilRequest_Passes(t *testing.T) {
	if err := ValidateOpus47Thinking(nil); err != nil {
		t.Errorf("nil request should pass, got: %v", err)
	}
}

func TestValidate_NilThinking_Passes(t *testing.T) {
	req := &dto.ClaudeRequest{Model: "claude-opus-4-7"}
	if err := ValidateOpus47Thinking(req); err != nil {
		t.Errorf("nil thinking should pass, got: %v", err)
	}
}

func TestValidate_EmptyType_Passes(t *testing.T) {
	// thinking.type 为空字符串 — 客户端可能发了 thinking:{} 但没设 type
	req := &dto.ClaudeRequest{
		Model: "claude-opus-4-7",
		Thinking: &dto.Thinking{
			Type: "",
		},
	}
	if err := ValidateOpus47Thinking(req); err != nil {
		t.Errorf("empty type should pass (no validation), got: %v", err)
	}
}

func TestValidate_Disabled_Passes(t *testing.T) {
	// 防御性：有客户端可能发 type="disabled" 试图关闭
	req := &dto.ClaudeRequest{
		Model:    "claude-opus-4-7",
		Thinking: &dto.Thinking{Type: "disabled"},
	}
	if err := ValidateOpus47Thinking(req); err != nil {
		t.Errorf("disabled should pass, got: %v", err)
	}
}

// silence unused
var _ = ptr
