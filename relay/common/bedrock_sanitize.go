package common

// SanitizeBedrockMessages removes thinking blocks with signatures from assistant messages
// to prevent "Invalid signature in thinking block" errors from Bedrock.
// It also removes empty text content blocks to prevent "text content blocks must be non-empty" errors.
func SanitizeBedrockMessages(messages []interface{}) []interface{} {
	for i, msg := range messages {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)
		if role != "assistant" {
			// For non-assistant messages, just remove empty text blocks
			content, ok := msgMap["content"].([]interface{})
			if !ok {
				// Check if content is empty string
				if strContent, ok := msgMap["content"].(string); ok && strContent == "" {
					msgMap["content"] = " "
					messages[i] = msgMap
				}
				continue
			}
			filtered := removeEmptyTextBlocks(content)
			if len(filtered) == 0 && len(content) > 0 {
				// If all blocks were empty, keep a single space text block
				filtered = []interface{}{map[string]interface{}{"type": "text", "text": " "}}
			}
			msgMap["content"] = filtered
			messages[i] = msgMap
			continue
		}

		content, ok := msgMap["content"].([]interface{})
		if !ok {
			continue
		}

		filtered := make([]interface{}, 0, len(content))
		for _, block := range content {
			blockMap, ok := block.(map[string]interface{})
			if !ok {
				filtered = append(filtered, block)
				continue
			}

			blockType, _ := blockMap["type"].(string)

			// Remove thinking blocks with signature (they cause validation errors on Bedrock)
			if blockType == "thinking" {
				if _, hasSignature := blockMap["signature"]; hasSignature {
					continue // skip this block
				}
			}

			// Remove empty text blocks
			if blockType == "text" {
				text, _ := blockMap["text"].(string)
				if text == "" {
					continue
				}
			}

			filtered = append(filtered, block)
		}

		// Ensure assistant message has at least one content block
		if len(filtered) == 0 {
			filtered = []interface{}{map[string]interface{}{"type": "text", "text": " "}}
		}

		msgMap["content"] = filtered
		messages[i] = msgMap
	}
	return messages
}

func removeEmptyTextBlocks(content []interface{}) []interface{} {
	filtered := make([]interface{}, 0, len(content))
	for _, block := range content {
		blockMap, ok := block.(map[string]interface{})
		if !ok {
			filtered = append(filtered, block)
			continue
		}
		blockType, _ := blockMap["type"].(string)
		if blockType == "text" {
			text, _ := blockMap["text"].(string)
			if text == "" {
				continue
			}
		}
		filtered = append(filtered, block)
	}
	return filtered
}

// ShouldRemoveTemperature returns true for models that reject the temperature parameter.
// Currently Claude Opus 4.7+ and models with thinking/adaptive mode reject temperature.
func ShouldRemoveTemperature(model string) bool {
	// Models that deprecate temperature
	deprecatedModels := []string{
		"claude-opus-4-7",
		"claude-opus-4-6",
		"anthropic.claude-opus-4-7",
		"anthropic.claude-opus-4-6",
	}
	for _, prefix := range deprecatedModels {
		if len(model) >= len(prefix) && model[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
