# Epub 电子书导出设计方案

## 目标

把已缓存的课程导出为符合 **EPUB 3.0** 规范的单个 `.epub` 文件，可在微信读书、Apple Books、多看、Calibre 及各类电纸书上阅读。

要求：

- **只有一个文件**，脱离本项目服务也能打开（图片全部内嵌）
- **两级目录**：章节 → 文章
- **代码块排版正确**（本项目 1405 篇含代码）
- **含评论区**（**默认全部留言** + 嵌套讨论，可切换或关闭）—— 对齐现有文档站导出的行为
- **零副作用**：不得影响任何现有功能（见第二节「隔离原则」，全部改动只有"新增文件 + 新增分支"）
- 不引入任何新的第三方依赖

---

## 一、现状与可复用资产（已实测核对）

### 内容从哪来

| 数据 | 位置 |
|---|---|
| 课程根任务 | `tasks` 表，`task_id` = 课程 ID，`raw` = `geek.ProductBase`（标题/简介/封面/作者） |
| 文章 | `tasks` 表同表，`task_pid` = 课程 ID，每行一篇，按 `id asc` 即正确阅读顺序 |
| 正文 | `Task.Raw` → `geek.ArticleData.Info.Content`（**HTML**） |
| 章节 | `geek.ArticleData.Info.ChapterTitle` |

> 参考实现：`internal/service/docsite.go:639` `MakeDocArchive` —— 它就是按 `TaskPid` 查子任务、逐篇取 `Info.Content` 的，epub 可以直接沿用这个数据获取方式。

### 可直接复用的代码

| 能力 | 位置 |
|---|---|
| 文档生成器接口 `DocGenerator` | `internal/service/docsite.go:31-40` |
| 正文 HTML → Markdown | `service.HTMLConvertMarkdown` |
| 文件名清洗（Windows 非法字符） | `service.VerifyFileName` |
| 图片 URL 匹配 / 提取 | `service.PorxyMatch`、`service.FindURLWithHTML`（`proxy.go:13,227`） |
| 图片缓存读写 | `global.Storage.Get/Put`，key = `resource/{md5(url)}`（见 `api/v2/file.go:31-34`） |
| 图片下载兜底（带 Referer） | 参照 `api/v2/file.go:71-108` |
| 导出接口与 type 分发 | `internal/api/v2/task.go:702-773` `Export` handler |
| **评论 HTML 生成（现成实现）** | `internal/service/docsite.go:719-776` `getCommentsHTML` —— 文档站导出已带评论，直接参照 |
| 评论正文反转义 | `internal/service/docsite.go:759` `utils.UnescapeComment` |
| 讨论查询 | `internal/api/v2/discussion.go` |
| `.epub` MIME | `libs/storage/mime.go` 已含 `application/epub+zip` |
| zip 打包 | **`archive/zip` 标准库**（EPUB 本质就是 zip，无需新依赖） |

### 实测数据（全库扫描 3647 篇，决定了下面的取舍）

| 指标 | 数值 | 对设计的影响 |
|---|---|---|
| 有 `ChapterTitle` | 3465 / 3647（449 个不同章节） | **可做真正的两级目录**（现有文档站没做） |
| 含 `<img>` | 2424 篇（66%），12689 个标签 / 11173 个去重 URL | 必须内嵌图片，否则离线看不全 |
| 图片主机 | 99% `static001.geekbang.org` | 正好命中现有代理配置，缓存可复用 |
| **本地图片缓存命中率** | **仅 13.7%**（1530 张） | 86.3% 需联网下载 → 导出耗时不可忽略 |
| 含 `<pre>` 代码块 | 1405 篇 | 代码排版是重点，不能糊 |
| 含 `<video>` | 99 篇 | **无法内嵌**，必须降级处理 |
| 含 `<audio>` / `<iframe>` | 0 / 0 | 无需处理 |
| `ContentMd` 字段 | **0 / 3647 全空** | 只能走 HTML 路线，不能指望现成 markdown |
| 有评论的文章 | 2919 / 3647（80%），篇均 **20.3 条**，中位数 9，P90 43，最多 696 | 评论可作为文章尾部区块，体量可控 |
| 评论总数 / 讨论总数 | **59,328 / 41,459** | 全库 raw 合计约 50MB + 27MB，纯文本，体量很小 |
| 评论正文平均长度 | 338 字节（最长 4143）；讨论正文 242 字节 | 文本为主，无图片依赖 |
| 评论含头像 | 88% 是 `static001.geekbang.org/account/avatar/`（**代理白名单已覆盖**），12% 是微信 `qlogo.cn`（**未覆盖且会过期**） | 头像不做内嵌（见第五节说明） |
| 讨论结构 | `author.nickname` + `discussion.discussion_content`，均 100% 有用户名 | 可渲染为评论下的嵌套回复 |

---

## 二、关键设计决策与改动范围

| 决策点 | 选择 | 理由 |
|---|---|---|
| 新依赖 | **零新增** | `archive/zip` + `golang.org/x/net/html` 都已在依赖中 |
| 正文转换路线 | HTML → 清洗 → XHTML（**不转 Markdown**） | 全库 3647 篇的 `ContentMd` 都为空；且 Markdown 会丢代码块/表格细节 |
| 章节层级 | 用 `ChapterTitle` 做两级 nav | 实测有值且分组合理，是相对现有导出的明显增量 |
| 图片 | **内嵌进 epub** | 否则离开网络就是一堆裂图 |
| 图片获取 | 先查本地缓存，未命中再下载**并回写缓存** | 顺带把网页端要用的图也缓存了，一次投入两处受益 |
| **是否含评论** | **含**，但可关（`?comments=0`） | 文档站导出已带评论（`docsite.go:176,470`），epub 不含会显得功能倒退；且评论是极客时间的精华内容 |
| **评论数量** | 默认**全部留言**（不限条数），可切 `hot`（每篇 20 条按赞）或 `0`（不含） | 用户明确要求保留全部留言。实测全库评论正文均 338 字节、讨论 242 字节，纯文本，即便整门课全量也只有几百 KB，不影响 epub 体积 |
| **评论头像** | **不内嵌**，只输出「用户名 · 👍N · 日期」文字行 | 12% 是微信 `qlogo.cn`（不在代理白名单、URL 会过期），内嵌必然出现裂图；且几百张头像图纯属浪费体积 |
| **讨论（讨论/回复）** | 跟随评论内嵌为嵌套缩进，默认**全部** | 41,459 条讨论正文均 242 字节，纯文本成本极低 |
| **评论位置** | 每篇文章**正文之后**，独立 `<section class="comments">` | 与文档站一致；电子书阅读器上可用样式弱化视觉权重 |
| 单篇导出 | 不做 | epub 的价值在"整本"，单篇用现有 Markdown 即可 |
| 接口 | 扩展现有 `GET /task/export?type=epub`（新增 `comments` 参数） | 改动最小，前端已有 type 分发逻辑 |

### 隔离原则：本功能只做加法

**硬性要求：不能影响任何现有功能。** 具体兑现方式：

| 原则 | 做法 |
|---|---|
| **不改共享函数的行为** | `docsite.go` 的 `getCommentsHTML`（在线文档站/本地文档站都在用）**一行都不动**。epub 在自己文件里写独立的评论渲染器，宁可重复 ~50 行代码，也不重构共享逻辑 |
| **不动 `DocGenerator` 接口** | 该接口目前只有一个实现（`GoldmarkDocGenerator`，`docsite.go:31-45`）。往里加 `MakeEpub` 会强迫 goldmark 生成器实现 epub，属于强耦合。**改为独立的 `EpubGenerator` 类型 + 独立构造函数** |
| **不改数据库** | 不加表、不加字段、不动 `initialize/gorm.go` 的 AutoMigrate 列表；纯读取 |
| **不改配置** | `config.yml` 与 `internal/config` 完全不新增 section，评论模式用请求参数/CLI flag 传 |
| **不改路由与中间件** | 复用已有 `GET /task/export`，只是多一个 `type=epub` 取值；不动 `router.go`、不动 JWT 中间件 |
| **不改任务下载链路** | epub 用自己的并发池（信号量 6），**不复用也不干扰** `internal/handler/task/download.go` 的任务 worker |
| **不影响现有导出** | `Export` handler 里 `markdown` / `docsite` 两个 case 保持原样，只**追加**一个 `case "epub"` |
| **前端不改旧分支** | `handleExport` 现有的 markdown 分支与 docsite 分支**保持原样**，epub 作为**新分支插在最前面**（见第八节，这里有个必须避开的坑） |
| **回写图片缓存要尊重既有开关** | 图片下载后仅当 `global.CONF.Site.Proxy.Cache == true` 才回写 `repo/{cache_prefix}/`，且 key 与代理完全一致（`resource/md5(url)`）→ 天然幂等，不可能破坏已有缓存 |

### 改动清单（全部落在哪）

| 文件 | 性质 | 说明 |
|---|---|---|
| `internal/service/epub.go` | **新增** | 骨架与编排 |
| `internal/service/epub_content.go` | **新增** | HTML → XHTML |
| `internal/service/epub_image.go` | **新增** | 图片收集/缓存/下载 |
| `internal/service/epub_comment.go` | **新增** | 评论 + 讨论渲染 |
| `cmd/cli/epub.go` | **新增** | CLI 命令 |
| `internal/api/v2/task.go` | 修改 **+1 个 case** | 现有 case 逐字不动 |
| `internal/types/task/task.go` | 修改 **+1 个字段** | 可选字段，无默认值语义变化 |
| `cmd/cmd.go` | 修改 **+1 行注册** | 纯追加 |
| `frontend/src/api/task.ts` | 修改 **+1 个 if 分支** | 插在 markdown 判断之前，markdown 那行不动 |
| `frontend/src/components/*.tsx`、`pages/*.tsx` | 修改 **+新按钮 + 新分支** | 旧分支不动 |
| `docsite.go` / `config.yml` / `router/*` / `model/*` / `initialize/*` | **完全不碰** | 见上表 |

原则上**没有任何一处是"改写既有逻辑"**，全部是"新增文件 + 新增分支"。

### 回归验证（确认旧功能没坏）

```bash
# 1. 编译与静态检查（含全量包）
go build ./... && go vet ./...

# 2. 前端类型检查与构建
cd frontend && npx tsc --noEmit && npm run build

# 3. 现有三种导出逐个回归（必须与改动前表现一致）
#    markdown：返回 tar.gz 附件
curl -s -o /tmp/a.tar.gz -w "%{http_code} %{content_type}\n" \
  "http://127.0.0.1:8090/v2/task/export?pid=<pid>&type=markdown"
tar -tzf /tmp/a.tar.gz | head
#    docsite：返回 JSON（含 doc 字段），且库里 message.doc 被更新
curl -s "http://127.0.0.1:8090/v2/task/export?pid=<pid>&type=docsite"
#    PDF：走原有 pdf 接口，行为不变

# 4. 确认未新增表/字段（schema 相关文件必须零改动，应无输出）
git diff --stat -- internal/model internal/initialize

# 5. 确认共享图片缓存未被污染：对比导出前后 repo/resource 的目录规模
find repo/resource -type f | wc -l   # 导出后只应增加（新图），不应减少
```

---

## 三、EPUB 包结构

```
mimetype                      ← 必须第 1 个条目、且必须 STORED（不压缩）
META-INF/
  container.xml               ← 指向 OEBPS/content.opf
OEBPS/
  content.opf                 ← 包文档：metadata + manifest + spine
  nav.xhtml                   ← EPUB 3 导航（两级目录）
  toc.ncx                     ← EPUB 2 兼容目录（老阅读器 / Kindle 转换用）
  style.css                   ← 正文样式
  cover.xhtml                 ← 封面页
  images/
    cover.jpg
    {md5(url)}.jpg|png|gif    ← 内嵌图片，以 URL 的 md5 命名天然去重
  text/
    001_{标题}.xhtml          ← 每篇正文一个文件
    002_{标题}.xhtml
```

### 两个必须遵守的规范细节

1. **`mimetype` 必须是 zip 的第一个条目，且用 `zip.Store`（不压缩）**，内容恰好为 `application/epub+zip`（无换行）。
   这是 EPUB 最经典的坑：用默认的 `zip.Deflate` 或顺序放错，部分阅读器会直接判定"不是有效 epub"。
   实现上要**单独先写这一个条目**（`zip.Writer.CreateHeader` + `Method: zip.Store`），再写其余文件。

2. **EPUB 3 要求 `dcterms:modified` 元数据**，且格式为 `YYYY-MM-DDThh:mm:ssZ`，缺失会导致 EPUBCheck 报错。

### `content.opf` 元数据映射

| OPF 字段 | 来源 |
|---|---|
| `dc:title` | `geek.ProductBase.Title` |
| `dc:creator` | `ProductBase.Author.Name`（**实测常为空**，兜底为「我的极客时间」） |
| `dc:language` | `global.CONF.I18N.DefaultLang`（默认 `zh-CN`） |
| `dc:identifier` | `urn:uuid:` + 由 `task_id` 派生的**稳定** UUID（同一课程重复导出 ID 不变） |
| `dc:date` | `ProductBase.Ctime`（Unix 秒 → 格式化） |
| `dcterms:modified` | 本次生成时间 |
| 封面 | `ProductBase.Cover.Square` → `Rectangle` → `Horizontal` 取第一个非空；同时写 `<meta name="cover" content="cover-image"/>` 兼容 EPUB 2 阅读器 |

---

## 四、正文 HTML → XHTML 转换

原始正文是极客时间下发的 HTML（可能不规范），转换链路：

```
Info.Content
  → html.Parse()                 （x/net/html 容错解析，自动补全未闭合标签）
  → 遍历 DOM 清洗
  → html.Render()                （自动输出 <img .../> 这类自闭合 void 元素）
  → xml.Unmarshal 校验           （兜底：不通过则降级为纯文本）
  → OEBPS/text/NNN_xxx.xhtml
```

**为什么这条链路基本能产出合规范 XHTML**：`x/net/html` 的渲染器会把 HTML 实体（如 `&nbsp;`）解码成真实字符再输出，只对 `& < > " '` 做转义；void 元素统一输出 `/>`。所以 `&nbsp;` 不会变成 XML 未定义实体的错误。剩下的风险用最后那步 XML 校验挡掉。

### 遍历时要做的事

| 处理 | 做法 |
|---|---|
| `<img src>` | 换成 `../images/{md5(url)}.ext`，并从 URL 的 `?wh=WxH` 解析出宽高写进属性（**实测图片 URL 都带这个尺寸提示**，可避免阅读器里排版错乱），同时补 `alt` |
| `<video>` | **降级**：有 poster 就用 poster 当静态图，并追加一句「本文含视频，请在网页端观看」；没有则整块移除 |
| `<script>` / `<style>` / `<iframe>` | 直接删除（阅读器不支持 JS，留着可能触发校验失败） |
| 站内链接 `href` | 保留文字、去掉链接（缓存内容里指向极客时间的链接已失效） |
| 代码块 `<pre><code>` | 保留结构，`class` 加 `language-*` 便于高亮；样式交给 CSS |
| 表格 | 原样保留，CSS 里处理窄屏溢出 |

### 样式（`style.css`）

```css
body   { font-family: "Noto Sans SC", serif; line-height: 1.7; }
pre    { font-family: monospace; font-size: 0.85em; white-space: pre-wrap;
         word-wrap: break-word; background: #f6f8fa; padding: .6em; }
img    { max-width: 100%; height: auto; }
table  { border-collapse: collapse; font-size: .9em; }
```

要点：`pre` 必须 `white-space: pre-wrap` + `word-wrap: break-word`，否则长代码行在窄屏电纸书上会溢出（本项目大量代码块，这条直接影响观感）。

---

## 五、评论与讨论

**现有导出已经带评论了** —— `MakeDocsite` / `MakeDocsiteLocal` 都会调 `getCommentsHTML`（`docsite.go:176` 全量、`:470` 可限量），只有 Markdown 归档不带。epub 应当对齐文档站的行为。

### 数据来源

| 内容 | 位置 |
|---|---|
| 文章评论 | `article_comments` 表，`aid` = 文章 id（由 `task.OtherId` 解析，同 `docsite.go:465`） |
| 评论字段 | `raw` → `geek.ArticleComment`：`user_name` / `comment_content` / `like_count` / `discussion_count` / `comment_ctime` / `replies` |
| 评论的讨论 | `article_comment_discussions` 表，`cid` = 评论 id（= `ArticleComment.Cid`） |
| 讨论字段 | `raw` → `geek.DiscussionData`：`author.nickname` / `discussion.discussion_content` / `likes_number` |

两处 `raw` 里的正文都是**转义过的 HTML 片段**，必须走 `utils.UnescapeComment`（与 `docsite.go:759` 一致）。

> 注意：`ArticleComment.Raw` 里本身就带 `replies` 数组（实测 5000 条抽样中 71.8% 非空、均 1 条），而 `article_comment_discussions` 是另一路"讨论"。两者互不重叠，要分别取。

### 渲染结构

直接追加进同一篇的 XHTML（不新增文件）：

```html
<section class="comments" epub:type="endnotes">
  <h3 class="comments-title">全部留言（128）</h3>
  <div class="comment">
    <p class="comment-meta">用户名 · 👍 12 · 2024-05-06</p>
    <div class="comment-body"><p>评论正文…</p></div>
    <div class="comment-replies">
      <div class="reply">
        <p class="comment-meta">讨论者 · 👍 3</p>
        <div class="comment-body"><p>讨论正文…</p></div>
      </div>
    </div>
  </div>
</section>
```

要点：

- **不渲染头像 `<img>`**。评论正文和讨论正文都是纯文本（实测零图片依赖），这是体积可控的关键；而且 12% 的头像是微信 `qlogo.cn`，既不在代理白名单里、URL 本身也会过期，内嵌必然裂图
- 评论正文同样是 HTML，**复用正文的清洗链路**（`html.Parse` → 清洗 → `html.Render`），但要多一步剔除 `<img>`
- 用户名、日期等文本单独转义，避免破坏 XHTML

### 数量与排序

| 模式 | 查询 | 说明 |
|---|---|---|
| `comments=all`（**默认**） | `WHERE aid=? ORDER BY id ASC` 不限量 | **保留全部留言**，顺序 = 上游返回顺序（与文档站 `getCommentsHTML(aid, 0)` 行为一致） |
| `comments=hot` | `WHERE aid=? ORDER BY like_count DESC LIMIT 20` | 只取"精选留言"，适合想要轻量电子书的场景 |
| `comments=0` | 不查 | 纯正文 |

讨论同理：`all`（默认，不限量）／`hot`（每评论 `likes_number DESC LIMIT 3`）。

**为什么全量也不会让 epub 变大**（实测依据）：

| 项 | 数值 |
|---|---|
| 全库评论 + 讨论体积 | 约 50MB + 27MB（**整库**，含 raw 里的冗余字段） |
| 实际渲染进 xhtml 的只有 | 用户名 + 正文 + 赞数 + 日期 ≈ 400 字节/条 |
| 单门课估算（100 篇 × 20 条） | ≈ **0.8MB 文本** |
| 极端课程估算（200 篇 × P99 192 条） | ≈ **15MB 文本** |

对比图片那边动辄几十 MB，评论的文本量可以忽略。真正的体积瓶颈始终是图片。

> 唯一需要留意的是**单个课程内评论最多的文章有 696 条**——全量模式下这篇文章的评论区会很长，但这是用户明确的选择。

### 性能：必须避免 N+1

全库 3,647 篇，逐篇查两张表会产生 **7,000+ 次查询**，CLI 全库导出会明显变慢。做法是**按 aid 批量查一次 + 内存分组**：

```go
// 1. 课程内所有文章的 aid 一次性取评论
db.Where("aid IN ?", aids).
   Order("aid, like_count DESC").Find(&comments)
// 2. 内存按 aid 分组，再逐篇按 limit 截断
// 3. 讨论同理：Where("cid IN ?", cids)
```

课程内文章数通常几十到几百，单次 `IN` 查询即可，无需额外分页。

### 样式（追加到 `style.css`）

```css
.comments        { margin-top: 2.5em; border-top: 1px solid #ddd; padding-top: .8em; }
.comments-title  { font-size: 1.05em; color: #555; }
.comment         { margin: 1em 0; }
.comment-meta    { font-size: .8em; color: #888; margin: 0 0 .2em; }
.comment-body    { font-size: .92em; color: #333; }
.comment-replies { margin-left: 1.2em; padding-left: .8em; border-left: 2px solid #eee; }
.reply .comment-body { font-size: .9em; color: #555; }
```

评论在阅读器里必须比正文**视觉权重更低**（小字号 + 灰字），且不加背景色 —— e-ink 上深色底反而更刺眼。

---

## 六、图片内嵌策略

```
收集：遍历所有正文，FindURLWithHTML 取 href/src/poster
     → 用 PorxyMatch(url) 过滤，只处理代理配置覆盖的域名
     → 按 URL 去重（11173 个去重 URL，重复引用只下一次）

获取：key = resource/md5(url)
     ├─ Storage.Get(key) 命中 → 直接用（实测 13.7% 命中）
     └─ 未命中 → HTTP GET（带 Referer，模仿 api/v2/file.go:71）
                 → Storage.Put(key) 回写缓存
                 → 失败：记为失败，该图替换为占位（不阻断整本导出）
```

**并发与保护**（因为 86.3% 要联网，必须有护栏）：

- 并发下载限 6（信号量），单图超时 15s
- 单图上限 10MB，整本图片总量上限 500MB，超限停止下载并记录
- 同一 URL 只下一次（`map[string]imageAsset`）
- 全部失败也不返回错误，只是图变占位 —— **不能因为一张图让整本导出失败**

**封面图**走同一套缓存/下载逻辑，扩展名由 `Content-Type` 决定。

---

## 七、后端实现

### 新增文件

| 文件 | 职责 |
|---|---|
| `internal/service/epub.go` | 主编排：查课程与文章 → 分组章节 → 生成 zip 各部件 → 返回 `*bytes.Buffer` |
| `internal/service/epub_content.go` | HTML → XHTML 转换（DOM 清洗、图片引用重写、视频降级、XML 校验兜底） |
| `internal/service/epub_image.go` | 图片收集、缓存查/写、并发下载、扩展名推导 |
| `internal/service/epub_comment.go` | 评论/讨论的批量加载、反转义、渲染、数量截断（见第五节） |
| `internal/service/epub.go.tpl`（可选） | `content.opf` / `nav.xhtml` / `toc.ncx` / `style.css` 模板 |

### 核心签名

```go
// ★ 不动 DocGenerator 接口（详见第二节隔离原则），EpubGenerator 完全独立

// epub.go
type EpubOptions struct {
    Comments string // "all"（默认，全部留言）| "hot"（每篇 20 条按赞）| "0"（不含）
}

func NormalizeEpubOptions(opt EpubOptions) EpubOptions // 空值 → Comments "all"

type EpubGenerator struct{}
func NewEpubGenerator() *EpubGenerator
func (e *EpubGenerator) MakeEpub(ctx context.Context, taskId, title, introHTML string,
    opt EpubOptions) (*bytes.Buffer, error)

// 内部步骤
func (e *EpubGenerator) loadChapters(ctx context.Context, taskId string) ([]epubChapter, error)
func (e *EpubGenerator) loadComments(ctx context.Context, aids []int64, opt EpubOptions) (map[int64][]epubComment, error)
func writeEpub(w *zip.Writer, book *epubBook) error   // 注意：mimetype 单独 Store
```

`epubChapter` 结构：

```go
type epubChapter struct {
    Title    string       // ChapterTitle，空则归入「正文」
    Articles []epubArticle
}
type epubArticle struct {
    Title    string
    Author   string
    Ctime    int64
    XHTML    string        // 已转换好的正文片段
    Comments []epubComment // 评论（可为空）
}
type epubComment struct {
    User      string
    Content   string // 已 Unescape + 清洗后的 HTML 片段
    LikeCount int64
    Ctime     int64
    Replies   []epubReply
}
```

> `Aid` 需保留在 `epubArticle` 上（`task.OtherId` 解析而来），评论就是靠它关联的。

章节顺序 = 按 `task_id asc` 遍历时的**首次出现顺序**（无需额外排序字段，天然与阅读顺序一致）；`ChapterTitle` 为空的文章统一放进一个「未分类」章节。

### API

在 `internal/api/v2/task.go` 的 `Export` handler 里加一个 case，**复用已有的前置校验**（任务完成度检查、product 反序列化）：

需先给 `task.TaskExportRequest`（`internal/types/task/task.go:146`）加一个字段（**纯追加，不影响现有 `pid`/`type` 的绑定**）：

```go
// 评论模式：all（默认）| hot | 0
Comments string `json:"comments,omitempty" form:"comments"`
```

然后**在现有 `case "markdown"` / `case "docsite"` 之后追加**（两个既有 case 逐字不动）：

```go
case "epub":
    dirName := service.VerifyFileName(product.Title)
    archiveName := dirName + ".epub"
    gen := service.NewEpubGenerator()
    opt := service.EpubOptions{Comments: req.Comments}
    buf, err := gen.MakeEpub(c, l.TaskId, product.Title, product.IntroHTML, opt)
    // ...
    c.Header("Content-Type", "application/epub+zip")
    c.Header("Content-Disposition", "attachment; filename="+url.QueryEscape(archiveName))
    c.Data(200, "application/epub+zip", buf.Bytes())
```

鉴权沿用现状（`private` 组，需 JWT），与 markdown / PDF 保持一致。

### CLI

新增 `cmd/cli/epub.go`，初始化流程完全复刻 `cmd/cli/docs.go:49-67`（Gorm → Logger → Storage）：

```bash
# 导出单门课程（默认含全部留言）
my-geektime cli epub --config=config.yml --taskid=<courseId> --output=./out

# 只取精选（每篇 20 条）/ 不含评论
my-geektime cli epub --config=config.yml --taskid=<courseId> --comments=hot --output=./out
my-geektime cli epub --config=config.yml --taskid=<courseId> --comments=0 --output=./out

# 导出全部课程（不带 --taskid）
my-geektime cli epub --config=config.yml --output=./out
```

`--comments` 不传即 `all`。

命令注册在 `cmd/cmd.go`（与现有 `NewSubCommandFunction` 写法一致）。CLI 场景尤其有价值：可以离线批量把整个库转成电子书。

---

## 八、前端改动

导出按钮在前端共出现 **4 处**（原设计只列了 2 处，已按实际代码补全）：

| 文件 | 位置 | 改动 |
|---|---|---|
| `frontend/src/api/task.ts:63` | `exportTask` | 新增 `epub` 分支（**插在 markdown 判断之前**）+ `comments` 参数 |
| `frontend/src/components/LessonDetail.tsx` | L944-990「导出Markdown/导出PDF」按钮组 | 加「导出Epub」按钮 |
| `frontend/src/components/TaskCard.tsx` | L256-270 | 加「导出Epub」按钮 |
| `frontend/src/pages/CollectList.tsx` | `handleExport`（L274）+ 按钮组（L570） | 加 epub 分支 + 按钮 |
| `frontend/src/pages/TaskList.tsx` | `handleExport`（L289）+ 按钮组 | 加 epub 分支 + 按钮 |

### ⚠️ 必须避开的一个坑

现有 `handleExport` 是**二分支**结构（`CollectList.tsx:274-297`、`TaskList.tsx:313-345`）：

```ts
if (type === 'markdown') { /* 走 blob 下载，文件名硬编码 `.tar.gz` */ }
else                     { /* 走 docsite，把返回的 doc 写进列表状态 */ }
```

epub **必须作为新分支插在 `if (type === 'markdown')` 之前**：

```ts
if (type === 'epub') {
  // blob 下载，文件名 `${safeName}.epub`（不能套用 `.tar.gz` 的后缀）
  // 自己的 loading 状态与超时
} else if (type === 'markdown') {
  /* 原样不动 */
} else {
  /* 原样不动 */
}
```

**为什么不能直接复用 markdown 分支**：那两个分支里下载文件名硬编码为 `.tar.gz`，塞进去会得到 `xxx.epub.tar.gz`。同理 `api/task.ts` 里 epub 要单独一个 `if`，因为需要**独立超时**，而 markdown 走全局 30 秒默认值——一旦合并条件，markdown 的超时行为也被改了，属于影响旧功能。

### 评论模式选择

默认「含全部留言」，另提供两种：

```
导出Epub ▾
  ├ 含全部留言        ← 默认
  ├ 含精选留言（每篇 20 条）
  └ 不含留言
```

> **超时这条是踩过坑的经验**：上一轮做数据备份时，导出接口后端要跑 40 秒，而前端 axios 全局超时是 30 秒（`utils/request.ts`），结果是"后端成功、前端报失败"。epub 要联网下 100 多张图，耗时只会更长，所以**必须单独放长超时**，且只作用于 epub 请求。

---

## 九、边界与风险

| 风险 | 处理 |
|---|---|
| `mimetype` 压缩或位置错误 → 阅读器拒认 | 单独 `zip.Store` 写第一个条目 |
| 正文 HTML 不规范 → XHTML 校验失败 | 逐篇 `xml.Unmarshal` 校验，不过则降级为纯文本，**不连累整本** |
| 图片下载慢（86% 未缓存） | 并发 6 + 单图 15s 超时 + 失败降级占位 |
| epub 文件过大 | 图片总量超 500MB 停止下载；单本建议 < 300MB |
| `<video>` 无法内嵌 | 明确降级为 poster + 提示文案 |
| 完全离线环境 | 只能内嵌已缓存的 13.7% 图片，其余为占位；README/UI 上说明 |
| 中文文件名 | 复用 `service.VerifyFileName` |
| 评论正文含 `<img>`（作者贴图） | 清洗时剔除，评论一律纯文本 |
| 全量留言导致某篇评论区很长（单篇最多 696 条） | 默认 `all` 是用户选择；超长时仍由 `hot` 模式兜底 |
| 全量模式下评论查询内存占用 | 按 `aid IN (...)` 批量取；若单课程评论量异常大，分批 IN（每 500 个 aid 一批） |
| 评论查询 N+1 | 按 `aid IN (...)` 批量查 + 内存分组，见第五节 |
| 评论里的用户名/正文破坏 XHTML | 文本转义 + 复用 `html.Render` 清洗链路，并纳入逐篇 XML 校验 |
| 单次导出耗时长 | 前端长超时 + 按钮 loading 状态；`.epub` 由浏览器直接下载不占内存 |

---

## 十、实施步骤

> 全程遵守第二节的隔离原则：**只新增文件、只新增分支，不改写既有逻辑**。

1. `internal/service/epub.go` — epub 骨架（zip 装配、`mimetype`、OPF、nav、NCX、CSS）+ `EpubGenerator`（**独立类型，不碰 `DocGenerator` 接口**）
2. `internal/service/epub_content.go` — HTML→XHTML 转换与清洗
3. `internal/service/epub_image.go` — 图片收集 / 缓存 / 并发下载
4. `internal/service/epub_comment.go` — 评论 + 讨论的批量加载、渲染、数量模式（默认 `all`）
5. `internal/types/task/task.go` 加 `Comments` 字段 + `internal/api/v2/task.go` `Export` **追加** `case "epub"`
6. `cmd/cli/epub.go`（含 `--comments` flag，默认 `all`）+ `cmd/cmd.go` **追加**一行注册
7. 前端：`api/task.ts` 追加 epub 分支 → `LessonDetail.tsx` / `TaskCard.tsx` / `CollectList.tsx` / `TaskList.tsx` 各加按钮与分支
8. **回归验证**（见第二节末尾：旧三种导出 + build/vet/tsc 必须全部照旧）
9. 功能验证（见下）

---

## 十一、验证方式

```bash
# 1. 用 CLI 导出一门课（默认含全部留言）
my-geektime cli epub --config=config.yml --taskid=<courseId> --output=./out

# 2. 检查 zip 结构：mimetype 必须是第一个且 Store 方式
unzip -l ./out/xxx.epub
unzip -v ./out/xxx.epub | head        # Method 列 mimetype 应为 Stored

# 3. 校验 mimetype 内容（不能有换行）
unzip -p ./out/xxx.epub mimetype | xxd | head -2

# 4. 检查目录与图片数量
unzip -l ./out/xxx.epub | grep -c "OEBPS/images/"
unzip -p ./out/xxx.epub OEBPS/nav.xhtml   # 确认两级目录

# 5. 检查评论（默认 all 模式）
unzip -p ./out/xxx.epub OEBPS/text/001_*.xhtml | grep -c 'class="comment"'
#    数值应等于该文章在库里的评论条数：
#    与 SQL 对比：SELECT count(*) FROM article_comments WHERE aid=<该文 aid>

# 6. 对比三种模式
my-geektime cli epub --taskid=<courseId> --comments=hot --output=./out_hot
my-geektime cli epub --taskid=<courseId> --comments=0   --output=./out_none
#    hot 模式下单篇评论数应为 20（或不足 20 时为实际条数）
#    none 模式下应为 0，且正文部分与 all 模式**逐字节一致**
```

人工验证清单：

- [ ] 用微信读书 / Calibre / Apple Books 打开，目录显示两级结构
- [ ] 断网状态下打开，图片正常显示（确认已内嵌）
- [ ] 代码块不溢出、可横向阅读
- [ ] 封面显示正确
- [ ] 章节顺序与网页端一致
- [ ] 含视频的课程（如 99 篇中的课程）打开不报错、降级文案正常
- [ ] 评论在文章末尾显示，字号小于正文、无裂图（确认头像未内嵌）
- [ ] 全量模式下单篇评论条数与 SQL 查询结果一致
- [ ] `comments=0` 导出的版本正文与含评论版本完全一致
- [ ] **回归**：现有的「导出Markdown」「导出PDF」「在线文档」表现与改动前完全相同（文件名、格式、交互都不变）
