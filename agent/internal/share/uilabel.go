package share

import (
	"fmt"
	"os"
	"strings"
)

// InstalledSuffix 是界面下拉框里追加给「模拟器内未安装」的分享目标的标记。
const InstalledSuffix = " (未安装)"

// shareOptionName 是 interface.json 中分享目标选项的名称。
const shareOptionName = "ShareAppOption"

// AnnotateOptions 是改写界面下拉框显示名所需的运行期信息。
type AnnotateOptions struct {
	// Installed 为各分享目标在模拟器内的安装情况；nil 表示设备不可用、情况未知。
	Installed map[string]bool
	// Server 是当前选择的游戏区服 ID（CN / TW）。
	// 下拉框始终列出全部分享目标，不按区服隐藏，因此这里不再用于追加「区服不匹配」
	// 之类的标记；保留该字段仅为兼容调用方。
	Server string
}

// ShareLabel 生成某个分享目标在界面下拉框里的完整显示名。
//
// 仅当设备在线且目标未安装时追加「(未安装)」标记；未做安装检测（设备未知）时
// 返回基准名。下拉框会列出所有分享目标（不受区服限制），因此不再为「不属于当前
// 区服面板」追加任何标记——选到非本服 APP 由运行期的任务前校验负责提示，而不是
// 在界面上预先标红。
func ShareLabel(app string, opts AnnotateOptions) string {
	name := DisplayName(app)
	if opts.Installed != nil && !opts.Installed[app] {
		return name + InstalledSuffix
	}
	return name
}

// TrimShareLabel 去掉显示名上的全部状态标记，返回基准名。
func TrimShareLabel(label string) string {
	name := strings.TrimSpace(label)
	if i := strings.Index(name, " ("); i >= 0 && strings.HasSuffix(name, ")") {
		name = name[:i]
	}
	return name
}

// AnnotateInterface 依据模拟器内的安装情况改写界面清单里分享目标的显示名。
//
// 采用逐行改写而不是整体重新序列化，是为了保持 interface.json 原有的键顺序与
// 缩进风格（MXU 生成的清单带有大量 $__mpe_code 等元数据，重排会造成巨大 diff）。
//
// opts.Installed 为 nil 表示「设备不可用、安装情况未知」，此时只清除未安装标记，
// 让清单回到只标基准名的干净状态。
func AnnotateInterface(path string, opts AnnotateOptions) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("读取界面清单失败: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	start, end := locateCases(lines, shareOptionName)
	if start < 0 {
		return false, fmt.Errorf("未在 %s 中找到 %s 的 cases 区块", path, shareOptionName)
	}

	changed := false
	for i := start; i < end; i++ {
		app, ok := caseNameAt(lines[i])
		if !ok {
			continue
		}
		if _, known := appPackages[app]; !known {
			continue
		}
		want := ShareLabel(app, opts)
		indent := leadingSpace(lines[i])
		content := `"label": ` + quote(want)
		if idx := labelLineIndex(lines, i+1, end); idx >= 0 {
			// 保留原有的行尾逗号，避免与后续字段的写法不一致
			if strings.HasSuffix(strings.TrimSpace(lines[idx]), ",") {
				content += ","
			}
			if lines[idx] != indent+content {
				lines[idx] = indent + content
				changed = true
			}
			continue
		}
		// 该 case 未声明 label 时紧跟 name 插入一行；后面还有字段才需要逗号
		comma := ""
		if j := nextFieldLine(lines, i+1, end); j >= 0 && strings.TrimSpace(lines[j]) != "}" {
			comma = ","
		}
		rest := append([]string{}, lines[i+1:]...)
		lines = append(lines[:i+1], indent+content+comma)
		lines = append(lines, rest...)
		end++
		changed = true
	}
	if !changed {
		return false, nil
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return false, fmt.Errorf("写回界面清单失败: %w", err)
	}
	return true, nil
}

// locateCases 定位指定 option 的 cases 数组区间 [start, end)。
func locateCases(lines []string, optionName string) (int, int) {
	optIdx := -1
	target := `"` + optionName + `": {`
	for i, line := range lines {
		if strings.TrimSpace(line) == target {
			optIdx = i
			break
		}
	}
	if optIdx < 0 {
		return -1, -1
	}
	for i := optIdx; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != `"cases": [` {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			switch strings.TrimSpace(lines[j]) {
			case "]", "],":
				return i + 1, j
			}
		}
		return -1, -1
	}
	return -1, -1
}

// caseNameAt 解析某行是否为 case 的 name 字段，返回其取值。
func caseNameAt(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, `"name":`) {
		return "", false
	}
	value := strings.TrimSpace(strings.TrimPrefix(trimmed, `"name":`))
	value = strings.TrimSuffix(value, ",")
	return unquote(value)
}

// labelLineIndex 在 [from, end) 范围内寻找 case 的 label 字段所在行。
func labelLineIndex(lines []string, from, end int) int {
	for i := from; i < end && i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, `"label":`) {
			return i
		}
		if trimmed == "}" || trimmed == "}," {
			return -1
		}
	}
	return -1
}

// nextFieldLine 返回 case 内 name 之后的下一个非空行下标。
func nextFieldLine(lines []string, from, end int) int {
	for i := from; i < end && i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "" {
			return i
		}
	}
	return -1
}

func leadingSpace(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// unquote 去掉 JSON 字符串两侧的引号，处理失败时原样返回。
func unquote(value string) (string, bool) {
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return value, false
	}
	return value[1 : len(value)-1], true
}

func quote(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
	return `"` + escaped + `"`
}
