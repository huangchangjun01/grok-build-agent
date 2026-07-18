# SpaceXC Grok Build → Go 重写规格说明

## Why
SpaceXC Grok Build 是一个强大的终端 AI 编程代理工具，使用 Rust 实现。需要 100% 按原功能逻辑使用 Go 语言重写后端服务，保持前端 ratatui TUI 不变，使用 DDD 架构设计，适配 MiniMax 大模型。

## 关键决策确认

| 决策项 | 选择 |
|--------|------|
| 前后端通信协议 | HTTP REST + WebSocket |
| 重写范围 | 全部 80+ crate 功能，无遗漏 |
| 存储方案 | SQLite + GORM |
| 高级功能 | 全部包含 (SubAgent/Goal/PlanMode/MCP) |
| 项目结构 | `/workspace/go-backend/` |
| 认证机制 | API Key 直连（无认证） |
| 配置格式 | YAML (config.yaml) |
| LLM 提供商 | MiniMax (MiniMax-M3) |
| 后端框架 | Gin |
| 前端 | ratatui TUI (保持不变) |

## 综合评估报告

### 原系统功能点清单（全覆盖）

| 分类 | 功能点 | 原Rust Crate |
|------|--------|-------------|
| **Agent 核心** | Agent 主循环 | xai-grok-shell |
| | 多模型支持 | xai-grok-models |
| | Streaming 响应 | async-openai |
| | 会话管理 CRUD | xai-grok-shell/session |
| | 会话持久化 (JSONL+SQLite) | xai-sqlite-journal |
| | 会话压缩(Compaction) | xai-grok-compaction |
| | 子代理(SubAgent) | xai-grok-subagent-resolution |
| | Plan Mode | xai-grok-shell/session/plan_mode |
| | Goal 系统 | xai-grok-shell/session/goal_* |
| | 回退(Rewind) | xai-grok-shell/session/rewind |
| | 会话分叉(Fork) | xai-grok-shell/session/fork |
| | 会话导出 | xai-grok-shell/session/export |
| | 会话搜索 | xai-grok-shell/session/storage/search |
| | 会话摘要 | xai-grok-shell/session/summary |
| | 提示历史 | xai-grok-shell/session/prompt_history |
| | 懒检测(Laziness) | xai-grok-shell/session/laziness |
| | 空闲提示(Idle Prompt) | xai-grok-shell/session/idle_prompt |
| **Agent 生命周期** | Agent 启动/停止 | xai-agent-lifecycle |
| | 崩溃处理 | xai-crash-handler |
| | 堆分析 | xai-grok-shell/heap_profile |
| **工具系统** | 文件读取(read_file) | xai-grok-tools |
| | 文件编辑(search_replace) | xai-grok-tools |
| | 文件写入(write) | xai-grok-tools |
| | 目录列表(list_dir) | xai-grok-tools |
| | 内容搜索(grep) | xai-grok-tools |
| | 文件搜索(glob) | xai-grok-tools |
| | Shell 执行(bash) | xai-grok-tools |
| | Web 搜索 | xai-grok-tools |
| | Web 抓取 | xai-grok-tools |
| | Todo 管理 | xai-grok-tools |
| | Task 管理/调度 | xai-grok-tools |
| | 询问用户 | xai-grok-tools |
| | LSP 集成 | xai-grok-tools |
| | 图片生成/编辑 | xai-grok-tools |
| | 视频生成 | xai-grok-tools |
| | Mermaid 图表渲染 | xai-grok-mermaid |
| | 代码补丁(patch) | xai-grok-tools |
| | 内存工具 | xai-grok-tools |
| | 工具协议 | xai-tool-protocol |
| | 工具运行时 | xai-tool-runtime |
| | 工具类型 | xai-tool-types |
| **MCP 集成** | MCP 服务器管理 | xai-grok-mcp |
| | MCP 工具发现 | xai-grok-mcp |
| | MCP 凭证/OAuth | xai-grok-mcp |
| | MCP HTTP 客户端 | xai-grok-mcp |
| | MCP 存活检测 | xai-grok-mcp |
| **工作区** | 文件系统监控 | xai-fsnotify |
| | Git 状态集成 | xai-gix-status |
| | 代码库图索引 | xai-codebase-graph |
| | 工作树(Worktree) | xai-fast-worktree |
| | 工作区 Hub | xai-grok-workspace |
| | 文件夹信任 | xai-grok-workspace/folder_trust |
| **配置** | 全局配置 | xai-grok-config |
| | 配置类型 | xai-grok-config-types |
| | 项目配置覆盖 | xai-grok-workspace/project_config |
| | 权限管理 | xai-grok-config-types/permission |
| | 模型配置 | xai-grok-shell/agent/config |
| | 环境变量 | xai-grok-env |
| | 路径管理 | xai-grok-paths |
| **认证** | OAuth/OIDC | xai-grok-auth |
| | 设备码认证 | xai-grok-auth/device_code |
| | Token 刷新 | xai-grok-auth/refresh |
| **通信** | ACP 协议 | xai-acp-lib |
| | Leader/Stdio 协议 | xai-grok-shell/leader |
| | Relay 中继 | xai-grok-shell/relay |
| **TUI 前端** | 全屏终端 UI | xai-grok-pager |
| | 多面板布局 | xai-grok-pager-minimal |
| | 渲染引擎 | xai-grok-pager-render |
| | 内联渲染 | xai-ratatui-inline |
| | 文本输入 | xai-ratatui-textarea |
| | 语法高亮 | xai-grok-pager-render/syntax |
| | Markdown 渲染 | xai-grok-markdown |
| | 内联图片 | xai-grok-pager/inline_media |
| **遥测** | 日志/追踪 | xai-grok-telemetry |
| | 指标 | xai-grok-telemetry |
| | 事件 | xai-grok-telemetry |
| | Sentry 集成 | xai-grok-telemetry |
| **其他** | 插件系统 | xai-grok-shell/plugin |
| | 钩子系统 | xai-grok-hooks |
| | 技能系统 | xai-grok-shell/extensions/skills |
| | 更新检测 | xai-grok-update |
| | 公告 | xai-grok-announcements |
| | 采样 | xai-grok-sampler |
| | 沙箱 | xai-grok-sandbox |
| | 密钥管理 | xai-grok-secrets |
| | 提示队列 | xai-prompt-queue |
| | 令牌估算 | xai-token-estimation |
| | 熔断器 | xai-circuit-breaker |
| | 文件工具 | xai-file-utils |
| | 系统电源 | xai-system-power |

### 架构对比

| 层次 | 原系统(Rust) | 重写后(Go) |
|------|-------------|-----------|
| 前端 | xai-grok-pager (ratatui) | **保持不变** (ratatui TUI) |
| 通信 | Leader/Stdio 协议 | **HTTP REST + WebSocket** |
| 后端 | xai-grok-shell | Gin + DDD 分层 |
| LLM | async-openai | Go HTTP Client → MiniMax API |
| 存储 | JSONL + SQLite | GORM + SQLite |
| 工具 | Rust 原生实现 | Go 原生实现 |
| MCP | xai-grok-mcp | Go MCP 客户端 |
| 配置 | TOML (config.toml) | YAML (config.yaml) |

## What Changes
- 后端服务从 Rust 重写为 Go (Gin框架)
- LLM 提供商从 Grok 切换为 MiniMax (MiniMax-M3)
- 架构采用 DDD (领域驱动设计) 分层
- 前端 ratatui TUI 保持不变，通过 HTTP REST + WebSocket 与后端通信
- 工具系统使用 Go 原生实现
- 存储层使用 GORM + SQLite
- 配置格式从 TOML 改为 YAML
- 认证从 OAuth 改为 API Key 直连

## Impact
- Affected specs: 新增 `go-backend` 服务
- Affected code: 新建 `/workspace/go-backend/` 目录，全部代码
- 前端 TUI 代码: 需适配新的 HTTP/WebSocket 通信协议

## ADDED Requirements

### Requirement: Go 后端服务
系统 SHALL 提供基于 Gin 框架的 Go 后端服务，100% 复现原 SpaceXC Grok Build 的全部功能。

### Requirement: MiniMax LLM 集成
系统 SHALL 使用 MiniMax API 作为 LLM 后端：
- Provider: minimax
- Base URL: `https://api.minimaxi.com/v1`
- Model: `MiniMax-M3`
- API Key: 已配置

### Requirement: DDD 架构
系统 SHALL 采用 DDD 分层架构：
- `internal/domain/` — 领域模型和业务逻辑
- `internal/application/` — 应用服务和用例
- `internal/infrastructure/` — 基础设施
- `internal/interfaces/` — HTTP/WebSocket 接口层

### Requirement: HTTP REST + WebSocket 通信
系统 SHALL 提供 HTTP REST API 用于同步操作，WebSocket 用于流式响应。

### Requirement: 完整工具系统
系统 SHALL 实现全部原系统工具。

### Requirement: 会话管理
系统 SHALL 支持会话 CRUD、持久化、压缩、分叉、回退、搜索、导出。

### Requirement: 高级功能
系统 SHALL 实现 SubAgent、Goal、PlanMode、MCP 等全部高级功能。