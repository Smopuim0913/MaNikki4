package emulator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/TianQuanDiWen/MaNikki4/agent/internal/state"
)

func TestServerIDOfPackage(t *testing.T) {
	cases := []struct {
		pkg  string
		want string
	}{
		{"com.papegames.nn4.cn", "CN"},
		{"com.shining.nikki4.tw", "TW"},
		{"com.papegames.nn4.tw", "TW"},
		{"com.papegames.nn4.mi", ""},
		{"com.other.game", ""},
	}
	for _, c := range cases {
		if got := ServerIDOfPackage(c.pkg); got != c.want {
			t.Errorf("ServerIDOfPackage(%q) = %q, want %q", c.pkg, got, c.want)
		}
	}
}

func TestSaveAndLoadRuntimeState(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}

	// 未写入时应回退到国服默认值，保证旧流程行为不变
	got := LoadRuntimeState(root)
	if got.Server != "CN" || got.Package != defaultPackageName {
		t.Fatalf("缺省状态 = %+v, want CN/%s", got, defaultPackageName)
	}

	if err := SaveRuntimeState(root, "TW", "com.shining.nikki4.tw", []string{"LINE", "Instagram"}); err != nil {
		t.Fatalf("SaveRuntimeState: %v", err)
	}
	got = LoadRuntimeState(root)
	if got.Server != "TW" || got.Package != "com.shining.nikki4.tw" {
		t.Fatalf("载入状态 = %+v", got)
	}
	if got.UpdatedAt == "" {
		t.Error("缺少 updated_at")
	}
	if len(got.ShareApps) != 2 || got.ShareApps[0] != "LINE" {
		t.Errorf("已安装分享目标未随状态保存: %v", got.ShareApps)
	}

	// server 为空时应能由包名反推
	if err := SaveRuntimeState(root, "", "com.papegames.nn4.cn", nil); err != nil {
		t.Fatalf("SaveRuntimeState: %v", err)
	}
	if got = LoadRuntimeState(root); got.Server != "CN" {
		t.Fatalf("反推区服失败: %+v", got)
	}
}

func TestRuntimeStateFileIsJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets", "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SaveRuntimeState(root, "TW", "com.papegames.nn4.tw", nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "assets", "config", state.FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("运行时文件应落在 assets/config: %v", err)
	}
	var state RuntimeState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if state.Server != "TW" {
		t.Fatalf("server = %q", state.Server)
	}
}
