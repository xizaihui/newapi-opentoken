package common

import (
	"encoding/base64"
	"fmt"
	"strings"
)

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

			// Handle document blocks: Bedrock only accepts application/pdf.
			// Convert text/* documents to inline text blocks; drop unsupported formats.
			if blockType == "document" {
				converted, drop := convertDocumentForBedrock(blockMap)
				if drop {
					continue
				}
				if converted != nil {
					filtered = append(filtered, converted)
					continue
				}
				// else: keep original (application/pdf)
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
		if blockType == "document" {
			converted, drop := convertDocumentForBedrock(blockMap)
			if drop {
				continue
			}
			if converted != nil {
				filtered = append(filtered, converted)
				continue
			}
		}
		filtered = append(filtered, block)
	}
	return filtered
}

// convertDocumentForBedrock inspects a document block and returns:
//   - (nil, false)    : keep the block as-is (it's a valid PDF)
//   - (textBlock, false) : replaced with an inline text block
//   - (nil, true)     : drop it entirely
func convertDocumentForBedrock(blockMap map[string]interface{}) (map[string]interface{}, bool) {
	source, _ := blockMap["source"].(map[string]interface{})
	if source == nil {
		return nil, true
	}
	srcType, _ := source["type"].(string)

	// url-source: Bedrock doesn't support non-pdf URLs either; drop if not pdf
	if srcType == "url" {
		urlStr, _ := source["url"].(string)
		if strings.HasSuffix(strings.ToLower(urlStr), ".pdf") {
			return nil, false
		}
		// Convert to a text block referencing the URL
		return map[string]interface{}{
			"type": "text",
			"text": fmt.Sprintf("[Document URL: %s]", urlStr),
		}, false
	}

	mediaType, _ := source["media_type"].(string)
	if mediaType == "application/pdf" {
		return nil, false
	}

	// base64 source with non-pdf media_type
	if srcType == "base64" || srcType == "" {
		dataStr, _ := source["data"].(string)
		if isTextualMediaType(mediaType) {
			decoded, err := base64.StdEncoding.DecodeString(dataStr)
			if err == nil {
				// Wrap document in text so Bedrock can process it
				text := string(decoded)
				// Cap extremely long bodies to avoid blowing up prompts (2MB)
				const maxLen = 2 * 1024 * 1024
				if len(text) > maxLen {
					text = text[:maxLen] + "\n\n[... document truncated]"
				}
				return map[string]interface{}{
					"type": "text",
					"text": text,
				}, false
			}
		}
		// Non-textual non-pdf (e.g. docx, xlsx): drop with a note
		return map[string]interface{}{
			"type": "text",
			"text": fmt.Sprintf("[Document omitted: unsupported media type %q on Bedrock]", mediaType),
		}, false
	}

	// Unknown source shape: drop
	return nil, true
}

func isTextualMediaType(mt string) bool {
	if strings.HasPrefix(mt, "text/") {
		return true
	}
	switch mt {
	case "application/json", "application/xml", "application/javascript",
		"application/x-yaml", "application/yaml", "application/toml":
		return true
	}
	return false
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
