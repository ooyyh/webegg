# WebEgg 架构

## 1. 总览

WebEgg 是单机架构，不依赖云端中转。Wails 桌面进程同时承载 UI、配置、会话存储、本地 HTTP/MCP 服务和工具执行器。

```text
┌──────────────────────────── WebEgg 桌面进程 ────────────────────────────┐
│                                                                        │
│  React UI  ◄──── Wails bindings ────  App / Go services                │
│      │                                  │                              │
│      │ 1. 用户发送任务                  │ 2. 写入 chat + command queue   │
│      ▼                                  ▼                              │
│  ChatStore  ◄──── Bridge ◄──── 127.0.0.1:8787 /api/bridge              │
│                                         ▲                              │
│                                         │ 3. 插件轮询 / 事件回传         │
│                                     MCP /mcp                           │
│                                         │                              │
│                                         ▼                              │
│                              项目沙箱 / 文件 / Git / Shell / 后台进程    │
└────────────────────────────────────────────────────────────────────────┘
                       ▲
                       │ Chrome extension Service Worker
                       │
             Gemini 网页标签页（内容脚本）
```

浏览器仅作为网页 AI 适配器；聊天、工具参数、结果和执行状态全部由 Wails 本地渲染。

## 2. 核心模块

| 模块 | 职责 |
|---|---|
| `app.go` / `main.go` | Wails 生命周期、绑定方法、执行模式、审批和桌面快照 |
| `internal/auth` | `we_` 令牌生成、SHA-256 哈希和常量时间校验 |
| `internal/config` | 项目授权、执行模式、调用熔断和状态目录 |
| `internal/chat` | 会话、消息、工具时间线、归档恢复、使用统计 |
| `internal/bridge` | 网页任务队列、命令租约、活动续租和事件映射 |
| `internal/server` | loopback HTTP、Bearer 校验、MCP 与桥接端点 |
| `internal/tools` | 项目沙箱、文件、搜索、Git、Shell、后台进程 |
| `internal/approval` | 参数哈希绑定、5 分钟有效、一次性本地审批 |
| `extension/` | MV3 后台适配器、保活、自动重注入、Gemini DOM 操作 |
| `frontend/` | Wails React UI、聊天渲染、图表和主题 |

## 3. 会话与桥接

### 3.1 本地会话

- 每个项目有独立会话列表。
- 新建会话自动生成初始化命令。
- 会话消息按 `user → tool… → assistant` 排序。
- 删除为软删除；归档记录可复制为一个新会话。
- 应用重启时 `RecoverInterrupted` 会把残留的排队/运行中状态标记为中断。

### 3.2 命令租约

- 插件轮询 `/api/bridge/commands` 领取任务。
- 每个任务租期 2 分钟。
- 执行中的插件携带 `activeCommandId` 轮询续租。
- 内容脚本中断后，租约会过期，任务可被另一个在线 Gemini 标签页重新领取。
- 完成后通过 `assistant.done` 或 `error` 事件从队列移除。

### 3.3 事件

| 事件 | 结果 |
|---|---|
| `ack` | 助手消息进入运行状态 |
| `assistant.delta` | 更新本地助手正文 |
| `tool.started` | 插入运行中的工具卡片 |
| `tool.finished` | 写入工具结果 |
| `assistant.done` | 完成助手消息和初始化状态 |
| `error` | 标记助手错误并释放命令 |

## 4. 插件运行模型

- `chrome.alarms` 每 30 秒唤醒 Service Worker。
- Service Worker 主动向 Gemini 标签页发送 `webegg:poke`。
- 内容脚本丢失时，通过 `chrome.scripting.executeScript` 重新注入。
- 内容脚本并发处理一个任务；执行期间持续轮询续租。
- 页面不注入 WebEgg UI，只执行输入、发送、回复监听和工具块解析。

## 5. 工具执行与安全

- 文件路径必须是项目相对路径。
- 解析规范真实路径并校验项目根包含性。
- 阻止 `..`、绝对路径、盘符、UNC、NT 设备路径、符号链接逃逸和敏感路径。
- 文件覆盖/补丁/删除必须携带最新 SHA-256。
- 修改前创建隔离备份。
- Shell 使用最小环境变量白名单和输出/超时限制。
- 后台进程拥有独立日志和进程树停止能力。
- 需批准模式下，敏感工具等待本地一次性审批；模型无法自行批准。

## 6. 状态目录

配置、会话、备份和进程日志位于系统用户配置目录：

```text
WebEgg/
├── config.json       # 项目、执行模式、令牌哈希、调用上限
├── chats.json        # 会话、消息和工具历史
├── backups/          # 文件修改/删除备份
└── processes/        # 后台进程日志
```

连接令牌明文只显示一次，配置中仅保存 SHA-256 哈希。

## 7. 已知边界

- Shell 和后台进程不是操作系统沙箱，拥有桌面用户同等权限。
- Gemini 页面 DOM 可能随上游改版变化；适配器需随页面结构更新。
- 浏览器完全关闭时无法执行网页 AI 任务；桌面端会保持任务排队并显示明确状态。
- 后台进程是 WebEgg 进程的子进程；应用正常关闭时会停止，异常崩溃可能需要手动清理残留进程。
