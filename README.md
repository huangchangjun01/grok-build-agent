# Grok Build

基于 Go 语言的终端 AI 编程代理工具，通过 LLM 驱动的 Agent 循环自动完成代码阅读、编写、搜索、执行等编程任务。

## 运行模式

| 模式 | 入口 | 说明 |
|------|------|------|
| **HTTP Server** | `cmd/server/main.go` | REST API + WebSocket，供外部客户端调用 |
| **Bubble Tea TUI** | `cmd/tui/main.go` | 终端原生 UI，交互式对话 |
| **Stdio 协议** | `cmd/stdio/main.go` | Leader/Stdio 协议，作为子进程被外部 TUI 驱动 |

## 快速开始

### 前置要求

- Go 1.25+
- 有效的 MiniMax API Key（或兼容 OpenAI 协议的 LLM 服务）

### 配置

编辑 `config.yaml`：

```yaml
llm:
  provider: "minimax"
  base_url: "https://api.minimaxi.com/v1"
  api_key: "your-api-key"
  default_model: "MiniMax-M3"
```

### 编译

```bash
# HTTP 服务
go build -o bin/grok-server ./cmd/server/

# Bubble Tea TUI
go build -o bin/grok-tui ./cmd/tui/

# Stdio 协议
go build -o bin/grok-stdio ./cmd/stdio/
```

### 运行

```bash
# 启动 HTTP 服务
./bin/grok-server

# 启动 TUI 终端
./bin/grok-tui
```

## 架构

```
├── cmd/                    # 入口程序
│   ├── server/             # HTTP 服务入口
│   ├── tui/                # Bubble Tea TUI 入口
│   └── stdio/              # Stdio 协议入口
├── internal/
│   ├── domain/             # 领域模型层
│   │   ├── agent/          # Agent 配置与状态
│   │   ├── conversation/   # 对话消息模型
│   │   ├── session/        # 会话实体与仓储接口
│   │   ├── tool/           # 工具接口定义
│   │   └── workspace/      # 工作区模型
│   ├── application/        # 应用服务层
│   │   ├── agent/          # Agent 核心循环、Prompt 构建、压缩、子代理、Goal、PlanMode
│   │   ├── session/        # 会话管理服务
│   │   └── tool/           # 工具注册与实现
│   ├── infrastructure/     # 基础设施层
│   │   ├── fs/             # 文件系统操作
│   │   ├── git/            # Git 集成
│   │   ├── llm/            # LLM 客户端（MiniMax / OpenAI 兼容）
│   │   ├── mcp/            # MCP 协议客户端
│   │   ├── persistence/    # SQLite + JSONL 存储
│   │   ├── shell/          # Shell 命令执行器
│   │   ├── telemetry/      # 遥测指标
│   │   └── web/            # Web 搜索与抓取
│   └── interfaces/         # 接口层
│       ├── http/           # Gin REST API + WebSocket
│       ├── tui/            # Bubble Tea 终端 UI
│       └── stdio/          # Leader/Stdio 协议
├── pkg/config/             # 配置系统
└── config.yaml             # 配置文件
```

## 核心功能

### Agent 循环

```
用户消息 → Prompt 构建 → LLM 调用 → 解析响应 → 执行工具 → 循环
```

- 支持多轮对话上下文管理
- 流式响应（SSE / WebSocket）
- 自动对话压缩（token 阈值触发）
- 最大 50 轮工具调用

### 25 个内置工具

| 类别 | 工具 |
|------|------|
| 文件操作 | `read_file`, `write_file`, `search_replace`, `list_dir`, `grep`, `glob` |
| 命令执行 | `bash`, `task`, `kill_task`, `task_output` |
| Web | `web_search`, `web_fetch` |
| 任务管理 | `todo_write`, `scheduler` |
| Git | `git` (status, diff, tracked_files) |
| 交互 | `ask_user_question` |
| 高级 | `enter_plan_mode`, `exit_plan_mode`, `update_goal` |
| 生成 | `image_gen`, `image_edit`, `video_gen`, `mermaid` |
| 其他 | `memory`, `lsp` |

### 会话管理

- 创建 / 列出 / 搜索 / 删除 / 重命名会话
- 会话分叉 (Fork) 与回退 (Rewind)
- 导出为 JSON / Markdown
- 自动生成会话摘要
- JSONL + SQLite 持久化存储

### 高级特性

- **SubAgent**：子代理创建与并行执行
- **Goal System**：目标追踪与规划
- **Plan Mode**：计划模式，先规划后执行
- **MCP**：Model Context Protocol 集成
- **Laziness Detection**：死循环检测与自动干预
- **Telemetry**：请求/Token/工具调用指标收集

## API 端点

### 会话

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/sessions` | 创建会话 |
| GET | `/api/sessions` | 列出会话 |
| GET | `/api/sessions/search?q=` | 搜索会话 |
| GET | `/api/sessions/:id` | 获取会话详情 |
| PUT | `/api/sessions/:id` | 重命名会话 |
| DELETE | `/api/sessions/:id` | 删除会话 |
| POST | `/api/sessions/:id/fork` | 分叉会话 |
| POST | `/api/sessions/:id/rewind` | 回退会话 |
| GET | `/api/sessions/:id/export?format=` | 导出会话 |
| GET | `/api/sessions/:id/messages` | 获取消息列表 |
| POST | `/api/sessions/:id/summary` | 生成摘要 |

### 对话

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/sessions/:id/chat` | 发送消息（同步） |
| WebSocket | `/ws/sessions/:id/chat` | 流式对话 |

### 工具

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/tools` | 列出所有工具 |
| POST | `/api/tools/:name` | 直接调用工具 |

### 工作区

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/workspace/status` | 工作区状态 |
| GET | `/api/workspace/files` | 文件列表 |
| GET | `/api/workspace/git` | Git 状态 |

## 技术栈

| 组件 | 技术 |
|------|------|
| 语言 | Go 1.25 |
| HTTP 框架 | Gin |
| WebSocket | gorilla/websocket |
| TUI | Bubble Tea (charmbracelet) |
| 数据库 | SQLite (modernc.org/sqlite，纯 Go 无需 CGO) |
| 存储 | JSONL + SQLite |
| 日志 | logrus |
| 配置 | viper |
| LLM | MiniMax M3 (OpenAI 兼容) |
| Markdown 渲染 | glamour + lipgloss |

## 配置项

```yaml
server:
  host: "0.0.0.0"       # 监听地址
  port: 8080             # 监听端口

llm:
  provider: "minimax"    # LLM 提供商
  base_url: "..."        # API 地址
  api_key: "..."         # API Key
  default_model: "MiniMax-M3"

database:
  driver: "sqlite"
  dsn: "./data/grok.db"

agent:
  max_turns: 50          # 最大工具调用轮次
  context_window: 200000 # 上下文窗口大小
  compaction_threshold: 0.8  # 压缩阈值
  enable_plan_mode: true
  enable_goal_system: true
  enable_subagent: true
  enable_mcp: true

tools:
  permissions:
    read_file: "allow"   # allow / ask / deny
    bash: "ask"
    write_file: "ask"
    # ...
```

工具权限支持三级控制：`allow`（自动允许）、`ask`（询问用户）、`deny`（禁止）。

## License

MIT