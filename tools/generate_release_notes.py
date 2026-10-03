#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import os
import sys
import json
import datetime
import subprocess
import urllib.request
import urllib.error
from pathlib import Path

# 最大文本限制，防止超长 Commit 导致 API 413/400
MAX_COMMITS_LEN = 10000  # 约 10KB
MAX_STAT_LEN = 4000      # 约 4KB
MAX_COMMITS_COUNT = 60

STATIC_TEMPLATE = """
---

## ⚠️ 使用说明与免责声明

* **完全开源免费**：本项目为开源非营利项目，遵循 MIT 开源协议，**完全免费，严禁任何形式的倒卖与商用付费**。
* **专注日常减负**：本工具纯粹用于自动化执行常规繁琐日常打卡，不包含任何破坏游戏平衡、篡改内存或解密封包等侵入性功能。
* **风险评估**：本项目为第三方非官方自动化工具，与《闪耀暖暖》官方运营商及开发商（叠纸游戏 Papergames）无关。使用自动化辅助可能违反游戏用户协议，请自行评估使用风险。因使用本工具造成的任何账号异常或数据损失，由使用者自行承担。

---

## 🙏 鸣谢

* 本项目自动化核心与图像推理引擎由 [MaaFramework](https://github.com/MaaXYZ/MaaFramework) 强力驱动。
* 桌面端交互界面基于 [MXU](https://github.com/MistEO/MXU) 及其自维护分支 [MXU_tqdw](https://github.com/TianQuanDiWen/MXU_tqdw)。
* OCR 基础模型与文字词典来自 [MaaCommonAssets](https://github.com/MaaXYZ/MaaCommonAssets)。
"""

def get_fallback_content():
    """读取上一阶段 git-cliff 产生的 CHANGES.md 作为安全兜底"""
    changes_path = Path("CHANGES.md")
    if changes_path.is_file():
        try:
            content = changes_path.read_text(encoding="utf-8").strip()
            if content:
                return content
        except Exception as e:
            print(f"[Warning] Failed to read CHANGES.md: {e}", file=sys.stderr)
    return ""

def get_git_info():
    """获取当前版本与上一版本之间的 Git 提交和 diff 统计，并施加严格的体积上限保护"""
    prev_tag = ""
    try:
        tags = subprocess.check_output(
            ["git", "tag", "--sort=-v:refname"],
            stderr=subprocess.DEVNULL,
            text=True,
            encoding="utf-8"
        ).strip().splitlines()
        current_tag = os.environ.get("TAG_NAME", "").strip()
        for t in tags:
            t = t.strip()
            if t and t != current_tag and t.startswith("v"):
                prev_tag = t
                break
    except Exception as e:
        print(f"[Warning] Failed to list git tags: {e}", file=sys.stderr)

    cmd = ["git", "log", "--pretty=format:%h - %s%n%b", "--no-merges"]
    if prev_tag:
        cmd.extend([f"{prev_tag}..HEAD", f"-n{MAX_COMMITS_COUNT}"])
        print(f"Comparing changes between {prev_tag} and HEAD (max {MAX_COMMITS_COUNT} commits)")
    else:
        cmd.extend([f"-n{MAX_COMMITS_COUNT}"])
        print(f"No previous tag found, analyzing up to {MAX_COMMITS_COUNT} recent commits")

    try:
        commits = subprocess.check_output(cmd, stderr=subprocess.DEVNULL, text=True, encoding="utf-8").strip()
    except Exception as e:
        print(f"[Warning] Failed to get git log: {e}", file=sys.stderr)
        commits = ""

    # 体积硬上限保护，防止超长 Commit 挤爆请求体
    if len(commits) > MAX_COMMITS_LEN:
        commits = commits[:MAX_COMMITS_LEN] + "\n\n... (提交内容已截断以防超出模型上下文上限)"

    try:
        cmd_stat = ["git", "diff", "--stat"]
        if prev_tag:
            cmd_stat.append(prev_tag)
        else:
            cmd_stat.extend(["HEAD~5", "HEAD"])
        stat = subprocess.check_output(cmd_stat, stderr=subprocess.DEVNULL, text=True, encoding="utf-8").strip()
    except Exception as e:
        print(f"[Warning] Failed to get git diff stat: {e}", file=sys.stderr)
        stat = ""

    if len(stat) > MAX_STAT_LEN:
        stat = stat[:MAX_STAT_LEN] + "\n... (文件改动统计已截断)"

    return prev_tag, commits, stat

def call_ai(token, base_url, model, prompt):
    """
    统一调用大模型。支持：
    1. Google Gemini 官方端点（默认）
    2. 任意第三方 OpenAI 兼容端点（当指定 AI_BASE_URL 时，如 DeepSeek, OpenRouter, OneAPI 等）
    """
    token = token.strip()
    base_url = (base_url or "").strip().rstrip("/")

    # 若未指定 base_url 或属于 Google 官方端点，默认走 Gemini 官方接口
    is_gemini = not base_url or "googleapis.com" in base_url

    if is_gemini:
        gemini_model = model or "gemini-3.5-flash-lite"
        if not base_url:
            base_url = "https://generativelanguage.googleapis.com/v1beta/models"

        if not base_url.endswith("/models"):
            target_url = f"{base_url}/models/{gemini_model}:generateContent?key={token}"
        else:
            target_url = f"{base_url}/{gemini_model}:generateContent?key={token}"

        print(f"Calling Google Gemini API (model: {gemini_model})...")
        payload = {
            "contents": [{"parts": [{"text": prompt}]}],
            "generationConfig": {"temperature": 0.3}
        }
        req = urllib.request.Request(
            target_url,
            data=json.dumps(payload).encode("utf-8"),
            headers={"Content-Type": "application/json"},
            method="POST"
        )
        with urllib.request.urlopen(req, timeout=40) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            return data["candidates"][0]["content"]["parts"][0]["text"].strip()

    # 任意第三方 OpenAI 兼容协议（当用户指定了自定义 AI_BASE_URL 时）
    target_url = f"{base_url}/chat/completions" if not base_url.endswith("/chat/completions") else base_url
    openai_model = model or "deepseek-chat"

    print(f"Calling OpenAI-compatible API at {target_url} (model: {openai_model})...")
    payload = {
        "model": openai_model,
        "messages": [
            {
                "role": "system",
                "content": (
                    "你是一个资深开源项目发布说明撰写专家。你正在为《闪耀暖暖》（Shining Nikki）"
                    "的自动化减负辅助工具 MaNikki4 编写版本更新说明。"
                    "请仅输出版本导语和主要更新模块列表，保持专业严谨、因果逻辑清晰。"
                )
            },
            {
                "role": "user",
                "content": prompt
            }
        ],
        "temperature": 0.3
    }
    req = urllib.request.Request(
        target_url,
        data=json.dumps(payload).encode("utf-8"),
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {token}",
            "User-Agent": "MaNikki4-Release-Notes-Generator"
        },
        method="POST"
    )
    with urllib.request.urlopen(req, timeout=40) as resp:
        data = json.loads(resp.read().decode("utf-8"))
        return data["choices"][0]["message"]["content"].strip()

def write_output(content):
    """将最终文案写入 GITHUB_OUTPUT 及本地 CHANGES.md"""
    output_file = os.environ.get("GITHUB_OUTPUT")
    if output_file:
        delimiter = "RELEASE_BODY_EOF"
        with open(output_file, "a", encoding="utf-8") as f:
            f.write(f"content<<{delimiter}\n{content}\n{delimiter}\n")
        print("Successfully written release notes to GITHUB_OUTPUT")

    try:
        with open("CHANGES.md", "w", encoding="utf-8") as f:
            f.write(content)
        print("Updated CHANGES.md locally")
    except Exception as e:
        print(f"[Warning] Failed to write CHANGES.md: {e}", file=sys.stderr)

def main():
    token = os.environ.get("AI_TOKEN", "").strip()
    base_url = os.environ.get("AI_BASE_URL", "").strip()
    model = os.environ.get("AI_MODEL", "").strip()
    tag_name = os.environ.get("TAG_NAME", "v0.1.0").strip()
    current_date = datetime.date.today().strftime("%Y-%m-%d")

    fallback_content = get_fallback_content()

    if not token:
        print("[Notice] No AI_TOKEN configured, falling back to git-cliff output.")
        write_output(fallback_content)
        return

    prev_tag, commits, stat = get_git_info()
    if not commits:
        print("[Notice] No commits found to summarize, falling back to git-cliff.")
        write_output(fallback_content)
        return

    # 模板拼接模式：仅让 AI 提炼导语和更新项，固定章节由 Python 自动拼装，降低 Token 消耗并彻底杜绝幻觉
    prompt = f"""请根据以下 Git 提交记录和改动统计，为 MaNikki4 的新版本 {tag_name} 提炼核心更新说明。

【输出要求】：
1. 第一行标题：# MaNikki4 {tag_name}  ({current_date})
2. 版本导语：紧跟标题后，用 1~2 句流畅的自然语言提炼概括本版本的核心更新重点与优化方向（例如：本版本新增评选赛自动参赛，优化了MuMu模拟器管理...）。
3. ## 主要更新：
   - 按功能模块组织分类（例如「评选赛」、「MuMu模拟器管理」、「日常流程与识别精度优化」等）。
   - 每个模块使用加粗标题：`* **新增「模块名称」**：` 或 `* **优化「模块名称」**：`。
   - 子项使用二级列表（`  * xxx`），深入说明改动原因、解决的具体痛点及稳定性提升，切忌原样复制单行 commit。
4. 注意：仅需输出上述内容！不要输出“运行环境”、“快速上手”、“免责声明”或“鸣谢”章节（这些将由系统自动追加）。

【本次变更信息】：
--- 改动统计 ---
{stat}

--- 提交记录 ---
{commits}
"""

    try:
        print("Calling AI to generate core release summary...")
        ai_summary = call_ai(token, base_url, model, prompt)
        if ai_summary and len(ai_summary.strip()) > 30:
            print("AI summary generated successfully, assembling with template...")
            # 模板拼接组装
            full_release_note = ai_summary.strip() + "\n" + STATIC_TEMPLATE.format(tag_name=tag_name).strip() + "\n"
            write_output(full_release_note)
            return
        else:
            print("[Warning] AI returned empty or too short response, using fallback.")
            write_output(fallback_content)
    except Exception as e:
        print(f"[Error] Failed to call AI API ({e}), falling back to git-cliff.", file=sys.stderr)
        write_output(fallback_content)

if __name__ == "__main__":
    main()
