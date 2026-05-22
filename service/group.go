package service

import (
	"strings"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// SplitUserGroups 把 user.Group 字符串按逗号分隔成多分组列表。
// 空白会被 trim 掉；纯空字符串返回空切片。
// 多分组语义：一个用户可被授权使用多个分组的模型；第一个分组为主分组（用于默认 UsingGroup）。
func SplitUserGroups(userGroup string) []string {
	if userGroup == "" {
		return nil
	}
	parts := strings.Split(userGroup, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		g := strings.TrimSpace(p)
		if g == "" {
			continue
		}
		if _, dup := seen[g]; dup {
			continue
		}
		seen[g] = struct{}{}
		result = append(result, g)
	}
	return result
}

// GetUserPrimaryGroup 获取用户的主分组（逗号分隔列表的第一项）。
// 用于计费的默认 UsingGroup。
func GetUserPrimaryGroup(userGroup string) string {
	groups := SplitUserGroups(userGroup)
	if len(groups) == 0 {
		return ""
	}
	return groups[0]
}

// applyGroupSpecialUsable 把单个分组的 GroupSpecialUsableGroup 配置 merge 到 groupsCopy。
func applyGroupSpecialUsable(groupsCopy map[string]string, userGroup string) {
	specialSettings, b := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Get(userGroup)
	if !b {
		return
	}
	for specialGroup, desc := range specialSettings {
		if strings.HasPrefix(specialGroup, "-:") {
			groupToRemove := strings.TrimPrefix(specialGroup, "-:")
			delete(groupsCopy, groupToRemove)
		} else if strings.HasPrefix(specialGroup, "+:") {
			groupToAdd := strings.TrimPrefix(specialGroup, "+:")
			groupsCopy[groupToAdd] = desc
		} else {
			groupsCopy[specialGroup] = desc
		}
	}
}

// GetUserUsableGroups 返回用户当前可用的分组及描述。
// userGroup 可以是逗号分隔的多分组（feat: per-user multi-group authorization, 2026-05-22）。
// 兼容旧的单分组语义：传入单个分组时行为完全等价于以前。
func GetUserUsableGroups(userGroup string) map[string]string {
	groupsCopy := setting.GetUserUsableGroupsCopy()
	userGroups := SplitUserGroups(userGroup)
	if len(userGroups) == 0 {
		return groupsCopy
	}
	for _, g := range userGroups {
		// 每个授权分组都应用一次 special 规则（+: / -: / 直接添加）
		applyGroupSpecialUsable(groupsCopy, g)
		// 确保该分组本身在结果集中
		if _, ok := groupsCopy[g]; !ok {
			groupsCopy[g] = "用户分组"
		}
	}
	return groupsCopy
}

func GroupInUserUsableGroups(userGroup, groupName string) bool {
	_, ok := GetUserUsableGroups(userGroup)[groupName]
	return ok
}

// GetUserAutoGroup 根据用户分组获取自动分组设置
func GetUserAutoGroup(userGroup string) []string {
	groups := GetUserUsableGroups(userGroup)
	autoGroups := make([]string, 0)
	for _, group := range setting.GetAutoGroups() {
		if _, ok := groups[group]; ok {
			autoGroups = append(autoGroups, group)
		}
	}
	return autoGroups
}

// GetUserGroupRatio 获取用户使用某个分组的倍率
// userGroup 用户分组（可能是逗号分隔的多分组）
// group 需要获取倍率的分组
//
// 查找顺序（新语义，兼容旧逻辑）：
//  1. 遍历用户的每个授权分组，查 GroupGroupRatio[userGroup][group]，命中则返回（用户组级覆盖）
//  2. 回落到全局 GroupRatio[group]
//
// 注意：Phase 2 会在最前面再加一层 UserGroupRatio[userId][group] 覆盖。
func GetUserGroupRatio(userGroup, group string) float64 {
	for _, g := range SplitUserGroups(userGroup) {
		if ratio, ok := ratio_setting.GetGroupGroupRatio(g, group); ok {
			return ratio
		}
	}
	return ratio_setting.GetGroupRatio(group)
}
