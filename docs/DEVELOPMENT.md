# 开发、测试、构建与发布

## 1. 环境

| 依赖 | 版本 |
|---|---|
| Go | 1.25+ |
| Wails | v2.13.x |
| Node.js | 22+ |
| npm | 10+ |
| WebView2 | Windows 现代版运行库 |

安装 Wails：

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
```

## 2. 本地开发

```powershell
git clone https://github.com/ooyyh/webegg.git
cd webegg

cd frontend
npm ci
cd ..

wails dev
```

`wails dev` 会启动 Vite 前端和桌面窗口，并自动生成 Wails 绑定。

## 3. 测试

```powershell
# Go 单元、集成和工具测试
go test ./...
go vet ./...

# 浏览器插件测试
cd extension
npm test
cd ..

# 前端类型检查与生产构建
cd frontend
npm ci
npm run build
cd ..
```

测试覆盖：

- 令牌生成与常量时间校验
- 配置/项目授权/执行模式持久化
- 审批参数哈希和一次性消费
- 文件完整生命周期、路径逃逸、敏感文件
- 目录创建/删除、文件移动/复制
- glob/grep、Shell、后台进程生命周期
- MCP 工具列表、真实调用和租约恢复
- 插件 MCP 客户端、审批重试、Gemini 工具块解析、自动续行规则

## 4. Windows 构建

```powershell
wails generate module
wails build -clean
```

生成：

```text
build/bin/WebEgg.exe
```

生成 NSIS 安装包：

```powershell
wails build -clean -nsis
```

## 5. 插件加载

1. 打开 `chrome://extensions/` 或 `edge://extensions/`。
2. 开启开发者模式。
3. 加载仓库中的 `extension/` 目录。
4. 修改插件代码后点击“重新加载”，并刷新 Gemini 页面。

## 6. GitHub Actions

### CI

`.github/workflows/ci.yml` 在 push/PR 时运行：

- Go 全量测试和 `go vet`
- 插件 Node 测试
- 前端生产构建
- Windows Wails 构建验证

### Release

`.github/workflows/release.yml`：

- push `v*` tag 自动触发
- 也支持手动 `workflow_dispatch` 输入 tag
- 运行测试和 Wails 构建
- 打包 Windows 便携包
- 创建 GitHub Release 并生成变更说明

发布新版本：

```powershell
git tag v0.9.7
git push origin v0.9.7
```

Release 资产：

```text
WebEgg.exe
WebEgg-windows-amd64.zip
```

压缩包内含桌面程序、扩展目录、README 和 docs。

## 7. 常见问题

### 插件一直等待

- 确认 Gemini 标签页仍打开并已登录。
- 在扩展管理页重新加载最新扩展。
- 刷新 Gemini 页面。
- 检查桌面端是否监听 `127.0.0.1:8787`。
- 插件每 30 秒会通过 alarms 主动唤醒内容脚本。

### 任务停在排队/运行中

- 确认扩展和 Gemini 页面仍在运行。
- 重启 WebEgg 会将残留任务标记为中断。
- 命令租约允许另一在线标签页重新领取任务。

### 开发服务器启动后消失

不要使用 `webegg_shell_run` 启动长驻服务器，应使用 `webegg_shell_start`，再用 `webegg_process_logs` 确认监听成功。

### 文件修改冲突

重新调用 `webegg_file_read` 或 `webegg_file_stat` 获取最新 `hash`，作为下一次写操作的 `expectedHash`。

## 8. 目录

```text
webegg/
├── app.go / main.go
├── internal/
├── frontend/
├── extension/
├── docs/
├── scripts/capture-ui.ps1
└── .github/workflows/
```
