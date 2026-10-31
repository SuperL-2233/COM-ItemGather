# COM-ItemGather

供两名成员使用的《Call of Mommy》商品创意库。核心流程是：随手输入 → 立即保存 → 后台 AI 整理 → 搜索与编辑 → 手动关联商品。

## 架构

- Vue 3 + TypeScript + Vite，响应式桌面/手机界面、PWA、IndexedDB 草稿和离线待同步队列。
- Go 1.24 后端，REST API、服务端 Session、SSE、持久化 AI Worker。
- SQLite（WAL、外键、busy timeout、版本化迁移），附件保存在受保护文件目录。
- 生产环境复用现有 **Nginx**：直接提供 `web/dist` 静态文件，并将 `/api/` 反向代理到 `127.0.0.1:8088`。不需要 Caddy，也不运行常驻 Node 服务。
- 唯一外部 AI 服务是 OpenAI-compatible Chat Completions API；图片不会发送给 AI。

## 目录

```text
cmd/com-itemgather/       服务与运维 CLI
internal/app/             API、认证、AI、SSE、附件和业务逻辑
internal/app/ai_prompt_v1.txt  独立版本化 AI System Prompt
internal/backup/          一致性备份归档与保留策略
internal/config/          环境变量配置
internal/database/        SQLite 连接与嵌入式迁移
web/src/                  Vue 页面、组件、API、离线与实时同步
web/public/               PWA Manifest、Service Worker 和图标
deploy/nginx/             独立 Nginx 站点示例
deploy/systemd/           后端服务示例
deploy/scripts/           构建与备份脚本
```

## 本地开发

要求 Go 1.24+、Node.js 18+、npm。应用不会自动读取 `.env`；本地可以用 `set -a; source .env; set +a` 加载，systemd 会使用 `EnvironmentFile`。

```bash
cp .env.example .env
# 编辑 .env：开发时设置 APP_ENV=development、APP_PUBLIC_URL=http://localhost:8088、
# COOKIE_SECURE=false，并将数据/备份路径改成本地可写路径。
set -a; source .env; set +a

go mod download
ADMIN_PASSWORD='请替换为至少10个字符的密码' \
  go run ./cmd/com-itemgather create-admin -username admin -nickname 管理员
```

启动后端（默认 `127.0.0.1:8088`）：

```bash
go run ./cmd/com-itemgather serve
```

另开终端启动前端开发服务器（`http://localhost:5173`，代理 `/api` 到 8088）：

```bash
cd web
npm ci
npm run dev
```

也可以先执行 `npm run build`，再只启动 Go，通过 `http://localhost:8088` 使用完整网站。开发环境未配置 AI Key 时，灵感仍会正常保存；整理任务会失败并可重试。

## 功能与数据设计

- 灵感：首次原文与当前正文分离；名称候选、内容类型、商品方向、标签和动态属性都允许为空。收藏按用户隔离。
- 商品：内部工作名称与最终品牌名分离，支持制作状态、动态属性和图片。
- 关联：独立多对多表，状态为备选/已采用/不采用；AI 不会创建商品或关联。
- 并发：灵感与商品都有 `version`，更新冲突返回 409；前端显示差异并允许合并。完整前后快照保存在 `revisions`，恢复会产生新版本。
- AI：`ai_tasks` 持久化，Worker 原子认领任务；内容版本和字段级人工修改状态防止旧结果覆盖。默认最多请求 4 次（首次 + 3 次重试）。
- 认证：Argon2id 密码散列；Session、邀请、重置令牌在数据库中只保存哈希；HttpOnly/SameSite Cookie 与 CSRF 校验。
- 附件：后端检查真实图片内容、像素、大小、数量和总配额；使用内部 ID 命名，下载必须经过登录。
- 搜索：参数化 SQLite `LIKE` 子串查询，可靠支持中文短词，覆盖正文、名称、标签、分类、描述和动态属性；可按记录类型、内容类型、商品方向和创建者筛选。
- 限流：登录失败单独限流，普通 API 按匿名 IP 或登录 Session 限制为每分钟 300 次；SSE 使用独立长连接。
- 实时同步：`events` 表保存递增事件 ID，SSE 支持 Last-Event-ID、心跳和重连后的全量必要刷新。

主要表：`users`、`sessions`、`invitations`、`password_reset_tokens`、`ideas`、`idea_attributes`、`products`、`product_attributes`、`idea_product_links`、`favorites`、`tags`、`idea_tags`、`attachments`、`revisions`、`ai_tasks`、`events`、`audit_logs`。迁移位于 `internal/database/migrations/`，按版本执行，不会删库重建。

## API 概览

所有业务 API 都要求登录；非 GET 请求要求当前 Session 的 `X-CSRF-Token`。错误统一为：

```json
{"error":{"code":"version_conflict","message":"这条灵感已被其他人修改","details":{}}}
```

- `/api/auth/*`：登录、退出、当前用户、邀请注册、资料、密码修改和重置。
- `/api/sessions*`：设备列表、撤销指定设备、退出其他设备。
- `/api/admin/*`：账号、邀请、密码重置（仅管理员）。
- `/api/ideas`、`/api/products`：列表、创建、详情、编辑、软删除、恢复、永久删除和历史恢复。
- `/api/ideas/:id/favorite`、`/api/ideas/:id/ai`：收藏、重新整理。
- `/api/links`：手动关联、修改采用状态、解除关联。
- `/api/attachments`：图片上传、认证下载、删除。
- `/api/search`、`/api/tags`、`/api/trash`、`/api/export`。
- `/api/events`：SSE；`/api/system/status`：不泄露密钥的系统状态；`/api/system/backup`：管理员立即备份。

灵感创建接受 `client_id` 作为幂等键。`PATCH` 和删除操作携带 `version`；旧版本返回 HTTP 409，不会静默覆盖。

## 环境变量

完整示例见 `.env.example`。主要配置：

| 变量 | 默认值/说明 |
|---|---|
| `APP_ENV` | `development`；生产设 `production` |
| `APP_LISTEN_ADDR` | `127.0.0.1:8088` |
| `APP_PUBLIC_URL` | 公开站点 URL，用于邀请/重置链接 |
| `DATABASE_PATH` / `ATTACHMENT_DIR` | SQLite 与附件路径 |
| `AI_BASE_URL` / `AI_API_KEY` | 兼容接口地址与仅服务端保存的密钥 |
| `AI_MODEL` | `deepseek-v4-flash` |
| `AI_TIMEOUT` / `AI_MAX_RETRIES` | `60s` / `3`（额外重试） |
| `AI_WORKER_CONCURRENCY` | `1` |
| `SESSION_DEFAULT_DAYS` / `SESSION_REMEMBER_DAYS` | `7` / `90` |
| `MAX_UPLOAD_SIZE` | `10MB` |
| `MAX_ATTACHMENTS_PER_IDEA` / `MAX_STORAGE_MB` | `10` / `2048` |
| `BACKUP_DIR` / `BACKUP_INTERVAL` | `backups` / `24h` |
| `BACKUP_RETENTION_DAYS` / `BACKUP_KEEP` | `30` 天 / 最多 `10` 份 |
| `COOKIE_SECURE` | HTTPS 自动为 true；生产必须保持 true |

旧的 `ADDR`、`PUBLIC_URL`、`AI_WORKERS`、`SESSION_TTL`、`REMEMBER_TTL`、`MAX_UPLOAD_MB` 和 `MAX_ATTACHMENTS` 仍兼容，但新部署应使用上表名称。

## 生产构建

```bash
./deploy/scripts/build.sh
# 或分别执行：
cd web && npm ci && npm run build && cd ..
mkdir -p bin
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o bin/com-itemgather ./cmd/com-itemgather
```

产物为 `web/dist/` 和 `bin/com-itemgather`。前端不需要 Node 常驻进程。

## 首次部署（现有 Nginx）

以下以 `/srv/com-itemgather` 和 `ideas.example.com` 为例，按实际环境调整；不要覆盖其他站点或证书配置。

1. 构建并复制项目，创建专用低权限用户以及可写的数据、附件和备份目录。
2. 将 `.env.example` 复制为 `/etc/com-itemgather/com-itemgather.env`，修改域名、路径和 AI 配置，权限设为仅管理员与服务账号可读（例如 `chmod 600`）。
3. 安装并启动后端服务：

   ```bash
   sudo cp deploy/systemd/com-itemgather.service /etc/systemd/system/
   sudo systemctl daemon-reload
   /srv/com-itemgather/bin/com-itemgather migrate
   ADMIN_PASSWORD='替换为强密码' /srv/com-itemgather/bin/com-itemgather \
     create-admin -username admin -nickname 管理员
   sudo systemctl enable --now com-itemgather
   journalctl -u com-itemgather -f
   ```

4. 编辑 `deploy/nginx/com-itemgather.conf.example` 中的域名、现有证书路径和静态目录。**不要运行其他证书工具，也不要替换现有证书配置。**
5. 如果目标站点文件已存在，先备份。将配置安装为独立站点并创建对应软链接；不要修改其他 `server` 块。示例：

   ```bash
   sudo cp /etc/nginx/sites-available/com-itemgather \
     /etc/nginx/sites-available/com-itemgather.before.$(date +%Y%m%d%H%M%S)  # 仅当文件已存在
   sudo cp deploy/nginx/com-itemgather.conf.example /etc/nginx/sites-available/com-itemgather
   sudo ln -s /etc/nginx/sites-available/com-itemgather /etc/nginx/sites-enabled/com-itemgather
   sudo nginx -t
   ```

   `nginx -t` 通过后，**只有获得明确授权**才执行 `sudo systemctl reload nginx`。本项目不会自动重启或重载 Nginx。

Nginx 示例包含 Vue History 模式的 `try_files ... /index.html`、静态哈希资源缓存、`/api/` 代理，以及 `/api/events` 的 `proxy_buffering off`、`proxy_cache off` 和 3600 秒超时。Go 同时发送 `X-Accel-Buffering: no` 和 20 秒 SSE 心跳。

## 备份、导出与恢复

设置页可导出不含认证信息的 JSON，包括灵感、商品、属性、标签和关联。完整服务器迁移使用备份归档，其中包含 `database.db` 与 `attachments/`。

```bash
./deploy/scripts/backup.sh
# 或加载环境后执行 bin/com-itemgather backup
journalctl -u com-itemgather --since today | grep -E '备份|backup'
```

备份使用 SQLite `VACUUM INTO` 生成一致性快照，再打包附件；不会直接复制在线数据库文件。归档同时受保留天数与份数限制。

恢复前必须停止后端，并另外保存当前数据。加载部署环境变量后，使用带明确确认的恢复命令：

```bash
sudo systemctl stop com-itemgather
set -a; source /etc/com-itemgather/com-itemgather.env; set +a
/srv/com-itemgather/bin/com-itemgather restore -archive /path/to/backup.tar.gz -yes
/srv/com-itemgather/bin/com-itemgather migrate
sudo systemctl start com-itemgather
```

`restore` 会拒绝不安全路径、非普通文件和缺少有效迁移记录的归档，并在替换前执行 SQLite 完整性检查。被替换的数据库、旧 WAL 文件和附件目录会以 `.pre-restore-唯一ID` 后缀保留；命令会打印这些路径。没有 `-yes` 时不会修改数据。不要删除唯一的恢复前副本。

恢复后登录，检查灵感、商品、关联和图片。附件路径只能由应用内部 ID 组成，不应对外公开 Nginx 静态目录。

## 运维 CLI

```bash
bin/com-itemgather serve
bin/com-itemgather migrate
ADMIN_PASSWORD='强密码' bin/com-itemgather create-admin -username admin -nickname 管理员
RESET_PASSWORD='新的强密码' bin/com-itemgather reset-password -username admin
bin/com-itemgather backup
bin/com-itemgather restore -archive /path/to/backup.tar.gz -yes
```

密码参数也支持 `-password`，但环境变量可避免出现在进程列表中。初始管理员只能由 CLI 创建；网页没有公开注册入口。

## 测试

```bash
go test ./...
go vet ./...
./deploy/scripts/build.sh
cd web
npx playwright install chromium
npm run test:e2e
```

AI 集成测试使用本机可控的 OpenAI-compatible 模拟服务，不消耗真实模型额度。浏览器测试启动真实 Go 后端、生产 Vue 静态文件和独立的临时数据库，覆盖持久登录、双人 SSE、商品关联、IndexedDB 草稿、断网重试不重复创建和纯图片投稿；执行前需先完成上述构建。

本次验收实际通过：

- `go test ./...` 与 `go vet ./...`，包括认证、权限、AI 失败重试、人工字段保护、Worker 重启恢复、并发冲突、历史恢复、SSE、附件、搜索筛选、限流和备份恢复。
- `./deploy/scripts/build.sh`：Vue 类型检查、生产静态构建和 Go 二进制构建。默认 npm 源在当前网络下超时，使用 `NPM_CONFIG_REGISTRY=https://registry.npmmirror.com` 执行同一脚本后通过。
- `npm run test:e2e`：Chromium 中 3 个场景全部通过，测试使用真实服务而非 Mock 前端 API。
- 在隔离的临时数据目录中实际执行 `create-admin`、`backup`、带确认的 `restore`、`migrate` 和 `reset-password`，均成功；缺少 `-yes` 的恢复命令会拒绝修改。

当前开发机没有 `nginx` 命令，因此未在本机执行 `nginx -t`，也未修改、重启或重载服务器上的 Nginx。生产安装独立站点配置后，仍必须先执行文档中的配置验证步骤。
