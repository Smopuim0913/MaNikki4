# MaNikki4 Project Context & Core Rules

> 📌 本文件为项目级工作区规则，供所有接入该项目的 AI 编码助手在每轮对话中自动载入，保持上下文一致性。

---

## 1. 项目基本信息与技术栈
* **项目名称**：MaNikki4
* **定位**：《闪耀暖暖》（Shining Nikki）全自动日常减负辅助工具。
* **技术栈构成**：
  * **Core Engine**：MaaFramework 视觉推理与自动化引擎（C++ 底层，由 `assets/resource/pipeline/` 中的 JSON 流水线驱动）。
  * **Agent Process**：原生 Go 语言编写的常驻服务程序（代码位于 `agent/` 目录，Go 1.24+ 构建），通过标准 CGo/Socket 接口与 MaaFramework 交互。
  * **UI Frontend**：基于 `TianQuanDiWen/MXU_tqdw`（定制版 MXU 桌面端），入口配置文件为 `assets/interface.json`。
  * **Vision Assets**：基于 MaaCommonAssets PP-OCR 离线文字识别模型，配合极少数固定模板图（`assets/resource/image/`）。

---

## 2. 核心架构与设计原则

### 2.1 极简轻量化（Go Agent）
* 拒绝引入臃肿的 Python 运行时环境，全部自定义决策与宿主控制均在 Go 原生 Agent 中完成，内存占用维持在 10MB 左右。
* Agent 支持两种工作模式：
  * `agent`：启动 Socket 服务，向 MaaFramework 注册自定义识别器（如 `MatchRowButton`）与自定义控制器。
  * `pretask`：预置任务执行器，负责 MuMu 模拟器路径探测、自启动、ADB 端口探测及游戏应用拉起。

### 2.2 动态对齐识别：`MatchRowButton`
* 用于解决材料列表带有上下滚动条、材料行动态排序时无法使用固定坐标的痛点。
* 原理：在指定 ROI 内同时提取关键词（如材料名）与按钮（如“前往/购买”），使用水平高度容差算法（$|Y_{btn} - Y_{kw}| \le \text{tolerance}$）进行几何配对并点击右侧按钮。

### 2.3 制衣引导槽位与业务策略
* **槽位 1（左侧卡槽）** ➔ `时空回廊制衣`：扫荡每日免费的时空回廊困难关卡。提供开关 `ShikongMainStageOption`（默认关闭，保护体力不刷附属普通关）。
* **槽位 2（右侧卡槽）** ➔ `主线制衣`：扫荡普通主线章节关卡，提供开关 `StoreOption`（默认开启，自动进服装店买金币材料）。
* **零误购红线**：关卡挑战中遇到“次数不足”或“体力不足”弹窗，**必须点击取消**，严防误扣粉钻。材料齐备提示“材料已收集完毕”时自动识别并退出。

---

## 3. 运行与适配基准环境
1. **模拟器**：必须为 **MuMu 模拟器 12**，必须在模拟器设置中开启 **ADB 本地调试**。
2. **分辨率**：**竖屏 9:16**，基准分辨率 **`720 × 1280`**（DPI 默认即可，MaaFramework 自适应缩放）。
3. **游戏主题**：游戏大厅主界面主题必须为 **“暖调回忆”**。严禁在换肤主题下运行，否则返回箭头底色与半透明度变化会导致视觉匹配置信度暴跌。

---

## 4. 编码与修改核心守则 (Surgical Changes)
1. **禁止误删核心资产**：
   * `闪暖-返回箭头.png` 与 `粉钻.png` 是目前仅有的必要图片模板，严禁误删或错误重命名。
2. **遵守命名与展示规范**：
   * 所有任务在 `assets/interface.json` 的 `task` 数组中均去除了“闪暖”前缀（如 `送礼`、`组队本`、`竞技场`、`抽卡`），并通过 `label` 字段配置带 emoji 的 UI 显示名（如 `💌 送礼`）。
   * 前置任务必须保留 `__MXU_PRETASK__` 名称前缀（`__MXU_PRETASK__启动模拟器与游戏`），不可修改其 name。
3. **流水线与构建约束**：
   * 修改 `assets/resource/pipeline/` 下的流程文件时，保持 MaaPipelineEditor (`$__mpe_`) 元数据的一致性。
   * 本地构建命令为 `.\build.ps1 -Action Build`，输出目录为 `install/`。
   * 修改 `assets/interface.json` 或 pipeline 时，应同步更新 `install/` 目录下的副本以便本地直接测试。
4. **文档同步与维护原则 (Documentation Sync - MANDATORY)**：
   * **代码与文档强绑定**：AI 助手对本项目进行任何功能增改、代码重构（Go Agent）、流水线改动（Pipeline JSON）、配置项更新（`interface.json`）或新增规则后，**必须在同一轮对话中及时补充或同步更新 `.agent/` 目录下的对应文档**（如 `PIPELINES.md`、`ARCHITECTURE.md`、`ENVIRONMENT.md`、`DEVELOPMENT.md`、`README.md` 等）。
   * **严禁遗留过时信息**：严禁出现实现逻辑已改动但文档仍记载旧逻辑/旧参数的情况，确保后续每轮 AI 对话都能基于最新的文档直接理解项目全局。
