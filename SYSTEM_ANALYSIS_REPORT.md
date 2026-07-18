# Grok Build 系统分析报告

> **项目名称**: Grok Build (grok)  
> **开发者**: xAI (SpaceXAI)  
> **技术栈**: Rust (2024 edition)  
> **分析日期**: 2026-07-18

---

## 目录

1. [系统概述](#1-系统概述)
2. [功能定位](#2-功能定位)
3. [核心架构设计](#3-核心架构设计)
4. [模块详细分析](#4-模块详细分析)
5. [核心业务流程](#5-核心业务流程)
6. [技术特性](#6-技术特性)
7. [配置与扩展体系](#7-配置与扩展体系)
8. [总结](#8-总结)

---

## 1. 系统概述

Grok Build 是 xAI 开发的终端级 AI 编码助手（AI Coding Agent），类似于 Claude Code、Cursor、GitHub Copilot Chat 等产品。它是一个基于 Rust 语言构建的本地命令行工具，通过与 xAI 的云端大语言模型（LLM）API 通信，为开发者提供代码生成、编辑、搜索、项目管理等全流程的 AI 辅助编程能力。

### 核心定位

- **终端原生**：直接在终端中运行的 TUI（Terminal User Interface）应用
- **Agent 驱动**：以 AI Agent 为核心，自主执行多步骤软件开发任务
- **工具丰富**：提供文件读写、代码搜索、终端命令执行、网络搜索、任务管理等完整工具集
- **可扩展**：支持插件系统、自定义子代理（Subagent）、MCP 协议、LSP 集成
- **多后端支持**：兼容 OpenAI Chat Completions API、OpenAI Responses API、Anthropic Messages API 三种推理后端

---

## 2. 功能定位

### 2.1 核心功能矩阵

| 功能类别 | 具体能力 | 对应工具 |
|---------|---------|---------|
| **文件操作** | 读取文件、搜索替换、目录浏览 | `ReadFile`, `SearchReplace`, `ListDir`, `Grep` |
| **代码搜索** | 基于 ripgrep 的内容搜索、语义搜索 | `Grep`, 语义搜索索引 |
| **终端执行** | Bash 命令执行、后台任务、终端输出获取 | `Bash`, `TaskOutput`, `WaitTasks`, `KillTask` |
| **网络能力** | 网页搜索、网页内容抓取 | `WebSearch`, `WebFetch` |
| **任务管理** | Todo 列表、子代理调度、定时任务 | `TodoWrite`, `Task`, `Scheduler` |
| **用户交互** | 提问、计划模式、目标追踪 | `AskUserQuestion`, `EnterPlanMode`, `ExitPlanMode`, `UpdateGoal` |
| **媒体生成** | 图片生成、图片编辑、视频生成 | `ImageGen`, `ImageEdit`, `ImageToVideo`, `ReferenceToVideo` |
| **扩展能力** | MCP 工具、LSP 诊断、监控 | `Lsp`, `Monitor`, MCP 协议工具 |

### 2.2 用户交互模式

- **全屏 TUI 模式**（默认）：使用 ratatui 框架构建的完整终端界面，支持交替屏幕
- **内联模式**（`--no-alt-screen`）：在终端原生滚动区域内运行
- **最小化模式**（`--minimal`）：实验性的滚动回退原生模式，仅在底部保留提示区域
- **无头模式**（`--headless`）：编程接口模式，支持 `--print` / `-p` 管道输出
- **聊天模式**（`--chat`）：简化的交互式聊天模式

### 2.3 子代理（Subagent）系统

系统内置三种子代理，支持用户自定义扩展：

| 内置子代理 | 功能描述 |
|-----------|---------|
| `general-purpose` | 全功能代理，拥有所有工具，用于自主研究和多步骤任务 |
| `explore` | 快速只读代码探索，偏好快速模型 |
| `plan` | 只读架构和实现规划 |

---

## 3. 核心架构设计

### 3.1 整体架构概览

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Grok Build System                             │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌──────────────────┐    ┌──────────────────────────────────────┐   │
│  │  xai-grok-pager  │    │          xai-grok-shell               │   │
│  │  (TUI 前端)       │◄──►│          (核心运行时)                  │   │
│  │                  │    │                                       │   │
│  │  - 终端渲染      │    │  ┌─────────────────────────────────┐ │   │
│  │  - 用户输入      │    │  │     xai-grok-agent              │ │   │
│  │  - 事件循环      │    │  │     (Agent 构建与管理)           │ │   │
│  │  - 滚动回退      │    │  │  - AgentBuilder                  │ │   │
│  │  - 模态窗口      │    │  │  - AgentDefinition               │ │   │
│  │  - 语法高亮      │    │  │  - PromptContext                 │ │   │
│  └──────────────────┘    │  │  - CompactionPolicy              │ │   │
│                          │  │  - ReminderPolicy                │ │   │
│                          │  └─────────────────────────────────┘ │   │
│                          │                                       │   │
│                          │  ┌─────────────────────────────────┐ │   │
│                          │  │     xai-grok-tools              │ │   │
│                          │  │     (工具系统)                   │ │   │
│                          │  │  - ToolRegistry                  │ │   │
│                          │  │  - Bash/ReadFile/SearchReplace   │ │   │
│                          │  │  - WebSearch/WebFetch            │ │   │
│                          │  │  - Task/TodoWrite/AskUserQuestion│ │   │
│                          │  │  - ImageGen/VideoGen             │ │   │
│                          │  │  - MCP Bridge                    │ │   │
│                          │  └─────────────────────────────────┘ │   │
│                          │                                       │   │
│                          │  ┌─────────────────────────────────┐ │   │
│                          │  │     xai-grok-sampler            │ │   │
│                          │  │     (采样/推理层)                │ │   │
│                          │  │  - SamplingClient (HTTP)         │ │   │
│                          │  │  - SamplerActor (并发控制)       │ │   │
│                          │  │  - 重试/退避/DoomLoop检测       │ │   │
│                          │  │  - 3种API后端适配                │ │   │
│                          │  └─────────────────────────────────┘ │   │
│                          │                                       │   │
│                          │  ┌─────────────────────────────────┐ │   │
│                          │  │     xai-grok-workspace          │ │   │
│                          │  │     (工作区管理)                 │ │   │
│                          │  │  - 文件系统(VFS)                 │ │   │
│                          │  │  - 权限管理                      │ │   │
│                          │  │  - Git/JJ 版本控制              │ │   │
│                          │  │  - 会话持久化/恢复              │ │   │
│                          │  │  - 文件夹信任                    │ │   │
│                          │  │  - MCP 服务器管理               │ │   │
│                          │  └─────────────────────────────────┘ │   │
│                          └──────────────────────────────────────┘   │
│                                                                      │
│  ┌──────────────────┐    ┌──────────────────────────────────────┐   │
│  │  xai-grok-config │    │        支持与服务层                    │   │
│  │  (配置管理)       │    │  ┌──────────┐ ┌──────────────────┐  │   │
│  │                  │    │  │  Telemetry│ │  HTTP/WebSocket  │  │   │
│  │  - 分层配置合并  │    │  │  (遥测)   │ │  (网络通信)      │  │   │
│  │  - 版本覆盖      │    │  └──────────┘ └──────────────────┘  │   │
│  │  - 管理策略      │    │  ┌──────────┐ ┌──────────────────┐  │   │
│  │  - 签名验证      │    │  │  Update  │ │  Token Estimation│  │   │
│  └──────────────────┘    │  │  (自动更新)│ │  (Token估算)     │  │   │
│                          │  └──────────┘ └──────────────────┘  │   │
│                          └──────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

### 3.2 分层架构

系统采用严格的分层架构，各层职责清晰：

| 层级 | Crate | 职责 |
|------|-------|------|
| **表示层** | `xai-grok-pager` + `xai-grok-pager-render` + `xai-grok-pager-minimal` | TUI 渲染、用户输入、事件处理 |
| **应用层** | `xai-grok-shell` | 会话管理、Agent 编排、工具调度、ACR 协议通信 |
| **领域层** | `xai-grok-agent` + `xai-grok-tools` | Agent 定义与构建、工具注册与执行 |
| **基础设施层** | `xai-grok-sampler` + `xai-grok-workspace` + `xai-grok-config` | API 通信、文件系统、配置管理 |

### 3.3 架构模式

#### Actor 模式（采样层）
`xai-grok-sampler` 采用 Actor 模型管理并发推理请求：

```
SamplerHandle (对外接口)
    │
    ▼
SamplerActor (Actor 核心)
    │
    ├── Per-request tasks (tokio::spawn)
    │   ├── SamplingClient (HTTP 通信)
    │   ├── 重试逻辑 (exponential backoff + jitter)
    │   └── DoomLoop 检测与恢复
    │
    └── Metrics (延迟统计、百分位计算)
```

#### 事件驱动架构（TUI 层）
`xai-grok-pager` 使用 Elm-like 单向数据流：

```
User Input → Action → Dispatch (纯函数) → State Mutation + Effects
                                              │
                                              ▼
                                         Effect → Async Task
                                              │
                                              ▼
                                         Render (ratatui)
```

#### 插件/扩展架构
- **插件发现**：从 `~/.grok/plugins/`、项目 `.grok/plugins/` 等位置发现插件
- **Agent 发现**：从 `.grok/agents/`、`~/.claude/agents/` 等位置发现自定义 Agent
- **MCP 集成**：通过 MCP 协议集成外部工具服务器
- **LSP 集成**：集成语言服务器协议提供诊断
- **Skills 系统**：可复用的技能模板

---

## 4. 模块详细分析

### 4.1 xai-grok-pager（TUI 前端）

**核心职责**：终端用户界面，负责渲染和交互。

**关键组件**：

| 组件 | 文件路径 | 功能 |
|------|---------|------|
| `AppView` | `app/app_view.rs` | 根组件，输入路由和绘制 |
| `AgentView` | `app/agent_view.rs` | 每个 Agent 的视图模型 |
| `Scrollback` | `scrollback/mod.rs` | 对话历史和滚动回退 |
| `PromptWidget` | `views/prompt_widget/mod.rs` | 用户输入提示组件 |
| `Modal` | `views/modal.rs` | 模态窗口（确认/错误/设置） |
| `Dispatch` | `app/dispatch.rs` | Action → State 纯函数转换 |
| `Effects` | `app/effects.rs` | Effect → Async 任务 |
| `EventLoop` | `app/event_loop.rs` | 偏置的 tokio::select! 事件循环 |

**三种屏幕模式**：

- **Fullscreen**（全屏）：使用交替屏幕，完整的 TUI 体验
- **Inline**（内联）：在终端原生滚动区域运行
- **Minimal**（最小化）：实验性模式，将已完成块打印到终端原生滚动回退，底部保留提示区域

**终端能力**：
- Kitty 键盘协议支持（DISAMBIGUATE_ESCAPE_CODES + REPORT_EVENT_TYPES）
- 鼠标捕获与原生选择模式切换
- 括号粘贴支持
- 焦点事件追踪
- 24位真彩色支持
- 光标样式控制（闪烁/稳定）

### 4.2 xai-grok-shell（核心运行时）

**核心职责**：作为系统的"胶水层"，连接所有子系统。

**关键模块**：

| 模块 | 功能 |
|------|------|
| `agent/` | Agent 服务器、中继连接、会话配置、子代理调度 |
| `session/` | 会话管理（ACR 协议）、持久化、工作区、目标追踪 |
| `tools/` | 工具注册与调度 |
| `sampling/` | 采样配置与类型 |
| `config/` | 运行时配置加载 |
| `auth/` | 认证管理与刷新 |
| `remote/` | 远程连接支持 |
| `plugin/` | 插件管理 |
| `leader/` | Leader 模式（多进程共享 Agent） |

**会话管理核心**：
- **ACR 协议**（Agent Client Protocol）：客户端与 Agent 服务端的通信协议
- **会话持久化**：会话可保存到远程存储，支持跨设备恢复
- **会话分叉**：支持从现有会话创建分支
- **工作区支持**：每个会话可关联独立的工作区

**Prompt 来源分类**（`PromptOrigin`）：

| 来源 | 说明 |
|------|------|
| `User` | 用户直接输入 |
| `TaskCompleted` | 后台任务完成自动唤醒 |
| `SubagentCompleted` | 子代理完成自动唤醒 |
| `NotificationDrain` | 空闲时通知批量排空 |
| `GoalSummary` | 目标编排器摘要触发 |
| `GoalClassifierNudge` | 目标验证阶段拒绝后触发 |
| `SchedulerFired` | 定时任务触发 |
| `PlanResume` | 计划审批恢复触发 |

### 4.3 xai-grok-agent（Agent 定义与构建）

**核心职责**：定义 Agent 的结构、构建流程和策略。

**关键结构**：

```rust
pub struct Agent {
    definition: AgentDefinition,      // Agent 定义
    prompt_context: PromptContext,    // 提示上下文
    system_prompt: String,            // 渲染后的系统提示
    tool_bridge: Arc<ToolBridge>,     // 工具桥接器
    reminder_policy: ReminderPolicy,  // 提醒策略
    compaction_policy: CompactionPolicy, // 压缩策略
    hosted_tools: Vec<HostedTool>,    // 服务端托管工具
    backend_search_enabled: bool,     // 后端搜索开关
}
```

**Agent 定义来源**：

1. **内置 Agent**（`BuiltinAgentName`）：`grok-build`、`grok-build-orchestrator`、`codex`、`browser-use` 等
2. **项目级 Agent**（`.grok/agents/*.md`）：从 cwd 到 git 根目录遍历
3. **用户级 Agent**（`~/.grok/agents/*.md`）：用户自定义
4. **兼容 Agent**（`~/.claude/agents/*.md`）：Claude Code 兼容
5. **捆绑 Agent**（`~/.grok/bundled/agents/*.md`）：最低优先级
6. **插件 Agent**：通过插件系统提供

**Agent 发现优先级**：项目级 > 内置 > 用户级 > 捆绑

**压缩策略**（`CompactionPolicy`）：
- 当 Token 使用量超过上下文窗口的阈值百分比（默认 85%）时自动触发
- 压缩后使用紧凑系统提示（`COMPACT_SYSTEM_PROMPT`）

### 4.4 xai-grok-tools（工具系统）

**核心职责**：定义、注册和执行 Agent 可用的所有工具。

**工具注册**：通过 `register_all()` 统一入口注册所有内置工具并注入共享资源。

**工具列表**（按类别）：

#### 文件操作工具
| 工具 | 功能 |
|------|------|
| `ReadFile` | 读取文件内容 |
| `SearchReplace` | 搜索替换文件内容 |
| `ListDir` | 列出目录内容 |
| `Grep` | 基于 ripgrep 的内容搜索 |
| `Lsp` | 语言服务器协议诊断 |

#### 终端工具
| 工具 | 功能 |
|------|------|
| `Bash` | 执行 Bash 命令 |
| `TaskOutput` | 获取终端命令输出 |
| `WaitTasks` | 等待后台任务完成 |
| `KillTask` | 终止后台任务 |
| `Monitor` | 监控工具 |

#### 网络工具
| 工具 | 功能 |
|------|------|
| `WebSearch` | 网络搜索 |
| `WebFetch` | 抓取网页内容 |

#### 任务管理工具
| 工具 | 功能 |
|------|------|
| `Task` | 启动子代理任务 |
| `TodoWrite` | 管理 Todo 列表 |
| `AskUserQuestion` | 向用户提问 |
| `Scheduler` | 定时任务（创建/删除/列表） |

#### 目标管理工具
| 工具 | 功能 |
|------|------|
| `UpdateGoal` | 更新目标状态 |
| `EnterPlanMode` | 进入计划模式 |
| `ExitPlanMode` | 退出计划模式 |

#### 媒体生成工具
| 工具 | 功能 |
|------|------|
| `ImageGen` | 生成图片 |
| `ImageEdit` | 编辑图片 |
| `ImageToVideo` | 图片转视频 |
| `ReferenceToVideo` | 参考图转视频 |

**工具输出限制**：
- 默认最大字节数：40 KB（≈ 10,000 tokens）
- Bash 输出字符限制：20,000 chars（≈ 5,000 tokens）
- MCP 工具输出限制：可通过环境变量配置

### 4.5 xai-grok-sampler（采样/推理层）

**核心职责**：管理与 LLM API 的通信，提供流式推理和重试机制。

**三层 API 设计**：

| 层级 | 组件 | 功能 |
|------|------|------|
| Layer 1 | `SamplingClient` | 原始 HTTP 请求，返回流 |
| Layer 2 | `stream` 模块 | 流转换，生成 `SamplingEvent` |
| Layer 3 | `SamplerHandle` + `SamplerActor` | Actor 管理并发请求、重试、取消 |

**三种 API 后端支持**：

| 后端 | API 端点 | 流事件类型 |
|------|---------|-----------|
| `ChatCompletions` | `/chat/completions` | `ChatCompletionChunk` |
| `Responses` | `/responses` | `ResponseStreamEvent` |
| `Messages` | `/messages` | `MessageStreamEvent` |

**统一会话 API**（`ConversationRequest`）：
- 提供统一的请求格式，内部自动转换为对应后端格式
- 支持 `conversation_collect()` 一键收集完整响应

**重试策略**：
- 默认最大重试次数可配置
- 指数退避 + 随机抖动（jitter）
- 支持 `Retry-After` 响应头（上限 120 秒）
- 支持 `x-should-retry` 响应头
- 速率限制阈值检测

**Doom Loop 检测**：
- 可选的服务端死循环检测与恢复
- 通过 `x-grok-doom-loop-check` 请求头启用
- `DoomLoopSignalCollector` 收集信号

**安全特性**：
- 401 归因回调：记录认证失败事件，仅传递 Bearer 前缀（不泄露完整 Token）
- 敏感 Header 自动脱敏（Authorization、API-Key、Token、Secret）

### 4.6 xai-grok-workspace（工作区管理）

**核心职责**：管理主机文件系统、版本控制、权限和会话持久化。

**关键模块**：

| 模块 | 功能 |
|------|------|
| `file_system/` | 虚拟文件系统抽象，支持 `ext_fs` 扩展 |
| `worktree/` | 工作树管理，跟踪文件变更 |
| `permission/` | 权限管理（文件读写、命令执行） |
| `session/` | 会话文件状态、Git/JJ 版本控制 |
| `trust/` | 文件夹信任管理 |
| `folder_trust/` | 信任边界检测 |
| `mcp/` | MCP 服务器管理 |
| `hub/` | Hub 模式（多进程共享） |
| `upload/` | 工作区环境上传 |
| `recovery/` | 会话恢复 |
| `preview_supervisor/` | 预览管理 |

**文件系统**：
- `ext_fs`：扩展文件系统，与主机文件系统交互
- `content`：文件内容管理，存储、读取和同步
- ripgrep 集成：高效内容搜索

**版本控制**：
- Git 集成（`git2`）
- JJ（Jujutsu）集成
- 文件状态追踪（`file_state`）

**RepoDirChain**：
- 一次 `git2` 发现 + 一次 cwd→root 遍历
- 被多个子系统复用（文件夹信任、项目配置、插件、Agent 发现）
- 防止 home 目录被误识别为 git 仓库

### 4.7 xai-grok-config（配置管理）

**核心职责**：分层配置加载与合并。

**配置合并优先级**（低→高）：

1. `/etc/grok/managed_config.toml`（系统级管理配置）
2. `$GROK_HOME/managed_config.toml`（用户级管理配置）
3. `$GROK_HOME/config.toml`（用户配置）
4. `$GROK_HOME/requirements.toml`（云端缓存，Ed25519 签名）
5. `/etc/grok/requirements.toml`（系统级需求配置）
6. macOS MDM 管理偏好（`ai.x.grok`，仅 macOS）

**版本覆盖**（`version_overrides`）：
- 每层配置可包含 `[[version_overrides]]` 规则
- 支持按版本范围条件应用配置
- 合并前各层独立应用

**安全策略**：
- `signed_policy`：Ed25519 签名验证
- `managed_cache`：管理配置缓存与同步
- `validate_requirements`：需求层可选 fail-closed 启动

---

## 5. 核心业务流程

### 5.1 启动流程

```
启动 (grok 命令)
    │
    ├── 1. 加载配置
    │   ├── 合并分层配置 (managed → user → requirements)
    │   └── 解析 Agent 配置
    │
    ├── 2. 认证刷新
    │   ├── 尝试刷新认证 Token
    │   └── 启动早期预取 (远程设置)
    │
    ├── 3. 解析命令行参数
    │   ├── --resume / --continue / --chat / --minimal
    │   ├── 权限模式 (--yolo / --auto / --permission-mode)
    │   └── 工作区选项
    │
    ├── 4. 会话启动
    │   ├── 新建会话 / 恢复会话 / 分叉会话
    │   └── 连接 Agent 服务器 (Leader 或直接)
    │
    ├── 5. 终端初始化
    │   ├── raw mode 启用
    │   ├── 交替屏幕 (Fullscreen) 或内联 (Inline/Minimal)
    │   ├── Kitty 键盘协议
    │   ├── 鼠标捕获 / 括号粘贴
    │   └── 主题应用
    │
    ├── 6. 事件循环
    │   ├── tokio::select! 偏置循环
    │   ├── 用户输入 → Action → Dispatch → State + Effects
    │   └── 渲染 (ratatui)
    │
    └── 7. 退出清理
        ├── 终端恢复
        ├── 会话摘要打印
        └── 恢复提示
```

### 5.2 推理请求流程

```
用户输入 Prompt
    │
    ├── 1. 构建 ConversationRequest
    │   ├── 系统提示（从 Agent 的 PromptContext 渲染）
    │   ├── 对话历史（消息列表）
    │   ├── 工具定义（ToolRegistry 生成）
    │   ├── 托管工具（服务端 WebSearch 等）
    │   └── 采样参数（model, temperature, max_tokens）
    │
    ├── 2. 发送到 SamplerActor
    │   ├── 生成 RequestId
    │   ├── 选择 API 后端
    │   │   ├── ChatCompletions → /chat/completions (SSE)
    │   │   ├── Responses → /responses (SSE)
    │   │   └── Messages → /messages (SSE)
    │   └── 注入 x-grok-* 请求头
    │
    ├── 3. 流式处理
    │   ├── 接收 SSE 事件流
    │   ├── 解析文本增量 / 工具调用增量
    │   ├── Doom Loop 事件拦截
    │   └── 上下文详情提取 (context_details)
    │
    ├── 4. 工具调用循环
    │   ├── 模型返回工具调用 → 执行工具
    │   ├── 工具结果注入对话历史
    │   └── 继续推理直到模型返回文本
    │
    └── 5. 响应完成
        ├── Token 统计更新
        ├── 自动压缩检查（超过阈值 → 触发压缩）
        └── 结果呈现给用户
```

### 5.3 工具执行流程

```
模型请求工具调用
    │
    ├── 1. 权限检查
    │   ├── PermissionMode 检查
    │   │   ├── Default: 询问用户确认
    │   │   ├── YOLO: 自动批准
    │   │   └── Plan: 只读操作
    │   └── 文件夹信任检查
    │
    ├── 2. 工具路由
    │   ├── 内置工具 (NewTool trait)
    │   │   ├── ReadFile / SearchReplace / ListDir / Grep
    │   │   ├── Bash / TaskOutput / WaitTasks / KillTask
    │   │   ├── WebSearch / WebFetch
    │   │   └── Task / TodoWrite / AskUserQuestion
    │   └── MCP 工具 (通过 ToolBridge 代理)
    │
    ├── 3. 工具执行
    │   ├── 参数验证与解析
    │   ├── 执行工具逻辑
    │   └── 输出截断 (DEFAULT_TOOL_OUTPUT_BYTES / DEFAULT_TOOL_OUTPUT_CHARS)
    │
    └── 4. 结果返回
        ├── 构建 ToolResult
        ├── 注入到对话历史
        └── 触发下一轮推理
```

### 5.4 子代理（Subagent）执行流程

```
Task 工具调用 (启动子代理)
    │
    ├── 1. 子代理发现
    │   ├── 按名称查找 (project > built-in > user > bundled > plugin)
    │   ├── 支持 qualified name (plugin:agent)
    │   └── 验证可用性 (toggle 检查)
    │
    ├── 2. 子代理构建
    │   ├── 加载 AgentDefinition
    │   ├── 构建 PromptContext (子代理受众)
    │   ├── 渲染系统提示
    │   └── 配置工具集 (受限工具集)
    │
    ├── 3. 子代理执行
    │   ├── 独立推理循环
    │   ├── 工具调用与结果处理
    │   └── 自动压缩支持
    │
    └── 4. 结果汇总
        ├── 子代理完成或超时
        ├── 结果返回父代理
        └── 触发 SubagentCompleted 自动唤醒
```

### 5.5 会话持久化与恢复

```
会话保存
    │
    ├── 本地存储
    │   ├── 会话元数据 (JSON)
    │   ├── 对话历史 (消息列表)
    │   ├── 文件状态快照
    │   └── 工具配置状态
    │
    └── 远程存储
        ├── 上传到远程 API
        ├── 会话共享 URL
        └── 跨设备恢复

会话恢复
    │
    ├── 本地查找
    │   ├── 按 session_id 查找
    │   ├── 按 cwd 查找
    │   └── 最近会话
    │
    └── 远程恢复
        ├── 从远程 API 下载
        ├── 回放对话历史
        └── 恢复会话状态
```

---

## 6. 技术特性

### 6.1 性能优化

- **异步运行时**：基于 tokio 的异步 I/O，支持高并发
- **Actor 模式**：SamplerActor 管理并发请求，避免锁竞争
- **流式处理**：SSE 流式接收推理结果，实时呈现
- **Token 估算**：`xai_token_estimation` 用于快速估算上下文使用量
- **单次序列化**：`StreamingChatRequest` 使用 `#[serde(flatten)]` 避免二次序列化
- **纯函数 Dispatch**：TUI 状态更新为纯函数，支持测试
- **RepoDirChain 复用**：一次 git 发现复用多次，避免重复 syscall
- **Writer 线程**：帧渲染写入器独立线程，避免阻塞 tokio 事件循环

### 6.2 安全设计

- **文件夹信任**：`folder_trust` 模块管理信任边界
- **权限模式**：Default / YOLO / Plan 三种模式
- **Bearer 安全**：401 回调仅传递 Token 前缀，不泄露完整凭据
- **敏感 Header 脱敏**：日志中自动脱敏 Authorization、API-Key、Token、Secret
- **签名验证**：Ed25519 签名的管理策略
- **终端标题清理**：过滤控制字符防止注入
- **panic 恢复**：Panic hook 自动恢复终端状态

### 6.3 跨平台支持

- **Linux**：主要目标平台
- **macOS**：支持 MDM 管理偏好
- **Windows**：控制台代码页 UTF-8 设置、VTP 启用、原生选择模式

### 6.4 可观测性

- **结构化日志**：`tracing` 框架，分层日志
- **Prometheus 指标**：工作区操作、上传、权限等指标
- **采样日志**：专门的 `sampling_log` 目标，记录 API 交互细节
- **遥测**：`xai-grok-telemetry` 集成
- **CPU 分析**：`cpu_profile` 支持
- **内存分析**：`heap_profile` 和内存追踪

### 6.5 扩展性

- **插件系统**：项目级/用户级插件，支持 Skills、Commands、Agents、Hooks、MCP
- **MCP 协议**：集成外部工具服务器
- **LSP 集成**：语言服务器诊断
- **自定义 Agent**：Markdown 文件定义，支持 YAML frontmatter
- **Skills 系统**：可复用的技能模板
- **定时任务**：`/loop` 命令支持 cron 表达式

---

## 7. 配置与扩展体系

### 7.1 配置优先级

```
环境变量 (最高)
    │
命令行参数
    │
用户配置 (~/.grok/config.toml)
    │
管理配置 (~/.grok/managed_config.toml)
    │
系统配置 (/etc/grok/managed_config.toml)
    │
需求配置 (requirements.toml, 签名)
    │
MDM 管理 (macOS)
    │
默认值 (最低)
```

### 7.2 Agent 扩展

通过 Markdown 文件定义自定义 Agent，支持 YAML frontmatter：

```markdown
---
name: my-agent
description: 我的自定义 Agent
tools: ["ReadFile", "Grep", "Bash"]
permissionMode: default
---
# Agent 提示正文
你是一个专业的代码审查助手...
```

放置位置：
- 项目级：`.grok/agents/my-agent.md`
- 用户级：`~/.grok/agents/my-agent.md`
- 兼容：`~/.claude/agents/my-agent.md`

### 7.3 插件系统

插件目录结构：
```
plugin-root/
├── manifest.toml       # 插件清单
├── skills/             # 技能定义
├── commands/           # 自定义命令
├── agents/             # 自定义 Agent
├── hooks/              # 生命周期钩子
└── mcp_servers/        # MCP 服务器配置
```

---

## 8. 总结

Grok Build 是一个设计精良的终端 AI 编码助手系统，具有以下特点：

### 架构优势

1. **分层清晰**：表示层、应用层、领域层、基础设施层职责分明
2. **模块化设计**：每个 crate 职责单一，依赖关系清晰
3. **Actor 并发模型**：采样层采用 Actor 模式，支持高并发推理
4. **事件驱动 TUI**：Elm-like 单向数据流，状态管理可预测
5. **多后端适配**：统一会话 API 适配三种 LLM 后端
6. **丰富的扩展性**：插件、自定义 Agent、MCP、LSP 多重扩展机制

### 技术亮点

1. **流式推理**：SSE 实时流式处理，低延迟呈现
2. **智能压缩**：上下文窗口阈值自动触发压缩
3. **子代理系统**：支持多级 Agent 协作
4. **安全设计**：多层权限控制、信任边界、凭据保护
5. **跨平台原生**：Linux/macOS/Windows 全面支持
6. **会话持久化**：本地+云端双重存储，支持跨设备恢复

### 代码规模

- 核心 Rust crates：40+
- 工作区成员：50+
- 代码行数：数十万行（预估）
- 测试覆盖：每个 crate 包含大量单元测试

该系统代表了 AI 编码助手领域的前沿工程实践，在架构设计、性能优化、安全性和可扩展性方面均表现出色。
