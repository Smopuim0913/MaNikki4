# MaNikki4 AI Agent 知识库与上下文索引

> 🤖 **本目录（`.agent/`）旨在为所有接入本项目的 AI Assistant / Coding Agent 提供开箱即用的全局架构、业务流程、流水线映射与开发规范。**  
> 新开启任何一轮 AI 对话时，只需阅读或引用本目录下的相关文档，即可瞬间掌握项目全貌，无需重新遍历源码和流水线进行逆向推导。

---

## 📚 知识库目录导航

| 文档 | 核心内容与关注点 |
| :--- | :--- |
| **[`rules/project_context.md`](./rules/project_context.md)** | **[核心]** 供 Antigravity / Agent 自动加载的工作区规则与核心事实摘要。包含核心约束、技术栈、环境要求与敏感禁忌。 |
| **[`ARCHITECTURE.md`](./ARCHITECTURE.md)** | **[架构]** 项目系统架构深度解析。包含 Go Agent、MaaFramework 底层、MXU 桌面端的通信机制、`MatchRowButton` 算法原理及模块职责分工。 |
| **[`PIPELINES.md`](./PIPELINES.md)** | **[流水线]** 14 个日常任务的流水线节点字典、执行入口（entry）、参数覆盖、跳转分支与异常自愈处理逻辑。 |
| **[`ENVIRONMENT.md`](./ENVIRONMENT.md)** | **[环境基准]** 运行与测试的基准环境配置（MuMu 12 模拟器、ADB 端口、竖屏 9:16、720×1280、“暖调回忆”主题的强约束理由）。 |
| **[`DEVELOPMENT.md`](./DEVELOPMENT.md)** | **[开发运维]** 本地编译构建流程（`build.ps1`）、测试运行指令、MPE 编辑器规范、图像资源规范及常见问题排查（Troubleshooting）。 |

---

## ⚡ 核心速记卡片（30秒掌握要点）

1. **项目定位**：《闪耀暖暖》（Shining Nikki）全自动日常减负辅助工具，完全开源非营利（MIT 协议）。
2. **三层技术架构**：
   * **展示层 (UI)**：自维护分支 [TianQuanDiWen/MXU_tqdw](https://github.com/TianQuanDiWen/MXU_tqdw)（基于 Web/Vue 定制桌面端）。
   * **业务与调度层 (Agent)**：纯原生 Go Agent（Go 1.24+ 构建于 `agent/`，常驻内存仅 ~10MB，零 Python 环境依赖）。
   * **推理与控制层 (Core)**：[MaaFramework](https://github.com/MaaXYZ/MaaFramework)（C++ 高性能底层，负责 OCR、模板匹配、ADB 截图与触控交互）。
3. **关键业务逻辑**：
   * **制衣引导双槽位映射**：第 1 套（左槽）= 时空回廊困难扫荡；第 2 套（右槽）= 主线材料章节扫荡与商店采购。
   * **行级动态配对**：自研 `MatchRowButton` 自定义识别器，通过水平几何容差 $|Y_{btn} - Y_{kw}| \le \text{tolerance}$ 将材料名与右侧“前往/购买”按钮精准绑定，不受滚动条和列表增减影响。
   * **零误购保障**：关卡扫荡中若弹出“次数不足”或“体力不足”购买提示，**一律自动点击取消**，绝不误花粉钻。
4. **视觉适配铁律**：游戏内主界面必须使用 **“暖调回忆”** 主题；换肤主题会导致图标颜色透明度变化进而引发模板匹配失效。
5. **文档同步铁律 (Documentation Sync)**：AI 助手任何修改代码、流水线或选项的行为，均**必须在当轮会话中同步更新补充 `.agent/` 下的相关文档**，严禁代码与文档脱节。
