// Package share 实现「分享到外部 APP」的目标定位。
//
// 设计要点：分享面板的可用 APP 由**游戏客户端区服**决定，与界面语言（简体/繁体）无关：
//   - 国服面板：QQ / 微信 / 微博 / 小红书，作者原实现以「小红书」为锚点做固定偏移；
//   - 台服面板：LINE / Instagram（台服不提供 Facebook），品牌名为拉丁字母，简繁界面下完全一致。
//
// 因此本识别器不读取语言资源，只依据 pretask 写入的运行时区服状态选择对应面板的取点策略。
package share

import (
	"encoding/json"
	"fmt"
	"image"
	"regexp"
	"strings"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/TianQuanDiWen/MaNikki4/agent/internal/state"
)

// Offset 描述相对锚点框的 [x, y, w, h] 偏移，与 pipeline 的 target_offset 语义一致。
type Offset [4]int

// Target 描述某个分享目标在当前面板上的取点方式。
// Anchor 为空表示直接点击命中的文本本身；否则以 Anchor 为锚点再叠加 Offset。
type Target struct {
	Labels []string `json:"labels,omitempty"` // 目标自身文本的候选正则，按优先级排列
	Anchor string   `json:"anchor,omitempty"` // 锚点文本正则，为空则不使用锚点
	Offset Offset   `json:"offset,omitempty"` // 仅在 Anchor 非空时生效
}

// serverPanel 描述一个区服的分享面板布局。
type serverPanel struct {
	DefaultApp string
	Targets    map[string]Target
}

// panels 是各区服分享面板的取点表。
// 国服沿用原流水线的「小红书锚点 + 固定偏移」，保证既有行为零回归。
var panels = map[string]serverPanel{
	"CN": {
		DefaultApp: "QQ",
		Targets: map[string]Target{
			"QQ":          {Labels: []string{"小红书"}, Anchor: `小红书`, Offset: Offset{-72, 3, -27, -4}},
			"WeChat":      {Labels: []string{"小红书"}, Anchor: `小红书`, Offset: Offset{-327, 2, -40, 0}},
			"Weibo":       {Labels: []string{"小红书"}, Anchor: `小红书`, Offset: Offset{105, 0, -34, -1}},
			"Xiaohongshu": {Labels: []string{"小红书"}, Anchor: `小红书`, Offset: Offset{0, 0, 0, 0}},
		},
	},
	"TW": {
		// 台服面板只提供 LINE 与 Instagram，且均为拉丁品牌名，简繁界面完全一致。
		DefaultApp: "LINE",
		Targets: map[string]Target{
			"LINE":      {Labels: []string{`(?i)^line$`, `(?i)\bline\b`}},
			"Instagram": {Labels: []string{`(?i)instagram`, `(?i)ins[a-z]*gram`, `(?i)^ig$`}},
		},
	},
}

// AppLabels 返回某区服下面板里出现的全部品牌名，用于日志与提示。
func AppLabels(server string) []string {
	panel, ok := panels[normalizeServer(server)]
	if !ok {
		return nil
	}
	names := make([]string, 0, len(panel.Targets))
	for name := range panel.Targets {
		names = append(names, name)
	}
	return names
}

// ResolveTarget 依据区服与期望 APP 解析实际取点策略。
// 当期望 APP 在当前区服面板上不存在时，回退到该区服的默认 APP 并返回 substituted=true，
// 从而实现「分享目标跟随实际启动的客户端」而不依赖用户额外切换。
func ResolveTarget(server, app string) (target Target, resolvedApp string, substituted bool, ok bool) {
	panel, ok := panels[normalizeServer(server)]
	if !ok {
		return Target{}, "", false, false
	}
	if key, exists := lookupApp(panel, app); exists {
		return panel.Targets[key], key, false, true
	}
	t, exists := panel.Targets[panel.DefaultApp]
	if !exists {
		return Target{}, "", false, false
	}
	return t, panel.DefaultApp, true, true
}

// lookupApp 以大小写不敏感的方式在面板中查找 APP，返回规范化的 case name。
// MXU 传入的取值大小写可能与 interface.json 中声明的不一致，故需归一。
func lookupApp(panel serverPanel, app string) (string, bool) {
	if app == "" {
		return "", false
	}
	if _, exists := panel.Targets[app]; exists {
		return app, true
	}
	for key := range panel.Targets {
		if strings.EqualFold(key, app) {
			return key, true
		}
	}
	return "", false
}

func normalizeServer(server string) string {
	server = strings.ToUpper(strings.TrimSpace(server))
	if server == "" {
		return "CN"
	}
	return server
}

// ShareTargetParam 是 ShareTarget 自定义识别器的配置参数。
// App 取值与 interface.json 中 ShareAppOption 的 case name 一致。
type ShareTargetParam struct {
	App string `json:"app"`
}

// NewShareTargetRunner 创建分享目标定位识别器。
// 区服来源为 pretask 写入的运行时状态；识别结果返回待点击区域，由节点的 Click 动作消费。
func NewShareTargetRunner(projectRoot string) maa.CustomRecognitionRunner {
	return maa.CustomRecognitionFunc(func(ctx *maa.Context, arg *maa.CustomRecognitionArg) (*maa.CustomRecognitionResult, bool) {
		if arg == nil || arg.Img == nil {
			return nil, false
		}

		param, err := parseShareTargetParam(arg.CustomRecognitionParam)
		if err != nil {
			fmt.Printf("[ShareTarget] 解析参数失败: %v, param=%s\n", err, arg.CustomRecognitionParam)
			return nil, false
		}
		if param.App == "" {
			fmt.Println("[ShareTarget] 错误: app 参数为空")
			return nil, false
		}

		st := state.Load(projectRoot)
		target, resolvedApp, substituted, ok := ResolveTarget(st.Server, param.App)
		if !ok {
			fmt.Printf("[ShareTarget] 未知区服 %q，无法定位分享目标\n", st.Server)
			return nil, false
		}
		if substituted {
			fmt.Printf("[ShareTarget] 提示: 当前为%s面板（可选 %s），期望的 %s 不可用，已自动改用 %s\n",
				st.Server, strings.Join(AppLabels(st.Server), " / "), param.App, resolvedApp)
		} else {
			fmt.Printf("[ShareTarget] 区服 %s，分享目标 %s\n", st.Server, resolvedApp)
		}

		// 目标 APP 未在模拟器内安装时，改用本区服面板上另一个已安装的应用
		if len(st.ShareApps) > 0 {
			if alt, found := PreferInstalled(st.Server, resolvedApp, installedSet(st.ShareApps)); found && alt != resolvedApp {
				fmt.Printf("[ShareTarget] 提示: %s 在模拟器内未安装，已改用 %s（请在模拟器中安装 %s 后即可分享到它）\n",
					resolvedApp, alt, resolvedApp)
				resolvedApp = alt
				if t, _, _, ok2 := ResolveTarget(st.Server, alt); ok2 {
					target = t
				}
			}
		}

		server := st.Server

		roi := arg.Roi
		if roi[2] <= 0 || roi[3] <= 0 {
			b := arg.Img.Bounds()
			roi = maa.Rect{0, 0, b.Dx(), b.Dy()}
		}

		// 先在节点声明的 ROI 内识别；台服面板布局可能与国服略有差异，故失败时再放宽到下半屏重试
		for _, searchRoi := range candidateRois(roi, arg.Img) {
			reco, err := ctx.RunRecognitionDirect(
				maa.RecognitionTypeOCR,
				&maa.OCRParam{ROI: maa.NewTargetRect(searchRoi)},
				arg.Img,
			)
			if err != nil || reco == nil || !reco.Hit || reco.Results == nil || len(reco.Results.All) == 0 {
				continue
			}

			box, text, found := matchTarget(reco, target)
			if !found {
				continue
			}

			fmt.Printf("[ShareTarget] 命中 %q -> box=%v (roi=%v)\n", text, box, searchRoi)
			return &maa.CustomRecognitionResult{
				Box:    box,
				Detail: fmt.Sprintf(`{"server":%q,"app":%q,"text":%q}`, server, resolvedApp, text),
			}, true
		}

		fmt.Printf("[ShareTarget] 未在分享面板上找到 %s（区服 %s）\n", resolvedApp, server)
		return nil, false
	})
}

// installedSet 把运行时状态里记录的「已安装 APP 列表」还原成集合。
func installedSet(apps []string) map[string]bool {
	set := make(map[string]bool, len(apps))
	for _, app := range apps {
		if app != "" {
			set[app] = true
		}
	}
	return set
}

// candidateRois 给出识别区域候选：节点自身 ROI 优先，其后放宽到画面下半部分。
func candidateRois(roi maa.Rect, img image.Image) []maa.Rect {
	shown := []maa.Rect{roi}
	if img != nil {
		b := img.Bounds()
		w, h := b.Dx(), b.Dy()
		widened := maa.Rect{0, h / 2, w, h - h/2}
		if roi != widened {
			shown = append(shown, widened)
		}
	}
	return shown
}

// matchTarget 在 OCR 结果中按目标策略计算点击区域。
func matchTarget(reco *maa.RecognitionDetail, target Target) (box maa.Rect, text string, ok bool) {
	if target.Anchor != "" {
		anchorRegex, err := regexp.Compile(target.Anchor)
		if err != nil {
			return maa.Rect{}, "", false
		}
		for _, res := range reco.Results.All {
			ocrRes, ok := res.AsOCR()
			if !ok || ocrRes == nil || ocrRes.Text == "" {
				continue
			}
			if !anchorRegex.MatchString(ocrRes.Text) && !anchorRegex.MatchString(strings.ReplaceAll(ocrRes.Text, " ", "")) {
				continue
			}
			return applyOffset(ocrRes.Box, target.Offset), ocrRes.Text, true
		}
		return maa.Rect{}, "", false
	}

	for _, pattern := range target.Labels {
		labelRegex, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		for _, res := range reco.Results.All {
			ocrRes, ok := res.AsOCR()
			if !ok || ocrRes == nil || ocrRes.Text == "" {
				continue
			}
			clean := strings.ReplaceAll(ocrRes.Text, " ", "")
			if labelRegex.MatchString(ocrRes.Text) || labelRegex.MatchString(clean) {
				return ocrRes.Box, ocrRes.Text, true
			}
		}
	}
	return maa.Rect{}, "", false
}

// applyOffset 把 target_offset 语义的偏移叠加到识别框上。
func applyOffset(box maa.Rect, offset Offset) maa.Rect {
	return maa.Rect{
		box[0] + offset[0],
		box[1] + offset[1],
		box[2] + offset[2],
		box[3] + offset[3],
	}
}

// parseShareTargetParam 兼容裸 JSON 与被二次编码成字符串的参数形态。
func parseShareTargetParam(raw string) (ShareTargetParam, error) {
	var param ShareTargetParam
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return param, nil
	}
	if strings.HasPrefix(raw, "\"") && strings.HasSuffix(raw, "\"") {
		var unquoted string
		if err := json.Unmarshal([]byte(raw), &unquoted); err == nil {
			raw = strings.TrimSpace(unquoted)
		}
	}
	if err := json.Unmarshal([]byte(raw), &param); err != nil {
		return ShareTargetParam{}, err
	}
	return param, nil
}
