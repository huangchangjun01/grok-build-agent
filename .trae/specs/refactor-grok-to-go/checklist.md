# Checklist: SpaceXC Grok Build Go 重写验证

## 项目结构
- [x] Go 项目目录结构符合 DDD 分层规范
- [x] go.mod 包含所有必要依赖
- [x] config.yaml 配置文件完整可加载

## 基础设施
- [x] MiniMax LLM 客户端可正常调用（streaming + function calling）
- [x] SQLite 数据库可正常初始化（LIKE-based search）
- [x] JSONL 文件读写正常（追加写入，日志式存储）
- [x] Shell 执行器可安全执行命令
- [x] 日志系统正常输出结构化日志

## 领域模型
- [x] Session/Message/Conversation 领域模型完整
- [x] Tool/ToolRegistry 接口定义清晰
- [x] AgentConfig/AgentState 模型完整

## 工具系统
- [x] ReadFile 工具：支持分页读取，行号范围
- [x] WriteFile 工具：创建/覆盖文件
- [x] SearchReplace 工具：精确搜索替换
- [x] ListDir 工具：列出目录内容
- [x] Grep 工具：正则搜索文件内容
- [x] Glob 工具：通配符匹配文件
- [x] Bash 工具：执行 shell 命令，支持超时
- [x] WebSearch 工具：网络搜索
- [x] WebFetch 工具：抓取网页内容
- [x] TodoWrite 工具：任务列表管理
- [x] Task 工具：后台任务创建/监控
- [x] AskUserQuestion 工具：向用户提问
- [x] EnterPlanMode/ExitPlanMode 工具
- [x] UpdateGoal 工具

## Agent 核心
- [x] Agent 主循环：消息 → LLM → 工具调用 → 循环
- [x] Streaming 响应：SSE 流式解析
- [x] 工具调用：正确解析 function call 并执行
- [x] 对话压缩：token 超阈值时自动压缩
- [x] 多模型切换：default/web_search/image/summary 模型可切换

## 会话管理
- [x] 会话创建：指定工作目录
- [x] 会话列表：搜索/过滤
- [x] 会话持久化：自动保存，可恢复
- [x] 会话分叉：从指定点分叉
- [x] 会话回退：回退到指定轮次
- [x] 会话导出：JSON/Markdown 格式
- [x] 会话摘要：自动生成标题

## 高级功能
- [x] SubAgent：创建/管理子代理
- [x] Goal 系统：创建/规划/追踪/验证
- [x] Plan Mode：进入/退出/审批
- [x] MCP 集成：服务器管理/工具发现
- [x] 懒检测：死循环检测与干预

## HTTP/WebSocket 接口
- [x] REST API 路由注册正确
- [x] 会话 CRUD API 全部可用
- [x] WebSocket 流式对话可正常通信
- [x] 工具 API 可列出和调用
- [x] 工作区 API 返回正确状态
- [x] 配置 API 可读写

## 自测验证
- [x] 服务可正常启动，无报错
- [x] 创建会话，发送消息，接收流式响应 全流程通过
- [x] 工具调用（文件读写/Shell执行）全流程通过
- [x] 会话持久化：JSONL文件写入正常
- [x] 服务正常关闭，端口释放

## 代码规范
- [x] 代码遵循 Go 官方编码规范
- [x] 日志完善，关键路径有日志记录
- [x] 错误处理完备，无 panic
- [x] 代码可读性强，注释清晰