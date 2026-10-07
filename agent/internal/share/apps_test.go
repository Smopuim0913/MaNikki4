package share

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPackageNames 固化各分享目标的 Android 包名，尤其是容易记错的小红书。
func TestPackageNames(t *testing.T) {
	want := map[string]string{
		"QQ":          "com.tencent.mobileqq",
		"WeChat":      "com.tencent.mm",
		"Weibo":       "com.sina.weibo",
		"Xiaohongshu": "com.xingin.xhs",
		"LINE":        "jp.naver.line.android",
		"Instagram":   "com.instagram.android",
	}
	for app, pkg := range want {
		got, ok := PackageName(app)
		if !ok {
			t.Fatalf("PackageName(%q) 未找到", app)
		}
		if got != pkg {
			t.Errorf("PackageName(%q) = %q, want %q", app, got, pkg)
		}
	}
	if _, ok := PackageName("Facebook"); ok {
		t.Error("Facebook 不应出现在分享目标中（台服不支持）")
	}
}

func TestDetectInstalled(t *testing.T) {
	installed := DetectInstalled([]string{
		"com.sina.weibo",
		"jp.naver.line.android",
		"com.other.app",
	})
	for app, want := range map[string]bool{
		"Weibo":       true,
		"LINE":        true,
		"QQ":          false,
		"WeChat":      false,
		"Xiaohongshu": false,
		"Instagram":   false,
	} {
		if installed[app] != want {
			t.Errorf("DetectInstalled[%q] = %v, want %v", app, installed[app], want)
		}
	}
}

// TestPreferInstalled 覆盖「所选 APP 未安装时改用本区服面板上已安装的目标」。
func TestPreferInstalled(t *testing.T) {
	installed := map[string]bool{"Weibo": true, "LINE": true}
	cases := []struct {
		name    string
		server  string
		app     string
		wantApp string
	}{
		{"已安装则原样保留", "CN", "Weibo", "Weibo"},
		{"国服默认APP未安装则挑已安装项", "CN", "QQ", "Weibo"},
		{"国服改选已安装项", "CN", "Xiaohongshu", "Weibo"},
		{"台服已安装", "TW", "LINE", "LINE"},
		{"台服未安装回退默认APP", "TW", "Instagram", "LINE"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := PreferInstalled(c.server, c.app, installed)
			if !ok {
				t.Fatal("PreferInstalled 返回 false")
			}
			if got != c.wantApp {
				t.Errorf("PreferInstalled(%s,%s) = %q, want %q", c.server, c.app, got, c.wantApp)
			}
		})
	}
}

// TestPreferInstalledKeepsUnknown 未做安装检测时（旧运行时状态）不应改变既有行为。
func TestPreferInstalledKeepsUnknown(t *testing.T) {
	got, _ := PreferInstalled("TW", "Instagram", nil)
	if got != "Instagram" {
		t.Fatalf("未检测时 = %q, want Instagram", got)
	}
	got, _ = PreferInstalled("TW", "QQ", nil)
	if got != "LINE" {
		t.Fatalf("未检测时区服回退失效: %q", got)
	}
}

func TestDisplayName(t *testing.T) {
	want := map[string]string{
		"QQ": "QQ", "WeChat": "微信", "Weibo": "微博",
		"Xiaohongshu": "小红书", "LINE": "LINE", "Instagram": "Instagram",
	}
	for app, name := range want {
		if got := DisplayName(app); got != name {
			t.Errorf("DisplayName(%q) = %q, want %q", app, got, name)
		}
	}
}

// TestShareLabel 固化显示名的标记拼接规则：未安装时追加「(未安装)」；与区服无关。
func TestShareLabel(t *testing.T) {
	cases := []struct {
		name    string
		app     string
		install *bool
		server  string
		want    string
	}{
		{"本区服已安装则不加标记", "Weibo", ptrBool(true), "CN", "微博"},
		{"本区服未安装", "QQ", ptrBool(false), "CN", "QQ (未安装)"},
		{"台服面板内的 LINE", "LINE", ptrBool(true), "TW", "LINE"},
		{"安装情况未知时不加未安装标记", "QQ", nil, "CN", "QQ"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opts := AnnotateOptions{Server: c.server}
			if c.install != nil {
				opts.Installed = map[string]bool{c.app: *c.install}
			}
			if got := ShareLabel(c.app, opts); got != c.want {
				t.Errorf("ShareLabel(%s, server=%s) = %q, want %q", c.app, c.server, got, c.want)
			}
		})
	}
}

func ptrBool(v bool) *bool { return &v }

// TestShareLabelUnknownServer 未知的区服取值不应误伤既有显示名。
func TestShareLabelUnknownServer(t *testing.T) {
	opts := AnnotateOptions{Server: "JP", Installed: map[string]bool{"QQ": true}}
	if got := ShareLabel("QQ", opts); got != "QQ" {
		t.Fatalf("未知区服时 = %q, want QQ", got)
	}
}

// TestZoneOfApp 固化「分享 APP 归属哪个区服面板」这一强绑定关系。
func TestZoneOfApp(t *testing.T) {
	for app, want := range map[string]string{
		"QQ": "CN", "WeChat": "CN", "Weibo": "CN", "Xiaohongshu": "CN",
		"LINE": "TW", "Instagram": "TW",
	} {
		if got := ZoneOfApp(app); got != want {
			t.Errorf("ZoneOfApp(%q) = %q, want %q", app, got, want)
		}
	}
	if ZoneOfApp("Facebook") != "" {
		t.Error("Facebook 不属于任何区服面板（台服不支持）")
	}
}

// TestTrimShareLabel 复合标记应能被整体剥离。
func TestTrimShareLabel(t *testing.T) {
	if got := TrimShareLabel("QQ (未安装)"); got != "QQ" {
		t.Fatalf("TrimShareLabel = %q, want QQ", got)
	}
}

// TestAnnotateInterface 校验「(未安装)」标记的写入与清除（仅依据安装情况，与区服无关）。
func TestAnnotateInterface(t *testing.T) {
	const sample = `{
    "option": {
        "OtherOption": {
            "cases": [
                { "name": "Yes", "label": "开启" }
            ]
        },
        "ShareAppOption": {
            "type": "select",
            "label": "分享目标APP",
            "cases": [
                {
                    "name": "QQ",
                    "label": "QQ",
                    "pipeline_override": {}
                },
                {
                    "name": "WeChat",
                    "label": "微信",
                    "pipeline_override": {}
                },
                {
                    "name": "LINE",
                    "label": "LINE",
                    "pipeline_override": {}
                }
            ],
            "default_case": "QQ"
        }
    }
}`
	dir := t.TempDir()
	path := filepath.Join(dir, "interface.json")
	if err := os.WriteFile(path, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}

	// 已安装 QQ、LINE，未安装微信：微信应标注「(未安装)」
	opts := AnnotateOptions{Installed: map[string]bool{"QQ": true, "LINE": true}}
	changed, err := AnnotateInterface(path, opts)
	if err != nil {
		t.Fatalf("AnnotateInterface: %v", err)
	}
	if !changed {
		t.Fatal("首次标注应产生变化")
	}
	assertLabel(t, path, "QQ", "QQ")
	assertLabel(t, path, "WeChat", "微信 (未安装)")
	assertLabel(t, path, "LINE", "LINE")

	// 再次写入相同结果不应改动文件
	changed, err = AnnotateInterface(path, opts)
	if err != nil || changed {
		t.Fatalf("幂等性失败: changed=%v err=%v", changed, err)
	}

	// 设备不可用时应清除安装标记回到基准名
	changed, err = AnnotateInterface(path, AnnotateOptions{})
	if err != nil || !changed {
		t.Fatalf("清除标记失败: changed=%v err=%v", changed, err)
	}
	assertLabel(t, path, "WeChat", "微信")
	assertLabel(t, path, "QQ", "QQ")
}

// assertLabel 从界面清单中取出指定 case 的 label 取值并比对。
func assertLabel(t *testing.T, path, name, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	start, end := locateCases(lines, shareOptionName)
	if start < 0 {
		t.Fatalf("未定位到 %s 的 cases 区块", shareOptionName)
	}
	for i := start; i < end; i++ {
		app, ok := caseNameAt(lines[i])
		if !ok || app != name {
			continue
		}
		idx := labelLineIndex(lines, i+1, end)
		if idx < 0 {
			t.Fatalf("case %q 缺少 label", name)
		}
		raw := strings.TrimSuffix(strings.TrimSpace(lines[idx]), ",")
		raw = strings.TrimSpace(strings.TrimPrefix(raw, `"label":`))
		got, _ := unquote(raw)
		if got != want {
			t.Errorf("case %q 的 label = %q, want %q", name, got, want)
		}
		return
	}
	t.Fatalf("未找到 case %q", name)
}
