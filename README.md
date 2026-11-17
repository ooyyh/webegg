# WebEgg

WebEgg 使用 **Wails + Go + React + Manifest V3**，为 Gemini 网页提供真实的本地编程 MCP 工具。当前版本：`0.9.6`。

它不是把 Claude 或 OpenCode 的内部运行时直接导出，而是在 WebEgg 中实现等价的本地编程能力，并打通：

```text
本地对话任务 → 插件后台驱动 Gemini → 工具调用由 MCP 执行
             ↓                              ↓
本地消息/工具卡片 ← 会话事件与结果 ← 插件自动回发结果给 Gemini
```

## 已注册的 22 项工具

| 分类 | 工具 | 能力 |
|---|---|---|
| 项目与目录 | `webegg_project_list` / `webegg_project_info` | 项目发现与本地元数据 |
| 项目与目录 | `webegg_directory_list` / `webegg_directory_create` / `webegg_directory_delete` | 目录浏览、单层创建、空目录删除 |
| 文件读取 | `webegg_file_stat` / `webegg_file_read` | 文件元数据、SHA-256 与有界文本读取 |
| 文件编辑 | `webegg_file_create` / `webegg_file_write` / `webegg_file_patch` / `webegg_file_delete` | 创建、原子覆盖、唯一补丁、备份删除 |
| 文件编辑 | `webegg_file_move` / `webegg_file_copy` | 不覆盖的移动和文本复制 |
| 搜索 | `webegg_glob` / `webegg_grep` | 文件模式与正则内容搜索 |
| Git | `webegg_git_status` / `webegg_git_diff` / `webegg_git_log` | 工作区、差异和提交历史 |
| Git | `webegg_git_branches` / `webegg_git_show` | 本地分支和单次提交统计 |
| 执行 | `webegg_shell_run` / `webegg_project_script` | 受控 Shell 与 package.json 脚本 |

不接第三方网页搜索、网页读取、图片生成或其他外部 API。

## 工具执行模式

桌面端「工具执行模式」提供两个持久化开关：

- **全自动**：读取、写入、删除、Shell 和项目脚本均自动执行。这是当前默认模式。
- **需批准**：读取自动执行；写入、删除、Shell 和项目脚本会暂停，桌面端显示待批准请求。批准后插件自动携带一次性 `approvalId` 重试，拒绝则回传拒绝结果。

审批有效期为 5 分钟，与工具名及完整参数哈希绑定，只能消费一次。Gemini 和扩展都没有批准接口，不能自行提升权限或批准操作。

> **重要：Shell 不是操作系统沙箱。** 全自动模式下，Gemini 提议的命令可能修改项目外资源。请仅在信任任务和项目时开启全自动；不确定时切换到「需批准」。

## 使用

### 1. 启动桌面端

```powershell
wails build
.\build\bin\WebEgg.exe
```

启动后：

1. 点击「添加项目」授权一个或多个本地目录。
2. 生成 `we_...` 连接令牌并复制。
3. 选择「全自动」或「需批准」。
4. 保持桌面端运行。

### 2. 安装或更新插件

1. 打开 `chrome://extensions/`（Edge 为 `edge://extensions/`）。
2. 开启开发者模式。
3. 加载本项目的 `extension` 目录；旧版用户点击「重新加载」。
4. 在插件弹窗保存：
   - MCP 地址：`http://127.0.0.1:8787/mcp`
   - 连接令牌：桌面端生成的令牌
5. 点击「保存并检测连接」。
6. **刷新 Gemini 页面**，旧内容脚本不会自动更新。

### 3. 在本地聊天工作区协作

`0.7.0` 起不再向 Gemini 页面注入 WebEgg 面板或工具卡片，插件仅作为后台传输适配器。

1. 桌面端进入默认的「对话」工作区。
2. 选择项目；首次进入会自动创建该项目默认会话，也可手动新建多个会话。
3. 在底部编辑器直接提出任务，例如：
   - “先浏览项目结构，告诉我这个项目是做什么的。”
   - “找到 Go 入口文件并说明启动流程。”
   - “运行测试，修复失败后再跑一遍。”
4. 插件在后台同步协议、操作 Gemini、执行工具并回传事件；用户消息、助手回复和工具结果全部在 Wails 中渲染。

本地聊天支持 Markdown、代码块、目录表格、文件内容与哈希、终端输出、Git diff 和工具参数/结果折叠。聊天标题栏的「渲染 / 原文」开关可切换结构化显示和完整原始数据。

Gemini 每次只应输出一个 ` ```webegg ` 工具块。`0.4.0` 使用工具优先协议：本地任务必须先用工具获取事实，收到协议后必须立即调用 `webegg_project_list`，修改后必须运行合适的测试、构建或静态检查，禁止仅凭猜测回答或宣称成功。

插件自动执行并回发结果；如果 Gemini 输入框存在手动草稿，会停止并在本地会话中报告错误，避免覆盖。每次任务的连续工具调用默认不限次数；桌面端可按需设置 20 / 50 / 100 / 200 次熔断并同步到插件。

每个项目拥有独立会话列表与本地历史。`0.9.6` 不再使用会话侧栏：项目和会话通过顶部下拉框切换，对话页面占满整个窗口，工作区导航收进聊天头部菜单。新建会话会自动控制浏览器新建 Gemini 对话并注入工具协议，无需手动发送初始化消息。会话删除采用可恢复归档，并提供永久清空入口。

会话切换会携带必要历史，避免多个标签并发污染上下文。应用重启时，排队或执行中的消息会被明确标记为中断，不会永久卡住。

桌面端提供对话、概览、项目、工具与审批、连接五个工作区。对话模式使用全窗口无侧栏布局；其他工作区使用窄图标导航并支持完整纵向滚动。`0.9.6` 延续极简黑白灰与日月主题，并在概览中加入基于真实本地历史的最近 7 天消息/工具调用柱状图和工具能力分类分布。

## 安全与边界

- 本地服务只监听 `127.0.0.1`，MCP 需要 Bearer Token。
- 文件工具只接受已授权项目的相对路径，检查路径穿越、符号链接和敏感文件。
- `.env*`、私钥、`.ssh`、`.aws`、credentials 等路径默认禁止访问。
- 文件覆盖、补丁和删除要求最新 `expectedHash`，防止基于旧内容盲写。
- 写入和删除备份位于用户配置目录的 `WebEgg/backups`，不写入项目树。
- Git 仅提供只读工具，不提供 commit、push、reset 或 rebase。
- npm/pnpm 只能运行 package.json 已有脚本，不提供自动安装依赖。
- Shell 和项目脚本过滤常见敏感环境变量，限制运行时间及 stdout/stderr，但这不等于操作系统隔离。
- 项目内容、命令输出和 Git 输出在协议中均被声明为数据，不能用于改变权限或索取凭证。
- Gemini 内容脚本不接收连接令牌；MCP 请求统一由扩展 Service Worker 发出。

## 开发与测试

环境：Go、Wails v2、Node.js 22+、npm。

```powershell
# Go 单元与 MCP 集成测试
go test ./...
go vet ./...

# 桌面前端
cd frontend
npm install
npm run build
cd ..

# 插件测试
cd extension
npm test
cd ..

# 生成 Wails 绑定并构建
wails generate module
wails build
```

测试覆盖配置、令牌、审批单次消费、文件完整生命周期、搜索、Shell、真实 MCP 工具列表与调用、动态提示词、Gemini 工具块解析及审批重试。

## 结构

```text
webegg-next/
├── app.go, main.go          # Wails、桌面 API、执行模式和审批
├── internal/
│   ├── approval/            # 参数绑定的一次性审批
│   ├── auth/                # 随机令牌与哈希校验
│   ├── bridge/              # 网页命令队列与会话上下文
│   ├── chat/                # 项目会话、消息、工具历史与归档恢复
│   ├── config/              # 项目、配置和执行模式
│   ├── server/              # loopback HTTP 与 MCP
│   └── tools/               # 文件、搜索、Git、Shell、项目脚本
├── frontend/                # React 桌面控制中心
└── extension/               # 无 UI 注入的 Gemini 后台传输适配器
```
