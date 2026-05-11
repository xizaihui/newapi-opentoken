package common

import (
	"strings"
	"sync"

	newApiCommon "github.com/QuantumNous/new-api/common"
)

const (
	OptionKeyBedrockBetaFlagsSupported   = "BedrockBetaFlagsSupported"
	OptionKeyBedrockBetaFlagsUnsupported = "BedrockBetaFlagsUnsupported"
)

var (
	bedrockBetaSupportedCache   map[string]bool
	bedrockBetaUnsupportedCache map[string]bool
	bedrockBetaCacheMu          sync.RWMutex
	bedrockBetaLastSupported    string
	bedrockBetaLastUnsupported  string
)

// getBedrockBetaFlags reads the supported/unsupported flags from OptionMap and caches them.
func getBedrockBetaFlags() (supported map[string]bool, unsupported map[string]bool) {
	newApiCommon.OptionMapRWMutex.RLock()
	rawSupported := newApiCommon.OptionMap[OptionKeyBedrockBetaFlagsSupported]
	rawUnsupported := newApiCommon.OptionMap[OptionKeyBedrockBetaFlagsUnsupported]
	newApiCommon.OptionMapRWMutex.RUnlock()

	bedrockBetaCacheMu.RLock()
	if rawSupported == bedrockBetaLastSupported && rawUnsupported == bedrockBetaLastUnsupported &&
		bedrockBetaSupportedCache != nil {
		s := bedrockBetaSupportedCache
		u := bedrockBetaUnsupportedCache
		bedrockBetaCacheMu.RUnlock()
		return s, u
	}
	bedrockBetaCacheMu.RUnlock()

	// Rebuild cache
	bedrockBetaCacheMu.Lock()
	defer bedrockBetaCacheMu.Unlock()

	// Double-check after acquiring write lock
	if rawSupported == bedrockBetaLastSupported && rawUnsupported == bedrockBetaLastUnsupported &&
		bedrockBetaSupportedCache != nil {
		return bedrockBetaSupportedCache, bedrockBetaUnsupportedCache
	}

	bedrockBetaSupportedCache = parseBedrockBetaFlags(rawSupported)
	bedrockBetaUnsupportedCache = parseBedrockBetaFlags(rawUnsupported)
	bedrockBetaLastSupported = rawSupported
	bedrockBetaLastUnsupported = rawUnsupported

	return bedrockBetaSupportedCache, bedrockBetaUnsupportedCache
}

func parseBedrockBetaFlags(raw string) map[string]bool {
	result := make(map[string]bool)
	for _, line := range strings.Split(raw, "\n") {
		flag := strings.TrimSpace(line)
		if flag != "" {
			result[flag] = true
		}
	}
	return result
}

// FilterBedrockBetaFlags filters the anthropic-beta flags for Bedrock compatibility.
// Logic:
//   - If supported list is non-empty: only allow flags in the supported list (whitelist mode)
//   - Additionally remove any flags in the unsupported list (blacklist mode)
//   - Unsupported list also does prefix matching (e.g. "prompt-caching" blocks "prompt-caching-2025-01-01")
func FilterBedrockBetaFlags(flags []string) []string {
	supported, unsupported := getBedrockBetaFlags()

	// If neither list is configured, pass through everything
	if len(supported) == 0 && len(unsupported) == 0 {
		return flags
	}

	filtered := make([]string, 0, len(flags))
	for _, flag := range flags {
		flag = strings.TrimSpace(flag)
		if flag == "" {
			continue
		}

		// Check unsupported list (exact match + prefix match)
		if isUnsupportedFlag(flag, unsupported) {
			continue
		}

		// If supported list exists, only allow flags in it
		if len(supported) > 0 && !supported[flag] {
			continue
		}

		filtered = append(filtered, flag)
	}
	return filtered
}

func isUnsupportedFlag(flag string, unsupported map[string]bool) bool {
	// Exact match
	if unsupported[flag] {
		return true
	}
	// Prefix match: e.g. unsupported has "prompt-caching", blocks "prompt-caching-2025-01-01"
	for blocked := range unsupported {
		if strings.HasPrefix(flag, blocked+"-") || strings.HasPrefix(flag, blocked+".") {
			return true
		}
	}
	return false
}
