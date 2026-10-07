package emulator

import (
	"strings"

	"github.com/TianQuanDiWen/MaNikki4/agent/internal/state"
)

// RuntimeState 是 pretask 解析结果在运行期共享给 Agent 的最小状态。
type RuntimeState = state.RuntimeState

// ServerIDOfPackage 返回包名所属区服的 ID，未知渠道返回空串。
func ServerIDOfPackage(pkg string) string {
	for _, server := range knownServers {
		for _, candidate := range server.Packages {
			if candidate == pkg {
				return server.ID
			}
		}
	}
	return ""
}

// SaveRuntimeState 由 pretask 调用，记录本次实际拉起的区服、包名与已安装的分享目标。
// Server 留空时会依据包名反推，便于调用方只传包名。
func SaveRuntimeState(projectRoot, server, pkg string, shareApps []string) error {
	server = strings.ToUpper(strings.TrimSpace(server))
	if server == "" {
		server = ServerIDOfPackage(pkg)
	}
	return state.Save(projectRoot, server, pkg, shareApps)
}

// LoadRuntimeState 读取 pretask 写入的区服与包名。
// 缺失时回退到国服默认值，保证旧流程与未执行 pretask 的场景行为不变。
func LoadRuntimeState(projectRoot string) RuntimeState {
	st := state.Load(projectRoot)
	if st.Server == "" {
		st.Server = ServerIDOfPackage(st.Package)
	}
	if st.Server == "" {
		st.Server = state.DefaultServer
	}
	return st
}
