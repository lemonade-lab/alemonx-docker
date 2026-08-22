# Docker Compose 管理器

[![CI](https://github.com/lemonade-lab/alemonx-docker/actions/workflows/ci.yml/badge.svg)](https://github.com/lemonade-lab/alemonx-docker/actions/workflows/ci.yml)

这是 ALemonX 的 Docker Compose 插件。它在一个受管项目库中保存多个 Compose 项目，并提供 Docker 环境、镜像和容器的日常管理。

## 功能

- **Compose 项目库**：新建、拖入本地 Compose 文件或从 HTTPS URL 下载；每个项目都有单独的命名目录和 `docker-compose.yml`。
- **可视化编辑**：使用服务表单阅读常见 Compose 配置，同时保留完整高级 YAML 编辑入口。
- **安全生命周期**：从项目页执行 `up -d`、`stop`、`restart`、`down`，以及 `ps` 状态、最近日志和 `.env` 环境变量编辑；关闭项目默认不会删除卷或镜像。
- **Docker 环境**：检查 Docker CLI、Compose 插件与守护进程；缺失时可调用 ALemonX 固定环境安装流程。
- **镜像管理**：查看、拉取、删除、打标签、推送，以及带确认的悬空/未使用镜像清理。
- **容器管理**：创建独立容器（`docker run -d` 白名单表单，仅命名卷）、查看容器（运行中优先）、批量启动/停止/重启、只读详情（inspect）、实时 CPU/内存/网络/磁盘统计与增量实时日志（前端轮询），以及 Compose 项目归属。
- **网络与卷**：查看、创建、只读详情和删除自定义网络与命名数据卷。
- **外部项目发现**：通过 `com.docker.compose.project` 标签展示宿主机上的 Compose 项目，并标记是否已纳入受管库。
- **容器页面**：识别容器发布的宿主端口，通过宿主同源代理内嵌打开容器 Web UI，并可指定访问路由；也可改用系统浏览器打开。
- **模板中心**：项目页的“模板中心”tab 展示仓库内 `recommendations.md` 解析出的清单（内置示例或在线 HTTPS 地址），可一键创建为受管项目。
- **私有仓库**：管理 `docker login` 的仓库凭据（登录/退出、已配置仓库列表）；密码经 `--password-stdin` 传输，绝不回显。

受管 Compose 项目保存在 `<workspace>/store/alemonx-docker/` 下；`<workspace>`
由启动参数 `--workspace` 或 `ALX_WORKSPACE` 决定。这样在 Docker 中挂载工作区后，
项目可随容器重启保留。首次使用新目录时会复制旧版用户配置中的项目，原目录不会
被删除。`docker login` 凭据仍由 Docker 自己的配置目录管理。插件不会原地修改拖入
的源文件，也不提供容器删除或任意命令终端。

### 安全模型

- runner 只接受受校验的项目 ID、镜像引用与资源名称；所有 Docker 命令以参数数组方式执行，并固定为 `docker compose -f <受管文件>`、`docker image`、`docker container`、`docker network`、`docker volume` 的白名单子命令，浏览器不能传入文件系统路径或 shell 文本。
- 危险操作（`down`、镜像清理、网络/卷删除、删除项目）都会在界面二次确认；`down` 绝不附带 `-v`、`--rmi` 等删除参数，不提供容器删除、宿主路径挂载或任意命令终端。
- `.env`、容器创建表单的环境变量与仓库密码只用于本次操作，不会回显到任务记录；命令输出自动截断。
- “实时”日志与统计采用前端 2 秒轮询（`logs --since` 增量、`stats --no-stream`），不新增宿主流式通道。

## 维护推荐清单

`recommendations.md` 位于仓库根目录，贡献者可直接编辑：每个 `## 名称` 小节对应一条推荐，字段用 `- 键：值` 书写：

```markdown
## 我的项目

- 描述：一句话说明
- 标签：web, 数据库
- 地址：https://example.com/docker-compose.yml   # 与“示例”二选一
- 示例：examples/my-project/docker-compose.yml    # 与“地址”二选一
```

没有提供 `地址` 或 `示例` 的条目会被解析器跳过；下载时仍会执行 HTTPS、大小与 Compose 结构校验。发布包会一并携带 `recommendations.md` 与 `examples/`。

## 安装

从 [Releases](https://github.com/lemonade-lab/alemonx-docker/releases) 下载当前平台的压缩包，解压后的 `alemonx-docker` 文件夹放入当前工作区的 `plugins/` 目录即可，即 `<workspace>/plugins/alemonx-docker/`。

## 开发

```bash
make check
make web
make build
```

前端使用 React + Vite，runner 使用 Go。前端测试（vitest）通过 `yarn --cwd frontend test` 运行；CI 在每次发布前执行 `go test ./...`、`go vet ./...`、清单校验、前端构建与测试，并为 Linux、macOS（Intel/Apple Silicon）和 Windows 交叉编译。
