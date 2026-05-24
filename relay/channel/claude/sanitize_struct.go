/*
Sanitize Claude *converted* (struct) request before sending to Anthropic.

针对从 OpenAI 格式转换过来的 ClaudeRequest，根据模型能力矩阵剥离/转换不兼容字段。
与 sanitize_passthrough.go 互补 — 那个改 JSON bytes，这个改 struct。

数据驱动：模型能力矩阵从 fork 部署 2026-05-22 起 1005 条 type=5 错误日志归纳：
  - claude-opus-4-7*:   拒绝 temperature/top_p/top_k（172+14=186 条），
                       拒绝 context_management 字段（102 条），
                       拒绝 thinking.type=enabled（6 条），要 adaptive
  - claude-sonnet-4-6*: temperature/top_p 必须二选一（170 条）
  - claude-haiku-4-5*:  保护性同上 + adaptive thinking 不支持
*/
package claude

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

// SanitizeClaudeStruct mutates req in place to ensure compatibility with
// Anthropic Messages API. Safe to call multiple times. Fails open: never panics.
//
// 调用点：relay-claude.go 在构造完 claudeRequest 准备 marshal 前。
//
// Returns true if any change was applied (for diagnostic logging).
func SanitizeClaudeStruct(req *dto.ClaudeRequest) bool {
	if req == nil {
		return false
	}
	changed := false
	model := req.Model

	// --- Opus 4.7 严格模式 -------------------------------------------------
	// 拒绝任何 sampling 参数，所有调温必须通过 thinking.effort 实现。
	if strings.HasPrefix(model, "claude-opus-4-7") {
		if req.Temperature != nil {
			req.Temperature = nil
			changed = true
		}
		if req.TopP != nil {
			req.TopP = nil
			changed = true
		}
		if req.TopK != nil {
			req.TopK = nil
			changed = true
		}
		// Opus 4.7 不识别 context_management 字段（来自 OpenAI codex CLI 等）
		if len(req.ContextManagement) > 0 {
			req.ContextManagement = nil
			changed = true
		}
		// thinking.enabled → adaptive（4.7 要求 adaptive）
		if req.Thinking != nil && req.Thinking.Type == "enabled" {
			req.Thinking.Type = "adaptive"
			if req.Thinking.Display == "" {
				req.Thinking.Display = "summarized"
			}
			// adaptive 不能设 BudgetTokens
			req.Thinking.BudgetTokens = nil
			changed = true
		}
	}

	// --- Sonnet 4.6 / Haiku 4.5 / Sonnet 4.7 等：temp 和 top_p 互斥 ----------
	// 这些模型如果同时设了两个会 400。保留 temperature（用户更常用更直觉）。
	if strings.HasPrefix(model, "claude-sonnet-4-6") ||
		strings.HasPrefix(model, "claude-sonnet-4-7") ||
		strings.HasPrefix(model, "claude-haiku-4-5") {
		if req.Temperature != nil && req.TopP != nil {
			req.TopP = nil
			changed = true
		}
	}

	// --- Haiku 不支持 adaptive thinking ------------------------------------
	// 双重保险（sanitize_passthrough.go 已经在 raw body 层处理过）
	if strings.Contains(model, "haiku") {
		if req.Thinking != nil && req.Thinking.Type == "adaptive" {
			req.Thinking = nil
			req.OutputConfig = nil
			changed = true
		}
	}

	if changed {
		common.SysLog("[claude-sanitize] (struct) applied compatibility fixes for model=" + model)
	}
	return changed
}
