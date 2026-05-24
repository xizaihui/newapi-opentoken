package claude

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

// ValidateOpus47Thinking 检测 Claude Opus 4.7 系列模型 + thinking.type="enabled" 的非法组合。
//
// 背景：Anthropic Opus 4.7 已不再支持 "enabled" 形式的 extended thinking 配置，
// 而是改为 thinking.type="adaptive" + thinking.display="summarized"。
// 上游对此变更采取了"接受但忽略"的兼容策略：客户端发 enabled 请求时上游返回 200，
// 但响应中不会出现 thinking content block，导致客户端付钱却拿不到 thinking 能力。
//
// 这个函数明确返回 error，由调用方转换成 400 让客户端立即修正调用方式。
//
// 触发条件（全部满足才返错）：
//  1. model 以 "claude-opus-4-7" 开头（包含 -effort/-thinking 后缀变体）
//  2. request.Thinking 字段存在
//  3. thinking.type == "enabled"
//
// 不触发的场景（白名单透传）：
//  - 其他 Opus 系列（4/4.5/4.6）+ enabled — 这些上游真实支持
//  - Sonnet 任何版本 + enabled — 真实支持
//  - Haiku 系列 — 由 adaptor.go 的 fixUnsupportedThinking 处理
//  - Opus 4.7 + thinking.type="adaptive" — 正确格式
//  - Opus 4.7 不带 thinking 字段
//
// 错误消息注意事项：
//   newapi 的 MaskSensitiveInfo 会用域名正则把 "thinking.type" 这种 X.Y 形态当成
//   subdomain 脱敏成 "***"。错误消息里所有形如 X.Y 的字段引用都改成 backtick + 方括号
//   语法（`thinking[type]=...`），避免脱敏破坏可读性。
func ValidateOpus47Thinking(req *dto.ClaudeRequest) error {
	if req == nil || req.Thinking == nil {
		return nil
	}
	if !strings.HasPrefix(req.Model, "claude-opus-4-7") {
		return nil
	}
	if req.Thinking.Type != "enabled" {
		return nil
	}
	return fmt.Errorf(
		"`thinking[type]=enabled` is not supported on model %s. "+
			"Use `thinking[type]=adaptive` together with `thinking[display]=summarized`, "+
			"or change the model name to append suffix `-effort-high`/`-effort-medium`/`-effort-low` "+
			"so the gateway converts it for you. "+
			"Background: Anthropic silently accepts the legacy enabled form but does not "+
			"actually invoke extended thinking on this model, so clients pay without getting the feature.",
		req.Model,
	)
}
