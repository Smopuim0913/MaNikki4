package emulator

import (
	"encoding/json"
	"testing"
)

func TestStripJSONComments(t *testing.T) {
	input := `{
		// 这是整行单行注释
		"url": "https://example.com/api/v1//test",
		"path": "C:\\Program Files//test\\app.exe", // 行尾注释
		"normal": 123
	}`

	stripped := stripJSONComments([]byte(input))

	var data struct {
		URL    string `json:"url"`
		Path   string `json:"path"`
		Normal int    `json:"normal"`
	}

	if err := json.Unmarshal(stripped, &data); err != nil {
		t.Fatalf("json.Unmarshal failed: %v\nStripped content:\n%s", err, string(stripped))
	}

	if data.URL != "https://example.com/api/v1//test" {
		t.Errorf("expected URL to be preserved, got %q", data.URL)
	}
	if data.Path != "C:\\Program Files//test\\app.exe" {
		t.Errorf("expected Path to be preserved, got %q", data.Path)
	}
	if data.Normal != 123 {
		t.Errorf("expected Normal to be 123, got %d", data.Normal)
	}
}

func TestPickPackageName(t *testing.T) {
	bothInstalled := []string{
		"com.android.shell",
		"com.papegames.nn4.cn",
		"com.shining.nikki4.tw",
	}
	twOnly := []string{"com.android.shell", "com.shining.nikki4.tw"}
	twGooglePlay := []string{"com.papegames.nn4.tw"}
	channelOnly := []string{"com.papegames.nn4.mi"}
	empty := []string{}

	cases := []struct {
		name     string
		install  []string
		prefer   string
		expected string
	}{
		{"双区服共存未指定时默认国服", bothInstalled, "", "com.papegames.nn4.cn"},
		{"双区服共存指定台服", bothInstalled, "TW", "com.shining.nikki4.tw"},
		{"双区服共存指定国服", bothInstalled, "CN", "com.papegames.nn4.cn"},
		{"仅台服官网版且未指定", twOnly, "", "com.shining.nikki4.tw"},
		{"台服 Google Play 版", twGooglePlay, "TW", "com.papegames.nn4.tw"},
		{"渠道服回退启发式", channelOnly, "", "com.papegames.nn4.mi"},
		{"指定区服未安装时回退到已安装客户端", twOnly, "CN", "com.shining.nikki4.tw"},
		{"空列表回退默认包名", empty, "", defaultPackageName},
	}
	for _, c := range cases {
		if got := pickPackageName(c.install, c.prefer); got != c.expected {
			t.Errorf("%s: expected %q, got %q", c.name, c.expected, got)
		}
	}
}

func TestInstalledServerLabels(t *testing.T) {
	labels := installedServerLabels([]string{
		"com.papegames.nn4.cn",
		"com.shining.nikki4.tw",
		"com.papegames.nn4.tw",
	})
	if len(labels) != 2 {
		t.Fatalf("expected 2 deduplicated server labels, got %v", labels)
	}
	if labels[0] != "国服" || labels[1] != "台服" {
		t.Errorf("unexpected labels: %v", labels)
	}
}

func TestParseLauncherActivity(t *testing.T) {
	output := "priority=0 preferredOrder=0 match=0x108000 specificIndex=-1 isDefault=false\ncom.shining.nikki4.tw/com.nikki.nn4lib.NN4PlayerActivity\n"
	if got := parseLauncherActivity(output, "com.shining.nikki4.tw"); got != "com.nikki.nn4lib.NN4PlayerActivity" {
		t.Errorf("expected NN4PlayerActivity, got %q", got)
	}
	if got := parseLauncherActivity("No activity found\n", "com.shining.nikki4.tw"); got != "" {
		t.Errorf("expected empty on unresolved activity, got %q", got)
	}
	if got := parseLauncherActivity(output, "com.papegames.nn4.cn"); got != "" {
		t.Errorf("expected empty for mismatched package, got %q", got)
	}
}

func TestExtractServerFromJSON(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		expected string
	}{
		{"裸字符串取值", `{"ServerOption":"TW","MuMuConfig":{"mumu_path":"D:/x"}}`, "TW"},
		{"对象形态取值", `{"ServerOption":{"name":"TW"}}`, "TW"},
		{"嵌套在 option 下", `{"option":{"ServerOption":"CN"}}`, "CN"},
		{"中文别名", `{"ServerOption":"台服"}`, "TW"},
		{"MXU select 实际形态 caseName", `{"MuMuConfig":{"type":"input","values":{"mumu_path":""}},"ServerOption":{"type":"select","caseName":"TW"}}`, "TW"},
		{"MXU select 默认国服", `{"ServerOption":{"type":"select","caseName":"CN"}}`, "CN"},
		{"MXU select 中文 caseName", `{"ServerOption":{"type":"select","caseName":"台服"}}`, "TW"},
		{"历史 switch+values 形态", `{"ServerOption":{"type":"switch","values":{"name":"TW"}}}`, "TW"},
		{"MXU 形态 values 内为 case", `{"ServerOption":{"type":"switch","values":{"case":"CN"}}}`, "CN"},
		{"MXU 形态 values 为裸串", `{"ServerOption":{"type":"switch","values":"TW"}}`, "TW"},
		{"缺失区服字段", `{"MuMuConfig":{"mumu_path":"D:/x"}}`, ""},
		{"非 JSON 输入", `D:/MuMu/MuMuNxMain.exe`, ""},
		{"损坏 JSON", `{not json`, ""},
	}
	for _, c := range cases {
		if got := extractServerFromJSON(c.raw); got != c.expected {
			t.Errorf("%s: expected %q, got %q", c.name, c.expected, got)
		}
	}
}

func TestParseOtherRunningInstances(t *testing.T) {
	// 单实例 JSON（仅 0 号机在运行）
	singleRunning := []byte(`{
		"index": "0",
		"is_process_started": true,
		"pid": 1234
	}`)
	if parseOtherRunningInstances(singleRunning, 0) {
		t.Errorf("expected false for single running instance when querying same index")
	}
	if !parseOtherRunningInstances(singleRunning, 1) {
		t.Errorf("expected true when querying different index from running single instance")
	}

	// 数组形式多实例（0 号和 1 号，其中 1 号在运行）
	multiRunning := []byte(`[
		{"index": "0", "is_process_started": false, "pid": 0},
		{"index": "1", "is_process_started": true, "pid": 5678}
	]`)
	if !parseOtherRunningInstances(multiRunning, 0) {
		t.Errorf("expected true for index 0 when index 1 is running")
	}
	if parseOtherRunningInstances(multiRunning, 1) {
		t.Errorf("expected false for index 1 when only index 1 is running")
	}

	// 数组形式多实例（所有实例均未运行）
	noneRunning := []byte(`[
		{"index": "0", "is_process_started": false, "pid": 0},
		{"index": "1", "is_process_started": false, "pid": 0}
	]`)
	if parseOtherRunningInstances(noneRunning, 0) {
		t.Errorf("expected false when no instances are running")
	}

	// 数值类型 index 兼容性测试（"index": 0 为 number 而非 string）
	numericRunning := []byte(`[
		{"index": 0, "is_process_started": false, "pid": 0},
		{"index": 1, "is_process_started": true, "pid": 5678}
	]`)
	if !parseOtherRunningInstances(numericRunning, 0) {
		t.Errorf("expected true for numeric index 0 when numeric index 1 is running")
	}
	if parseOtherRunningInstances(numericRunning, 1) {
		t.Errorf("expected false for numeric index 1 when only numeric index 1 is running")
	}

	// 异常数据 Fail-Safe 保守测试（损坏 JSON 应默认返回 true 保护多开）
	corrupted := []byte(`invalid json data`)
	if !parseOtherRunningInstances(corrupted, 0) {
		t.Errorf("expected true (Fail-Safe) when parsing corrupted JSON")
	}
}
