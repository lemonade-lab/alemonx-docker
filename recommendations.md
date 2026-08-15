# 推荐 Compose 项目

<!--
此文件由 alemonx-docker 插件解析为“推荐”列表。贡献者/开发者可直接编辑：

每条推荐使用一个 `## 名称` 小节，字段如下（描述与标签可选）：
- 描述：一句话说明
- 标签：逗号分隔的关键词
- 地址：HTTPS 下载地址，指向 docker-compose.yml（与“示例”二选一）
- 示例：仓库内 examples/<目录>/docker-compose.yml（与“地址”二选一，无需联网）

下载时会套用插件的 HTTPS/大小限制与 Compose 结构校验；未提供地址或示例的
条目会被解析器跳过。
-->

## Paperless-ngx

- 描述：文档管理系统，支持扫描件 OCR 与全文检索
- 标签：文档, OCR, 自托管
- 地址：https://raw.githubusercontent.com/paperless-ngx/paperless-ngx/main/docker/compose/docker-compose.postgres.yml

## Immich

- 描述：自托管照片与视频备份，支持人脸识别与时间线
- 标签：照片, 备份, 自托管
- 地址：https://raw.githubusercontent.com/immich-app/immich/main/docker/docker-compose.yml

## Mealie

- 描述：食谱管理与膳食计划，支持多语言与扫描导入
- 标签：食谱, 生活, 自托管
- 地址：https://raw.githubusercontent.com/mealie-recipes/mealie/mealie-next/docker/docker-compose.yml

## Stirling-PDF

- 描述：本地运行的 PDF 工具箱：合并、拆分、压缩、转换与 OCR
- 标签：PDF, 工具, 隐私
- 地址：https://raw.githubusercontent.com/Stirling-Tools/Stirling-PDF/main/docker/compose/docker-compose.yml

## Wallabag

- 描述：稍后阅读与书签归档，保存网页内容供离线阅读
- 标签：稍后阅读, 书签, 自托管
- 地址：https://raw.githubusercontent.com/wallabag/wallabag/master/compose.yaml

## Changedetection.io

- 描述：网页变化监控，价格、正文或元素变化时发送通知
- 标签：监控, 通知, 自托管
- 地址：https://raw.githubusercontent.com/dgtlmoon/changedetection.io/master/docker-compose.yml

## MinIO

- 描述：兼容 S3 的对象存储，适合私有备份与应用存储
- 标签：对象存储, S3, 备份
- 地址：https://raw.githubusercontent.com/minio/minio/master/docs/orchestration/docker-compose/docker-compose.yaml

## SnowLuma

- 描述：面向 QQ 的 TypeScript 运行时，提供 OneBot v11 与 WebUI 管理（端口 5099，另含 VNC/noVNC）
- 标签：QQ, OneBot, 机器人
- 地址：https://raw.githubusercontent.com/SnowLuma/SnowLuma.Docker.Framework/main/docker-compose.yml

## NapCat

- 描述：基于 NTQQ 的无头 QQ 机器人框架，OneBot 11 与 WebUI（端口 6099）；需要按宿主设置 NAPCAT_UID/NAPCAT_GID
- 标签：QQ, OneBot, 机器人
- 地址：https://raw.githubusercontent.com/NapNeko/NapCat-Docker/main/compose/ws.yml

## LuckyLillia

- 描述：OneBot 11 / Satori / Milky 协议机器人；WebUI 端口 3080，填 AUTO_LOGIN_QQ 可重启自动登录
- 标签：QQ, OneBot, 机器人
- 示例：examples/luckylillia/docker-compose.yml

## Nginx 静态站点

- 描述：轻量示例：用 Nginx 提供静态站点
- 标签：示例, 静态站点
- 示例：examples/nginx-static/docker-compose.yml

## PostgreSQL 17

- 描述：PostgreSQL 17 数据库，导入后请先修改 POSTGRES_PASSWORD
- 标签：数据库, 示例
- 示例：examples/postgres/docker-compose.yml

## Redis 7

- 描述：Redis 7 内存数据库，开启 AOF 持久化
- 标签：缓存, 示例
- 示例：examples/redis/docker-compose.yml

## MinIO

- 描述：S3 兼容对象存储，控制台端口 9001
- 标签：对象存储, 示例
- 示例：examples/minio/docker-compose.yml

## Uptime Kuma

- 描述：自托管监控面板，支持网页与 API 检查
- 标签：监控, 示例
- 示例：examples/uptime-kuma/docker-compose.yml
