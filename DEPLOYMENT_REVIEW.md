# 部署检查报告

检查日期：2026-10-31。基准：`design-edited.txt`，代码提交 `7eb9950`。

**结论：可以构建和运行，具备部署架构；目前不建议直接作为正式数据的长期生产服务上线。** 已复现账号草稿串用、AI 覆盖新人工修改等问题。应先修复下列问题，再进行目标服务器的部署验收。

本次检查未修改业务代码、数据库迁移或部署配置；补充测试和隔离运行数据放在 `/tmp`。没有连接生产服务器，没有修改、重启或重载 Nginx。

## 实际执行结果

| 检查 | 结果与边界 |
| --- | --- |
| Go 测试 | `go test -count=1 ./...` 全部通过，包括现有备份恢复测试。沙箱最初禁止测试服务器监听端口，放宽本机测试权限后完成。 |
| Go 静态检查 | `go vet ./...` 通过。 |
| 后端生产构建 | `CGO_ENABLED=0 go build -trimpath -ldflags='-s -w'` 通过，得到 Linux amd64 静态二进制 `/tmp/com-itemgather-review`。 |
| 前端类型检查、生产构建 | `npm run build` 通过，包含 `vue-tsc -b` 和 Vite 生产构建。使用现有安装的依赖，未重新执行 `npm ci`。 |
| 原有浏览器测试 | 3/3 通过：持久登录、AI 整理、双浏览器 SSE、手动关联；离线草稿、文字提交幂等与显式退出清理；纯图片投稿。使用本次新构建的后端及前端。 |
| 补充后端验收 | 4 个用例均复现缺陷，正确行为的断言失败，详见问题 2、3、5、6。 |
| 补充浏览器验收 | 2 个用例均复现缺陷，正确行为的断言失败，详见问题 1、4。 |
| 部署脚本语法 | `bash -n deploy/scripts/build.sh deploy/scripts/backup.sh` 通过。 |
| 生产 Nginx、systemd、真实 AI、2GB 服务器负载 | 未验收。当前环境没有 Nginx；没有使用真实 AI Key，也没有目标服务器的运行信息。 |

浏览器测试的本机请求最初受代理影响，出现端口已占用的误报。只读检查确认端口空闲后，为 localhost 设置代理绕过，使用 `/tmp` 内的临时配置和隔离端口 `18098` 完成验收。未复用或停止已有服务。

## 已复现的问题

### 1. P1：撤销当前设备后，本地草稿和待同步队列会串到下一账号

对应需求：6.6、9.3、测试场景 44。

复现：管理员在首页留下草稿和待同步内容 → 设置／登录设备／撤销当前设备 → 同一浏览器登录普通成员。结果：普通成员看到管理员未提交的草稿，管理员的待同步内容还被自动创建为普通成员的灵感。

浏览器实际输出：草稿为“管理员尚未提交的私密草稿-验收”；待同步内容“管理员未同步的队列-验收”的创建者变成“第二位成员”。

原因：[offline.ts](/home/macroladlezilla/COM-ItemGather/web/src/offline.ts:25) 使用全站共享的 `composer` 键和不带用户归属的 outbox；[SettingsView.vue](/home/macroladlezilla/COM-ItemGather/web/src/views/SettingsView.vue:18) 撤销当前设备只清除内存登录状态；[IdeasView.vue](/home/macroladlezilla/COM-ItemGather/web/src/views/IdeasView.vue:29) 在下一账号进入首页时读取草稿并自动同步全部队列。401 失效处理也没有清理或隔离本地内容。

修复要求：草稿和队列按用户 ID 隔离；设备撤销、登录过期、主动退出统一处理账号本地状态；同步任务校验所属账号，避免账号切换后继续执行旧任务。

### 2. P1：完整重新生成会覆盖任务启动之后的人工修改

对应需求：5.7，测试场景 28、33。

复现：发起“重新生成全部字段” → 暂缓模型响应 → 保存新的人工标题 → 放行模型响应。实际标题从“任务启动之后才写的人工标题”变回“绿色风暴”。后保存的人工编辑没有获得保护。

原因：[ai.go](/home/macroladlezilla/COM-ItemGather/internal/app/ai.go:338) 对 `full` 任务无条件移除人工字段保护，只检查正文版本，不检查这些字段是否在任务创建之后发生变化。启动时确认覆盖已有内容，不能覆盖随后新保存的修改。

修复要求：任务保存来源记录版本或字段版本；完整生成也只覆盖启动时已经确认可覆盖的字段。执行期间新增的人工作业应保留或明确产生冲突。

### 3. P1：普通保存误将全部整理字段锁为人工字段，后续整理不更新

对应需求：4.2、5.7，以及“搜索与补充 → 再整理”的核心工作流。

复现：完成一次整理 → 用户只修改正文并按详情页实际请求保存 → 运行普通重新整理。测试模型已返回第二轮摘要，记录仍保留第一轮“青苹果饮料”，状态却是 `succeeded`。

原因：[IdeaDetailView.vue](/home/macroladlezilla/COM-ItemGather/web/src/views/IdeaDetailView.vue:17) 每次发送全部字段；[ideas.go](/home/macroladlezilla/COM-ItemGather/internal/app/ideas.go:406) 只要字段出现在请求中就标为人工修改，未比较是否真正改变。改正文甚至只改备注，也会保护所有标题、摘要、标签、属性。

修复要求：前端仅提交改动字段，或后端比较新旧值后再更新人工标记；保留真正被用户修改的字段，允许其他字段继续自动整理。

### 4. P2：图片上传响应丢失后，重试会产生重复附件

对应需求：9.3、10，测试场景 43。

复现：服务器已成功保存一张图片，模拟浏览器未收到成功响应 → 点击队列重试。实际结果为 **1 条灵感、2 个相同图片附件**。

原因：[IdeasView.vue](/home/macroladlezilla/COM-ItemGather/web/src/views/IdeasView.vue:22) 仅在收到响应后移除待传文件；[attachments.go](/home/macroladlezilla/COM-ItemGather/internal/app/attachments.go:96) 每次生成新的附件 ID，上传没有客户端幂等键。文字创建的 `client_id` 不能保护附件。

修复要求：为每个待传附件生成并持久化幂等 ID，服务端建立唯一约束并返回首次上传结果。否则重复重试还会占用存储或触发图片数量上限。

### 5. P2：恢复历史版本取消了 AI 任务，却没有更新数据库中的 AI 状态

对应需求：5.7、7.2，测试场景 16、29。

复现：修改正文 → 排队整理 → 恢复旧正文。实际数据库中活跃任务数为 `0`，记录 `ai_status` 仍为 `pending`。界面会持续显示整理中，直到后续操作再次改变状态。

原因：[revisions.go](/home/macroladlezilla/COM-ItemGather/internal/app/revisions.go:39) 在内存设置 `idle` 并取消任务，但后面的 `UPDATE ideas` 没有写入 `ai_status`、`ai_error`。

修复要求：正文恢复、任务取消、AI 状态及历史快照在同一事务中保持一致。

### 6. P2：标签维护绕过版本、历史与持久化 SSE 事件

对应需求：7.1、7.2、8、11，测试场景 31、33。

复现：对版本 2 的灵感重命名其标签。标签已改变，但灵感版本仍为 `2`，事件表最大 ID 也未增加。旧客户端再次保存不会因这次变更收到版本冲突，其他浏览器不能靠本次 SSE 通知更新。

原因：[misc.go](/home/macroladlezilla/COM-ItemGather/internal/app/misc.go:200) 更新标签后仅调用 `hub.Notify()`；[refreshIdeaTags](/home/macroladlezilla/COM-ItemGather/internal/app/misc.go:260) 只改 `tags_json`。SSE 实际读取 `events` 表，没有新增事件便没有可发送的变更。

修复要求：标签变更及受影响灵感的版本、更新时间、历史、事件统一事务处理；明确其人工修改保护语义。

## 代码与部署文档检查发现

### 7. P1：在线备份没有保证数据库快照与附件属于同一个一致状态

对应需求：10、12.2。

[backup.go](/home/macroladlezilla/COM-ItemGather/internal/backup/backup.go:46) 先 `VACUUM INTO`，然后复制当前附件目录。备份期间附件删除和永久删除没有与此共享协调机制；快照中仍引用的图片可能在复制目录前已经被删除，而归档仍成功完成。恢复只检查 SQLite 完整性及迁移记录，不核验附件引用是否都有实际文件。

这是代码检查确认的并发窗口，**本次没有进行该竞态的运行时复现**。现有备份测试验证了静态条件下的恢复，不能证明边备份边删除图片时的一致性。

修复要求：对快照与附件复制实施统一协调，或保留快照所引用文件直至归档完成，并在完成前核验引用。临时运维措施可以是在服务停止期间执行完整备份，但不能据此宣称在线自动备份已满足要求。

### 8. P1（部署步骤）：首次初始化命令未加载生产环境，也未明确执行账号

对应需求：17。

[README 首次部署步骤](/home/macroladlezilla/COM-ItemGather/README.md:141) 创建了环境文件，却直接执行 `migrate` 和 `create-admin`。程序只读进程环境，不自动读取文件；[默认数据库路径](/home/macroladlezilla/COM-ItemGather/internal/config/config.go:54) 是当前目录下的 `data/app.db`。从其他工作目录执行时，管理员可能创建到另一份数据库，systemd 启动后使用生产路径而没有管理员。

如果初始化由 root 执行，还须确保生成的数据目录和文件归专用服务用户可读写；数据库父目录由程序以 `0700` 创建，不能仅创建账号而忽略文件归属。

修复要求：把加载生产环境、工作目录、服务用户及目录归属写成完整初始化命令，保证 CLI 和 systemd 使用同一份数据库。此项未在真实服务器执行。

### 9. 部署前必须确认的兼容性与功能差距

- [Nginx 示例](/home/macroladlezilla/COM-ItemGather/deploy/nginx/com-itemgather.conf.example:6) 使用 `http2 on`。该指令从 Nginx 1.25.1 才引入；若现有服务器版本更旧，需调整配置。版本依据：[Nginx 官方文档](https://nginx.org/en/docs/http/ngx_http_v2_module.html#http2)。这是条件性兼容问题，尚不知道目标服务器是否受影响。
- 灵感卡片没有显示内容类型、纯图片缩略图，也没有需求中直接“重新整理”的失败重试按钮；失败提示链接会进入详情页。灵感详情页没有收藏入口。对应需求 4.1、4.2、5.8，位置：[IdeaCard.vue](/home/macroladlezilla/COM-ItemGather/web/src/components/IdeaCard.vue:12)、[IdeaDetailView.vue](/home/macroladlezilla/COM-ItemGather/web/src/views/IdeaDetailView.vue:35)。
- 全局搜索设置创建者条件时会完全跳过商品查询，商品专用搜索界面也没有创建者筛选，见 [misc.go](/home/macroladlezilla/COM-ItemGather/internal/app/misc.go:94)。
- `AI_MAX_RETRIES` 默认值正确，但 [配置校验](/home/macroladlezilla/COM-ItemGather/internal/config/config.go:65) 允许额外重试到 10 次，与需求“总计最多 4 次请求”的硬限制不一致。

## 按设计核对的总体覆盖

| 设计范围 | 检查结论 |
| --- | --- |
| Go、Vue、SQLite、Nginx、systemd 轻量架构 | 已有实现与部署示例，构建通过；目标服务器仍待验收。 |
| 灵感、商品、多对多关联、动态属性、原文保留 | 核心工作流通过现有测试；页面交互仍有上述缺项。 |
| 邀请、密码、Session、权限、CSRF | 已有实现和现有测试；账号切换时的本地数据隔离未达标。 |
| 持久化 AI、结果校验、失败重试、重启恢复 | 基本实现与现有测试通过；人工字段保护的两种场景未达标。 |
| 乐观锁、修改历史、SSE | 普通记录更新已有保护；历史恢复状态和标签维护存在缺陷。 |
| 图片验证、认证访问、缩略图 | 普通上传通过；图片上传重试幂等性不足。 |
| 中文搜索、JSON 导出、回收站 | 已有实现及部分测试；商品创建者筛选有缺口。 |
| PWA、IndexedDB、断网重试 | 基础场景通过；跨账号隔离和附件重试场景失败。 |
| 数据库迁移、备份恢复 | 静态备份恢复测试通过；在线附件一致性仍需修复和验证。 |

## 重新验收条件

先处理账号隔离、AI 人工修改保护、在线备份一致性与初始化步骤，并修复上述已复现的状态、版本和图片重试问题。然后重跑现有测试及补充失败用例。

目标服务器还需实际核验：CPU 架构与构建产物匹配、后端端口空闲、专用账号和目录权限、域名与证书路径、AI 兼容接口可用、`nginx -t`、systemd 启动及重启、HTTPS 下的 Cookie／SSE／PWA，以及一次包含真实图片的备份恢复。Nginx 配置验证通过后，按既有授权流程平滑重载。

补充用例保存在 `/tmp/com-itemgather-deployment-review-test.go`、`/tmp/com-itemgather-deployment-review-overlay.json` 和 `/tmp/com-itemgather-review-e2e/`。Go 用例通过 overlay 加入编译，没有写入业务源码目录。
