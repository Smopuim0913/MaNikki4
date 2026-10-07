package share

import "strings"

// appPackages 是分享目标 APP 在 Android 上的包名，用于检测模拟器内是否已安装。
//
// 注意：小红书的实际包名是 com.xingin.xhs（行吟信息科技（上海）有限公司）。
// 网上流传的 com.xiaohongshu.android 并非其包名，勿采用。
var appPackages = map[string]string{
	"QQ":          "com.tencent.mobileqq",
	"WeChat":      "com.tencent.mm",
	"Weibo":       "com.sina.weibo",
	"Xiaohongshu": "com.xingin.xhs",
	"LINE":        "jp.naver.line.android",
	"Instagram":   "com.instagram.android",
}

// appDisplayNames 是界面下拉框里的显示名。品牌名（LINE / Instagram / QQ）保持原文，
// 中文名保持中文，便于与游戏内分享面板上的文字对应。
var appDisplayNames = map[string]string{
	"QQ":          "QQ",
	"WeChat":      "微信",
	"Weibo":       "微博",
	"Xiaohongshu": "小红书",
	"LINE":        "LINE",
	"Instagram":   "Instagram",
}

// appOrder 固定界面下拉框的展示顺序：先国服面板，再台服面板。
var appOrder = []string{"QQ", "WeChat", "Weibo", "Xiaohongshu", "LINE", "Instagram"}

// ZoneOfApp 返回某个分享目标所属的区服分享面板 ID（CN / TW），未收录时返回空串。
func ZoneOfApp(app string) string {
	for _, zone := range []string{"CN", "TW"} {
		if AppOnPanel(zone, app) {
			return zone
		}
	}
	return ""
}

// ZoneLabelName 返回区服 ID 的中文名，用于界面提示；未知取值原样返回。
func ZoneLabelName(server string) string {
	switch normalizeServer(server) {
	case "CN":
		return "国服"
	case "TW":
		return "台服"
	}
	return strings.TrimSpace(server)
}

// AppOnPanel 判定某个 APP 是否出现在指定区服的分享面板上。
// 面板不存在的区服一律返回 false，调用方据此保留原取值而不是静默改写。
func AppOnPanel(server, app string) bool {
	panel, ok := panels[normalizeServer(server)]
	if !ok {
		return false
	}
	_, exists := panel.Targets[app]
	return exists
}

func Apps() []string {
	out := make([]string, 0, len(appOrder))
	out = append(out, appOrder...)
	return out
}

// PackageName 返回分享目标 APP 的 Android 包名。
func PackageName(app string) (string, bool) {
	pkg, ok := appPackages[app]
	return pkg, ok
}

// DisplayName 返回分享目标在界面上的显示名（不含安装状态标记）。
func DisplayName(app string) string {
	if name, ok := appDisplayNames[app]; ok && name != "" {
		return name
	}
	return app
}

// DetectInstalled 依据设备上已安装的包名判定各分享目标是否可用。
// 返回结果覆盖全部已知 APP，未出现的即视为未安装。
func DetectInstalled(installedPkgs []string) map[string]bool {
	present := make(map[string]bool, len(installedPkgs))
	for _, pkg := range installedPkgs {
		pkg = strings.TrimSpace(pkg)
		if pkg != "" {
			present[pkg] = true
		}
	}
	result := make(map[string]bool, len(appPackages))
	for app, pkg := range appPackages {
		result[app] = present[pkg]
	}
	return result
}

// PreferInstalled 在区服面板内挑选一个实际可用的分享目标。
// 期望的 APP 属于本区服面板且已安装时原样返回；否则退而求其次挑一个已安装的
// （默认 APP 优先）。若该区服面板上的 APP 全部未安装，仍返回原解析结果，
// 由调用方负责提示用户，避免出现「静默什么都不做」。
func PreferInstalled(server, app string, installed map[string]bool) (string, bool) {
	panel, ok := panels[normalizeServer(server)]
	if !ok {
		return "", false
	}
	if len(installed) == 0 {
		// 未做安装检测（例如旧版运行时状态），保持既有行为
		if key, exists := lookupApp(panel, app); exists {
			return key, true
		}
		return panel.DefaultApp, true
	}
	if key, exists := lookupApp(panel, app); exists && installed[key] {
		return key, true
	}
	if installed[panel.DefaultApp] {
		return panel.DefaultApp, true
	}
	for _, name := range appOrder {
		if _, exists := panel.Targets[name]; exists && installed[name] {
			return name, true
		}
	}
	return app, true
}
