package share

import (
	"encoding/json"
	"testing"
)

// TestResolveTarget 覆盖「分享目标跟随区服」的核心约定：
// 国服面板只有 QQ/微信/微博/小红书，台服面板只有 LINE/Instagram（不含 Facebook）。
func TestResolveTarget(t *testing.T) {
	cases := []struct {
		name            string
		server          string
		app             string
		wantApp         string
		wantSubstituted bool
		wantOK          bool
	}{
		{"国服默认QQ", "CN", "QQ", "QQ", false, true},
		{"国服小红书", "CN", "Xiaohongshu", "Xiaohongshu", false, true},
		{"国服选LINE则回退QQ", "CN", "LINE", "QQ", true, true},
		{"台服选LINE", "TW", "LINE", "LINE", false, true},
		{"台服选Instagram", "TW", "Instagram", "Instagram", false, true},
		{"台服选QQ则回退LINE", "TW", "QQ", "LINE", true, true},
		{"台服选Facebook则回退LINE", "TW", "Facebook", "LINE", true, true},
		{"未知区服", "JP", "LINE", "", false, false},
		{"空区服按国服处理", "", "Weibo", "Weibo", false, true},
		{"大小写不敏感", "tw", "instagram", "Instagram", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			target, gotApp, substituted, ok := ResolveTarget(c.server, c.app)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if !ok {
				return
			}
			if gotApp != c.wantApp {
				t.Errorf("app = %q, want %q", gotApp, c.wantApp)
			}
			if substituted != c.wantSubstituted {
				t.Errorf("substituted = %v, want %v", substituted, c.wantSubstituted)
			}
			_ = target
		})
	}
}

// TestTaiwanPanelHasNoFacebook 固化「台服不提供 Facebook」这一事实。
func TestTaiwanPanelHasNoFacebook(t *testing.T) {
	if _, _, _, ok := ResolveTarget("TW", "Facebook"); !ok {
		t.Fatal("TW 面板解析失败")
	}
	target, app, substituted, _ := ResolveTarget("TW", "Facebook")
	if !substituted || app != "LINE" {
		t.Fatalf("Facebook 应被替换: app=%q substituted=%v", app, substituted)
	}
	if target.Labels == nil {
		t.Error("回退目标缺少标签")
	}
}

// TestParseShareTargetParam 覆盖裸 JSON 与被二次编码为字符串两种形态。
func TestParseShareTargetParam(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{`{"app":"LINE"}`, "LINE"},
		{`"{\"app\":\"Instagram\"}"`, "Instagram"},
		{``, ""},
		{`null`, ""},
	}
	for _, c := range cases {
		got, err := parseShareTargetParam(c.raw)
		if err != nil {
			t.Fatalf("parse %q: %v", c.raw, err)
		}
		if got.App != c.want {
			t.Errorf("parse %q -> %q, want %q", c.raw, got.App, c.want)
		}
	}
}

// TestApplyOffset 校验与 pipeline target_offset 一致的偏移语义。
func TestApplyOffset(t *testing.T) {
	box := [4]int{100, 200, 50, 40}
	got := applyOffset(box, Offset{-72, 3, -27, -4})
	want := [4]int{28, 203, 23, 36}
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestAppLabels 保证台服面板的可选品牌名与文档描述一致。
func TestAppLabels(t *testing.T) {
	labels := AppLabels("TW")
	encoded, _ := json.Marshal(labels)
	t.Logf("台服可选: %s", encoded)
	if len(labels) != 2 {
		t.Fatalf("台服应有 2 个分享目标，实际 %v", labels)
	}
}
