# WebEgg 工具参考

当前注册 26 项 MCP 工具。工具定义由桌面端 `tools/list` 实时提供，提示词不会写死工具列表。

## 权限标记

- **读取**：只读工具，任何执行模式下自动运行。
- **批准**：需批准模式下由本地桌面确认；全自动模式下直接执行。

## 项目与目录

| 工具 | 权限 | 说明 |
|---|---|---|
| `webegg_project_list` | 读取 | 返回授权项目 ID 和名称 |
| `webegg_project_info` | 读取 | 返回单个项目元数据，不暴露物理路径 |
| `webegg_directory_list` | 读取 | 列出项目相对目录，跳过敏感路径和符号链接 |
| `webegg_directory_create` | 批准 | 创建单层目录，父目录必须存在 |
| `webegg_directory_delete` | 批准 | 只允许删除空目录 |

## 文件读取

| 工具 | 权限 | 说明 |
|---|---|---|
| `webegg_file_stat` | 读取 | 文件大小、时间和 SHA-256 |
| `webegg_file_read` | 读取 | 行范围 UTF-8 文本读取，附 hash 和截断信息 |

## 文件编辑

| 工具 | 权限 | 说明 |
|---|---|---|
| `webegg_file_create` | 批准 | 创建新 UTF-8 文件，不覆盖已有文件 |
| `webegg_file_write` | 批准 | 原子覆盖，要求 `expectedHash`，修改前备份 |
| `webegg_file_patch` | 批准 | 唯一文本替换，要求 hash 和唯一匹配 |
| `webegg_file_delete` | 批准 | 备份后删除 |
| `webegg_file_move` | 批准 | 项目内移动/重命名，不覆盖目标 |
| `webegg_file_copy` | 批准 | 复制 UTF-8 文本文件，不覆盖目标 |

所有写操作要求项目相对路径；敏感文件默认拒绝。

## 搜索

| 工具 | 权限 | 说明 |
|---|---|---|
| `webegg_glob` | 读取 | `**` glob 文件匹配 |
| `webegg_grep` | 读取 | Go 正则内容搜索，可附 glob 过滤和结果上限 |

## Git

| 工具 | 权限 | 说明 |
|---|---|---|
| `webegg_git_status` | 读取 | 分支、上游差异和变更条目 |
| `webegg_git_diff` | 读取 | 工作区/暂存区 unified diff |
| `webegg_git_log` | 读取 | 最近提交，不返回作者邮箱 |
| `webegg_git_branches` | 读取 | 本地分支与当前分支 |
| `webegg_git_show` | 读取 | 单次提交元数据和变更统计 |

Git 仓库根必须与授权项目根完全一致；不提供 commit、push、reset 等写操作。

## 执行与后台进程

| 工具 | 权限 | 说明 |
|---|---|---|
| `webegg_shell_run` | 批准 | 短时前台命令，默认 60 秒，有输出限制 |
| `webegg_shell_start` | 批准 | 启动长期后台命令/开发服务器，立即返回进程 ID |
| `webegg_process_list` | 读取 | 列出后台进程、PID、运行状态和退出码 |
| `webegg_process_logs` | 读取 | 读取最近 64 KiB 进程日志 |
| `webegg_process_stop` | 批准 | 停止进程及其子进程树 |
| `webegg_project_script` | 批准 | 运行 package.json 已有 npm/pnpm 脚本，不安装依赖 |

### 长驻服务器规则

开发服务器必须使用 `webegg_shell_start`，不能使用 `webegg_shell_run`。启动后应：

1. 调用 `webegg_process_list` 确认进程仍在运行。
2. 调用 `webegg_process_logs` 确认日志出现监听地址。
3. 只有以上验证成功后才能向用户报告服务器已启动。

## 常见错误码

| 错误码 | 含义 |
|---|---|
| `PROJECT_NOT_FOUND` | 项目授权不存在 |
| `PATH_ESCAPE` | 路径越过项目根 |
| `SENSITIVE_PATH` | 敏感路径被拦截 |
| `SYMLINK_BLOCKED` | 符号链接操作被拒绝 |
| `FILE_CONFLICT` | expectedHash 与当前文件不一致 |
| `PATCH_NOT_UNIQUE` | 补丁匹配次数不是 1 |
| `PARENT_NOT_FOUND` | 父目录不存在 |
| `PROCESS_NOT_FOUND` | 后台进程 ID 不存在 |
| `APPROVAL_REQUIRED` | 本地审批尚未完成 |
| `APPROVAL_REJECTED` | 用户拒绝操作 |
| `TIMEOUT` / `COMMAND_TIMEOUT` | 连接或命令超时 |
| `OUTPUT_TOO_LARGE` | 命令输出超过限制 |

## MCP 调用格式

模型每轮只应输出一个工具块：

```webegg
{"tool":"webegg_file_read","arguments":{"projectId":"proj_xxx","path":"README.md"}}
```

WebEgg 插件解析、执行并通过桥接事件把结果写入本地聊天，然后回传给 Gemini 继续任务。
