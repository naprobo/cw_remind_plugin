# CW Remind Plugin

Mattermost 的多阶段期限提醒插件。输入 `/remind` 打开设置窗口，可为同一期限追加多个提醒时点。

详细需求和扩展设计见 [SPEC.md](SPEC.md)。

## 构建

需要 Go 1.24+、Node.js/npm、GNU Make（Windows 可使用 WSL）。

```bash
make dist
```

产物为 `dist/com.cw.remind-0.1.0.tar.gz`。在 Mattermost System Console > Plugins > Plugin Management 上传并启用。服务器需允许插件上传。

### Docker Compose 构建（推荐）

本机只需 Docker，无需安装 Go 或 Node.js：

```bash
docker compose run --rm builder
```

首次会构建包含 Go 1.24 和 Node.js 20 的 builder image。Go module、Go build、npm 及 `node_modules` 使用 Docker named volumes 缓存，后续构建会更快。命令会依次执行格式化、Go 测试、多平台编译、TypeScript 检查、WebApp 构建及打包。

## 开发验证

```bash
go test ./server/...
cd webapp
npm install
npm run check-types
npm run build
```

> MVP 调度器面向单 Mattermost 节点。部署到 HA 集群前，请按 SPEC 中的扩展设计引入集群任务锁。
