/*
检测请求体里的 Anthropic prompt-caching 标记 (cache_control).

为什么需要：newapi 日志层只看上游返回的 cache_creation_tokens (W) / cache_tokens (R)，
无法区分以下两种 W=0 R=0 情况：
  1. 客户端根本没声明 cache_control
  2. 客户端声明了 cache_control 但 prefix < 1024 token 被 Anthropic 静默忽略

本模块在请求进入 newapi adapter 时扫描请求体（结构化 struct），把 cache_control
出现的位置和数量提取出来，塞进 RelayInfo.Other map，最终写入 logs.other 字段。

扫描点（按 Anthropic API 文档）：
  - system 块: string | []content_block ；每个 block 可带 cache_control
  - messages[].content[]: text / image / tool_use / tool_result 块；每个可带 cache_control
  - tools[]: 每个工具定义可带 cache_control

输出（设入 RelayInfo.Other）：
  cache_control_declared      bool   — 是否任意位置有 cache_control 标记
  cache_control_marks         int    — 标记总数
  cache_control_in_system     int    — system 块里的标记数
  cache_control_in_messages   int    — messages 内容块里的标记数
  cache_control_in_tools      int    — tools 块里的标记数
*/
package service

import (
	"github.com/QuantumNous/new-api/dto"
)

// CacheControlScan 表示扫描结果。零值 = 没声明。
type CacheControlScan struct {
	Declared      bool
	TotalMarks    int
	InSystem      int
	InMessages    int
	InTools       int
	// Thinking 相关 — 用于诊断 "客户端发了 thinking.enabled 但模型不支持" 等问题
	ThinkingType   string // "" | "enabled" | "adaptive" | other
	ThinkingBudget int    // budget_tokens (0 if not set)
}

// Has reports whether anything was declared.
func (s CacheControlScan) Has() bool { return s.Declared }

// hasCacheControlMarker checks an arbitrary map[string]any for a non-nil cache_control field.
// Anthropic only treats cache_control as active when it's a JSON object (not null/false).
func hasCacheControlMarker(m map[string]any) bool {
	v, ok := m["cache_control"]
	if !ok || v == nil {
		return false
	}
	// Accept either {"type":"ephemeral"} or any non-empty object
	if mm, isMap := v.(map[string]any); isMap {
		return len(mm) > 0
	}
	// be lenient — non-object truthy values also count
	return true
}

// scanContentSlice walks a JSON-decoded content array (e.g. messages[].content
// or system as block array) and counts cache_control markers on each block.
func scanContentSlice(arr []any) int {
	count := 0
	for _, item := range arr {
		blk, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if hasCacheControlMarker(blk) {
			count++
		}
	}
	return count
}

// scanSystem handles ClaudeRequest.System which can be string | []block.
func scanSystem(system any) int {
	if system == nil {
		return 0
	}
	switch v := system.(type) {
	case []any:
		return scanContentSlice(v)
	case []map[string]any:
		// rare but possible if pre-decoded
		count := 0
		for _, m := range v {
			if hasCacheControlMarker(m) {
				count++
			}
		}
		return count
	}
	return 0
}

// scanTools handles ClaudeRequest.Tools / OpenAI tools which are usually []any
// after JSON decode, with each tool being a map.
func scanTools(tools any) int {
	if tools == nil {
		return 0
	}
	switch v := tools.(type) {
	case []any:
		count := 0
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if hasCacheControlMarker(m) {
					count++
				}
			}
		}
		return count
	}
	return 0
}

// scanClaudeMessages walks ClaudeRequest.Messages slice and counts content
// blocks that carry cache_control.
func scanClaudeMessages(msgs []dto.ClaudeMessage) int {
	count := 0
	for _, m := range msgs {
		if m.Content == nil {
			continue
		}
		switch v := m.Content.(type) {
		case []any:
			count += scanContentSlice(v)
		case []map[string]any:
			for _, blk := range v {
				if hasCacheControlMarker(blk) {
					count++
				}
			}
		}
	}
	return count
}

// scanOpenAIMessages walks OpenAI-style messages and counts content blocks
// that carry cache_control. Uses two strategies:
//
//  1. ParseContent() returns []MediaContent which has a typed CacheControl
//     json.RawMessage field — fast and reliable for the common path.
//  2. Fallback to raw any-based scan in case ParseContent() drops blocks of
//     unknown content type that still carry cache_control.
func scanOpenAIMessages(msgs []dto.Message) int {
	count := 0
	for i := range msgs {
		m := &msgs[i]
		parsed := m.ParseContent()
		parsedHasCC := false
		for _, blk := range parsed {
			if len(blk.CacheControl) > 0 && string(blk.CacheControl) != "null" {
				count++
				parsedHasCC = true
			}
		}
		// Fallback raw scan only when ParseContent saw nothing — handles
		// non-standard / pre-decoded block layouts.
		if !parsedHasCC {
			if arr, ok := m.Content.([]any); ok {
				count += scanContentSlice(arr)
			}
		}
	}
	return count
}

// DetectClaudeCacheControl scans an Anthropic-style request for cache_control markers.
// Safe on nil — returns zero value.
func DetectClaudeCacheControl(req *dto.ClaudeRequest) CacheControlScan {
	var s CacheControlScan
	if req == nil {
		return s
	}
	s.InSystem = scanSystem(req.System)
	s.InMessages = scanClaudeMessages(req.Messages)
	s.InTools = scanTools(req.Tools)
	s.TotalMarks = s.InSystem + s.InMessages + s.InTools
	s.Declared = s.TotalMarks > 0
	if req.Thinking != nil {
		s.ThinkingType = req.Thinking.Type
		if req.Thinking.BudgetTokens != nil {
			s.ThinkingBudget = *req.Thinking.BudgetTokens
		}
	}
	return s
}

// DetectOpenAICacheControl scans an OpenAI-compat request for cache_control
// markers (clients sometimes pass them through in content blocks when target
// channel is Anthropic).
func DetectOpenAICacheControl(req *dto.GeneralOpenAIRequest) CacheControlScan {
	var s CacheControlScan
	if req == nil {
		return s
	}
	s.InMessages = scanOpenAIMessages(req.Messages)
	s.InTools = scanTools(req.Tools)
	s.TotalMarks = s.InMessages + s.InTools
	s.Declared = s.TotalMarks > 0
	return s
}

// MergeIntoOther writes the scan result into a map suitable for logs.other.
// Only writes when something was declared, to avoid bloating untouched logs.
func (s CacheControlScan) MergeIntoOther(other map[string]any) {
	if s.Declared {
		other["cache_control_declared"] = true
		other["cache_control_marks"] = s.TotalMarks
		if s.InSystem > 0 {
			other["cache_control_in_system"] = s.InSystem
		}
		if s.InMessages > 0 {
			other["cache_control_in_messages"] = s.InMessages
		}
		if s.InTools > 0 {
			other["cache_control_in_tools"] = s.InTools
		}
	} else {
		// still record explicit "not declared" so D bucket is splittable in queries
		other["cache_control_declared"] = false
	}
	// Always record thinking config if present (small overhead, useful for diagnostics)
	if s.ThinkingType != "" {
		other["thinking_type"] = s.ThinkingType
		if s.ThinkingBudget > 0 {
			other["thinking_budget"] = s.ThinkingBudget
		}
	}
}
