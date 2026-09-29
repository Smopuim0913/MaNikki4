# MaNikki4 开发运维与排错指南

本文档汇集了日常开发构建、流水线编辑规范、单元测试指令与常见故障排查方法。

---

## 1. 本地构建与编译工作流

项目的打包构建由根目录下的 PowerShell 脚本 [`build.ps1`](file:///d:/Projects/Go/MaNikki4/build.ps1) 全自动化管理。

### 1.1 一键完整构建
```powershell
# 执行全自动跨组件构建
.\build.ps1 -Action Build
```
构建流程内部包含以下步骤：
1. **依赖检查**：自动检测 `go`、`uv`（或 Python）、`npx` 等工具链版本；
2. **下载与解压组件**：
   * 自动下载匹配的 `MaaFramework` 预编译二进制资产到 `deps/`；
   * 自动更新 `assets/MaaCommonAssets` Git 子模块（OCR 模型与字典）；
   * 自动下载定制桌面端 UI `TianQuanDiWen/MXU_tqdw`；
3. **编译 Go Agent**：编译 `agent/cmd/manikki-agent` 并输出二进制 `MaNikki4.Agent.exe`；
4. **资源校验**：运行 `@nekosu/maa-tools check` 与 `tools/validate_schema.py` 严格校验 Pipeline JSON Schema；
5. **打包输出**：将可执行程序、配置文件、资源与依赖组装到 `install/` 目录中。

### 1.2 本地运行与即时调试
构建完成后，直接运行生成产物：
```powershell
cd install
.\MaNikki4.exe
```

---

## 2. 自动化验证与测试指令

在提交代码或修改关键逻辑后，必须执行以下测试验证：

### 2.1 运行 Go 单元测试
```powershell
cd agent
go test -v ./...
```
* **测试覆盖点**：
  * `internal/matcher/row_button_test.go`：校验行级容差算法（$|Y_{btn} - Y_{kw}| \le \text{tolerance}$）在各种偏移行、噪点行输入下的空间配对准确率。
  * `internal/emulator/emulator_test.go`：校验模拟器路径探测与命令拼装逻辑。

### 2.2 验证 Pipeline 语法与 Schema
```powershell
uv run --with jsonschema==4.26.0 --with referencing==0.37.0 python tools/validate_schema.py --schema-dir deps/tools --resource-dirs assets/resource --interface-files assets/interface.json
```

---

## 3. 流水线编辑与资产规范

### 3.1 使用 MaaPipelineEditor (MPE)
* 项目支持通过 [MaaPipelineEditor](https://github.com/MaaXYZ/MaaPipelineEditor) 可视化编辑 `assets/resource/pipeline/*.json`。
* 每个 JSON 文件头部均带有 `$__mpe_config_...` 元数据，文件重命名时必须同步修正其中的 `filename` 和 `filePath`，避免 MPE 加载错乱。

### 3.2 节点命名与跳转规则
* 统一使用清晰的动宾或“模块_子操作”命名：如 `制衣_确认在主界面`、`竞技场_推荐搭配`。
* 共享跳转节点：使用 `公共流程.json` 中的 `公共：返回主菜单` 作为所有模块执行完毕后的收尾节点。
* 遇到条件分支优先使用 `focus` 或状态机分发，尽量避免深层嵌套。

### 3.3 图像模板规范
* 存放路径：`assets/resource/image/`
* **掩码规则**：对于存在动态光效、微光粒子或背景随时间微调的图标，必须使用纯绿透明掩码 `#00FF00` 抹去背景，仅保留固定核心形状。
* **极简原则**：能用 OCR（结合正则）解决的绝不截图；非必须的图标严禁随意塞入仓库。

---

## 4. 知识库与文档同步规范 (AI 必读)

为确保不同对话轮次的 AI 助手均能快速对齐最新项目状态，制定以下文档维护铁律：
* **及时性**：任何关于代码重构（Go Agent）、流水线增改（Pipeline JSON）、任务配置变更（`interface.json`）的操作，**必须在当次回复内同步更新 `.agent/` 目录下的对应文档**。
* **责任范围**：
  * 流水线变更（节点改动、entry 变动、条件覆盖）➔ 同步更新 [`PIPELINES.md`](./PIPELINES.md)
  * 架构或核心算法调整（如识别器扩展、通信机制）➔ 同步更新 [`ARCHITECTURE.md`](./ARCHITECTURE.md)
  * 编译依赖与排错案例 ➔ 同步更新 [`DEVELOPMENT.md`](./DEVELOPMENT.md)
  * 模拟器或视觉基准调整 ➔ 同步更新 [`ENVIRONMENT.md`](./ENVIRONMENT.md)
  * 全局规则与核心约束变更 ➔ 同步更新 [`rules/project_context.md`](./rules/project_context.md)

---

## 5. 常见问题与排错手册 (Troubleshooting)

### Q1: 模拟器启动了，但界面卡在“等待模拟器连接”或报错无法找到 ADB 设备？
* **排查**：打开 MuMu 12 模拟器的「设置中心」➔ 查看「本地 ADB 调试」是否未开启。
* **解决**：开启本地 ADB 调试，重启模拟器。检查 Windows 命令行执行 `adb devices` 是否能列出模拟器设备。

### Q2: 制衣任务跳过了扫荡，直接退出了？
* **排查**：
  1. 检查游戏内【设计中心】➔【制衣引导】的槽位是否为空。
  2. 检查该套装材料是否已经刷齐（游戏内显示“材料已收集完毕”属于正常自愈退出）。
  3. 确认是否将主线套放到了第 1 套而执行的是时空回廊任务。

### Q3: 游戏内主界面返回箭头频繁点击失效或识别超时？
* **排查**：检查当前游戏大厅主题是否被切换成了非默认的换肤主题。
* **解决**：进入游戏大厅右上角更换主题，改回 **“暖调回忆”** 主题即可恢复。

### Q4: 外部 APP 分享后没有自动回到游戏？
* **排查**：模拟器内是否已安装所选社交 APP（QQ/微信/微博/小红书）。
* **解决**：确保模拟器内至少安装了一种所选 APP；或者在任务配置中切换为已安装的对应平台。
