# COM-ItemGather

供团队使用的《Call of Mommy》游戏设定库，集中记录灵感、公司、商品、道具和合成配方。灵感流程是：随手输入 → 立即保存 → 后台 AI 整理 → 搜索与编辑 → 手动关联正式设定。

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
- 公司：中英文名、别名、背景设定与单张 Logo（不超过 3 MiB）；支持就地创建、按中英文及别名搜索、查看旗下物品和合并重复公司。现有商品里的公司文本在迁移中自动转为公司记录，保留关联。
- 商品：中英文名、所属公司选择、制作状态、动态属性和图片。分类、制作状态和所属公司点击后在原框中搜索，首项“＋”切换为原位新建输入框；支持方向键、Enter、Esc、取消与重试。分类与制作状态为团队共享选项，初始提供四种制作状态；新增选项即使暂未使用也会保留。制作状态可在选项右侧删除，也可按 Shift+Delete；已使用时须指定替代状态，统一更新商品、道具和配方（包含回收站），至少保留一个状态。删除会记录修改历史并通知其他成员，使用情况变化时需重新确认。公司可只填名称快速创建，也可展开完整资料表单。
- 商品浏览：支持列表和卡片视图，排序菜单按中文名称、修改时间、创建时间分组，各提供正序和倒序。中文名按拼音排列，数字按自然顺序排列；无中文名的商品置后，未命名商品置于最后。默认创建时间最新在前，浏览器会记住视图和排序选择。排序在服务端分页前执行，搜索、筛选和自动加载保持同一顺序。
- 分类删除：下拉选项右侧提供“删除”，也可选中后按 Shift+Delete。未使用分类直接确认删除；已使用分类可清空物品上的分类或替换为其他分类。配方把分类作为材料时必须选择替代分类，相同材料数量会合并。处理范围包含回收站记录，商品、道具与配方本身保留，并记录修改历史。确认期间关联内容变化会要求重新确认。已删除分类不会被旧草稿自动恢复；需要重新使用时可显式新建同名分类，之后也可以恢复引用它的旧历史版本。
- 道具：独立列表与详情，记录使用类别、触发方式、效果、多条获得来源及各自价格、冷却／次数／消耗／期限、模型与功能进度；使用类别默认为主动／被动，选择“＋”可在原位置新增团队共享类别。
- 合成配方：每个商品或道具产物对应一个页面，页面内可维护多套独立保存、复制、验证和恢复的配方。材料支持商品、道具和商品分类；材料行数自由增减，数量为正整数，重复材料自动合并。选择面板默认展开商品、收起道具，搜索同时覆盖两组。未命名产物仍需先创建为具体商品或道具。
- 新内容支持多人更新提示、版本冲突处理、修改历史、回收站、全局搜索和 JSON 导出。配方引用的已删除物品保留名称与删除标识，永久删除前需先解除配方引用。公司 Logo 更换校验失败时保留原图，保存成功后才清理旧文件。
- 关联：独立多对多表，状态为备选/已采用/不采用；AI 不会创建商品或关联。
- 并发：灵感与商品都有 `version`，更新冲突返回 409；前端显示差异并允许合并。标签维护同样递增版本并记录历史与 SSE 事件。恢复会产生新版本并取消旧整理任务。
- AI：`ai_tasks` 持久化，Worker 原子认领任务；内容版本和字段级人工修改状态防止旧结果覆盖。完整重新生成也会保留任务启动后的人工作业，未改动的字段不会误锁。总计最多请求 4 次（首次 + 最多 3 次重试）。
- 认证：Argon2id 密码散列；Session、邀请、重置令牌在数据库中只保存哈希；HttpOnly/SameSite Cookie 与 CSRF 校验。
- 附件：后端检查真实图片内容、像素、大小、数量和总配额；使用内部 ID 命名，下载必须经过登录。
- 筛选：灵感、商品、道具和全局搜索统一使用漏斗弹层，左侧切换条件，右侧搜索选项，支持已选提示、一键清除、Esc 和外部点按关闭；商品、道具和全局搜索同组多选取并集，不同条件取交集。灵感浏览范围保留单选。
- 搜索：各列表、全局搜索、关联搜索和物品选择统一为输入即查，连续输入合并为停顿 250 ms 后的请求，Enter 立即查询；取消过期请求，加载时保留现有结果。参数化 SQLite `LIKE` 子串查询可靠支持中文短词；全局搜索支持记录类型、内容类型、商品方向和创建者多选，并将关键词与条件保存在网址中。内容类型与商品方向仅筛选灵感。全局搜索每类最多展示 50 条，超出时提示缩小范围；配方结果只读取产物和配方数量，进入详情后加载完整配方与材料。
- 限流：登录失败单独限流，普通 API 按匿名 IP 或登录 Session 限制为每分钟 300 次；SSE 使用独立长连接。
- 实时同步：`events` 表保存递增事件 ID，SSE 支持 Last-Event-ID、心跳和重连后的全量必要刷新。前端每个标签页共用一条实时连接，避免展开多套配方时耗尽连接。

本机草稿和待同步队列按账号隔离。登录失效时保留原账号的本机内容，重新登录同一账号后可继续同步；主动退出或撤销当前设备会清理该账号本机内容，并停止旧同步任务。旧版没有账号归属的本机草稿和队列会保留，但不会自动归属或提交给下一账号；升级前应尽量先提交旧版尚未同步的内容。既有人工字段标记保持不变；旧版误锁字段的记录可在确认内容后执行一次“重新生成全部字段”。

新增设定表：`companies`、`product_categories`、`product_statuses`、`tool_use_categories`、`recipe_groups`、`recipes`、`recipe_ingredients`、`setting_idea_links`。商品与道具共用物品身份，由 `products.item_kind` 区分，以共享附件、版本和灵感关联。

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
- `/api/product-options/categories`、`/api/product-options/statuses`：读取或新建团队共享商品分类、制作状态。`GET /api/product-options/statuses/usage?id=...` 查看使用情况；`DELETE /api/product-options/statuses` 携带 `id`、`token` 与可选 `replacement` 删除状态。`GET /api/product-options/categories/usage?name=...` 查看删除影响并取得确认令牌；`DELETE /api/product-options/categories` 携带 `name`、`token` 与可选 `replacement` 删除分类。
- `/api/companies`：公司资料、合并、回收站与历史；`/api/tools`：道具及其历史与附件；`/api/tool-categories`：共享使用类别。
- `/api/recipe-groups`：按产物归组的页面；`/api/recipe-groups/:id/recipes`：添加配方；`/api/recipes/:id`：编辑配方及其历史、回收站操作。
- `/api/links`：灵感与商品／道具的关联；`/api/setting-links`：灵感与公司／配方的关联。
- `/api/attachments`：图片上传、认证下载、删除。
- `/api/search`、`/api/tags`、`/api/trash`、`/api/export`。
- `/api/events`：SSE；`/api/system/status`：不泄露密钥的系统状态；`/api/system/backup`：管理员立即备份。

灵感创建和附件上传都接受 `client_id` 作为幂等键，重复上传返回同一个附件。`PATCH` 和删除操作携带 `version`；旧版本返回 HTTP 409，不会静默覆盖。前端业务请求携带 `X-User-ID`，防止旧标签页或旧同步任务借用切换后的账号 Cookie 写入数据。

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
| `MAX_UPLOAD_SIZE` | `3MiB` |
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

1. 在目标 CPU 架构的构建环境生成产物，将项目复制到 `/srv/com-itemgather`。代码和静态文件应可读，专用服务账号只需写入数据与备份目录。以下命令只创建本站目录；如果同名配置已存在，先备份再更新。

   ```bash
   getent passwd com-itemgather >/dev/null || sudo useradd --system --user-group \
     --home-dir /srv/com-itemgather --shell /usr/sbin/nologin com-itemgather
   sudo install -d -m 700 -o com-itemgather -g com-itemgather \
     /srv/com-itemgather/data /srv/com-itemgather/data/attachments /srv/com-itemgather/backups
   sudo install -d -m 750 -o root -g com-itemgather /etc/com-itemgather
   if ! sudo test -e /etc/com-itemgather/com-itemgather.env; then
     sudo install -m 640 -o root -g com-itemgather .env.example /etc/com-itemgather/com-itemgather.env
   fi
   sudoedit /etc/com-itemgather/com-itemgather.env
   ```

2. 在环境文件中修改 `APP_PUBLIC_URL`、数据库／附件／备份路径、AI 配置，保持 `COOKIE_SECURE=true`。环境文件使用可由 Bash 加载的赋值语法；含空格的值用引号包住。检查端口：`sudo ss -ltnp 'sport = :8088'`。如果已有监听，为本站选用空闲端口，并同步修改 `APP_LISTEN_ADDR` 和本站 Nginx 代理地址。
3. 使用统一管理脚本初始化并启动后端。该脚本切换到项目目录、加载上述生产环境，root 执行时再降权到 `com-itemgather`，保证 CLI 与 systemd 使用同一数据库和文件归属。密码通过交互输入，不写入命令行：

   ```bash
   sudo cp deploy/systemd/com-itemgather.service /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo /srv/com-itemgather/deploy/scripts/manage.sh migrate
   sudo /srv/com-itemgather/deploy/scripts/manage.sh create-admin -username admin -nickname 管理员
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

Nginx 示例包含 Vue History 模式的 `try_files ... /index.html`、静态哈希资源缓存、`/api/` 代理，以及 `/api/events` 的 `proxy_buffering off`、`proxy_cache off` 和 3600 秒超时。Go 同时发送 `X-Accel-Buffering: no` 和 20 秒 SSE 心跳，撤销 Session 或停机时关闭相关连接。示例默认不要求 HTTP/2 新指令；如需启用，按配置注释匹配服务器 Nginx 版本。

首次上线检查 HTTPS 登录、邀请注册、双浏览器 SSE、图片访问、AI 失败重试及一次完整备份恢复。`migrate` 会按顺序应用新增迁移，不会删除现有内容。服务账号需拥有既有数据库、WAL、附件及备份目录的读写权限；升级时保留 `/srv/com-itemgather/data` 和 `backups`。

已有服务升级时，先用管理脚本备份，再停止本站后端，替换代码／构建产物并运行迁移，最后启动后端。不要让旧二进制继续操作升级后的数据库。无需因此重启 Nginx 或其他项目。

## 备份、导出与恢复

设置页可导出不含认证信息的 JSON，包括灵感、公司、商品、道具、配方、使用类别、属性、标签和关联（`format_version: 2`）。完整服务器迁移使用备份归档，其中包含 `database.db` 与 `attachments/`。

```bash
sudo /srv/com-itemgather/deploy/scripts/backup.sh
# 或 sudo /srv/com-itemgather/deploy/scripts/manage.sh backup
journalctl -u com-itemgather --since today | grep -E '备份|backup'
```

备份使用 SQLite `VACUUM INTO` 生成一致性快照；快照与附件复制持有跨进程附件锁，上传、图片删除和永久删除使用同一把锁。复制后释放附件锁再压缩归档，同时核验数据库引用的原件、缩略图及原件大小。自动备份、网页备份和 CLI 备份通过另一把锁串行生成及清理归档。归档同时受保留天数与份数限制，不会直接复制在线数据库文件。

恢复前必须停止后端，并另外保存当前数据。加载部署环境变量后，使用带明确确认的恢复命令：

```bash
sudo systemctl stop com-itemgather
sudo /srv/com-itemgather/deploy/scripts/manage.sh restore -archive /path/to/backup.tar.gz -yes
sudo /srv/com-itemgather/deploy/scripts/manage.sh migrate
sudo systemctl start com-itemgather
```

`restore` 会拒绝正在运行的服务、不安全路径、非普通文件、缺少有效迁移记录或缺少图片的归档，并在替换前执行 SQLite 完整性及附件检查。被替换的数据库、旧 WAL 文件和附件目录会以 `.pre-restore-唯一ID` 后缀保留；命令会打印这些路径。没有 `-yes` 时不会修改数据。归档需可由服务账号读取，不要删除唯一的恢复前副本。

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

上面的裸二进制命令用于已正确加载环境的终端。生产环境推荐使用 `sudo /srv/com-itemgather/deploy/scripts/manage.sh 命令`，统一加载环境和执行账号。脚本默认读取 `/etc/com-itemgather/com-itemgather.env`，可以通过 `ENV_FILE` 指定其他配置；开发时可使用 `ENV_FILE=/绝对路径/.env ./deploy/scripts/manage.sh migrate`。

## 测试

```bash
go test ./...
go vet ./...
./deploy/scripts/build.sh
cd web
npx playwright install chromium
npm run test:e2e
```

AI 集成测试使用本机可控的 OpenAI-compatible 模拟服务，不消耗真实模型额度。浏览器测试启动真实 Go 后端、生产 Vue 静态文件和独立的临时数据库；执行前需先完成上述构建。如果默认测试端口已被占用，可设置 `E2E_BASE_URL=http://127.0.0.1:18098`；代理环境应将 `127.0.0.1,localhost` 加入 `NO_PROXY` 和 `no_proxy`，不要停止其他服务。

当前检查与已有验收记录：

- `go test ./...` 与 `go vet ./...`，新增公司身份与合并、道具隔离与类别去重、配方数量校验与引用保护、历史／回收站／导出、Logo 替换与 3 MiB 上限、旧公司／附件／历史迁移校验、分类与状态去重／筛选／恢复／导出及保留所有物品引用的选项迁移、分类删除与显式恢复、配方分类替换、数量溢出回滚与历史保护，新增道具／全局多选筛选、每类搜索结果上限、配方摘要与完整详情隔离、5,000 条混合记录查询，并覆盖完整重新生成期间的人工修改保护、普通整理、历史恢复、标签版本和事件、账号请求绑定、SSE 撤销与停机关闭、图片幂等、跨进程互斥、并发备份与图片删除、缺图归档拒绝及原有工作流。
- `./deploy/scripts/build.sh`：`npm ci` 干净安装、Vue 类型检查、前端生产构建和 Go 静态二进制构建均通过。当前网络使用 `NPM_CONFIG_REGISTRY=https://registry.npmmirror.com`。
- `npm run test:e2e`：Chromium 中 **32 个场景全部通过**，覆盖持久登录、双人 SSE、商品关联、离线草稿、文字与图片重试幂等、纯图片缩略图、退出／撤销清理、会话过期及跨账号隔离、旧版本机数据隔离、普通重新整理后的页面刷新、详情收藏、失败卡片重试和商品创建者筛选；新增公司就地创建、Logo 大小限制、按英文选择公司及改名同步、共享道具类别与多来源、商品／道具混合材料、数量合并、多套配方、多人冲突合并和手机布局验收；新增分类／制作状态／公司原位搜索与新建、键盘操作、中文输入确认保护、取消和清空、失败重试、多人选项共享与状态名称冲突对比；分类键盘删除／取消、草稿保留、已用分类替换、旧草稿冲突、配方引用保护、并发重确认与材料数量合并；统一漏斗筛选、各列表连续输入请求合并、Enter 查询、加载时保留结果、过期响应取消、网址多选条件恢复、实时刷新时保留关联搜索输入及手机弹层。
- 在 `/tmp` 隔离目录实际通过管理脚本执行迁移、初始管理员创建、临时服务启动、在线备份、运行中恢复拒绝、停机恢复及密码重置；额外检查了空格路径、环境文件覆盖、当前测试用户的文件归属和缺失配置拒绝。root 降权到专用账号的实际运行仍需在目标服务器验收。

当前开发机没有 `nginx` 命令，因此未在本机执行 `nginx -t`，也未修改、重启或重载服务器上的 Nginx。生产安装独立站点配置后，仍必须先执行文档中的配置验证步骤。
