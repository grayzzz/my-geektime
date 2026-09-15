# CLAUDE.md

本文档为 Claude Code (claude.ai/code) 在此代码库中开发时提供指导。

## 沟通方式
- 默认中文回复；代码、命令、变量名、文件路径保持英文
- 结论先行，简洁直接，不先铺垫背景
- 不谄媚，不夸"这是个很好的问题"，不以"当然可以"开头
- 给真实判断——方案有问题直接指出，发现更好做法主动说明
## Git
- 不自动 `git commit` 或 `git push`，除非我明确要求
- 提交前先展示将要提交的变更摘要
- commit message 使用简洁英文
## 红线操作
以下操作即使在 auto-accept 模式下也必须先问我：
- 删除文件、目录或 git 历史
- 修改 `.env`、密钥、token、证书、CI/CD 配置
- `git push`、`git rebase`、`git reset --hard`、强制推送
- 公开发布（`npm publish`、生产部署等）

## 项目概述

My GeekTime 是一个 Go + React 全栈应用，用于缓存和在线浏览"极客时间"的内容。支持缓存 VIP 数据永久观看、一键发布课程为在线文档、一键下载音视频资源到本地、导出课程为 EPUB / PDF 电子书。

## 技术栈

- **后端**: Go 1.25, Gin 框架, GORM ORM, Protocol Buffers (gogo/protobuf)
- **前端**: React 18, TypeScript, Vite 5, TailwindCSS, Zustand 状态管理, React Router v6
- **数据库**: SQLite (默认), MySQL, PostgreSQL (通过 GORM)
- **内容解析**: `chromedp` 驱动无头浏览器渲染页面 + `html-to-markdown/v2` 转 Markdown（docsite / pdf / epub 导出链路的共用基础）
- **AI**: 直连 OpenAI 兼容 HTTP 接口（DeepSeek / Qwen / GLM 等），**不引入厂商 SDK**，用于文章摘要与文章问答
- **前端关键库**: `hls.js`（HLS 播放）、`react-markdown` + `remark-gfm`（文章渲染）、`lucide-react`（图标）、`axios` / `clsx` / `tailwind-merge`
- **构建**: Makefile, Docker 多阶段构建

## 关键目录

```
cmd/              CLI 入口 (api/server, cli/* 子命令)
internal/
  api/v2/         HTTP 处理器，每个领域一个文件：article, ai, backup, base, collect, comment,
                  discussion, file, product, progress, setting, sys_dict, task, user
  config/         配置结构体和加载逻辑 (server, db, jwt, storage, browser, i18n, site, ai)
  global/         全局单例 (DB, config, JWT, logger, HTTP 客户端, goroutine pool)
  handler/        资源服务和任务下载处理器
  initialize/     初始化逻辑: DB, logger, JWT, storage, i18n, theme
  middleware/     中间件: CORS, JWT 鉴权, 超时控制
  model/          Protocol Buffer 生成的模型 (.pb.go) + .proto 源文件
  router/         Gin 路由注册 (router.go 串联所有路由)
  service/        业务逻辑 (product, task, audio, video, proxy, docsite, email, ai, backup,
                  sys_dict, pdf*, epub*)
  types/          请求/响应 DTO, 按领域组织 (ai, base, collect, geek, progress, setting, sys_dict, task, user)
libs/             公共工具: browser, color, db, m3u8 解析/下载, pool, rest, schedule(定时调度),
                  storage, utils, zhttp
frontend/         React + Vite 单页应用
web/              构建后的前端静态资源 (go:embed 嵌入；index.html 与 assets/ 已在 .gitignore)
i18n/             国际化文件 (zh-CN.yml, en-US.yml)
docker/           Docker Compose 部署配置 (含 MySQL)
docs/             项目文档 (EPUB_EXPORT.md, DEPLOY_CLOUD.md, STARTUP.md, deployment-oracle-free.md)
.claude/          agents/dev-server.md (本机前后端启停流程) + skills/ (superpowers-zh)
.omp/             编辑器 LSP 配置 (config.yml, lsp.yml)
```

> **运行产物/本地私有文件**：`*.exe`、`*.log`、`mygeektime.db`、`repo/`(下载与代理缓存)、
> `web/index.html` + `web/assets/`、`custom_config.yaml`、`.claude/settings.local.json`、
> `.workbuddy/` 均已在 `.gitignore` 中。它们不是源码，**不要提交，也不要拿来判断代码变更**。

## 架构

- **入口**: `main.go` 通过 `embed.FS` 嵌入 `i18n/`、`web/`、`config.yml`，传给 `cmd.Execute()`。
  ⚠️ 因 `web/` 被嵌入，**前端产物更新后必须重新编译 Go 二进制**，否则 exe 里是旧前端（静默过期）。
- **CLI** (`cmd/`): 使用 `cliz` 库。两个主命令: `server` (HTTP 服务) 和 `cli`。
  `cli` 下共 9 个子命令：`config`(生成配置模板)、`data`(初始化极客时间数据)、`docs` / `docs-local`
  (生成在线文档)、`label`、`redirect`(校验跳转链接)、`backup` / `restore`(数据库备份与恢复 tar.gz)、
  `epub`(导出缓存课程为 EPUB)。
- **HTTP 服务** (`internal/api/v2/`): Handler 调用 `service/` 层。**多数**模型为 Protocol Buffer
  生成结构体，内嵌 GORM 标签；`ai_summary.go`、`progress.go` 等为手写 GORM 结构体，不走 protoc。
- **路由** (`internal/router/`): 单个 `NewRouter()` 函数注册所有路由，每个领域一个文件。
  分为公开组 (无需 JWT) 和私有组 (需 JWT)。
- **全局状态**: `global/` 包保存启动时初始化的单例: DB 连接、配置、JWT 签名器、日志、HTTP 客户端、goroutine pool、i18n 翻译器。
- **业务领域**:
  - Product/Article: 调用极客时间 API 获取内容，缓存文章/课程，支持专栏/案例/每日一学等类型
  - Task: 后台异步任务，用于下载流媒体内容 (m3u8/video/audio)，通过定时器调度器管理
  - Auth: JWT 鉴权，支持用户名/邮箱注册登录，GitHub OAuth 回调
  - Storage: 本地文件系统或可扩展存储驱动，用于存放下载的媒体文件
  - 导出 (`service/docsite.go`、`pdf*.go`、`epub*.go`): 把缓存的课程/文章导出为在线文档站、
    PDF、EPUB。EPUB 为多文件协作实现 (`epub.go` 骨架 + `epub_content.go` 正文 + `epub_comment.go`
    留言 + `epub_image.go` 图片 + `epub_shrink.go` 图片压缩)，完整设计见 `docs/EPUB_EXPORT.md`。
  - AI 助手 (`service/ai.go`): 文章摘要（结果缓存进 `AISummary` 表）与文章问答，依赖 `ai` 配置节
  - Backup (`service/backup.go`): 数据库全量备份/恢复，CLI 与 HTTP 双通道
  - Collect / Progress: 收藏列表与阅读进度
  - Setting / SysDict (`setting.go`、`sys_dict.go`): 站点设置与系统字典
- **前端**: React SPA，基于路由的懒加载代码分割。Zustand 管理认证状态。`components/ui/` 提供 UI 组件库。API 调用通过 axios + 类型化 hooks。页面见 `frontend/src/pages/`（CaseList、CollectList、DictList、LessonList、PvipList、Setting、TaskList、UserList、Login、Register、GitHubCallback 等）。

## 常用开发命令

### 本地端口

| 服务 | 地址 | 配置项 |
|------|------|--------|
| 后端 API | `http://127.0.0.1:8090` | `config.yml` → `server.http_port` |
| 前端 dev server | `http://localhost:3000` | `frontend/vite.config.ts` |
| MySQL | `127.0.0.1:3326` | `config.yml` → `database.source` |

前后端启停（启动/重启/停掉）已封装为 `.claude/agents/dev-server.md` 中的 **dev-server 子代理**，
Windows 下有一堆 Bash 踩坑约束（PATH、taskkill、端口探测等），**照它执行，不要临时发挥**。

### 后端

```bash
# 代码检查
golangci-lint run ./...

# 编译 (先 go vet，再注入 buildTime/buildCommit/buildBranch)
make build

# 安装 git hooks (pre-commit, commit-msg, pre-push)
make githook

# 编译 + gofmt + 打印帮助
make run

# 运行全部测试
go test ./...

# 运行指定测试
go test ./internal/service/... -run TestEmail
```

> 已知既有失败：`go test ./internal/service/` 中的 `TestHtmlURLProxyReplace_VoidElements` 会失败
> （`proxy.go` 给 `<img>` 注入 `referrerpolicy`，测试期望值未同步）。**与你的改动无关，不要误改**。

### CLI 工具命令

```bash
./my-geektime cli backup   --config=config.yml   # 备份数据库为 tar.gz
./my-geektime cli restore  --config=config.yml   # 从 tar.gz 恢复
./my-geektime cli epub     --config=config.yml   # 导出缓存课程为 EPUB
```

### 前端

```bash
cd frontend

# 安装依赖
npm install

# 开发服务器 (热更新, 代理后端请求)
npm run dev

# 生产构建 (tsc 类型检查 + vite build)
npm run build

# 代码检查
npm run lint

# 预览构建产物
npm run preview
```

### 完整构建与运行

```bash
# 先构建前端，再编译后端 (生成嵌入静态资源的 Production 二进制)
make web    # npm install + npm run build，产物拷到 web/
make build  # 编译 Go 二进制

# 启动服务
./my-geektime server --config=config.yml
```

> `make web` 已内联 `npm install`。前端产物落在 `web/`，由 `//go:embed` 打进 exe —— 改了前端**必须重跑
> `make web` + `make build`**，只重启进程不会生效。

### Docker

```bash
# 多架构镜像构建并推送
make image

# 本地 docker-compose 启动 (MySQL + 后端)
cd docker && docker-compose up -d
```

## 配置

顶层配置节（`internal/config/config.go`）: `server`, `jwt`, `i18n`, `database`, `storage`, `browser`,
`site`, `ai`。

- **嵌套关系容易记错**：`proxy`、`email`、`cookie` 都在 **`site` 下面**（`site.proxy.*`、
  `site.email.smtp.*`、`site.cookie.geektime`），不是顶层节。
- `site.cookie.geektime` 是抓取/缓存极客时间内容所用的登录 Cookie，属于敏感信息。
- `site.play.type` 为 `origin | local`，`site.download` / `site.cache` 控制下载与缓存开关。
- **`ai` 节**：`base_url` / `api_key` / `model`，三项任一为空则 AI 功能不可用并提示先配置。
  可经网页「设置页 → AI 服务」填写保存，也可直接改配置后重启。
- **`custom_config.yaml` 是整体替换，不是深合并**（`cmd/api/app.go:50`）：只要它存在且非空，
  就**完全取代** `config.yml`。踩坑点：改完 `config.yml` 却没生效，多半是这个文件在覆盖。
  它是设置页保存配置时 `yaml.Marshal(global.CONF)` 落盘的**整个配置快照**，
  内含明文 JWT secret 与数据库密码 —— 已 gitignore，**不要提交、不要外发**。
- `config.local.yml` 只是社区/README 的约定用法，代码里**没有自动加载**；
  要用必须显式 `--config=config.local.yml`。
- 数据库驱动: `mysql` / `postgres` / `sqlite`（当前 `config.yml` 用 `mysql`，走本机 3326）。
- 存储: `local` 驱动在 `/object` URL 前缀下提供下载文件的静态服务（`storage.directory` 默认 `repo`）。
- 代理缓存: `site.proxy.cache` 为 `true` 时，`/v2/file/proxy` 会把上游图片缓存到
  `repo/<cache_prefix>`（默认 `resource`），key 为 URL 的 md5。

## 重要模式

- **Handler 模式**: 每个处理器是一个结构体，通过 `NewXxx()` 构造函数创建。方法接收 `*gin.Context`。响应使用 `global.OK(c, data)` 或 `global.FAIL(c, msg, ...)`。
- **Model 模式**: 领域模型**大多**通过 `.proto` 文件定义并生成 `.pb.go`，内嵌 `Model` 基础结构
  (id, created, updated, deleted)，用 GORM 标签做 ORM 映射。但存在手写模型（如 `ai_summary.go`、
  `progress.go`）不走 protoc —— 改模型前先确认它属于哪一类，别去改一个不存在的 `.proto`。
- **任务系统**: 课程/文章被排列为任务 (TASK_STATUS_PENDING -> RUNNING -> FINISHED/ERROR)。定时器调度器 (`libs/schedule/timer_wheel.go`) 在后台处理下载任务。
- **m3u8 流媒体**: `libs/m3u8/` 处理 HLS 流解析和视频/音频内容下载。
- **代理**: `service/proxy.go` 通过本地代理端点 (`/v2/file/proxy`) 替换极客时间 CDN 链接，处理跨域和访问控制。
- **导出链路**（docsite / pdf / epub）: 共用同一套「HTML 渲染 → Markdown → 打包」流程，
  通过 `embed` 内嵌模板（`docsite.html.tpl`、`pdf.html.tpl`）产出。改动导出逻辑时注意别破坏
  其它两个格式；EPUB 的完整规则（默认导出全部留言、目录层级、零副作用）见 `docs/EPUB_EXPORT.md`。
- **图片压缩（EPUB）**: `service/epub_shrink.go` 把导出的图片压到最大宽 1440px、JPEG quality 88，
  面积平均降采样（勿改回最近邻，会糊），小于 150KB 的图直接放行（防扁平图发虚）。
  ⚠️ 缓存里写的是**原始字节**，只有内存副本被压缩 —— 该缓存与 `/v2/file/proxy` 共享，key 为 URL 的 md5，
  写入压缩后的数据会污染代理缓存、影响在线浏览。改这块务必保持「落盘原图、内存压缩」。
- **前端下载守卫**: 导出类请求（尤其 EPUB）走 `frontend/src/api/task.ts` 的 blob 分支，
  已校验 `blob.size === 0` 与 zip 魔数 `PK`。后端返回 200 空 body（例如旧版二进制没有对应
  `case`）时前端会明确报错，不会再出现「提示成功但下载 0 字节」。加新导出类型时沿用该守卫。

<!-- superpowers-zh:begin (do not edit between these markers) -->
# Superpowers-ZH 中文增强版

本项目已安装 superpowers-zh 技能框架（20 个 skills）。

## 核心规则

1. **收到任务时，先检查是否有匹配的 skill** — 哪怕只有 1% 的可能性也要检查
2. **设计先于编码** — 收到功能需求时，先用 brainstorming skill 做需求分析
3. **测试先于实现** — 写代码前先写测试（TDD）
4. **验证先于完成** — 声称完成前必须运行验证命令

## 可用 Skills

Skills 位于 `.claude/skills/` 目录，每个 skill 有独立的 `SKILL.md` 文件。

- **brainstorming**: 在任何创造性工作之前必须使用此技能——创建功能、构建组件、添加功能或修改行为。在实现之前先探索用户意图、需求和设计。
- **chinese-code-review**: 中文 review 沟通参考——话术模板、分级标注（必须修复/建议修改/仅供参考）、国内团队常见反模式应对。仅在用户显式 /chinese-code-review 时调用，不要根据上下文自动触发。
- **chinese-commit-conventions**: 中文 commit 与 changelog 配置参考——Conventional Commits 中文适配、commitlint/husky/commitizen 中文模板、conventional-changelog 中文配置。仅在用户显式 /chinese-commit-conventions 时调用，不要根据上下文自动触发。
- **chinese-documentation**: 中文文档排版参考——中英文空格、全半角标点、术语保留、链接格式、中文文案排版指北约定。仅在用户显式 /chinese-documentation 时调用，不要根据上下文自动触发。
- **chinese-git-workflow**: 国内 Git 平台配置参考——Gitee、Coding.net、极狐 GitLab、CNB 的 SSH/HTTPS/凭据/CI 接入差异与镜像同步配置。仅在用户显式 /chinese-git-workflow 时调用，不要根据上下文自动触发。
- **dispatching-parallel-agents**: 当面对 2 个以上可以独立进行、无共享状态或顺序依赖的任务时使用
- **executing-plans**: 当你有一份书面实现计划需要在单独的会话中执行，并设有审查检查点时使用
- **finishing-a-development-branch**: 当实现完成、所有测试通过、需要决定如何集成工作时使用——通过提供合并、PR 或清理等结构化选项来引导开发工作的收尾
- **mcp-builder**: MCP 服务器构建方法论 — 系统化构建生产级 MCP 工具，让 AI 助手连接外部能力
- **receiving-code-review**: 收到代码审查反馈后、实施建议之前使用，尤其当反馈不明确或技术上有疑问时——需要技术严谨性和验证，而非敷衍附和或盲目执行
- **requesting-code-review**: 完成任务、实现重要功能或合并前使用，用于验证工作成果是否符合要求
- **subagent-driven-development**: 当在当前会话中执行包含独立任务的实现计划时使用
- **systematic-debugging**: 遇到任何 bug、测试失败或异常行为时使用，在提出修复方案之前执行
- **test-driven-development**: 在实现任何功能或修复 bug 时使用，在编写实现代码之前
- **using-git-worktrees**: 当需要开始与当前工作区隔离的功能开发，或在执行实现计划之前使用——通过原生工具或 git worktree 回退机制确保隔离工作区存在
- **using-superpowers**: 在开始任何对话时使用——确立如何查找和使用技能，要求在任何响应（包括澄清性问题）之前调用 Skill 工具
- **verification-before-completion**: 在宣称工作完成、已修复或测试通过之前使用，在提交或创建 PR 之前——必须运行验证命令并确认输出后才能声称成功；始终用证据支撑断言
- **workflow-runner**: 在 Claude Code / OpenClaw / Cursor 中直接运行 agency-orchestrator YAML 工作流——无需 API key，使用当前会话的 LLM 作为执行引擎。当用户提供 .yaml 工作流文件或要求多角色协作完成任务时触发。
- **writing-plans**: 当你有规格说明或需求用于多步骤任务时使用，在动手写代码之前
- **writing-skills**: 当创建新技能、编辑现有技能或在部署前验证技能是否有效时使用

## 如何使用

当任务匹配某个 skill 时，使用 `Skill` 工具加载对应 skill 并严格遵循其流程。绝不要用 Read 工具读取 SKILL.md 文件。

如果你认为哪怕只有 1% 的可能性某个 skill 适用于你正在做的事情，你必须调用该 skill 检查。
<!-- superpowers-zh:end -->
