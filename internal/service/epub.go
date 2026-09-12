package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"strings"
	"time"

	"github.com/zkep/my-geektime/internal/global"
	"github.com/zkep/my-geektime/internal/model"
	"github.com/zkep/my-geektime/internal/types/geek"
	"go.uber.org/zap"
)

// EPUB 3.0 电子书导出。
//
// 隔离原则（不得违反）：本文件与 epub_*.go 只做加法——
//   - 不动 DocGenerator 接口（该接口只有 GoldmarkDocGenerator 一个实现）
//   - 不动 docsite.go 的 getCommentsHTML（在线/本地文档站共用）
//   - 不改数据库、配置、路由、中间件
//   - 不复用下载任务 worker，用独立并发池
//
// EPUB 本质是 zip，因此零新增第三方依赖（archive/zip + x/net/html 都已在依赖里）。

const (
	epubMimetype       = "application/epub+zip"
	epubFormatVersion  = "3.0"
	epubDefaultLang    = "zh-CN"
	epubDefaultAuthor  = "我的极客时间"
	epubDefaultChapter = "正文"
	epubOPFPath        = "OEBPS/content.opf"
)

// EpubOptions 导出选项。
type EpubOptions struct {
	// Comments 评论模式：all（默认，全部留言）| hot（每篇 20 条按赞）| 0（不含）
	Comments string
}

// NormalizeEpubOptions 归一化选项，空值一律按「全部留言」处理。
func NormalizeEpubOptions(opt EpubOptions) EpubOptions {
	switch strings.ToLower(strings.TrimSpace(opt.Comments)) {
	case epubCommentsHot, "精选":
		opt.Comments = epubCommentsHot
	case epubCommentsOff, "none", "no", "false", "off", "不包含", "关闭":
		opt.Comments = epubCommentsOff
	default:
		opt.Comments = epubCommentsAll
	}
	return opt
}

// EpubGenerator epub 生成器。
// 刻意独立于 DocGenerator 接口：给接口加方法会强迫 Goldmark 生成器实现 epub，属于强耦合。
type EpubGenerator struct{}

// NewEpubGenerator 创建 epub 生成器。
func NewEpubGenerator() *EpubGenerator {
	return &EpubGenerator{}
}

// epubChapter 一章（对应正文里的 ChapterTitle）。
type epubChapter struct {
	Title    string
	Articles []*epubArticle
}

// epubArticle 一篇文章。
type epubArticle struct {
	Index   int
	TaskId  string
	Aid     int64
	Title   string
	Author  string
	Ctime   int64
	Content string // 原始 HTML 正文
	Href    string // OEBPS 内路径，如 text/001.xhtml
	Body    string // 转换后的 XHTML 片段（正文 + 评论）
}

// epubBook 装配完成的电子书。
type epubBook struct {
	Title      string
	Author     string
	Language   string
	Identifier string
	Date       string
	Modified   string
	Intro      string // 课程简介（HTML）
	Cover      *epubImageAsset
	Chapters   []*epubChapter
	Images     []*epubImageAsset
}

// articles 按顺序摊平所有文章。
func (b *epubBook) articles() []*epubArticle {
	out := make([]*epubArticle, 0, 64)
	for _, ch := range b.Chapters {
		out = append(out, ch.Articles...)
	}
	return out
}

// MakeEpub 把一门已缓存的课程导出为单个 epub 文件。
//
// 说明：图片全部内嵌，导出的文件脱离本项目服务也能正常阅读；
// 但图片需要联网补齐（实测本机缓存命中率约 14%），因此耗时可能较长，
// 调用方（HTTP handler / CLI）需自行准备足够长的超时。
func (e *EpubGenerator) MakeEpub(ctx context.Context, taskId, title, introHTML string,
	opt EpubOptions) (*bytes.Buffer, error) {
	opt = NormalizeEpubOptions(opt)
	start := time.Now()

	var root model.Task
	if err := global.DB.WithContext(ctx).Model(&model.Task{}).
		Where(&model.Task{TaskId: taskId}).First(&root).Error; err != nil {
		return nil, fmt.Errorf("query task failed: %w", err)
	}
	var product geek.ProductBase
	if err := json.Unmarshal(root.Raw, &product); err != nil {
		return nil, fmt.Errorf("unmarshal product failed: %w", err)
	}
	if strings.TrimSpace(title) == "" {
		title = strings.TrimSpace(product.Title)
	}
	if title == "" {
		title = strings.TrimSpace(root.TaskName)
	}
	if introHTML == "" {
		introHTML = product.IntroHTML
	}

	chapters, err := e.loadChapters(ctx, taskId)
	if err != nil {
		return nil, err
	}
	// 文件序号跨章节连续分配
	index := 0
	for _, ch := range chapters {
		for _, a := range ch.Articles {
			index++
			a.Index = index
			a.Href = fmt.Sprintf("text/%03d.xhtml", index)
		}
	}

	// 评论：按 aid 批量加载，避免 N+1
	aids := make([]int64, 0, index)
	for _, a := range flattenArticles(chapters) {
		if a.Aid > 0 {
			aids = append(aids, a.Aid)
		}
	}
	comments, err := loadEpubComments(ctx, aids, opt)
	if err != nil {
		return nil, err
	}

	// 图片：先收集去重，再并发获取，最后才做正文转换（转换时需要知道每张图的落点）
	images := newEpubImageCollector()
	coverURL := firstNonEmpty(product.Cover.Square, product.Cover.Rectangle, product.Cover.Horizontal)
	images.add(coverURL)
	for _, ch := range chapters {
		for _, a := range ch.Articles {
			images.addFromHTML(a.Content)
		}
	}
	images.addFromHTML(introHTML)
	images.fetch(ctx)

	// 正文与评论转换
	bodyConv := newEpubConverter(images.href)
	textConv := newEpubTextConverter()
	for _, ch := range chapters {
		for _, a := range ch.Articles {
			body := bodyConv.Body(a.Content)
			if strings.TrimSpace(body) == "" {
				body = `<p class="media-hint">本讲以音视频内容为主，请前往网页端观看。</p>`
			}
			if list := comments[a.Aid]; len(list) > 0 {
				// 前置换行：让 comments=0 与含评论版本的正文部分成为严格前缀关系，便于校验
				body += "\n" + textConv.Body(buildCommentsHTML(list, opt.Comments))
			}
			a.Body = body
		}
	}

	lang := strings.TrimSpace(global.CONF.I18N.DefaultLang)
	if lang == "" {
		lang = epubDefaultLang
	}
	author := strings.TrimSpace(product.Author.Name)
	if author == "" {
		author = epubDefaultAuthor
	}
	book := &epubBook{
		Title:      title,
		Author:     author,
		Language:   lang,
		Identifier: "urn:uuid:" + epubUUID(taskId),
		Modified:   time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		Intro:      introHTML,
		Chapters:   chapters,
		Images:     images.list(),
	}
	date := product.Ctime
	if date <= 0 {
		date = int(root.CreatedAt)
	}
	if date > 0 {
		book.Date = time.Unix(int64(date), 0).UTC().Format(time.RFC3339)
	}
	// 封面图：与正文图片共用同一套缓存/下载逻辑，天然去重
	if asset, ok := images.asset(coverURL); ok {
		book.Cover = asset
	}

	var buf bytes.Buffer
	if err := writeEpub(&buf, book); err != nil {
		return nil, err
	}

	// book.Images 已包含封面（封面与正文图片共用同一套收集逻辑），无需重复计数
	global.LOG.Info("epub generated",
		zap.String("taskId", taskId),
		zap.Int("chapters", len(chapters)),
		zap.Int("articles", index),
		zap.Int("images", len(book.Images)),
		zap.String("comments", opt.Comments),
		zap.Int("size", buf.Len()),
		zap.Duration("cost", time.Since(start)))
	return &buf, nil
}

// flattenArticles 摊平章节内文章。
func flattenArticles(chapters []*epubChapter) []*epubArticle {
	out := make([]*epubArticle, 0, 64)
	for _, ch := range chapters {
		out = append(out, ch.Articles...)
	}
	return out
}

// loadChapters 读取课程下全部文章并按 ChapterTitle 分组。
//
// 章节顺序 = 按 id asc 遍历时的首次出现顺序，天然与阅读顺序一致，
// 不需要额外的排序字段。ChapterTitle 为空的文章统一归入「正文」。
func (e *EpubGenerator) loadChapters(ctx context.Context, taskId string) ([]*epubChapter, error) {
	var tasks []model.Task
	if err := global.DB.WithContext(ctx).Model(&model.Task{}).
		Where(&model.Task{TaskPid: taskId}).
		Order("id asc").
		Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("query tasks failed: %w", err)
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("no tasks found for task %s, "+
			"please check if the task has completed downloading", taskId)
	}
	chapters := make([]*epubChapter, 0, 8)
	index := make(map[string]*epubChapter)
	for i := range tasks {
		task := &tasks[i]
		var articleData geek.ArticleData
		if err := json.Unmarshal(task.Raw, &articleData); err != nil {
			global.LOG.Warn("epub article unmarshal failed",
				zap.String("taskId", task.TaskId), zap.Error(err))
			continue
		}
		content := articleData.Info.Content
		if strings.TrimSpace(content) == "" {
			content = articleData.Info.Cshort
		}
		title := strings.TrimSpace(articleData.Info.Title)
		if title == "" {
			title = strings.TrimSpace(task.TaskName)
		}
		chapterTitle := strings.TrimSpace(articleData.Info.ChapterTitle)
		if chapterTitle == "" {
			chapterTitle = epubDefaultChapter
		}
		chapter, ok := index[chapterTitle]
		if !ok {
			chapter = &epubChapter{Title: chapterTitle}
			index[chapterTitle] = chapter
			chapters = append(chapters, chapter)
		}
		chapter.Articles = append(chapter.Articles, &epubArticle{
			TaskId:  task.TaskId,
			Aid:     epubParseAid(task.OtherId),
			Title:   title,
			Author:  strings.TrimSpace(articleData.Info.Author.Name),
			Ctime:   int64(articleData.Info.Ctime),
			Content: content,
		})
	}
	if len(chapters) == 0 {
		return nil, fmt.Errorf("no readable article found for task %s", taskId)
	}
	return chapters, nil
}

// epubParseAid 从 task.other_id 解析文章 id（评论靠它关联，与 docsite.go 的做法一致）。
func epubParseAid(otherId string) int64 {
	var aid int64
	if _, err := fmt.Sscanf(strings.TrimSpace(otherId), "%d", &aid); err != nil {
		return 0
	}
	return aid
}

// epubUUID 由 taskId 派生稳定 UUID：同一门课程重复导出，dc:identifier 不变。
func epubUUID(taskId string) string {
	h := epubMD5("my-geektime:" + taskId)
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// writeEpub 装配 epub 包。
func writeEpub(buf *bytes.Buffer, book *epubBook) error {
	zw := zip.NewWriter(buf)
	// EPUB 规范硬要求：mimetype 必须是 zip 的第一个条目，且必须用 STORE（不压缩），
	// 内容恰好为 application/epub+zip 且不带换行。
	// 用默认的 Deflate 或顺序放错，部分阅读器会直接判定「不是有效 epub」。
	fw, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return err
	}
	if _, err := fw.Write([]byte(epubMimetype)); err != nil {
		return err
	}

	parts := []struct {
		name    string
		content string
	}{
		{"META-INF/container.xml", epubContainerXML()},
		{"OEBPS/content.opf", epubContentOPF(book)},
		{"OEBPS/nav.xhtml", epubNavXHTML(book)},
		{"OEBPS/toc.ncx", epubTOCNCX(book)},
		{"OEBPS/style.css", epubStyleCSS},
	}
	for _, part := range parts {
		if err := zipWriteString(zw, part.name, part.content); err != nil {
			return err
		}
	}
	if book.Cover != nil {
		if err := zipWriteString(zw, "OEBPS/cover.xhtml", epubCoverXHTML(book)); err != nil {
			return err
		}
	}
	for _, a := range book.articles() {
		doc := epubArticleDocument(book, a)
		if err := zipWriteString(zw, "OEBPS/"+a.Href, doc); err != nil {
			return err
		}
	}
	// 图片本来就是压缩格式，再 Deflate 纯属浪费 CPU，直接 STORE
	for _, img := range book.Images {
		if book.Cover != nil && img.URL == book.Cover.URL {
			continue
		}
		if err := zipWriteBytesStored(zw, "OEBPS/"+img.ManifestHref, img.Data); err != nil {
			return err
		}
	}
	if book.Cover != nil {
		if err := zipWriteBytesStored(zw, "OEBPS/"+book.Cover.ManifestHref, book.Cover.Data); err != nil {
			return err
		}
	}
	return zw.Close()
}

func zipWriteString(zw *zip.Writer, name, content string) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, content)
	return err
}

func zipWriteBytesStored(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func epubContainerXML() string {
	return xmlHeader +
		`<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">` + "\n" +
		`  <rootfiles>` + "\n" +
		`    <rootfile full-path="` + epubOPFPath + `" media-type="application/oebps-package+xml"/>` + "\n" +
		`  </rootfiles>` + "\n" +
		`</container>` + "\n"
}

const xmlHeader = `<?xml version="1.0" encoding="utf-8"?>` + "\n"

type epubManifestItem struct {
	ID        string
	Href      string
	MediaType string
	Props     string
}

// epubContentOPF 生成包文档（metadata + manifest + spine）。
func epubContentOPF(book *epubBook) string {
	items := []epubManifestItem{
		{ID: "nav", Href: "nav.xhtml", MediaType: "application/xhtml+xml", Props: "nav"},
		{ID: "ncx", Href: "toc.ncx", MediaType: "application/x-dtbncx+xml"},
		{ID: "css", Href: "style.css", MediaType: "text/css"},
	}
	spine := make([]string, 0, len(book.Chapters)+4)
	if book.Cover != nil {
		items = append(items, epubManifestItem{
			ID: "cover", Href: "cover.xhtml", MediaType: "application/xhtml+xml"})
		items = append(items, epubManifestItem{
			ID: "cover-image", Href: book.Cover.ManifestHref,
			MediaType: book.Cover.MediaType, Props: "cover-image"})
		spine = append(spine, "cover")
	}
	spine = append(spine, "nav")
	for i, a := range book.articles() {
		id := fmt.Sprintf("page_%d", i+1)
		items = append(items, epubManifestItem{
			ID: id, Href: a.Href, MediaType: "application/xhtml+xml"})
		spine = append(spine, id)
	}
	for i, img := range book.Images {
		if book.Cover != nil && img.URL == book.Cover.URL {
			continue
		}
		items = append(items, epubManifestItem{
			ID: fmt.Sprintf("img_%d", i+1), Href: img.ManifestHref, MediaType: img.MediaType})
	}

	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<package xmlns="http://www.idpf.org/2007/opf" version="` + epubFormatVersion +
		`" unique-identifier="bookid" xml:lang="` + epubEscape(book.Language) + `">` + "\n")
	b.WriteString(`  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">` + "\n")
	b.WriteString(`    <dc:identifier id="bookid">` + epubEscape(book.Identifier) + `</dc:identifier>` + "\n")
	b.WriteString(`    <dc:title>` + epubEscape(book.Title) + `</dc:title>` + "\n")
	b.WriteString(`    <dc:creator>` + epubEscape(book.Author) + `</dc:creator>` + "\n")
	b.WriteString(`    <dc:language>` + epubEscape(book.Language) + `</dc:language>` + "\n")
	if book.Date != "" {
		b.WriteString(`    <dc:date>` + epubEscape(book.Date) + `</dc:date>` + "\n")
	}
	// EPUB 3 必须提供 dcterms:modified，格式固定为 YYYY-MM-DDThh:mm:ssZ（UTC），
	// 缺失会被 EPUBCheck 判为错误。
	b.WriteString(`    <meta property="dcterms:modified">` + book.Modified + `</meta>` + "\n")
	b.WriteString(`    <dc:publisher>` + epubEscape(epubDefaultAuthor) + `</dc:publisher>` + "\n")
	if book.Cover != nil {
		// 兼容 EPUB 2 阅读器识别封面
		b.WriteString(`    <meta name="cover" content="cover-image"/>` + "\n")
	}
	b.WriteString(`  </metadata>` + "\n")
	b.WriteString(`  <manifest>` + "\n")
	for _, item := range items {
		b.WriteString(`    <item id="` + item.ID + `" href="` + epubEscape(item.Href) +
			`" media-type="` + item.MediaType + `"`)
		if item.Props != "" {
			b.WriteString(` properties="` + item.Props + `"`)
		}
		b.WriteString(`/>` + "\n")
	}
	b.WriteString(`  </manifest>` + "\n")
	b.WriteString(`  <spine toc="ncx">` + "\n")
	for _, idref := range spine {
		b.WriteString(`    <itemref idref="` + idref + `"/>` + "\n")
	}
	b.WriteString(`  </spine>` + "\n")
	b.WriteString(`</package>` + "\n")
	return b.String()
}

// epubNavXHTML 生成 EPUB 3 导航文档（两级目录：章节 → 文章）。
func epubNavXHTML(book *epubBook) string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(epubHTMLElement(book.Language, true))
	b.WriteString("\n<head>\n<meta charset=\"utf-8\"/>\n<title>目录</title>\n")
	b.WriteString(`<link rel="stylesheet" type="text/css" href="style.css"/>` + "\n</head>\n<body>\n")
	b.WriteString(`  <nav epub:type="toc" id="toc">` + "\n    <h1>目录</h1>\n    <ol>\n")
	for _, ch := range book.Chapters {
		b.WriteString(`      <li><a href="` + epubEscape(ch.Articles[0].Href) + `">` +
			epubEscape(ch.Title) + `</a>` + "\n")
		b.WriteString(`        <ol>` + "\n")
		for _, a := range ch.Articles {
			b.WriteString(`          <li><a href="` + epubEscape(a.Href) + `">` +
				epubEscape(a.Title) + `</a></li>` + "\n")
		}
		b.WriteString(`        </ol>` + "\n      </li>\n")
	}
	b.WriteString("    </ol>\n  </nav>\n")
	// landmarks 帮助阅读器定位封面/正文
	b.WriteString(`  <nav epub:type="landmarks" id="landmarks" hidden="hidden">` + "\n    <ol>\n")
	if book.Cover != nil {
		b.WriteString(`      <li><a epub:type="cover" href="cover.xhtml">封面</a></li>` + "\n")
	}
	b.WriteString(`      <li><a epub:type="toc" href="nav.xhtml">目录</a></li>` + "\n")
	if articles := book.articles(); len(articles) > 0 {
		b.WriteString(`      <li><a epub:type="bodymatter" href="` +
			epubEscape(articles[0].Href) + `">正文</a></li>` + "\n")
	}
	b.WriteString("    </ol>\n  </nav>\n</body>\n</html>\n")
	return b.String()
}

// epubTOCNCX 生成 EPUB 2 兼容目录（老阅读器与 Kindle 转换会用到）。
func epubTOCNCX(book *epubBook) string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1" xml:lang="` +
		epubEscape(book.Language) + `">` + "\n")
	b.WriteString("  <head>\n")
	b.WriteString(`    <meta name="dtb:uid" content="` + epubEscape(book.Identifier) + `"/>` + "\n")
	b.WriteString(`    <meta name="dtb:depth" content="2"/>` + "\n")
	b.WriteString(`    <meta name="dtb:totalPageCount" content="0"/>` + "\n")
	b.WriteString(`    <meta name="dtb:maxPageNumber" content="0"/>` + "\n")
	b.WriteString("  </head>\n")
	b.WriteString(`  <docTitle><text>` + epubEscape(book.Title) + `</text></docTitle>` + "\n")
	b.WriteString("  <navMap>\n")
	playOrder := 0
	for ci, ch := range book.Chapters {
		playOrder++
		id := fmt.Sprintf("np_%d", ci+1)
		b.WriteString(`    <navPoint id="` + id + `" playOrder="` + fmt.Sprint(playOrder) + `">` + "\n")
		b.WriteString(`      <navLabel><text>` + epubEscape(ch.Title) + `</text></navLabel>` + "\n")
		b.WriteString(`      <content src="` + epubEscape(ch.Articles[0].Href) + `"/>` + "\n")
		for ai, a := range ch.Articles {
			playOrder++
			b.WriteString(`      <navPoint id="` + fmt.Sprintf("%s_%d", id, ai+1) +
				`" playOrder="` + fmt.Sprint(playOrder) + `">` + "\n")
			b.WriteString(`        <navLabel><text>` + epubEscape(a.Title) + `</text></navLabel>` + "\n")
			b.WriteString(`        <content src="` + epubEscape(a.Href) + `"/>` + "\n")
			b.WriteString("      </navPoint>\n")
		}
		b.WriteString("    </navPoint>\n")
	}
	b.WriteString("  </navMap>\n</ncx>\n")
	return b.String()
}

// epubCoverXHTML 封面页：封面图 + 书名 + 作者 + 课程简介。
func epubCoverXHTML(book *epubBook) string {
	if book.Cover == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(epubHTMLElement(book.Language, false))
	b.WriteString("\n<head>\n<meta charset=\"utf-8\"/>\n<title>" + epubEscape(book.Title) + "</title>\n")
	b.WriteString(`<link rel="stylesheet" type="text/css" href="style.css"/>` + "\n</head>\n<body>\n")
	b.WriteString(`<div class="cover-page">` + "\n")
	b.WriteString(`<img class="cover-image" src="` + epubEscape(book.Cover.ManifestHref) +
		`" alt="` + epubEscape(book.Title) + `"/>` + "\n")
	b.WriteString(`<h1 class="book-title">` + epubEscape(book.Title) + `</h1>` + "\n")
	b.WriteString(`<p class="book-author">` + epubEscape(book.Author) + `</p>` + "\n")
	if intro := epubIntroBody(book); intro != "" {
		b.WriteString(`<div class="book-intro">` + intro + `</div>` + "\n")
	}
	b.WriteString("</div>\n</body>\n</html>\n")
	return b.String()
}

// epubIntroBody 把课程简介转成 XHTML；转换失败时返回空串（封面页仍可生成）。
func epubIntroBody(book *epubBook) string {
	if strings.TrimSpace(book.Intro) == "" {
		return ""
	}
	return newEpubConverter(nil).Body(book.Intro)
}

// epubArticleDocument 生成单篇文章的 XHTML 文档。
//
// 每篇独立做 XML 校验：不通过的降级为纯文本，避免其中一个坏标签毁掉整本书。
func epubArticleDocument(book *epubBook, a *epubArticle) string {
	head := "<h1 class=\"article-title\">" + epubEscape(a.Title) + "</h1>\n"
	if meta := epubArticleMeta(a); meta != "" {
		head += "<p class=\"article-meta\">" + meta + "</p>\n"
	}
	doc := epubXHTMLDocument(book.Language, a.Title, head+a.Body, "style.css")
	if epubIsValidXML(doc) {
		return doc
	}
	global.LOG.Warn("epub article xhtml invalid, fallback to plain text",
		zap.String("taskId", a.TaskId), zap.String("title", a.Title))

	plain := epubPlainText(a.Content)
	fallbackHead := "<h1 class=\"article-title\">" + epubEscape(a.Title) + "</h1>\n"
	body := "<p class=\"media-hint\">（正文格式无法转换为电子书，以下为纯文本内容）</p>\n"
	if plain != "" {
		body += "<p>" + html.EscapeString(plain) + "</p>\n"
	}
	doc = epubXHTMLDocument(book.Language, a.Title, fallbackHead+body, "style.css")
	if epubIsValidXML(doc) {
		return doc
	}
	return epubXHTMLDocument(book.Language, a.Title,
		"<h1 class=\"article-title\">"+epubEscape(a.Title)+"</h1>\n<p>正文内容不可用。</p>\n", "style.css")
}

func epubArticleMeta(a *epubArticle) string {
	parts := make([]string, 0, 2)
	if a.Author != "" {
		parts = append(parts, epubEscape(a.Author))
	}
	if a.Ctime > 0 {
		parts = append(parts, time.Unix(a.Ctime, 0).Format(time.DateOnly))
	}
	return strings.Join(parts, " · ")
}

// epubHTMLElement 拼 <html> 开始标签。
// epub:type 需要声明 epub 命名空间，否则 nav.xhtml 无法通过校验。
func epubHTMLElement(lang string, withEPUB bool) string {
	s := `<html xmlns="http://www.w3.org/1999/xhtml"`
	if withEPUB {
		s += ` xmlns:epub="http://www.idpf.org/2007/ops"`
	}
	s += ` xml:lang="` + epubEscape(lang) + `" lang="` + epubEscape(lang) + `">`
	return s
}

// epubXHTMLDocument 拼装完整 XHTML 文档。
// cssHref 是相对当前文档的样式表路径。
func epubXHTMLDocument(lang, title, body, cssHref string) string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString("<!DOCTYPE html>\n")
	b.WriteString(epubHTMLElement(lang, false))
	b.WriteString("\n<head>\n")
	b.WriteString(`<meta charset="utf-8"/>` + "\n")
	b.WriteString("<title>" + epubEscape(title) + "</title>\n")
	b.WriteString(`<link rel="stylesheet" type="text/css" href="` + epubEscape(cssHref) + `"/>` + "\n")
	b.WriteString("</head>\n<body>\n")
	b.WriteString(body)
	b.WriteString("\n</body>\n</html>\n")
	return b.String()
}

// epubEscape XML 文本/属性转义。html.EscapeString 产出的实体在 XML 中同样合法。
func epubEscape(s string) string {
	return html.EscapeString(epubStripInvalidXMLChars(s))
}

// epubStyleCSS 正文样式。
//
// 两个关键点：
//   - pre 必须 white-space: pre-wrap + word-wrap: break-word，
//     否则长代码行在窄屏电纸书上会溢出（本项目大量文章含代码块）；
//   - 评论字号小于正文且用灰色，不加背景色——e-ink 上深色底反而更刺眼。
const epubStyleCSS = `@charset "utf-8";

body { font-family: "Noto Sans SC", "Source Han Sans SC", "PingFang SC", serif;
       line-height: 1.7; margin: 0 1em; color: #111; }
h1, h2, h3, h4, h5, h6 { line-height: 1.4; margin: 1em 0 .5em; }
p { margin: .6em 0; }
img { max-width: 100%; height: auto; }
a { color: #0645ad; text-decoration: none; }
pre { font-family: monospace; font-size: .85em; white-space: pre-wrap;
      word-wrap: break-word; background: #f6f8fa; padding: .6em; }
code { font-family: monospace; font-size: .9em; }
pre code { font-size: 1em; }
table { border-collapse: collapse; max-width: 100%; font-size: .9em; }
th, td { border: 1px solid #ccc; padding: .3em .5em; }
blockquote { margin: .8em 0; padding-left: .8em; border-left: 3px solid #ddd; color: #555; }
figure { margin: 1em 0; }
.article-title { font-size: 1.35em; margin-bottom: .2em; }
.article-meta { font-size: .8em; color: #888; margin-top: 0; }
.media-hint { font-size: .85em; color: #888; }

.cover-page { text-align: center; margin-top: 15%; }
.cover-image { max-width: 80%; }
.book-title { font-size: 1.6em; margin-top: 1.2em; }
.book-author { font-size: 1em; color: #555; }
.book-intro { text-align: left; font-size: .9em; color: #444; margin-top: 2em; }

.comments { margin-top: 2.5em; border-top: 1px solid #ddd; padding-top: .8em; }
.comments-title { font-size: 1.05em; color: #555; }
.comment { margin: 1em 0; }
.comment-meta { font-size: .8em; color: #888; margin: 0 0 .2em; }
.comment-body { font-size: .92em; color: #333; }
.comment-replies { margin-left: 1.2em; padding-left: .8em; border-left: 2px solid #eee; }
.reply { margin: .6em 0; }
.reply .comment-body { font-size: .9em; color: #555; }
`
