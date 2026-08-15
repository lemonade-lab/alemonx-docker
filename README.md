# Docker Compose 管理器

[![CI](https://github.com/lemonade-lab/alemonx-docker/actions/workflows/ci.yml/badge.svg)](https://github.com/lemonade-lab/alemonx-docker/actions/workflows/ci.yml)

这是 ALemonX 的 Docker Compose 插件。它在一个受管项目库中保存多个 Compose 项目，并提供 Docker 环境、镜像和容器的日常管理。

## 功能

- **Compose 项目库**：新建、拖入本地 Compose 文件或从 HTTPS URL 下载；每个项目都有单独的命名目录和 `docker-compose.yml`。
- **可视化编辑**：使用服务表单阅读常见 Compose 配置，同时保留完整高级 YAML 编辑入口。
- **安全生命周期**：从项目页执行 `up -d`、`stop`、`restart`、`down`；关闭项目不会删除卷或镜像。
- **Docker 环境**：检查 Docker CLI、Compose 插件与守护进程；缺失时可调用 ALemonX 固定环境安装流程。
- **资源管理**：查看、拉取和删除镜像；查看容器、最近日志，以及启动、停止、重启容器。
- **容器页面**：识别容器发布的宿主端口，通过宿主同源代理内嵌打开容器 Web UI，并可指定访问路由；也可改用系统浏览器打开。

项目保存在用户配置目录的 `alx-docker/projects/` 下。插件不会原地修改拖入的源文件，也不提供卷删除、容器删除、批量清理或任意命令终端。

## 安装

从 [Releases](https://github.com/lemonade-lab/alemonx-docker/releases) 下载当前平台的压缩包，解压后的 `alemonx-docker` 文件夹放入 ALemonX 插件目录即可。

## 开发

```bash
make check
make web
make build
```

前端使用 React + Vite，runner 使用 Go。前端测试（vitest）通过 `yarn --cwd frontend test` 运行；CI 在每次发布前执行 `go test ./...`、`go vet ./...`、清单校验、前端构建与测试，并为 Linux、macOS（Intel/Apple Silicon）和 Windows 交叉编译。
