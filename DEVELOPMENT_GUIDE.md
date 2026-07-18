# Grok Build 二次开发指南

> 基于 Grok Build (xAI 终端 AI 编码助手) 开源项目，打造你自己的开发工具 Agent

---

## 目录

1. [项目概述与二次开发方向](#1-项目概述与二次开发方向)
2. [环境搭建](#2-环境搭建)
3. [项目结构与核心模块](#3-项目结构与核心模块)
4. [二次开发路径](#4-二次开发路径)
5. [定制化方案详解](#5-定制化方案详解)
6. [开发与调试](#6-开发与调试)
7. [构建与发布](#7-构建与发布)
8. [落地步骤清单](#8-落地步骤清单)

---

## 1. 项目概述与二次开发方向

### 1.1 项目本质

Grok Build 是一个终端级 AI 编码助手，核心能力：

- 与 LLM API 通信，获取推理结果
- 提供 20+ 内置工具（文件操作、终端执行、网络搜索、任务管理等）
- 支持自定义 Agent、插件、MCP 协议、LSP 集成
- TUI 终端界面，Elm-like 事件驱动架构

### 1.2 二次开发方向矩阵

| 方向 | 难度 | 改动规模 | 适用场景 |
|------|------|---------|---------|
| **自定义 Agent** | 低 | 仅添加 Markdown 文件 | 定义特定领域的编码助手 |
| **插件开发** | 中 | 独立仓库/目录 | 添加工具、命令、技能 |
| **品牌定制** | 中 | 修改配置和资源 | 打造自有品牌终端工具 |
| **API 后端替换** | 中 | 修改 sampler 配置 | 接入自有 LLM 服务 |
| **工具扩展** | 中高 | 修改 tools crate | 添加新工具能力 |
| **深度定制** | 高 | 跨 crate 修改 | 完全自定义的 Agent 产品 |

### 1.3 推荐开发路径

```
第一阶段：自定义 Agent（1-2 天）
    ↓
第二阶段：品牌与配置定制（2-3 天）
    ↓
第三阶段：插件开发（3-5 天）
    ↓
第四阶段：工具扩展（5-7 天）
    ↓
第五阶段：深度定制（按需）
```

---

## 2. 环境搭建

### 2.1 系统要求

- **操作系统**：Linux（推荐）、macOS、Windows
- **Rust 工具链**：1.89+（由 `rust-toolchain.toml` 指定）
- **Git**：2.40+
- **依赖库**：
  - Linux：`libssl-dev`, `pkg-config`, `libgit2-dev`
  - macOS：Xcode Command Line Tools
  - Windows：Visual Studio Build Tools

### 2.2 克隆与构建

```bash
# 克隆仓库
git clone https://github.com/huangchangjun01/grok-build-agent.git
cd grok-build-agent

# 安装 Rust 工具链
rustup update
rustup show  # 确认工具链版本

# 安装系统依赖（Ubuntu/Debian）
sudo apt-get install -y libssl-dev pkg-config libgit2-dev cmake

# 构建项目
cargo build --release

# 运行
./target/release/grok --help
```

### 2.3 开发工作流

```bash
# 开发模式构建（快速迭代）
cargo build

# 运行开发版本
cargo run -- [args]

# 运行测试
cargo test

# 运行特定 crate 测试
cargo test -p xai-grok-agent

# 代码检查
cargo clippy --all-targets
```

### 2.4 工作区结构

```
grok-build-agent/
├── Cargo.toml              # 工作区根配置
├── crates/
│   └── codegen/
│       ├── xai-grok-shell/     # 核心运行时（二进制入口）
│       ├── xai-grok-agent/     # Agent 定义与构建
│       ├── xai-grok-tools/     # 工具系统
│       ├── xai-grok-sampler/   # 采样/推理层
│       ├── xai-grok-workspace/ # 工作区管理
│       ├── xai-grok-config/    # 配置管理
│       ├── xai-grok-pager/     # TUI 前端
│       └── ...                 # 其他支撑 crate
├── bin/                    # 辅助脚本
├── prod/                   # 生产部署配置
└── third_party/            # 第三方依赖
```

---

## 3. 项目结构与核心模块

### 3.1 核心 Crate 依赖关系

```
xai-grok-pager (TUI 前端)
    └── xai-grok-shell (核心运行时)
            ├── xai-grok-agent (Agent 构建)
            │       ├── xai-grok-tools (工具系统)
            │       └── xai-grok-config (配置)
            ├── xai-grok-sampler (推理层)
            └── xai-grok-workspace (工作区)
```

### 3.2 关键入口文件

| 文件 | 作用 |
|------|------|
| `crates/codegen/xai-grok-shell/src/bin/main.rs` | 主程序入口 |
| `crates/codegen/xai-grok-shell/src/lib.rs` | Shell 核心模块 |
| `crates/codegen/xai-grok-agent/src/builder.rs` | Agent 构建器 |
| `crates/codegen/xai-grok-agent/src/config.rs` | Agent 定义 |
| `crates/codegen/xai-grok-tools/src/lib.rs` | 工具注册入口 |
| `crates/codegen/xai-grok-sampler/src/client.rs` | API 通信客户端 |
| `crates/codegen/xai-grok-config/src/loader.rs` | 配置加载器 |

### 3.3 扩展点总览

| 扩展点 | 位置 | 方式 |
|--------|------|------|
| 自定义 Agent | `.grok/agents/*.md` | Markdown 文件 |
| 插件 | `~/.grok/plugins/` | 独立仓库/目录 |
| 自定义工具 | `xai-grok-tools` | Rust 代码 |
| 品牌配置 | `config.toml` | 配置文件 |
| API 后端 | `xai-grok-sampler` | 配置/代码 |
| MCP 服务器 | 外部进程 | MCP 协议 |

---

## 4. 二次开发路径

### 4.1 路径一：自定义 Agent（最低门槛）

**适合场景**：想快速创建一个特定领域的编码助手

**改动范围**：零代码，仅添加 Markdown 文件

**步骤**：

1. 在项目根目录创建 `.grok/agents/` 目录
2. 创建 Agent 定义文件

```markdown
<!-- .grok/agents/my-code-reviewer.md -->
---
name: my-code-reviewer
description: 专业代码审查助手，擅长安全审计和性能分析
tools: Read, Grep, Bash
permissionMode: default
---
# 系统提示

你是一个专业的代码审查助手。你的职责是：
1. 审查代码中的安全漏洞
2. 分析性能瓶颈
3. 检查代码风格和最佳实践

对每个问题，请提供：
- 问题描述
- 风险等级（高/中/低）
- 修复建议
- 示例代码
```

3. 使用：

```bash
grok --agent my-code-reviewer
```

**Agent 定义的完整配置项**：

```yaml
---
name: my-agent                    # Agent 名称（必填）
description: 简要描述              # Agent 描述（必填）
tools:                            # 工具白名单
  - Read
  - Bash
  - Grep
  - Edit
  - WebSearch
  - Agent(general-purpose, explore)  # 允许的子代理类型
disallowedTools:                  # 禁止的工具
  - Write
permissionMode: default           # 权限模式: default | yolo | plan
skills:                           # 预加载的技能
  - my-skill
promptMode: extend                # 提示模式: extend | replace
agentsMd: true                    # 是否加载 AGENTS.md
---
# 系统提示正文（Markdown 格式）
```

### 4.2 路径二：品牌定制

**适合场景**：打造自有品牌的终端 AI 工具

**改动范围**：配置文件 + 少量代码

**步骤**：

#### 2.1 修改二进制名称

编辑 `crates/codegen/xai-grok-shell/Cargo.toml`：

```toml
[[bin]]
name = "mydev"          # 自定义二进制名称
path = "src/bin/main.rs"
```

#### 2.2 修改默认配置

创建 `~/.grok/config.toml`（或修改代码中的默认值）：

```toml
[model]
default = "grok-4"

[permissions]
default_mode = "default"

[ui]
theme = "dark"
```

#### 2.3 自定义系统提示

编辑 `crates/codegen/xai-grok-agent/src/config.rs` 中的 `AgentDefinition::default_grok_build()` 方法，修改 `prompt_body` 字段。

#### 2.4 修改品牌名称

在代码中搜索替换品牌相关字符串：

- 命令名称：`grok` → `mydev`
- 配置目录：`~/.grok/` → `~/.mydev/`
- 环境变量前缀：`GROK_` → `MYDEV_`

**关键修改文件**：

| 文件 | 修改内容 |
|------|---------|
| `xai-grok-config/src/paths.rs` | 配置目录路径 |
| `xai-grok-shell/src/bin/main.rs` | CLI 名称和描述 |
| `README.md` | 项目文档 |
| 各 crate 的 `Cargo.toml` | 包名和描述 |

### 4.3 路径三：插件开发

**适合场景**：添加可复用的工具、命令或技能

**改动范围**：独立仓库，零侵入

**插件目录结构**：

```
my-plugin/
├── .claude-plugin/
│   └── plugin.json       # 插件清单（必填）
├── skills/               # 技能定义
│   └── my-skill/
│       └── SKILL.md
├── commands/             # 自定义命令
│   └── my-command.md
├── agents/               # 自定义 Agent
│   └── my-agent.md
├── hooks/                # 生命周期钩子
│   └── hooks.json
└── mcp_servers/          # MCP 服务器配置
    └── my-server.json
```

**插件清单示例**：

```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "我的自定义插件",
  "author": "Your Name",
  "homepage": "https://github.com/you/my-plugin",
  "components": {
    "skills": ["my-skill"],
    "commands": ["my-command"],
    "agents": ["my-agent"]
  }
}
```

**技能文件示例**（`skills/my-skill/SKILL.md`）：

```markdown
---
name: my-skill
description: 自动生成 API 文档
---

# API 文档生成技能

## 触发条件
当用户要求生成 API 文档时激活。

## 工作流程
1. 扫描项目中的 API 路由定义
2. 提取请求/响应结构
3. 生成 OpenAPI 规范文档
4. 生成 Markdown 格式的 API 文档
```

**安装插件**：

```bash
# 从本地目录安装
grok plugin install ./my-plugin

# 从 Git 仓库安装
grok plugin install https://github.com/you/my-plugin.git

# 从市场安装
grok plugin install my-plugin
```

### 4.4 路径四：API 后端替换

**适合场景**：接入自有 LLM 服务或使用其他模型提供商

**改动范围**：配置文件 + sampler 代码

**方式一：通过配置修改 API 端点**

```toml
# ~/.grok/config.toml
[api]
base_url = "https://your-api.example.com/v1"
api_key = "your-api-key"

[model]
default = "your-model-name"
```

**方式二：修改 sampler 代码**

编辑 `crates/codegen/xai-grok-sampler/src/client.rs`，修改 `SamplingClient` 的 API 端点配置。

**方式三：添加新的 API 后端**

在 `xai-grok-sampler` 中实现新的后端适配器，参考现有的三种后端：

- `ChatCompletions`：OpenAI 兼容 API
- `Responses`：OpenAI Responses API
- `Messages`：Anthropic Messages API

### 4.5 路径五：工具扩展

**适合场景**：添加自定义工具能力

**改动范围**：`xai-grok-tools` crate

**添加新工具的步骤**：

1. 在 `crates/codegen/xai-grok-tools/src/implementations/grok_build/` 下创建新模块

```rust
// my_tool/mod.rs
use xai_grok_tools::types::tool::{NewTool, ToolContext, ToolResult};

pub struct MyTool;

impl MyTool {
    pub fn new() -> Self {
        Self
    }
}

#[async_trait::async_trait]
impl NewTool for MyTool {
    fn namespace(&self) -> &str {
        "GrokBuild"
    }

    fn name(&self) -> &str {
        "my_tool"
    }

    fn description(&self) -> &str {
        "我的自定义工具：执行 XXX 操作"
    }

    fn parameters(&self) -> serde_json::Value {
        serde_json::json!({
            "type": "object",
            "properties": {
                "param1": {
                    "type": "string",
                    "description": "参数1描述"
                }
            },
            "required": ["param1"]
        })
    }

    async fn execute(
        &self,
        context: &ToolContext,
        params: serde_json::Value,
    ) -> ToolResult {
        // 实现工具逻辑
        let param1 = params["param1"].as_str().unwrap_or("");
        // ... 执行操作 ...
        ToolResult::ok(format!("执行结果: {}", param1))
    }
}
```

2. 在 `mod.rs` 中注册新工具

```rust
pub mod my_tool;

// 在 register_all() 或类似函数中
tool_config.tools.push((&my_tool::MyTool::new()).into());
```

### 4.6 路径六：深度定制

**适合场景**：打造完全自定义的 AI Agent 产品

**改动范围**：跨多个 crate 的深度修改

**主要方向**：

| 方向 | 说明 | 涉及 crate |
|------|------|-----------|
| 自定义 TUI | 替换终端 UI | `xai-grok-pager` |
| 自定义 Agent 运行时 | 修改 Agent 执行逻辑 | `xai-grok-shell` + `xai-grok-agent` |
| 自定义工作区 | 修改文件系统操作 | `xai-grok-workspace` |
| 自定义配置系统 | 修改配置加载逻辑 | `xai-grok-config` |
| 自定义采样 | 修改推理流程 | `xai-grok-sampler` |

---

## 5. 定制化方案详解

### 5.1 方案一：快速原型（1-2 天）

**目标**：创建一个有自定义名称和 Agent 的终端工具

**步骤**：

```bash
# 1. Fork 仓库
git clone https://github.com/your-org/grok-build-agent.git my-dev-agent
cd my-dev-agent

# 2. 创建自定义 Agent
mkdir -p .grok/agents
cat > .grok/agents/default.md << 'EOF'
---
name: my-dev
description: 我的开发助手
tools: Read, Edit, Bash, Grep, WebSearch, Agent(general-purpose)
permissionMode: default
---
你是一个专业的开发助手，专注于帮助用户完成软件开发任务。
EOF

# 3. 修改二进制名称
sed -i 's/name = "grok"/name = "mydev"/' crates/codegen/xai-grok-shell/Cargo.toml

# 4. 构建
cargo build --release

# 5. 测试
./target/release/mydev --help
```

### 5.2 方案二：品牌产品（1-2 周）

**目标**：创建完整的自有品牌 AI 编码工具

**关键修改清单**：

1. **二进制名称**：`grok` → `mydev`
2. **配置目录**：`~/.grok/` → `~/.mydev/`
3. **环境变量**：`GROK_*` → `MYDEV_*`
4. **系统提示**：自定义 Agent 角色和行为
5. **默认模型**：配置默认使用的 LLM 模型
6. **品牌元素**：Logo、配色、文档

**代码修改清单**：

| 文件路径 | 修改项 |
|---------|--------|
| `xai-grok-shell/Cargo.toml` | `[[bin]] name` |
| `xai-grok-config/src/paths.rs` | `grok_home()` 等函数 |
| `xai-grok-shell/src/bin/main.rs` | CLI 名称、描述、版本 |
| `xai-grok-agent/src/config.rs` | 默认 Agent 定义 |
| `xai-grok-pager/src/app/mod.rs` | UI 标题 |
| 各 crate `Cargo.toml` | 包名和描述 |

### 5.3 方案三：垂直领域工具（2-4 周）

**目标**：针对特定领域（如安全审计、性能优化、DevOps）的专用工具

**步骤**：

1. 创建领域专用 Agent 定义
2. 开发领域专用技能（Skills）
3. 添加领域专用工具（如漏洞扫描、性能分析）
4. 集成领域专用 MCP 服务器
5. 定制 TUI 界面

---

## 6. 开发与调试

### 6.1 本地开发

```bash
# 开发模式运行
cargo run -- --no-alt-screen

# 启用调试日志
RUST_LOG=debug cargo run

# 指定日志级别
RUST_LOG=xai_grok_agent=trace,xai_grok_sampler=debug cargo run

# 无头模式测试
cargo run -- --headless --print "你好，请分析这个项目"
```

### 6.2 测试

```bash
# 运行所有测试
cargo test --workspace

# 运行特定 crate 测试
cargo test -p xai-grok-agent

# 运行测试并显示输出
cargo test -- --nocapture

# 运行特定测试
cargo test -p xai-grok-agent test_name
```

### 6.3 代码质量

```bash
# Clippy 检查
cargo clippy --all-targets --all-features

# 格式化
cargo fmt --all

# 检查格式
cargo fmt --all -- --check
```

### 6.4 常见问题

**Q: 编译失败，提示缺少 OpenSSL**
```bash
# Ubuntu/Debian
sudo apt-get install libssl-dev pkg-config
# macOS
brew install openssl
```

**Q: 运行时找不到配置文件**
```bash
# 创建默认配置目录
mkdir -p ~/.grok
# 或设置自定义路径
export GROK_HOME=/path/to/config
```

**Q: API 认证失败**
```bash
# 设置 API Key
export GROK_API_KEY=your-api-key
# 或写入配置文件
echo '[api]\napi_key = "your-api-key"' >> ~/.grok/config.toml
```

---

## 7. 构建与发布

### 7.1 发布构建

```bash
# 优化构建
cargo build --release

# 查看二进制大小
ls -lh target/release/grok

# 使用 strip 减小体积
strip target/release/grok
```

### 7.2 交叉编译

```bash
# 添加目标平台
rustup target add x86_64-unknown-linux-musl
rustup target add x86_64-apple-darwin
rustup target add x86_64-pc-windows-msvc

# 交叉编译
cargo build --release --target x86_64-unknown-linux-musl
```

### 7.3 打包分发

```bash
# 创建发布包
mkdir -p dist/mydev-1.0.0-linux-x86_64/
cp target/release/mydev dist/mydev-1.0.0-linux-x86_64/
cp README.md LICENSE dist/mydev-1.0.0-linux-x86_64/
tar -czf dist/mydev-1.0.0-linux-x86_64.tar.gz -C dist mydev-1.0.0-linux-x86_64/
```

### 7.4 版本管理

```bash
# 更新版本号
# 编辑各 crate 的 Cargo.toml 中的 version 字段

# 创建 Git 标签
git tag v1.0.0
git push origin v1.0.0
```

---

## 8. 落地步骤清单

### 第一阶段：基础搭建（第 1-2 天）

- [ ] Fork 项目仓库
- [ ] 搭建 Rust 开发环境
- [ ] 成功构建项目（`cargo build --release`）
- [ ] 运行并测试基本功能
- [ ] 创建项目级自定义 Agent 定义文件
- [ ] 验证自定义 Agent 可用

### 第二阶段：品牌定制（第 3-5 天）

- [ ] 修改二进制名称为自定义名称
- [ ] 修改配置目录路径
- [ ] 修改环境变量前缀
- [ ] 自定义默认系统提示
- [ ] 修改 CLI 帮助文本和描述
- [ ] 更新项目 README 和文档
- [ ] 构建并验证品牌定制效果

### 第三阶段：功能扩展（第 6-10 天）

- [ ] 开发自定义技能（Skills）
- [ ] 开发自定义命令
- [ ] 创建插件包
- [ ] 测试插件安装和使用
- [ ] 配置 MCP 服务器（如需要）
- [ ] 扩展自定义工具（如需要）

### 第四阶段：API 适配（第 11-13 天）

- [ ] 配置 API 端点
- [ ] 配置默认模型
- [ ] 测试 API 连接
- [ ] 调优采样参数
- [ ] 测试重试和错误处理

### 第五阶段：测试与发布（第 14-15 天）

- [ ] 运行完整测试套件
- [ ] 执行代码质量检查（clippy + fmt）
- [ ] 构建发布版本
- [ ] 编写使用文档
- [ ] 创建发布包
- [ ] 发布到 GitHub Releases
- [ ] 编写 CHANGELOG

### 第六阶段：持续迭代（长期）

- [ ] 收集用户反馈
- [ ] 修复 Bug
- [ ] 添加新功能
- [ ] 更新 Agent 定义
- [ ] 同步上游更新
- [ ] 优化性能
- [ ] 扩展文档

---

## 附录

### A. 关键 Crate 参考

| Crate | 版本 | 功能 |
|-------|------|------|
| `xai-grok-shell` | 0.2.102 | 核心运行时，二进制入口 |
| `xai-grok-agent` | - | Agent 定义与构建 |
| `xai-grok-tools` | - | 工具注册与执行 |
| `xai-grok-sampler` | - | LLM API 通信 |
| `xai-grok-workspace` | - | 文件系统与工作区 |
| `xai-grok-config` | - | 配置管理 |
| `xai-grok-pager` | - | TUI 前端 |

### B. 重要环境变量

| 变量 | 说明 |
|------|------|
| `GROK_HOME` | 配置目录（默认 `~/.grok`） |
| `GROK_API_KEY` | API 认证密钥 |
| `RUST_LOG` | 日志级别控制 |
| `GROK_WEB_FETCH` | 启用 Web Fetch 功能 |
| `GROK_DISABLE_TELEMETRY` | 禁用遥测 |

### C. 配置文件示例

```toml
# ~/.grok/config.toml
[api]
base_url = "https://api.x.ai/v1"
api_key = "your-api-key"

[model]
default = "grok-4"

[permissions]
default_mode = "default"

[ui]
theme = "dark"

# 自定义 Agent 路径
[agents]
project_paths = [".grok/agents"]

# 插件配置
[plugins]
enabled = true
auto_update = true

# 工具配置
[toolset.bash]
max_timeout_secs = 600

[toolset.ask_user_question]
timeout_enabled = true
timeout_secs = 300
```

### D. 常用命令速查

```bash
# 启动交互式会话
grok

# 使用特定 Agent
grok --agent my-agent

# 恢复上次会话
grok --resume

# 继续上次会话
grok --continue

# 聊天模式
grok --chat

# 无头模式
grok --headless --print "查询"

# 安装插件
grok plugin install ./my-plugin

# 列出插件
grok plugin list

# 更新插件
grok plugin update

# 管理市场
grok plugin marketplace add https://github.com/user/marketplace.git
grok plugin marketplace list
```

---

> **提示**：本文档基于 [Grok Build 系统分析报告](SYSTEM_ANALYSIS_REPORT.md) 编写，建议先阅读分析报告以深入理解系统架构，再参考本指南进行二次开发。