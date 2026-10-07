// Package state 保存 pretask 与运行期 Agent 之间共享的最小运行时状态。
//
// 独立成包是为了让 emulator（写入方）与 share（读取方）都只依赖本包，避免循环引用。
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// DefaultServer 是未探测到区服时的回退值，保证旧流程行为不变。
	DefaultServer = "CN"
	// DefaultPackage 是国服客户端包名。
	DefaultPackage = "com.papegames.nn4.cn"
	// FileName 是运行时状态文件名。独立于 maa_pi_config.json，避免被 MXU 回写覆盖。
	FileName = "manikki_runtime.json"
)

// RuntimeState 是 pretask 解析结果在运行期共享给 Agent 的最小状态。
type RuntimeState struct {
	Server    string   `json:"server"`     // 区服 ID：CN / TW
	Package   string   `json:"package"`    // 实际拉起的游戏包名
	ShareApps []string `json:"share_apps"` // 模拟器内已安装的分享目标 APP（case name）
	UpdatedAt string   `json:"updated_at"` // RFC3339，便于排查
}

// Save 由 pretask 调用，记录本次实际拉起的区服、包名与已安装的分享目标。
func Save(projectRoot, server, pkg string, shareApps []string) error {
	st := RuntimeState{
		Server:    strings.ToUpper(strings.TrimSpace(server)),
		Package:   strings.TrimSpace(pkg),
		ShareApps: shareApps,
		UpdatedAt: time.Now().Format(time.RFC3339),
	}
	if st.Package == "" {
		st.Package = DefaultPackage
	}
	if st.Server == "" {
		st.Server = DefaultServer
	}
	data, err := json.MarshalIndent(st, "", "    ")
	if err != nil {
		return err
	}
	path := ResolvePath(projectRoot, true)
	return os.WriteFile(path, data, 0o644)
}

// Load 读取 pretask 写入的运行时状态；缺失时回退到国服默认值。
func Load(projectRoot string) RuntimeState {
	fallback := RuntimeState{Server: DefaultServer, Package: DefaultPackage}
	path := ResolvePath(projectRoot, false)
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	var st RuntimeState
	if err := json.Unmarshal(data, &st); err != nil {
		return fallback
	}
	if st.Server == "" {
		st.Server = DefaultServer
	}
	if st.Package == "" {
		st.Package = DefaultPackage
	}
	return st
}

// ResolvePath 定位运行时状态文件路径（与 maa_pi_config.json 同目录策略）。
func ResolvePath(projectRoot string, ensureDir bool) string {
	candidates := []string{
		filepath.Join(projectRoot, "config", FileName),
		filepath.Join(projectRoot, "assets", "config", FileName),
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	for _, p := range candidates {
		if info, err := os.Stat(filepath.Dir(p)); err == nil && info.IsDir() {
			return p
		}
	}
	target := candidates[0]
	if ensureDir {
		_ = os.MkdirAll(filepath.Dir(target), 0o755)
	}
	return target
}
