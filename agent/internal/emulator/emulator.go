package emulator

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/TianQuanDiWen/MaNikki4/agent/internal/runtimepath"
	"github.com/TianQuanDiWen/MaNikki4/agent/internal/share"
)

const (
	defaultPackageName = "com.papegames.nn4.cn"
	defaultActivity    = "com.nikki.nn4lib.NN4PlayerActivity"
	defaultADBAddress  = "127.0.0.1:16384"
)

// serverDef 描述一个区服及其发行渠道包名（按探测优先级排列）。
type serverDef struct {
	ID       string
	Label    string
	Packages []string
}

// knownServers 列出已知区服。台服存在两个发行渠道：
//   - com.shining.nikki4.tw：台服官网直下版（单 APK，大陆玩家主要渠道）
//   - com.papegames.nn4.tw：Google Play 版（主包 + OBB，需谷歌三件套）
var knownServers = []serverDef{
	{ID: "CN", Label: "国服", Packages: []string{"com.papegames.nn4.cn"}},
	{ID: "TW", Label: "台服", Packages: []string{"com.shining.nikki4.tw", "com.papegames.nn4.tw"}},
}

var knownExecutables = []string{
	"MuMuNxMain.exe",
	"MuMuPlayer.exe",
	"MuMuManager.exe",
	"NemuPlayer.exe",
}

// Run 启动供 MXU Controller 前置执行的预任务。
func Run(args []string) error {
	flags := flag.NewFlagSet("pretask", flag.ContinueOnError)
	root := flags.String("root", ".", "project root containing maafw and resource")
	if err := flags.Parse(args); err != nil {
		return err
	}

	paths, err := runtimepath.Resolve(*root)
	if err != nil {
		return fmt.Errorf("resolve runtime path: %w", err)
	}

	// 1. 读取既有配置中定义的 ADB 端口、路径与实例序号
	cfgADBAddress, cfgADBPath, cfgMuMuPath, vmIndex := loadConfig(paths.Root)
	adbAddress := defaultADBAddress
	if cfgADBAddress != "" {
		adbAddress = cfgADBAddress
	}

	// 2. 解析 MXU 传入的 option 参数
	var inputPath, inputServer string
	remaining := flags.Args()
	if len(remaining) > 0 {
		inputPath = extractMuMuPathFromJSON(remaining[len(remaining)-1])
		inputServer = extractServerFromJSON(remaining[len(remaining)-1])
	}
	if inputPath == "" {
		inputPath = cfgMuMuPath
	}
	// pretask 参数缺失时回退到 MXU 已保存的配置（兼容不同版本 Client 的序列化差异）
	if inputServer == "" {
		inputServer = loadConfiguredServer(paths.Root)
	}
	if inputServer == "" && len(remaining) > 0 {
		fmt.Printf("[Emulator] 未解析到区服选项，改用自动探测。原始 option 参数: %s\n", remaining[len(remaining)-1])
	}

	// 3. 定位 MuMu 路径（显式输入 -> 正在运行 -> 注册表关联 -> 默认目录 -> 置顶弹窗）
	mumuPath, wasAutoDetected, err := resolveMuMuPath(inputPath, true)
	if err != nil {
		return fmt.Errorf("定位 MuMu 模拟器失败: %w", err)
	}
	fmt.Printf("[Emulator] 确认 MuMu 路径: %s\n", mumuPath)

	// 4. 自动检测或弹窗选择时写回配置供 UI 回显
	if wasAutoDetected || inputPath == "" {
		if err := savePathToConfig(paths.Root, mumuPath); err != nil {
			fmt.Printf("[Emulator] 警告: 写回配置文件失败: %v\n", err)
		} else {
			fmt.Println("[Emulator] 模拟器路径已写入 MXU 配置文件")
		}
	}

	// 5. 定位 adb.exe
	adbExe := resolveADBPath(paths.Lib, mumuPath, cfgADBPath)
	if adbExe == "" {
		return errors.New("未找到可用的 adb.exe，请检查环境")
	}

	// 6. 确保模拟器已启动并且 Android 系统完全就绪
	if err := ensureEmulatorRunning(mumuPath, adbExe, adbAddress, vmIndex); err != nil {
		return fmt.Errorf("启动/连接模拟器失败: %w", err)
	}

	// 7. 拉起闪耀暖暖并确保其运行就绪
	pkg, err := launchGameApp(mumuPath, adbExe, adbAddress, vmIndex, inputServer)
	if err != nil {
		return fmt.Errorf("拉起游戏应用失败: %w", err)
	}

	// 8. 提前校验分享任务的目标 APP：未安装直接中止，省得整轮跑完才发现分享不出去
	installed := ListInstalledPackages(adbExe, adbAddress)
	serverID := ServerIDOfPackage(pkg)
	if err := checkShareAppSelection(paths.Root, serverID, installed); err != nil {
		return err
	}

	// 9. 把实际拉起的区服、包名与已安装的分享目标共享给运行期 Agent（分享逻辑据此联动）
	shareApps := installedShareApps(installed)
	if err := SaveRuntimeState(paths.Root, serverID, pkg, shareApps); err != nil {
		fmt.Printf("[Emulator] 警告: 写入运行时区服状态失败: %v\n", err)
	} else {
		fmt.Printf("[Emulator] 区服状态已记录: %s (%s)\n", serverID, pkg)
		if len(shareApps) > 0 {
			fmt.Printf("[Emulator] 模拟器内已安装的分享目标: %s\n", strings.Join(displayShareApps(shareApps), "、"))
		}
	}

	fmt.Println("[Emulator] 启动准备完成，无缝移交 Controller")
	return nil
}

// installedShareApps 把设备上的包名列表折算成已安装的分享目标 APP 名称。
func installedShareApps(installed []string) []string {
	flags := share.DetectInstalled(installed)
	apps := make([]string, 0, len(share.Apps()))
	for _, app := range share.Apps() {
		if flags[app] {
			apps = append(apps, app)
		}
	}
	return apps
}

func displayShareApps(apps []string) []string {
	out := make([]string, 0, len(apps))
	for _, app := range apps {
		out = append(out, share.DisplayName(app))
	}
	return out
}

// checkShareAppSelection 在任务开始前校验「分享」任务所选目标是否真的可用。
// 勾选了分享却选了一个模拟器里没装的 APP 时直接中止，避免整轮跑完才发现分享不出去。
func checkShareAppSelection(projectRoot, serverID string, installed []string) error {
	selected, found := share.SelectedApp(projectRoot)
	if !found || selected == "" {
		return nil
	}
	display := share.DisplayName(selected)
	if !share.DetectInstalled(installed)[selected] {
		pkg, _ := share.PackageName(selected)
		installedApps := displayShareApps(installedShareApps(installed))
		suggestion := "模拟器内当前没有任何可用的分享目标，请先安装一个（QQ / 微信 / 微博 / 小红书 / LINE / Instagram）"
		if len(installedApps) > 0 {
			suggestion = "可改为 " + strings.Join(installedApps, "、")
		}
		return fmt.Errorf(
			"「分享」任务选择的 %s 尚未在模拟器内安装（包名 %s）。\n请在模拟器中安装该 APP，或在「分享」任务里把「分享目标APP」%s 后再开始",
			display, pkg, suggestion)
	}
	// 所选 APP 不属于当前区服面板时不在此中止：下拉框始终列出全部 APP，
	// 运行期 ShareTarget 识别器会依据运行时区服状态自动改用本服可用项（见 share/target.go）。
	return nil
}

// extractMuMuPathFromJSON 从 MXU 传入的 JSON 字符串中提取 mumu_path
func extractMuMuPathFromJSON(rawJSON string) string {
	if !strings.HasPrefix(strings.TrimSpace(rawJSON), "{") {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &data); err != nil {
		return ""
	}
	if val, ok := data["mumu_path"].(string); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	if sub, ok := data["MuMuConfig"].(map[string]any); ok {
		if val, ok := sub["mumu_path"].(string); ok && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

// extractServerFromJSON 从 MXU 传入的 JSON 字符串中提取区服选项（ServerOption）。
// 不同版本 Client 的取值形态可能是裸字符串、{"name": "TW"} 或嵌套在 option 下，这里统一兼容。
func extractServerFromJSON(rawJSON string) string {
	if !strings.HasPrefix(strings.TrimSpace(rawJSON), "{") {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &data); err != nil {
		return ""
	}
	if server := normalizeServerValue(data["ServerOption"]); server != "" {
		return server
	}
	if sub, ok := data["option"].(map[string]any); ok {
		return normalizeServerValue(sub["ServerOption"])
	}
	return ""
}

// normalizeServerValue 把任意形态的区服取值归一化为内部 ID（CN / TW）。
// MXU 对 select 选项的实际序列化为 {选项名: {"type": "select", "caseName": "TW"}}，
// 对 input 为 {"type": "input", "values": {...}}，故需同时兼容 caseName 与递归下探 values。
func normalizeServerValue(value any) string {
	return normalizeServerValueDepth(value, 0)
}

func normalizeServerValueDepth(value any, depth int) string {
	if depth > 4 {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return normalizeServerToken(typed)
	case map[string]any:
		for _, key := range []string{"caseName", "name", "value", "case", "selected", "id", "values"} {
			if nested, ok := typed[key]; ok {
				if server := normalizeServerValueDepth(nested, depth+1); server != "" {
					return server
				}
			}
		}
	}
	return ""
}

// normalizeServerToken 识别常见区服别名，无法识别时返回空串。
func normalizeServerToken(token string) string {
	switch strings.ToUpper(strings.TrimSpace(token)) {
	case "TW", "TAIWAN", "ZH_TW", "CN_TW", "台服", "台灣", "台湾", "台港澳":
		return "TW"
	case "CN", "MAINLAND", "ZH_CN", "国服", "官服", "大陆":
		return "CN"
	}
	return ""
}

// resolveMuMuPath 通用解析 MuMu 可执行文件绝对路径。allowDialog 控制在所有自动探测未命中时是否呼出 WinForms 置顶弹窗。
func resolveMuMuPath(inputPath string, allowDialog bool) (string, bool, error) {
	if inputPath != "" {
		clean := filepath.Clean(inputPath)
		if fileExists(clean) {
			return clean, false, nil
		}
		if exe := searchKnownExeInDir(clean); exe != "" {
			return exe, false, nil
		}
	}

	if p := findPathFromRunningProcess(); p != "" {
		return p, true, nil
	}
	if p := findPathFromRegistry(); p != "" {
		return p, true, nil
	}
	if p := findPathFromDefaultProgramFiles(); p != "" {
		return p, true, nil
	}

	if !allowDialog {
		return "", false, nil
	}

	fmt.Println("[Emulator] 未自动检测到默认安装路径，正在呼出置顶文件选择框...")
	selected, err := openFileDialog()
	if err != nil || selected == "" {
		return "", false, errors.New("未选择模拟器路径")
	}
	return selected, true, nil
}

// findPathFromRunningProcess 查询系统中正在运行的 MuMu 进程路径
func findPathFromRunningProcess() string {
	psCmd := `Get-Process | Where-Object { $_.ProcessName -match '^(MuMuNxMain|MuMuPlayer|MuMuManager|NemuPlayer)$' } | Select-Object -ExpandProperty Path -First 1`
	cmd := exec.Command("powershell", "-NoProfile", "-Command", psCmd)
	if out, err := cmd.Output(); err == nil {
		p := strings.TrimSpace(string(out))
		if p != "" && fileExists(p) {
			return p
		}
	}
	return ""
}

// findPathFromRegistry 查询 Windows 注册表中的关联指令与安装目录
func findPathFromRegistry() string {
	assocKeys := []string{
		`HKLM\SOFTWARE\Classes\MuMuPlayer.apk\shell\open\command`,
		`HKCU\Software\Classes\MuMuPlayer.apk\shell\open\command`,
		`HKLM\SOFTWARE\Classes\Applications\Nemux.exe\shell\open\command`,
		`HKCU\Software\Classes\Applications\Nemux.exe\shell\open\command`,
	}
	for _, key := range assocKeys {
		if out, err := exec.Command("reg", "query", key, "/ve").Output(); err == nil {
			if exe := extractExeFromCmd(string(out)); exe != "" {
				return exe
			}
		}
	}

	regKeys := []string{
		`HKLM\SOFTWARE\NetEase\MuMuPlayer-12.0`,
		`HKCU\Software\NetEase\MuMuPlayer-12.0`,
		`HKLM\SOFTWARE\NetEase\MuMuPlayer`,
		`HKCU\Software\NetEase\MuMuPlayer`,
	}
	for _, key := range regKeys {
		for _, v := range []string{"InstallDir", "AppPath"} {
			if out, err := exec.Command("reg", "query", key, "/v", v).Output(); err == nil {
				val := parseRegValue(string(out))
				if fileExists(val) {
					return val
				}
				if exe := searchKnownExeInDir(val); exe != "" {
					return exe
				}
			}
		}
	}
	return ""
}

// extractExeFromCmd 从注册表命令串（如 "C:\path\app.exe" -i "%1"）提取有效 exe
func extractExeFromCmd(output string) string {
	val := parseRegValue(output)
	if val == "" {
		return ""
	}
	val = strings.Trim(val, `"`)
	lower := strings.ToLower(val)
	if idx := strings.Index(lower, ".exe"); idx != -1 {
		candidate := strings.Trim(val[:idx+4], `"`)
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

func parseRegValue(output string) string {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && (fields[1] == "REG_SZ" || fields[1] == "REG_EXPAND_SZ") {
			return strings.Join(fields[2:], " ")
		}
	}
	return ""
}

// findPathFromDefaultProgramFiles 检查标准 ProgramFiles 路径
func findPathFromDefaultProgramFiles() string {
	progRoots := []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramW6432")}
	subDirs := []string{`Netease\MuMuPlayer-12.0`, `Netease\MuMu Player 12`, `Netease\MuMuPlayer`, `MuMu\emulator\nemu9`}
	for _, root := range progRoots {
		if root == "" {
			continue
		}
		for _, sub := range subDirs {
			if exe := searchKnownExeInDir(filepath.Join(root, sub)); exe != "" {
				return exe
			}
		}
	}
	return ""
}

// searchFileInDirs 在 baseDir 及其各级子目录/相对路径中探测目标文件
func searchFileInDirs(baseDir, fileName string, relativeDirs ...string) string {
	for _, rel := range append([]string{""}, relativeDirs...) {
		p := filepath.Clean(filepath.Join(baseDir, rel, fileName))
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func searchKnownExeInDir(dir string) string {
	for _, name := range knownExecutables {
		if p := searchFileInDirs(dir, name, "nx_main", "shell", "EmulatorShell"); p != "" {
			return p
		}
	}
	return ""
}

func findMuMuCli(mumuPath string) string {
	return searchFileInDirs(filepath.Dir(mumuPath), "mumu-cli.exe", "..", "nx_main", "../nx_main", "../../nx_main")
}

func resolveADBPath(libDir, mumuPath, cfgADBPath string) string {
	if cfgADBPath != "" && fileExists(cfgADBPath) {
		return cfgADBPath
	}
	if p := searchFileInDirs(filepath.Dir(mumuPath), "adb.exe", "nx_main", "shell", "../nx_main", "../shell"); p != "" {
		return p
	}
	if p := filepath.Join(libDir, "adb.exe"); fileExists(p) {
		return p
	}
	if p, err := exec.LookPath("adb.exe"); err == nil {
		return p
	}
	return ""
}

// openFileDialog 借助轻量 PowerShell WinForms 呼出原生置顶文件选择框（TopMost=true）
func openFileDialog() (string, error) {
	psCmd := `Add-Type -AssemblyName System.Windows.Forms; $f = New-Object System.Windows.Forms.OpenFileDialog; $f.Title = '请选择 MuMu 模拟器主程序 (如 MuMuNxMain.exe 或 MuMuPlayer.exe)'; $f.Filter = 'MuMu可执行程序 (*.exe)|MuMu*.exe;Nemu*.exe;*.exe|所有文件 (*.*)|*.*'; $t = New-Object System.Windows.Forms.Form; $t.TopMost = $true; if ($f.ShowDialog($t) -eq [System.Windows.Forms.DialogResult]::OK) { Write-Output $f.FileName }`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psCmd)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	res := strings.TrimSpace(string(out))
	if res == "" {
		return "", errors.New("用户取消选择模拟器路径")
	}
	return res, nil
}

// ensureEmulatorRunning 确保模拟器启动并在指定端口就绪（严格配置 cmd.Dir 避免闪退）
func ensureEmulatorRunning(mumuPath, adbExe, adbAddress string, vmIndex int) error {
	_ = exec.Command(adbExe, "connect", adbAddress).Run()
	if isEmulatorReady(adbExe, adbAddress) {
		fmt.Println("[Emulator] 检测到 MuMu 模拟器已处于运行就绪状态")
		return nil
	}

	fmt.Println("[Emulator] 正在启动 MuMu 模拟器...")
	cliExe := findMuMuCli(mumuPath)
	if cliExe != "" {
		cmd := exec.Command(cliExe, "control", "-v", fmt.Sprintf("%d", vmIndex), "launch")
		cmd.Dir = filepath.Dir(cliExe)
		_ = cmd.Start()
	} else {
		cmd := exec.Command(mumuPath)
		cmd.Dir = filepath.Dir(mumuPath)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("启动进程失败: %w", err)
		}
	}

	fmt.Printf("[Emulator] 等待模拟器系统与 ADB 就绪 (%s)...\n", adbAddress)
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(1500 * time.Millisecond)
		_ = exec.Command(adbExe, "connect", adbAddress).Run()
		if isEmulatorReady(adbExe, adbAddress) {
			fmt.Println("[Emulator] MuMu 模拟器系统已完全就绪！")
			return nil
		}
	}
	return fmt.Errorf("等待 MuMu 模拟器启动就绪超时 (60s, 目标: %s)", adbAddress)
}

func isEmulatorReady(adbExe, adbAddress string) bool {
	out, err := exec.Command(adbExe, "-s", adbAddress, "get-state").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "device" {
		return false
	}
	bootOut, err := exec.Command(adbExe, "-s", adbAddress, "shell", "getprop", "sys.boot_completed").Output()
	return err == nil && strings.TrimSpace(string(bootOut)) == "1"
}

// IsDeviceOnline 判断指定地址上的设备是否已通过 adb 连接（不要求系统完全启动）。
func IsDeviceOnline(adbExe, adbAddress string) bool {
	out, err := exec.Command(adbExe, "-s", adbAddress, "get-state").CombinedOutput()
	return err == nil && strings.TrimSpace(string(out)) == "device"
}

// ProbeADBEnv 返回可用于探测设备状态的 adb 可执行文件与设备地址。
// 与 pretask 的区别：不启动模拟器、不弹窗，供界面启动前的轻量探测使用。
func ProbeADBEnv(projectRoot string) (adbExe, adbAddress string, ok bool) {
	paths, err := runtimepath.Resolve(projectRoot)
	if err != nil {
		return "", "", false
	}
	cfgADBAddress, cfgADBPath, cfgMuMuPath, _ := loadConfig(paths.Root)
	adbAddress = defaultADBAddress
	if cfgADBAddress != "" {
		adbAddress = cfgADBAddress
	}
	mumuPath := cfgMuMuPath
	if mumuPath == "" {
		if detected, _, err := resolveMuMuPath("", false); err == nil {
			mumuPath = detected
		}
	}
	adbExe = resolveADBPath(paths.Lib, mumuPath, cfgADBPath)
	if adbExe == "" {
		return "", "", false
	}
	return adbExe, adbAddress, true
}

// ProjectRootOf 依据运行路径解析项目根目录，供外部命令复用。
func ProjectRootOf(root string) (string, error) {
	paths, err := runtimepath.Resolve(root)
	if err != nil {
		return "", err
	}
	return paths.Root, nil
}

// ListInstalledPackages 读取设备上已安装的所有包名。
func ListInstalledPackages(adbExe, adbAddress string) []string {
	out, err := exec.Command(adbExe, "-s", adbAddress, "shell", "pm", "list", "packages").Output()
	if err != nil {
		return nil
	}
	var pkgs []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "package:"))
		if line != "" {
			pkgs = append(pkgs, line)
		}
	}
	return pkgs
}

// pickPackageName 在已安装列表中挑选本次要拉起的游戏包名。
// preferredServer 非空时只在该区服的候选包名中挑选，否则按 knownServers 顺序探测。
func pickPackageName(installed []string, preferredServer string) string {
	installedSet := make(map[string]struct{}, len(installed))
	for _, pkg := range installed {
		installedSet[pkg] = struct{}{}
	}

	for _, server := range knownServers {
		if preferredServer != "" && !strings.EqualFold(server.ID, preferredServer) {
			continue
		}
		for _, pkg := range server.Packages {
			if _, ok := installedSet[pkg]; ok {
				return pkg
			}
		}
	}

	// 兜底：渠道服等变体（如 com.papegames.nn4.mi），沿用历史启发式规则
	for _, pkg := range installed {
		if strings.Contains(pkg, "nn4") || strings.Contains(pkg, "nikki4") {
			return pkg
		}
	}
	return defaultPackageName
}

// serverLabel 返回包名对应的区服名称，未知渠道返回"未知渠道"。
func serverLabel(pkg string) string {
	for _, server := range knownServers {
		for _, candidate := range server.Packages {
			if candidate == pkg {
				return server.Label
			}
		}
	}
	return "未知渠道"
}

// serverLabelByID 返回区服 ID 对应的名称，未知 ID 返回空串。
func serverLabelByID(id string) string {
	for _, server := range knownServers {
		if strings.EqualFold(server.ID, id) {
			return server.Label
		}
	}
	return ""
}

// installedServerLabels 返回设备上已安装的全部区服名称（用于双区服共存时提示用户）。
func installedServerLabels(installed []string) []string {
	installedSet := make(map[string]struct{}, len(installed))
	for _, pkg := range installed {
		installedSet[pkg] = struct{}{}
	}
	var labels []string
	for _, server := range knownServers {
		for _, pkg := range server.Packages {
			if _, ok := installedSet[pkg]; ok {
				labels = append(labels, server.Label)
				break
			}
		}
	}
	return labels
}

func isAppRunning(adbExe, adbAddress, pkg string) bool {
	out, err := exec.Command(adbExe, "-s", adbAddress, "shell", "pidof", pkg).Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

// startActivity 通过 am start 拉起指定包名的 Activity。
func startActivity(adbExe, adbAddress, pkg, activity string) error {
	return exec.Command(adbExe, "-s", adbAddress, "shell", "am", "start", "-n", pkg+"/"+activity).Run()
}

// resolveLauncherActivity 运行时查询指定包名的真实 LAUNCHER Activity，查询失败返回空串。
func resolveLauncherActivity(adbExe, adbAddress, pkg string) string {
	out, err := exec.Command(adbExe, "-s", adbAddress, "shell", "cmd", "package", "resolve-activity",
		"--brief", "-c", "android.intent.category.LAUNCHER", pkg).Output()
	if err != nil {
		return ""
	}
	return parseLauncherActivity(string(out), pkg)
}

// parseLauncherActivity 从 resolve-activity --brief 的输出中解析出 Activity 类名。
func parseLauncherActivity(output, pkg string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, pkg+"/") {
			continue
		}
		if activity := strings.TrimPrefix(line, pkg+"/"); activity != "" {
			return activity
		}
	}
	return ""
}

// waitAppRunning 轮询等待目标应用进程出现。
func waitAppRunning(adbExe, adbAddress, pkg string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if isAppRunning(adbExe, adbAddress, pkg) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// launchOrFocusApp 统一启动或将应用激活至前台。
// 降级链：mumu-cli -> am start（内置 Activity）-> am start（运行时解析 Activity）-> monkey
func launchOrFocusApp(mumuPath, adbExe, adbAddress string, vmIndex int, pkg string, allowMonkey bool) {
	if cliExe := findMuMuCli(mumuPath); cliExe != "" {
		cmd := exec.Command(cliExe, "control", "-v", fmt.Sprintf("%d", vmIndex), "app", "launch", "--package", pkg)
		cmd.Dir = filepath.Dir(cliExe)
		if err := cmd.Run(); err == nil && waitAppRunning(adbExe, adbAddress, pkg, 3*time.Second) {
			return
		}
		fmt.Println("[Emulator] mumu-cli 未能确认拉起应用，降级至 ADB 指令...")
	}
	if startActivity(adbExe, adbAddress, pkg, defaultActivity) == nil {
		return
	}
	if activity := resolveLauncherActivity(adbExe, adbAddress, pkg); activity != "" && activity != defaultActivity {
		fmt.Printf("[Emulator] 内置 Activity 失效，改用运行时解析的 %s 重试...\n", activity)
		if startActivity(adbExe, adbAddress, pkg, activity) == nil {
			return
		}
	}
	if allowMonkey {
		_ = exec.Command(adbExe, "-s", adbAddress, "shell", "monkey", "-p", pkg, "1").Run()
	}
}

// launchGameApp 拉起指定区服的游戏客户端，返回实际使用的包名。
func launchGameApp(mumuPath, adbExe, adbAddress string, vmIndex int, preferredServer string) (string, error) {
	installed := ListInstalledPackages(adbExe, adbAddress)
	pkg := pickPackageName(installed, preferredServer)
	fmt.Printf("[Emulator] 目标游戏区服: %s，包名: %s\n", serverLabel(pkg), pkg)

	if labels := installedServerLabels(installed); len(labels) > 1 {
		fmt.Printf("[Emulator] 提示: 检测到同时安装了 %s，本次使用 %s，如需切换请在区服选项中指定\n",
			strings.Join(labels, "、"), serverLabel(pkg))
	}
	if wanted := serverLabelByID(preferredServer); wanted != "" && wanted != serverLabel(pkg) {
		fmt.Printf("[Emulator] 警告: 未检测到%s客户端，实际拉起的是 %s (%s)，请检查区服选项\n",
			wanted, serverLabel(pkg), pkg)
	}

	if isAppRunning(adbExe, adbAddress, pkg) {
		fmt.Println("[Emulator] 检测到游戏进程已在运行，唤起至前台...")
		launchOrFocusApp(mumuPath, adbExe, adbAddress, vmIndex, pkg, false)
		time.Sleep(2 * time.Second)
		return pkg, nil
	}

	fmt.Println("[Emulator] 正在拉起《闪耀暖暖》游戏应用...")
	launchOrFocusApp(mumuPath, adbExe, adbAddress, vmIndex, pkg, true)

	fmt.Println("[Emulator] 等待游戏进程就绪...")
	deadline := time.Now().Add(20 * time.Second)
	retried := false
	startTime := time.Now()
	for time.Now().Before(deadline) {
		time.Sleep(1 * time.Second)
		if isAppRunning(adbExe, adbAddress, pkg) {
			fmt.Println("[Emulator] 游戏进程已确立，预留缓冲移交 Controller...")
			time.Sleep(3 * time.Second)
			return pkg, nil
		}
		if !retried && time.Since(startTime) >= 10*time.Second {
			fmt.Println("[Emulator] 启动用时较长，重新尝试发送拉起指令...")
			launchOrFocusApp(mumuPath, adbExe, adbAddress, vmIndex, pkg, true)
			retried = true
		}
	}
	return "", fmt.Errorf("等待游戏应用启动超时 (20s, 包名: %s)", pkg)
}

// resolveConfigPath 统一管理配置文件优先级探测与目标目录创建
func resolveConfigPath(projectRoot string, ensureDir bool) string {
	candidates := []string{
		filepath.Join(projectRoot, "config", "maa_pi_config.json"),
		filepath.Join(projectRoot, "assets", "config", "maa_pi_config.json"),
	}
	for _, p := range candidates {
		if fileExists(p) {
			return p
		}
	}
	target := candidates[0]
	if ensureDir {
		_ = os.MkdirAll(filepath.Dir(target), 0o755)
	}
	return target
}

// loadConfig 从现有配置文件加载已配置的 ADB 地址、路径、模拟器路径与实例序号
func loadConfig(projectRoot string) (string, string, string, int) {
	cfgPath := resolveConfigPath(projectRoot, false)
	if !fileExists(cfgPath) {
		return "", "", "", 0
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return "", "", "", 0
	}
	var root struct {
		ADB struct {
			Address string `json:"address"`
			ADBPath string `json:"adb_path"`
			Config  struct {
				Extras struct {
					MuMu struct {
						Index int `json:"index"`
					} `json:"mumu"`
				} `json:"extras"`
			} `json:"config"`
		} `json:"adb"`
		Option struct {
			MuMuPath   string `json:"mumu_path"`
			MuMuConfig struct {
				MuMuPath string `json:"mumu_path"`
			} `json:"MuMuConfig"`
		} `json:"option"`
	}
	if err := json.Unmarshal(stripJSONComments(data), &root); err == nil {
		mumuPath := root.Option.MuMuPath
		if mumuPath == "" {
			mumuPath = root.Option.MuMuConfig.MuMuPath
		}
		return strings.TrimSpace(root.ADB.Address), strings.TrimSpace(root.ADB.ADBPath), strings.TrimSpace(mumuPath), root.ADB.Config.Extras.MuMu.Index
	}
	return "", "", "", 0
}

// loadConfiguredServer 从 MXU 配置文件读取已保存的区服选项，作为 pretask 参数缺失时的兜底来源。
func loadConfiguredServer(projectRoot string) string {
	cfgPath := resolveConfigPath(projectRoot, false)
	if !fileExists(cfgPath) {
		return ""
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return ""
	}
	var root struct {
		Option struct {
			ServerOption any `json:"ServerOption"`
		} `json:"option"`
	}
	if err := json.Unmarshal(stripJSONComments(data), &root); err != nil {
		return ""
	}
	return normalizeServerValue(root.Option.ServerOption)
}

// savePathToConfig 写入或更新配置至 maa_pi_config.json
func savePathToConfig(projectRoot, mumuPath string) error {
	targetPath := resolveConfigPath(projectRoot, true)
	root := make(map[string]any)
	if fileExists(targetPath) {
		if data, err := os.ReadFile(targetPath); err == nil {
			_ = json.Unmarshal(stripJSONComments(data), &root)
		}
	}

	optMap, ok := root["option"].(map[string]any)
	if !ok {
		optMap = make(map[string]any)
		root["option"] = optMap
	}
	cfgSub, ok := optMap["MuMuConfig"].(map[string]any)
	if !ok {
		cfgSub = make(map[string]any)
		optMap["MuMuConfig"] = cfgSub
	}
	cfgSub["mumu_path"] = filepath.ToSlash(mumuPath)

	newBytes, err := json.MarshalIndent(root, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(targetPath, newBytes, 0o644)
}

// ShutdownEmulator 安全优雅地关闭 MuMu 模拟器实例，防止残留唤醒且绝不误伤其他多开实例。
// closeLauncherOpt 控制是否在模拟器实例安全退出后直接结束 MuMu 启动器主面板（默认开启 true）。
func ShutdownEmulator(projectRoot string, closeLauncherOpt ...bool) error {
	closeLauncher := true
	if len(closeLauncherOpt) > 0 {
		closeLauncher = closeLauncherOpt[0]
	}

	cfgADBAddress, cfgADBPath, cfgMuMuPath, vmIndex := loadConfig(projectRoot)
	adbAddress := defaultADBAddress
	if cfgADBAddress != "" {
		adbAddress = cfgADBAddress
	}

	mumuPath, _, err := resolveMuMuPath(cfgMuMuPath, false)
	if err != nil {
		return fmt.Errorf("定位 MuMu 模拟器失败: %w", err)
	}
	if mumuPath == "" {
		fmt.Println("[Emulator] 未检测到正在运行或已安装的模拟器，静默跳过关机")
		return nil
	}

	cliExe := findMuMuCli(mumuPath)
	adbExe := resolveADBPath("", mumuPath, cfgADBPath)

	// 1. 获取目标实例初始状态与专属 PID (供定向关机与超时兜底，防止误伤多开)
	var initialInfo mumuVMInfo
	if cliExe != "" {
		info, err := queryVMInfo(cliExe, vmIndex)
		if err == nil {
			initialInfo = info
			if !initialInfo.IsProcessStarted {
				fmt.Printf("[Emulator] 实例 %d 当前未在运行，无需关机\n", vmIndex)
				if adbExe != "" {
					_ = exec.Command(adbExe, "disconnect", adbAddress).Run()
				}
				return nil
			}
		}
	}

	// 2. 发送安全关机指令，确保安卓虚拟机数据正常落盘
	if cliExe != "" {
		fmt.Printf("[Emulator] 正在使用 mumu-cli 发送关机指令 (实例: %d)...\n", vmIndex)
		cmd := exec.Command(cliExe, "control", "-v", fmt.Sprintf("%d", vmIndex), "shutdown")
		cmd.Dir = filepath.Dir(cliExe)
		_ = cmd.Run()
	} else if adbExe != "" {
		// 降级方案：若未找到 mumu-cli，尝试通过 ADB 发送软关机指令通知安卓系统落盘
		fmt.Println("[Emulator] 未检测到 mumu-cli，尝试通过 ADB 发送软关机指令...")
		_ = exec.Command(adbExe, "-s", adbAddress, "shell", "reboot", "-p").Run()
	}

	// 3. 切断 ADB 连接，杜绝守护机制因端口探测轮询而再次唤醒模拟器
	if adbExe != "" {
		_ = exec.Command(adbExe, "disconnect", adbAddress).Run()
	}

	// 4. 定向等待该实例退出（最长 15 秒，通过 CLI info 轮询，保障 Android 充分落盘且不派生 tasklist 外部进程）
	vmTerminated := false
	lastInfo := initialInfo
	if cliExe != "" {
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(500 * time.Millisecond)
			if info, err := queryVMInfo(cliExe, vmIndex); err == nil {
				lastInfo = info
				if !info.IsProcessStarted {
					vmTerminated = true
					break
				}
			}
		}
	} else {
		// 无 CLI 场景退化：留出 3 秒基础落盘缓冲后退出
		time.Sleep(3 * time.Second)
	}

	// 5. 若目标实例超时未退出，仅针对该实例专属 PID 定向强杀，绝不误伤多开
	if !vmTerminated && cliExe != "" {
		fmt.Printf("[Emulator] 实例 %d 关机超时，执行专属 PID 定向兜底清理...\n", vmIndex)
		if lastInfo.PID > 0 {
			killPID(lastInfo.PID)
		}
		if lastInfo.HeadlessPID > 0 {
			killPID(lastInfo.HeadlessPID)
		}
	}

	// 6. 检查是否存在其他正在运行的实例：仅在用户开启选项且无其他多开实例时直接结束启动器主面板（绝不触碰 MuMuNxService 等系统底层服务）
	if cliExe != "" && closeLauncher {
		if !hasOtherRunningInstances(cliExe, vmIndex) {
			fmt.Println("[Emulator] 已开启关闭启动器，正在退出 MuMu 启动器主面板...")
			launcherExe := filepath.Base(mumuPath)
			if launcherExe == "" {
				launcherExe = "MuMuNxMain.exe"
			}
			_ = exec.Command("taskkill", "/F", "/IM", launcherExe).Run()
		} else {
			fmt.Println("[Emulator] 检测到其他模拟器实例仍在运行，保留 MuMu 启动器环境")
		}
	}

	fmt.Println("[Emulator] MuMu 模拟器已安全关闭")
	return nil
}

type mumuVMInfo struct {
	Index            any  `json:"index"`
	IsProcessStarted bool `json:"is_process_started"`
	PID              int  `json:"pid"`
	HeadlessPID      int  `json:"headless_pid"`
}

// queryVMInfo 通过 mumu-cli 查询特定实例的运行状态与专属 PID
func queryVMInfo(cliExe string, vmIndex int) (mumuVMInfo, error) {
	cmd := exec.Command(cliExe, "info", "-v", fmt.Sprintf("%d", vmIndex))
	cmd.Dir = filepath.Dir(cliExe)
	out, err := cmd.Output()
	if err != nil {
		return mumuVMInfo{}, err
	}
	var info mumuVMInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return mumuVMInfo{}, err
	}
	return info, nil
}

// hasOtherRunningInstances 检查系统中除当前实例外是否还有其他 MuMu 实例正在运行（保护多开）
func hasOtherRunningInstances(cliExe string, currentVMIndex int) bool {
	cmd := exec.Command(cliExe, "info", "-v", "all")
	cmd.Dir = filepath.Dir(cliExe)
	out, err := cmd.Output()
	if err != nil {
		// 命令异常时采取 Fail-Safe 保守策略，默认判定有其他实例，防止误杀主面板
		return true
	}
	return parseOtherRunningInstances(out, currentVMIndex)
}

func parseOtherRunningInstances(out []byte, currentVMIndex int) bool {
	targetIndexStr := fmt.Sprintf("%d", currentVMIndex)

	// 尝试反序列化为数组（多实例场景）
	var list []mumuVMInfo
	if err := json.Unmarshal(out, &list); err == nil {
		for _, vm := range list {
			if fmt.Sprintf("%v", vm.Index) != targetIndexStr && vm.IsProcessStarted {
				return true
			}
		}
		return false
	}

	// 尝试反序列化为单个对象（单实例场景）
	var single mumuVMInfo
	if err := json.Unmarshal(out, &single); err == nil {
		if fmt.Sprintf("%v", single.Index) != targetIndexStr && single.IsProcessStarted {
			return true
		}
		return false
	}

	// 解析完全异常时，同样采取 Fail-Safe 保守策略
	return true
}

// killPID 定向强制终止指定 PID 进程
func killPID(pid int) {
	if pid <= 0 {
		return
	}
	_ = exec.Command("taskkill", "/F", "/PID", fmt.Sprintf("%d", pid)).Run()
}

// stripJSONComments 轻量剥离 JSONC 中的 // 注释，仅做基础双引号包裹判断以防误伤 URL
func stripJSONComments(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		clean := strings.TrimRight(line, "\r")
		for idx := strings.Index(clean, "//"); idx != -1; {
			if strings.Count(clean[:idx], `"`)%2 == 0 {
				clean = clean[:idx]
				break
			}
			next := strings.Index(clean[idx+2:], "//")
			if next == -1 {
				break
			}
			idx += 2 + next
		}
		lines[i] = clean
	}
	return []byte(strings.Join(lines, "\n"))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
