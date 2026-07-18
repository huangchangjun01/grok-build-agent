# Tasks: SpaceXC Grok Build Go 重写

## Phase 1: 项目脚手架与基础设施

- [x] Task 1: 初始化 Go 项目结构和 DDD 分层目录
  - 创建 go.mod, 安装 Gin / gorilla/websocket / logrus / viper / mattn/go-sqlite3 等依赖
  - 创建 DDD 四层目录: internal/domain, internal/application, internal/infrastructure, internal/interfaces
  - 创建 cmd/server/main.go 入口

- [x] Task 2: 配置管理系统
  - 实现 config.yaml 配置文件加载（使用 viper）
  - 包含 LLM 配置(MiniMax)、数据库配置、服务端口、日志级别、权限规则、模型参数
  - 支持环境变量覆盖

- [x] Task 3: 日志系统
  - 基于 logrus 实现结构化日志，支持级别、格式化、文件输出

- [x] Task 4: 数据库初始化（SQLite FTS 全文搜索）
  - SQLite 数据库初始化，创建 FTS5 全文搜索表
  - 创建 session 元数据表、索引表
  - 不依赖 GORM，使用原生 database/sql + mattn/go-sqlite3

## Phase 2: 领域模型层 (Domain Layer)

- [x] Task 5: 会话领域模型 (Session Domain)
  - Session 实体、SessionRepository 接口
  - Message 实体（user/assistant/system/tool 角色）
  - ToolCall 实体（请求/响应）
  - Conversation 值对象（消息列表）

- [x] Task 6: 工具领域模型 (Tool Domain)
  - Tool 接口定义（Name, Description, Parameters, Execute）
  - ToolRegistry 接口（注册/查找工具）
  - ToolResult 值对象

- [x] Task 7: Agent 领域模型 (Agent Domain)
  - AgentConfig 值对象（模型、温度、top_p、max_tokens 等）
  - AgentState 枚举（idle, thinking, executing, waiting）
  - AgentContext 聚合根

- [x] Task 8: 工作区领域模型 (Workspace Domain)
  - Workspace 实体（根目录、Git 状态）
  - FileInfo 值对象
  - FileChange 值对象（变更追踪）

## Phase 3: 基础设施层 (Infrastructure Layer)

- [x] Task 9: MiniMax LLM 客户端
  - 实现 OpenAI 兼容的 Chat Completions API 调用
  - 支持 streaming (SSE) 响应解析
  - 支持 function calling (tool use)
  - 支持多模型切换（default/web_search/image/summary）
  - 实现请求重试和熔断器

- [x] Task 10: JSONL 存储 + SQLite 搜索实现
  - 实现 JSONL 文件读写（每行一条 JSON 消息，追加写入，日志式）
  - 实现会话消息的持久化存储（JSONL 主存储）
  - 实现 SQLite FTS5 全文搜索索引（消息内容索引）
  - 实现会话元数据管理（创建/更新时间等）
  - 实现会话搜索（关键词搜索 + 全文搜索）
  - 实现索引更新（JSONL 写入时同步更新 SQLite 索引）

- [x] Task 11: Shell 执行器
  - 实现安全的 shell 命令执行
  - 支持超时控制、工作目录设置、环境变量注入
  - 输出捕获（stdout/stderr）、退出码
  - 支持后台任务执行和输出流式读取

- [x] Task 12: Web 搜索与抓取
  - 实现 Web 搜索功能（集成搜索引擎 API）
  - 实现 Web 页面抓取（HTTP GET + HTML 解析）

- [x] Task 13: 文件系统工具
  - 文件读取（支持分页/行号范围）
  - 文件写入（创建/覆盖）
  - 目录列表
  - 文件搜索 glob
  - 内容搜索 grep（正则匹配）
  - 搜索替换（search_replace）

- [x] Task 14: Git 集成
  - Git 状态查询（分支、变更文件）
  - Git diff 获取
  - Git 文件追踪

- [x] Task 15: 文件系统监控
  - 基于 fsnotify 实现文件变更监控
  - 变更事件去重和聚合

## Phase 4: 工具系统 (Tool System)

- [x] Task 16: 工具注册与调度框架
  - ToolRegistry 实现（注册/查找/列表）
  - 工具执行调度器
  - 工具权限检查

- [x] Task 17: 核心工具实现
  - ReadFile 工具
  - WriteFile 工具
  - SearchReplace 工具
  - ListDir 工具
  - Grep 工具
  - Glob 工具
  - Bash 工具
  - WebSearch 工具
  - WebFetch 工具
  - TodoWrite 工具
  - Task 工具（创建/监控/取消后台任务）
  - AskUserQuestion 工具
  - KillTask 工具
  - TaskOutput 工具（获取后台任务输出）

- [x] Task 18: 高级工具实现
  - ImageGen 工具（图片生成）
  - ImageEdit 工具（图片编辑）
  - VideoGen 工具（视频生成）
  - Mermaid 图表渲染工具
  - LSP 工具集成
  - Memory 工具（存储/搜索记忆）
  - EnterPlanMode / ExitPlanMode 工具
  - UpdateGoal 工具
  - Scheduler 工具（定时任务）

## Phase 5: Agent 核心循环 (Agent Core Loop)

- [x] Task 19: Agent 主循环
  - 实现 Agent 主循环：接收消息 → 构建 prompt → 调用 LLM → 解析响应 → 执行工具 → 循环
  - 支持多轮对话上下文管理
  - 支持 streaming 响应转发到 WebSocket

- [x] Task 20: Prompt 构建器
  - 系统 prompt 模板（Agent 角色定义 + 工具描述 + 规则）
  - 对话历史格式化（user/assistant/tool 消息）
  - Token 限制管理（截断策略）

- [x] Task 21: 工具调用解析与执行
  - 解析 LLM 返回的 function call
  - 并行/串行执行工具调用
  - 工具结果格式化回传 LLM

- [x] Task 22: 对话压缩 (Compaction)
  - 实现对话压缩策略（token 阈值触发）
  - 保留关键上下文（工具调用结果、文件变更）
  - 压缩后重建对话历史

## Phase 6: 会话管理 (Session Management)

- [x] Task 23: 会话 CRUD
  - 创建会话（指定工作目录）
  - 列出会话（支持搜索/过滤）
  - 获取会话详情
  - 删除会话
  - 会话重命名

- [x] Task 24: 会话持久化与恢复
  - 会话自动保存（每轮对话后）
  - 会话恢复（从数据库加载历史）
  - 会话元数据管理（创建时间、更新时间、消息数等）

- [x] Task 25: 会话分叉 (Fork)
  - 从指定消息点分叉新会话
  - 保持分叉历史关联

- [x] Task 26: 会话回退 (Rewind)
  - 回退到指定对话轮次
  - 清理后续消息

- [x] Task 27: 会话导出
  - 导出为 JSON/Markdown 格式
  - 导出指定范围的消息

- [x] Task 28: 会话摘要
  - 自动生成会话标题
  - 会话内容摘要

## Phase 7: 高级功能

- [x] Task 29: SubAgent 子代理系统
  - 子代理创建与管理
  - 子代理间通信
  - 子代理生命周期管理
  - 子代理上下文继承

- [x] Task 30: Goal 目标系统
  - Goal 创建与追踪
  - Goal 规划器（分解为子任务）
  - Goal 策略器（执行策略）
  - Goal 摘要器（进展总结）
  - Goal 验证器（完成检查）
  - Goal 停止检测器

- [x] Task 31: Plan Mode 计划模式
  - 进入/退出计划模式
  - 计划审批流程
  - 计划状态持久化

- [x] Task 32: MCP 集成
  - MCP 客户端实现（JSON-RPC over stdio/HTTP）
  - MCP 服务器管理（启动/停止/监控）
  - MCP 工具发现与注册
  - MCP 存活检测
  - MCP 凭证管理

- [x] Task 33: 懒检测 (Laziness Detection)
  - 检测 Agent 是否陷入死循环
  - 自动干预和提示

- [x] Task 34: 插件系统
  - 插件加载机制
  - 插件钩子注册
  - 技能系统 (Skills)

- [x] Task 35: 遥测与监控
  - 请求/响应指标收集
  - Token 使用统计
  - 错误追踪
  - 会话指标

## Phase 8: HTTP/WebSocket 接口层

- [x] Task 36: Gin HTTP API 路由与中间件
  - 路由注册（RESTful 风格）
  - CORS、日志、恢复中间件
  - 请求验证

- [x] Task 37: 会话 API
  - POST /api/sessions — 创建会话
  - GET /api/sessions — 列出会话
  - GET /api/sessions/:id — 获取会话详情
  - DELETE /api/sessions/:id — 删除会话
  - POST /api/sessions/:id/fork — 分叉会话
  - POST /api/sessions/:id/rewind — 回退会话
  - GET /api/sessions/:id/export — 导出会话
  - GET /api/sessions/search — 搜索会话

- [x] Task 38: 对话 API
  - POST /api/sessions/:id/chat — 发送消息（同步）
  - WebSocket /ws/sessions/:id/chat — 流式对话（WebSocket）

- [x] Task 39: 工具 API
  - GET /api/tools — 列出可用工具
  - POST /api/tools/:name — 直接调用工具

- [x] Task 40: 工作区 API
  - GET /api/workspace/status — 工作区状态
  - GET /api/workspace/files — 文件列表
  - GET /api/workspace/git — Git 状态

- [x] Task 41: 配置 API
  - GET /api/config — 获取配置
  - PUT /api/config — 更新配置

- [x] Task 42: WebSocket 流式通信
  - 实现 WebSocket 连接管理
  - 流式转发 LLM 响应（文本增量）
  - 流式转发工具调用状态
  - 心跳保活

## Phase 9: 前端 TUI 适配

- [ ] Task 43: TUI 通信适配
  - 修改 Rust TUI 前端，将 Leader/Stdio 通信替换为 HTTP/WebSocket
  - 适配新的 API 端点和数据格式
  - 保持原有 UI 交互体验

- [ ] Task 44: 编译与集成
  - 确保 Rust TUI 可以编译并与 Go 后端通信
  - 启动脚本（Go 后端 + TUI 前端联调）

## Phase 10: 测试与验证

- [x] Task 45: 单元测试
  - 领域模型单元测试
  - 工具系统单元测试
  - LLM 客户端单元测试

- [x] Task 46: 集成测试
  - Agent 主循环集成测试
  - 会话管理集成测试
  - API 端到端测试

- [x] Task 47: 启动服务与自测
  - 启动 Go 后端服务
  - 全流程测试：创建会话 → 发送消息 → LLM 响应 → 工具调用 → 结果返回
  - 测试所有核心工具
  - 测试会话管理功能
  - 修复测试中发现的问题

- [ ] Task 48: 推送到 refactor 分支
  - 创建 refactor 分支
  - 提交所有代码
  - 推送到远程仓库

# Task Dependencies

- Task 2-4 依赖 Task 1
- Task 5-8 依赖 Task 1
- Task 9 依赖 Task 2, Task 7
- Task 10 依赖 Task 4, Task 5
- Task 11-15 依赖 Task 1
- Task 16 依赖 Task 6
- Task 17-18 依赖 Task 16, Task 11-15
- Task 19 依赖 Task 7, Task 9
- Task 20-21 依赖 Task 19
- Task 22 依赖 Task 19, Task 5
- Task 23-28 依赖 Task 5, Task 10
- Task 29-32 依赖 Task 19, Task 23
- Task 36-41 依赖 Task 23-28, Task 19
- Task 42 依赖 Task 19, Task 36
- Task 43 依赖 Task 36-42
- Task 44 依赖 Task 43
- Task 45-47 依赖 Task 1-44
- Task 48 依赖 Task 47