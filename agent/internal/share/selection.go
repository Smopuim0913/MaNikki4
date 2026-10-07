package share

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/TianQuanDiWen/MaNikki4/agent/internal/state"
)

// shareTaskName 是 interface.json 中分享任务的名称，用于在运行前校验其选项取值。
const shareTaskName = "分享"

// mxuConfigName 是 MXU 保存实例与任务勾选状态的配置文件名。
const mxuConfigName = "mxu-MaNikki4.json"

// ServerOptionName 是 interface.json 中区服选项的名称。
const ServerOptionName = "ServerOption"

// SelectedServer 读取用户在 MXU 里当前选择的游戏区服（CN / TW）。
//
// 区服选项挂在 pretask 任务上，MXU 会把它持久保存在实例的任务列表里，因此这里
// 遍历任务取值：先取启用中的任务（用户可能添加过多份 pretask，禁用的不算数），
// 其次才是被禁用的；都没有时回落到 pretask 实际运行记录的运行时状态。
func SelectedServer(projectRoot string) string {
	path := locateMXUConfig(projectRoot)
	if path != "" {
		if server := readServerFromConfig(path); server != "" {
			return server
		}
	}
	if st := state.Load(projectRoot); st.Server != "" {
		return normalizeServer(st.Server)
	}
	return state.DefaultServer
}

func readServerFromConfig(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cfg struct {
		LastActiveInstance string `json:"lastActiveInstanceId"`
		Instances          []struct {
			ID    string `json:"id"`
			Tasks []struct {
				TaskName     string         `json:"taskName"`
				Enabled      bool           `json:"enabled"`
				OptionValues map[string]any `json:"optionValues"`
			} `json:"tasks"`
		} `json:"instances"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ""
	}

	ordered := make([]int, 0, len(cfg.Instances))
	for i, ins := range cfg.Instances {
		if ins.ID != "" && ins.ID == cfg.LastActiveInstance {
			ordered = append([]int{i}, ordered...)
			continue
		}
		ordered = append(ordered, i)
	}
	// 两轮扫描：启用中的任务优先，避免被历史残留的禁用副本带偏。
	for _, wantEnabled := range []bool{true, false} {
		for _, idx := range ordered {
			for _, task := range cfg.Instances[idx].Tasks {
				if task.Enabled != wantEnabled {
					continue
				}
				if server := canonicalServer(extractServerValue(task.OptionValues[ServerOptionName])); server != "" {
					return server
				}
			}
		}
	}
	return ""
}

// extractServerValue 兼容 MXU 对 select 选项的多种序列化形态。
func extractServerValue(value any) string {
	return extractServerValueDepth(value, 0)
}

func extractServerValueDepth(value any, depth int) string {
	if depth > 4 || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return canonicalServer(typed)
	case map[string]any:
		for _, key := range []string{"caseName", "name", "value", "case", "selected", "values"} {
			if nested, ok := typed[key]; ok {
				if server := extractServerValueDepth(nested, depth+1); server != "" {
					return server
				}
			}
		}
	}
	return ""
}

// canonicalServer 把 CN / TW / 国服 / 台服 等写法归一到区服 ID。
func canonicalServer(value string) string {
	switch v := strings.TrimSpace(value); {
	case strings.EqualFold(v, "CN"), strings.EqualFold(v, "国服"):
		return "CN"
	case strings.EqualFold(v, "TW"), strings.EqualFold(v, "台服"):
		return "TW"
	}
	return ""
}

// SelectedApp 读取 MXU 配置中「分享」任务当前勾选的目标 APP。
// 仅当该任务确实处于勾选状态时才返回 found=true，避免误伤未勾选分享的场景。
func SelectedApp(projectRoot string) (app string, found bool) {
	path := locateMXUConfig(projectRoot)
	if path == "" {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var cfg struct {
		LastActiveInstance string `json:"lastActiveInstanceId"`
		Instances          []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Tasks []struct {
				TaskName     string         `json:"taskName"`
				Enabled      bool           `json:"enabled"`
				OptionValues map[string]any `json:"optionValues"`
			} `json:"tasks"`
		} `json:"instances"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", false
	}

	// 优先读取当前活动实例，其次按配置顺序取第一个命中的
	ordered := make([]int, 0, len(cfg.Instances))
	for i, ins := range cfg.Instances {
		if ins.ID != "" && ins.ID == cfg.LastActiveInstance {
			ordered = append([]int{i}, ordered...)
			continue
		}
		ordered = append(ordered, i)
	}
	for _, idx := range ordered {
		for _, task := range cfg.Instances[idx].Tasks {
			if task.TaskName != shareTaskName || !task.Enabled {
				continue
			}
			if app := extractAppValue(task.OptionValues[shareOptionName]); app != "" {
				return app, true
			}
			return "", true // 勾选了但未解析到取值，交由调用方决定如何处理
		}
	}
	return "", false
}

// locateMXUConfig 在源码目录与发布目录中定位 MXU 配置文件。
func locateMXUConfig(projectRoot string) string {
	candidates := []string{
		filepath.Join(projectRoot, "assets", "config", mxuConfigName),
		filepath.Join(projectRoot, "config", mxuConfigName),
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// extractAppValue 兼容 MXU 对 select 选项的多种序列化形态：
// 裸字符串、{"caseName": "TW"}、{"type":"select","caseName":"TW"}、{"value":"TW"} 等。
func extractAppValue(value any) string {
	return extractAppValueDepth(value, 0)
}

func extractAppValueDepth(value any, depth int) string {
	if depth > 4 || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return canonicalApp(typed)
	case map[string]any:
		for _, key := range []string{"caseName", "name", "value", "case", "selected", "values"} {
			if nested, ok := typed[key]; ok {
				if app := extractAppValueDepth(nested, depth+1); app != "" {
					return app
				}
			}
		}
	}
	return ""
}

// canonicalApp 把任意写法的 APP 取值归一到 interface.json 中的 case name。
func canonicalApp(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if _, ok := appPackages[value]; ok {
		return value
	}
	for _, app := range appOrder {
		if strings.EqualFold(app, value) || value == DisplayName(app) {
			return app
		}
	}
	return ""
}
