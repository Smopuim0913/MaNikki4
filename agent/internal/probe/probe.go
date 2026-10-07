// Package probe 实现界面启动前的轻量设备探测。
//
// 目的：MXU 读取 assets/interface.json 后才渲染选项下拉框，而「哪些分享 APP 已安装」
// 只有在模拟器运行时才能知道。因此在启动界面之前先跑一次探测，把未安装的条目
// 标注为「XXX (未安装)」，用户点开下拉框就能一眼看出该选哪个。
package probe

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TianQuanDiWen/MaNikki4/agent/internal/emulator"
	"github.com/TianQuanDiWen/MaNikki4/agent/internal/share"
)

// Run 执行探测并把结果写回界面清单。设备不可用时只清理标记，不视为失败。
func Run(args []string) error {
	flags := flag.NewFlagSet("probe", flag.ContinueOnError)
	root := flags.String("root", ".", "project root (repo root, containing assets/)")
	if err := flags.Parse(args); err != nil {
		return err
	}

	interfacePath, err := locateInterface(*root)
	if err != nil {
		return err
	}

	// 区服来自用户的持久化选择，与设备是否在线无关，优先解析以便始终标注下拉框。
	server := share.SelectedServer(*root)
	zone := share.ZoneLabelName(server)
	fmt.Printf("[Probe] 当前游戏区服: %s\n", zone)

	adbExe, adbAddress, ok := emulator.ProbeADBEnv(*root)
	if !ok {
		_, _ = share.AnnotateInterface(interfacePath, share.AnnotateOptions{Server: server})
		fmt.Printf("[Probe] 未找到可用的 adb.exe，跳过分享目标安装检测（已按%s标注分享目标）\n", zone)
		return nil
	}

	if !emulator.IsDeviceOnline(adbExe, adbAddress) {
		// 模拟器尚未启动：清理上次留下的标记，只保留区服相关的标注
		if changed, err := share.AnnotateInterface(interfacePath, share.AnnotateOptions{Server: server}); err == nil && changed {
			fmt.Println("[Probe] 模拟器未连接，已清除分享目标的「(未安装)」标记")
		}
		fmt.Printf("[Probe] 模拟器未连接 (%s)，跳过分享目标检测；启动模拟器后重新打开界面即可看到标记\n", adbAddress)
		return nil
	}

	installedPkgs := emulator.ListInstalledPackages(adbExe, adbAddress)
	installed := share.DetectInstalled(installedPkgs)
	changed, err := share.AnnotateInterface(interfacePath, share.AnnotateOptions{Installed: installed, Server: server})
	if err != nil {
		return err
	}

	present, missing := summarize(installed, server)
	fmt.Printf("[Probe] 已安装: %s\n", joinOrNone(present))
	fmt.Printf("[Probe] 未安装: %s\n", joinOrNone(missing))
	if changed {
		fmt.Println("[Probe] 界面清单已更新：未安装的分享目标标注「(未安装)」")
	} else {
		fmt.Println("[Probe] 界面清单无需更新")
	}
	return nil
}

// summarize 按安装情况给当前区服面板内的分享目标分组。
// 下拉框始终列出全部 APP，因此这里只统计当前区服面板内的安装状态。
func summarize(installed map[string]bool, server string) (present, missing []string) {
	for _, app := range share.Apps() {
		if !share.AppOnPanel(server, app) {
			continue
		}
		if installed[app] {
			present = append(present, share.DisplayName(app))
		} else {
			missing = append(missing, share.DisplayName(app)+share.InstalledSuffix)
		}
	}
	return present, missing
}

func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "（无）"
	}
	return strings.Join(items, "、")
}

// locateInterface 在源码目录与发布目录中定位界面清单。
func locateInterface(root string) (string, error) {
	candidates := []string{
		filepath.Join(root, "assets", "interface.json"),
		filepath.Join(root, "interface.json"),
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("未找到 interface.json（已查找 %s）", strings.Join(candidates, ", "))
}
