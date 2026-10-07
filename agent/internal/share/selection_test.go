package share

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSelectedServer 固化「区服取自用户当前选择」的解析规则：
// 禁用副本优先让位于启用副本，避免历史上重复添加的 pretask 干扰判断。
func TestSelectedServer(t *testing.T) {
	t.Run("启用中的 pretask 优先", func(t *testing.T) {
		root := t.TempDir()
		writeMXUConfig(t, root, `{
  "lastActiveInstanceId": "inst-1",
  "instances": [
    {"id": "inst-1", "tasks": [
      {"taskName": "__MXU_PRETASK__启动模拟器与游戏", "enabled": false,
       "optionValues": {"ServerOption": {"type": "select", "caseName": "CN"}}},
      {"taskName": "__MXU_PRETASK__启动模拟器与游戏", "enabled": true,
       "optionValues": {"ServerOption": {"type": "select", "caseName": "TW"}}}
    ]}
  ]
}`)
		if got := SelectedServer(root); got != "TW" {
			t.Fatalf("SelectedServer = %q, want TW", got)
		}
	})

	t.Run("兼容中文取值与裸字符串", func(t *testing.T) {
		root := t.TempDir()
		writeMXUConfig(t, root, `{"instances":[{"id":"i","tasks":[
      {"taskName":"p","enabled":true,"optionValues":{"ServerOption":"台服"}}
    ]}]}`)
		if got := SelectedServer(root); got != "TW" {
			t.Fatalf("中文区服名归一化失败: %q", got)
		}
	})

	t.Run("无配置时回退国服", func(t *testing.T) {
		if got := SelectedServer(t.TempDir()); got != "CN" {
			t.Fatalf("缺少配置时应回退 CN, got %q", got)
		}
	})
}

func writeMXUConfig(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, "assets", "config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, mxuConfigName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSelectedApp 覆盖 MXU 配置里「分享」任务勾选状态的三种情形。
func TestSelectedApp(t *testing.T) {
	t.Run("勾选并选择了 LINE", func(t *testing.T) {
		root := t.TempDir()
		writeMXUConfig(t, root, `{
  "lastActiveInstanceId": "inst-1",
  "instances": [
    {"id": "inst-1", "tasks": [
      {"taskName": "送礼", "enabled": true, "optionValues": {}},
      {"taskName": "分享", "enabled": true, "optionValues": {"ShareAppOption": {"type": "select", "caseName": "LINE"}}}
    ]}
  ]
}`)
		app, found := SelectedApp(root)
		if !found || app != "LINE" {
			t.Fatalf("SelectedApp = %q, %v; want LINE, true", app, found)
		}
	})

	t.Run("未勾选分享", func(t *testing.T) {
		root := t.TempDir()
		writeMXUConfig(t, root, `{"instances":[{"id":"i","tasks":[
      {"taskName":"分享","enabled":false,"optionValues":{"ShareAppOption":{"type":"select","caseName":"QQ"}}}
    ]}]}`)
		app, found := SelectedApp(root)
		if found {
			t.Fatalf("未勾选时不应返回: %q", app)
		}
	})

	t.Run("兼容裸字符串与中文名", func(t *testing.T) {
		root := t.TempDir()
		writeMXUConfig(t, root, `{"instances":[{"id":"i","tasks":[
      {"taskName":"分享","enabled":true,"optionValues":{"ShareAppOption":"微信"}}
    ]}]}`)
		if app, _ := SelectedApp(root); app != "WeChat" {
			t.Fatalf("中文名归一化失败: %q", app)
		}
	})

	t.Run("配置文件缺失时不报错", func(t *testing.T) {
		if _, found := SelectedApp(t.TempDir()); found {
			t.Fatal("配置缺失时不应返回 found")
		}
	})
}
