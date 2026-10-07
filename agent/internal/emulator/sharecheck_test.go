package emulator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeShareTask(t *testing.T, root, app string, enabled bool) {
	t.Helper()
	dir := filepath.Join(root, "assets", "config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"instances":[{"id":"i","tasks":[{"taskName":"分享","enabled":` +
		boolLiteral(enabled) + `,"optionValues":{"ShareAppOption":{"type":"select","caseName":"` + app + `"}}}]}]}`
	if err := os.WriteFile(filepath.Join(dir, "mxu-MaNikki4.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func boolLiteral(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// TestCheckShareAppSelection 固化「勾选分享但目标未安装时提前中止」这一约定。
func TestCheckShareAppSelection(t *testing.T) {
	t.Run("未安装则中止", func(t *testing.T) {
		root := t.TempDir()
		writeShareTask(t, root, "QQ", true)
		err := checkShareAppSelection(root, "CN", []string{"jp.naver.line.android"})
		if err == nil {
			t.Fatal("目标未安装时应返回错误")
		}
		if !strings.Contains(err.Error(), "com.tencent.mobileqq") {
			t.Errorf("错误信息应给出包名便于排查: %v", err)
		}
		if !strings.Contains(err.Error(), "LINE") {
			t.Errorf("错误信息应提示已安装的可选项: %v", err)
		}
	})

	t.Run("已安装则放行", func(t *testing.T) {
		root := t.TempDir()
		writeShareTask(t, root, "LINE", true)
		if err := checkShareAppSelection(root, "TW", []string{"jp.naver.line.android"}); err != nil {
			t.Fatalf("已安装时不应报错: %v", err)
		}
	})

	t.Run("未勾选分享则放行", func(t *testing.T) {
		root := t.TempDir()
		writeShareTask(t, root, "QQ", false)
		if err := checkShareAppSelection(root, "CN", nil); err != nil {
			t.Fatalf("未勾选时不应报错: %v", err)
		}
	})

	t.Run("无配置文件则放行", func(t *testing.T) {
		if err := checkShareAppSelection(t.TempDir(), "CN", nil); err != nil {
			t.Fatalf("缺少配置时不应报错: %v", err)
		}
	})
}

// TestInstalledShareApps 校验包名列表到分享目标的折算。
func TestInstalledShareApps(t *testing.T) {
	got := installedShareApps([]string{"com.sina.weibo", "jp.naver.line.android", "com.other"})
	if len(got) != 2 || got[0] != "Weibo" || got[1] != "LINE" {
		t.Fatalf("installedShareApps = %v", got)
	}
}

// TestShareAppSelectionWritesRuntimeState 端到端：运行时状态需带上已安装的分享目标。
func TestShareAppSelectionWritesRuntimeState(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	apps := installedShareApps([]string{"com.instagram.android"})
	if err := SaveRuntimeState(root, "TW", "com.shining.nikki4.tw", apps); err != nil {
		t.Fatal(err)
	}
	got := LoadRuntimeState(root)
	if len(got.ShareApps) != 1 || got.ShareApps[0] != "Instagram" {
		t.Fatalf("运行时状态未保存分享目标: %+v", got)
	}
}
