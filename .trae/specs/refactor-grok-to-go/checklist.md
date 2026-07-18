# Checklist: SpaceXC Grok Build Go 重写验证

## 项目结构
- [ ] Go 项目目录结构符合 DDD 分层规范
- [ ] go.mod 包含所有必要依赖
- [ ] config.yaml 配置文件完整可加载

## 基础设施
- [ ] MiniMax LLM 客户端可正常调用（streaming + function calling）
- [ ] SQLite 数据库可正常初始化与迁移
- [ ] Shell 执行器可安全执行命令
- [ ] 日志系统正常输出结构化日志

## 领域模型
- [ ] Session/Member/Conversation 领域模型完整
- [ ] Tool/ToolRegistry 接口定义清晰
- [ ] AgentConfig/AgentState 模型完整

## 工具系统
- [ ] ReadFile 工具：支持分页读取，行号范围
- [ ] WriteFile 工具：创建/覆盖文件
- [ ] SearchReplace 工具：精确搜索替换
- [ ] ListDir 工具：列出目录内容
- [ ] Grep 工具：正则搜索文件内容
- [ ] Glob 工具：通配符匹配文件
- [ ] Bash 工具：执行 shell 命令，支持超时
- [ ] WebSearch 工具：网络搜索
- [ ] WebFetch 工具：抓取网页内容
- [ ] TodoWrite 工具：任务列表管理
- [ ] Task 工具：后台任务创建/监控
- [ ] AskUserQuestion 工具：向用户提问
- [ ] EnterPlanMode/ExitPlanMode 工具
- [ ] UpdateGoal 工具

## Agent 核心
- [ ] Agent 主循环：消息 → LLM → 工具调用 → 循环
- [ ] Streaming 响应：SSE 流式解析
- [ ] 工具调用：正确解析 function call 并执行
- [ ] 对话压缩：token 超阈值时自动压缩
- [ ] 多模型切换：default/web_search/image/summary 模型可切换

## 会话管理
- [ ] 会话创建：指定工作目录
- [ ] 会话列表：搜索/过滤
- [ ] 会话持久化：自动保存，可恢复
- [ ] 会话分叉：从指定点分叉
- [ ] 会话回退：回退到指定轮次
- [ ] 会话导出：JSON/Markdown 格式
- [ ] 会话摘要：自动生成标题

## 高级功能
- [ ] SubAgent：创建/管理子代理
- [ ] Goal 系统：创建/规划/追踪/验证
- [ ] Plan Mode：进入/退出/审批
- [ ] MCP 集成：服务器管理/工具发现
- [ ] 懒检测：死循环检测与干预

## HTTP/WebSocket 接口
- [ ] REST API 路由注册正确
- [ ] 会话 CRUD API 全部可用
- [ ] WebSocket 流式对话可正常通信
- [ ] 工具 API 可列出和调用
- [ ] 工作区 API 返回正确状态
- [ ] 配置 API 可读写

## 前端适配
- [ ] Rust TUI 可编译并与 Go 后端通信
- [ ] 流式响应正确渲染在 TUI 中
- [ ] 工具调用状态正确显示

## 自测验证
- [ ] 服务可正常启动，无报错
- [ ] 创建会话 → 发送消息 → 接收流式响应 全流程通过
- [ ] 工具调用（文件读写/Shell执行/Web搜索）全流程通过
- [ ] 会话持久化：重启后恢复会话正常
- [ ] 服务正常关闭，端口释放

## 代码规范
- [ ] 代码遵循 Go 官方编码规范
- [ ] 日志完善，关键路径有日志记录
- [ ] 错误处理完备，无 panic
- [ ] 代码可读性强，注释清晰