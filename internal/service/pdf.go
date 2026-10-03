package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/cdproto/page"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpuModel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/zkep/my-geektime/internal/global"
	"github.com/zkep/my-geektime/internal/model"
	"github.com/zkep/my-geektime/internal/types/geek"
	"github.com/zkep/my-geektime/libs/utils"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// PDFComment 评论数据（含讨论）
type PDFComment struct {
	UserHeader      string             `json:"user_header,omitempty"`
	UserName        string             `json:"user_name,omitempty"`
	LikeCount       int64              `json:"like_count,omitempty"`
	DiscussionCount int64              `json:"discussion_count,omitempty"`
	Content         string             `json:"comment_content,omitempty"`
	Time            string             `json:"time,omitempty"`
	Discussions     []PDFDiscussion    `json:"discussions,omitempty"`
}

// PDFDiscussion 讨论/回复数据
type PDFDiscussion struct {
	Avatar           string             `json:"avatar,omitempty"`
	Nickname         string             `json:"nickname,omitempty"`
	ReplyNickname    string             `json:"reply_nickname,omitempty"`
	Content          string             `json:"discussion_content,omitempty"`
	Time             string             `json:"time,omitempty"`
	LikesNumber      int64              `json:"likes_number,omitempty"`
	ChildDiscussions []PDFChildDiscussion `json:"child_discussions,omitempty"`
}

// PDFChildDiscussion 子讨论/回复
type PDFChildDiscussion struct {
	AuthorNickname string `json:"author_nickname,omitempty"`
	ReplyNickname  string `json:"reply_nickname,omitempty"`
	Content        string `json:"content,omitempty"`
}

// PDFData PDF 生成数据
type PDFData struct {
	Title    string
	Content  template.HTML
	Comments []PDFComment
}

// ============================================================================
// 渲染资源管理：章节 PDF 磁盘缓存 + 单 Chrome 实例复用 + 全局并发闸门
//
// 背景（2026-10-03 实测）：课程级 PDF 导出在 32 章的课上必然超时。
// 原实现每章都 chromedp.NewContext（= 启动一个全新 Chrome 进程），并发仅 3，
// 实测吞吐 ≈ 24 章 / 20 分钟。下面三项改造把重复导出变成近乎瞬时。
// ============================================================================

// chapterRenderSem 全局章节渲染并发上限，**跨请求共享**。
// 单请求内部并发见 GenerateCoursePDF 的 maxConcurrency；多个课程同时导出时
// 用这个闸门保证 Chrome 总 tab 数有界，避免把机器内存打爆。
//
// 2026-10-03 由 8 提到 12：单请求并发同时由 6 提到 8，闸门必须大于单请求并发，
// 否则「两个课程同时导出」会退化成串行等待（第二个请求的章节卡在闸门上）。
var chapterRenderSem = make(chan struct{}, 12)

// chapterFlight 合并「同一章被并发渲染」的重复工作。
//
// 为什么需要：客户端断开（或 http.TimeoutHandler 到期）后，handler goroutine
// 并不会被杀掉 —— newPdfChrome 刻意用 context.WithoutCancel 让渲染跑完。
// 于是「用户重试」会与「上一次还在跑的渲染」同时渲染同一章，既浪费 CPU，
// 又会争抢同一个缓存文件。singleflight 让后来者直接等第一次的结果。
var chapterFlight singleflight.Group

// pdfProxyTransport 供「每章的临时 HTTP server 把 /v2/file/proxy 转发回本进程」使用。
//
// ⚠️ 必须是包级单例，不能每章 `&http.Transport{}`：一章 60~68 张图全走它，
// 而零值 Transport 的 MaxIdleConnsPerHost=2 ⇒ 绝大多数连接用完即弃、无法复用，
// 「省一次建连」根本没省到。目标是 127.0.0.1，故用不挂环境代理的 loopback 版本。
var pdfProxyTransport = global.NewLoopbackTransport()

// 单次课程级导出的整体时间预算（detCtx），必须**小于** middleware.PDFTimeout 的
// 60 分钟硬超时，否则 detCtx 先到期时 http.TimeoutHandler 不会再写响应体。
//
// 每章预算按实测反推：单章渲染 100~150s（图片经 /v2/file/proxy 回环拉取是主瓶颈），
// 并发 8 ⇒ 每章摊到约 19s；实测 32 章用了 27 分钟（约 50s/章，因多个 tab 共享一个
// Chrome 实例有资源争抢），故取 60s/章 留足余量。
const (
	pdfCourseBudgetPerChapter = 60 * time.Second
	pdfCourseBudgetMin        = 30 * time.Minute
	pdfCourseBudgetMax        = 55 * time.Minute
)

// pdfChrome 持有一个 Chrome 实例（浏览器进程），为每章开新 tab 复用。
type pdfChrome struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// newPdfChrome 启动一个 Chrome 实例。
// 刻意用 context.WithoutCancel 剥离调用方的取消信号：HTTP 请求超时不应该
// 把已经排队的渲染一起干掉（虽然外层响应已经写不回去了）。
func newPdfChrome(ctx context.Context) *pdfChrome {
	parent := context.Background()
	if ctx != nil && ctx.Err() == nil {
		parent = context.WithoutCancel(ctx)
	}
	c, cancel := chromedp.NewContext(parent)
	return &pdfChrome{ctx: c, cancel: cancel}
}

func (p *pdfChrome) close() {
	if p != nil && p.cancel != nil {
		p.cancel()
	}
}

// ---------- 章节 PDF 磁盘缓存 ----------

const (
	pdfCacheDirName  = "my-geektime/pdf-chapter-cache"
	pdfCacheMaxBytes = int64(2) << 30 // 2GB，超出后按修改时间淘汰最旧的
)

var (
	pdfCacheDirOnce sync.Once
	pdfCacheDirPath string
)

// pdfCacheDir 返回章节 PDF 缓存目录（用户级 cache 目录，不污染项目工作区）。
func pdfCacheDir() string {
	pdfCacheDirOnce.Do(func() {
		base, err := os.UserCacheDir()
		if err != nil {
			base = os.TempDir()
		}
		pdfCacheDirPath = filepath.Join(base, filepath.FromSlash(pdfCacheDirName))
		//nolint:errcheck // 缓存目录创建失败只会导致缓存不命中，不该阻断导出
		os.MkdirAll(pdfCacheDirPath, 0755)
		global.LOG.Info("pdf.cache.dir", zap.String("path", pdfCacheDirPath))
	})
	return pdfCacheDirPath
}

func pdfCachePaths(taskId string) (pdfPath, hashPath string) {
	dir := pdfCacheDir()
	safe := VerifyFileName(taskId)
	return filepath.Join(dir, safe+".pdf"), filepath.Join(dir, safe+".sha256")
}

// htmlFingerprint 用渲染前的 HTML 全文做指纹。
// 正文或评论一变指纹就变 ⇒ 缓存自动失效，不会返回过期内容。
func htmlFingerprint(html []byte) string {
	sum := sha256.Sum256(html)
	return hex.EncodeToString(sum[:])[:32]
}

func loadCachedPDF(taskId, fingerprint string) ([]byte, bool) {
	pdfPath, hashPath := pdfCachePaths(taskId)
	// 先比对指纹文件，避免把「内容已变但文件名没变」的旧 PDF 当成命中
	if got, err := os.ReadFile(hashPath); err != nil || strings.TrimSpace(string(got)) != fingerprint {
		return nil, false
	}
	data, err := os.ReadFile(pdfPath)
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

func storeCachedPDF(taskId, fingerprint string, pdf []byte) {
	pdfPath, hashPath := pdfCachePaths(taskId)
	// ⚠️ 临时文件名必须唯一。同一章可能被并发渲染：上一次请求被客户端断开后
	// handler goroutine 仍会继续跑完（见 newPdfChrome 的说明），此时新请求又起一个
	// goroutine 渲染同一章。若用固定的 `pdfPath + ".tmp"`，两个 writer 会交错写同一个
	// 文件，rename 之后缓存里就是**损坏的 PDF**（实测：合并时报
	// "validatePages: cannot dereference pageNodeDict"，且损坏版本比完整版本更大）。
	tmp, err := os.CreateTemp(filepath.Dir(pdfPath), filepath.Base(pdfPath)+"-*.tmp")
	if err != nil {
		global.LOG.Warn("pdf.cache.createTemp failed", zap.Error(err))
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(pdf); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		global.LOG.Warn("pdf.cache.write failed", zap.Error(err))
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		global.LOG.Warn("pdf.cache.close failed", zap.Error(err))
		return
	}
	if err := os.Rename(tmpName, pdfPath); err != nil {
		os.Remove(tmpName)
		global.LOG.Warn("pdf.cache.rename failed", zap.Error(err))
		return
	}
	//nolint:errcheck // 指纹写失败只是下次缓存不命中
	os.WriteFile(hashPath, []byte(fingerprint), 0644)
}

// prunePDFCache 缓存总量超限时按修改时间淘汰最旧的文件。失败静默。
func prunePDFCache() {
	type entry struct {
		path string
		size int64
		mod  time.Time
	}
	dir := pdfCacheDir()
	items, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var list []entry
	var total int64
	for _, it := range items {
		if it.IsDir() || !strings.HasSuffix(it.Name(), ".pdf") {
			continue
		}
		info, err := it.Info()
		if err != nil {
			continue
		}
		p := filepath.Join(dir, it.Name())
		list = append(list, entry{path: p, size: info.Size(), mod: info.ModTime()})
		total += info.Size()
	}
	if total <= pdfCacheMaxBytes {
		return
	}
	sort.Slice(list, func(i, j int) bool { return list[i].mod.Before(list[j].mod) })
	freed := int64(0)
	for _, e := range list {
		if total-freed <= pdfCacheMaxBytes {
			break
		}
		if os.Remove(e.path) == nil {
			//nolint:errcheck // 指纹文件删不掉只会让该条缓存不命中
			os.Remove(strings.TrimSuffix(e.path, ".pdf") + ".sha256")
			freed += e.size
		}
	}
	global.LOG.Info("pdf.cache.pruned",
		zap.Int64("beforeBytes", total),
		zap.Int64("freedBytes", freed))
}

var pdfPruneOnce sync.Once

// ============================================================================
// 单章 / 课程 PDF 生成
// ============================================================================

// GenerateArticlePDF 生成单篇文章 PDF
func GenerateArticlePDF(ctx context.Context, taskId string) ([]byte, error) {
	// 脱离 HTTP 请求上下文，使用独立超时避免被客户端断开影响
	detCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	var l model.Task
	if err := global.DB.WithContext(detCtx).
		Where(&model.Task{TaskId: taskId}).First(&l).Error; err != nil {
		return nil, fmt.Errorf("query task failed: %w", err)
	}
	var articleData geek.ArticleData
	if err := json.Unmarshal(l.Raw, &articleData); err != nil {
		return nil, fmt.Errorf("unmarshal article data failed: %w", err)
	}
	pdfPruneOnce.Do(prunePDFCache)
	pdfBytes, fromCache, err := generateSinglePDF(detCtx, nil, l.TaskId, l.TaskName, l.OtherId, articleData)
	if err != nil {
		return nil, err
	}
	global.LOG.Info("GenerateArticlePDF",
		zap.String("taskId", taskId),
		zap.Int("pdfSize", len(pdfBytes)),
		zap.Bool("fromCache", fromCache))
	return pdfBytes, nil
}

// queryCourseChapters 取一门课的全部章节（按 id 升序），用一个短 context。
func queryCourseChapters(pid string) ([]*model.Task, error) {
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer probeCancel()
	var tasks []*model.Task
	err := global.DB.WithContext(probeCtx).
		Where("task_pid = ? AND task_type = ? AND deleted_at = 0", pid, TASK_TYPE_ARTICLE).
		Order("id asc").
		Find(&tasks).Error
	return tasks, err
}

// ---------- 课程级合并 PDF 缓存 ----------
//
// 章节缓存（loadCachedPDF）只省掉「渲染」这一环。但每次导出仍要：
//   1. 为每章重建渲染 HTML —— 含 fetchArticleComments（评论 + N+1 讨论查询）、
//      HtmlURLProxyReplace 正则、模板执行、SHA-256 指纹；
//   2. 把 N 份章节 PDF 重新合并成一份大 PDF（实测 44 章 / 99MB 约 15s）。
// 实测（2026-10-03）：同一门 44 章课，章节缓存 100% 命中的请求仍耗时 4~5 分钟，
// 而首次冷渲染是 12m32s。也就是说「重复导出」远没有达到应有的速度。
//
// 课程缓存直接落盘保存**最终合并结果**，命中即用，跳过 1、2 两步。

func courseCachePaths(pid string) (pdfPath, hashPath string) {
	dir := pdfCacheDir()
	safe := "course-" + VerifyFileName(pid)
	return filepath.Join(dir, safe+".pdf"), filepath.Join(dir, safe+".sha256")
}

// courseFingerprint 用「各章 task.Raw 的 SHA-256」聚合作为课程指纹。
//
// 之所以用 Raw 而不是渲染后的 HTML：Raw 直接来自 DB 里已加载的 tasks，
// 算一次指纹是毫秒级，**可以在渲染之前就判定能否命中缓存**；而 HTML 指纹
// 必须先付出评论查询 + 模板渲染的代价才能得到，等于没省。
//
// ⚠️ 取舍：评论内容变化**不会**让课程缓存失效（评论独立存在 article_comment 表）。
// 这是为把重复导出压到秒级而有意付出的代价；需要拉取最新评论时，
// 删除 <pid>.sha256（或整个缓存目录）即可。正文（Raw）变化会自动失效。
func courseFingerprint(tasks []*model.Task) string {
	h := sha256.New()
	for _, t := range tasks {
		sum := sha256.Sum256(t.Raw)
		h.Write([]byte(t.TaskId))
		h.Write([]byte{':'})
		h.Write(sum[:])
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func storeCoursePDF(pdfPath, hashPath, fingerprint string, pdf []byte) error {
	// 与章节缓存同理：临时文件名必须唯一，避免并发导出同一门课时交错写坏文件。
	tmp, err := os.CreateTemp(filepath.Dir(pdfPath), filepath.Base(pdfPath)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(pdf); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, pdfPath); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.WriteFile(hashPath, []byte(fingerprint), 0644)
}

// GenerateCoursePDFToFile 生成（或复用）课程 PDF，返回**磁盘文件路径**。
//
// 返回路径而非 []byte 的原因：课程 PDF 可达上百 MB，用 c.File 下发时
// net/http 会自动带上 Content-Length 并走流式拷贝，避免在内存里再复制一份。
// 之前用 c.Writer.Write(pdfBytes) 发 99MB，响应没有 Content-Length（只能 chunked），
// 浏览器无法判断完整性 —— 实测在 12 分钟的长等待后于 10.2MB 处
// 报 net::ERR_FAILED（DevTools: Size 10,487 kB）。
func GenerateCoursePDFToFile(ctx context.Context, pid string) (string, error) {
	return GenerateCoursePDFToFileWithProgress(ctx, pid, nil)
}

// GenerateCoursePDFToFileWithProgress 同 GenerateCoursePDFToFile，额外上报进度。
// onProgress 的线程安全与轻量要求同 GenerateCoursePDFWithProgress。
func GenerateCoursePDFToFileWithProgress(ctx context.Context, pid string, onProgress func(CoursePDFProgress)) (string, error) {
	tasks, err := queryCourseChapters(pid)
	if err != nil {
		return "", fmt.Errorf("query chapter tasks failed: %w", err)
	}
	if len(tasks) == 0 {
		return "", fmt.Errorf("no chapter tasks found for course %s", pid)
	}

	pdfPath, hashPath := courseCachePaths(pid)
	fingerprint := courseFingerprint(tasks)

	if got, err := os.ReadFile(hashPath); err == nil && strings.TrimSpace(string(got)) == fingerprint {
		if fi, err := os.Stat(pdfPath); err == nil && fi.Size() > 0 {
			global.LOG.Info("pdf.course.cacheHit",
				zap.String("pid", pid),
				zap.Int("chapters", len(tasks)),
				zap.Int64("pdfBytes", fi.Size()))
			// 命中课程缓存的快路径也要上报一次：作业化下载靠它立刻转入「可下载」，
			// 否则前端会一直停在 pending，白等轮询。
			if onProgress != nil {
				onProgress(CoursePDFProgress{
					Phase:     PDFJobDone,
					Total:     len(tasks),
					Done:      len(tasks),
					CacheHits: len(tasks), // 命中课程缓存 == 全部章节都不需要重渲染
				})
			}
			return pdfPath, nil
		}
	}

	merged, err := GenerateCoursePDFWithProgress(ctx, pid, onProgress)
	if err != nil {
		return "", err
	}

	if err := storeCoursePDF(pdfPath, hashPath, fingerprint, merged); err != nil {
		// 写缓存失败不应拖垮本次导出：退回临时文件照样能把 PDF 发给用户。
		global.LOG.Warn("pdf.course.cacheStore failed, fallback to temp file",
			zap.String("pid", pid), zap.Error(err))
		tmp, terr := os.CreateTemp("", "geektime-course-*.pdf")
		if terr != nil {
			return "", terr
		}
		if _, werr := tmp.Write(merged); werr != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			return "", werr
		}
		tmp.Close()
		return tmp.Name(), nil
	}
	return pdfPath, nil
}

// CoursePDFProgress 是课程 PDF 生成过程中的进度快照，供作业化下载（pdfjob.go）回报前端。
type CoursePDFProgress struct {
	// Phase 取值就是 pdfjob.go 里的 PDFJob* 状态常量（rendering / merging / done）
	Phase     string
	Total     int
	Done      int
	CacheHits int
}

// GenerateCoursePDF 生成整门课程 PDF（所有章节合并为一个 PDF）
func GenerateCoursePDF(ctx context.Context, pid string) ([]byte, error) {
	return GenerateCoursePDFWithProgress(ctx, pid, nil)
}

// GenerateCoursePDFWithProgress 同 GenerateCoursePDF，额外通过 onProgress 上报进度。
//
// ⚠️ onProgress 会被各章节 goroutine **并发**调用 ⇒ 实现方必须自己保证线程安全，
// 且回调必须足够轻量（只写内存）；在里面做 IO 会直接拖慢渲染。onProgress 可为 nil。
func GenerateCoursePDFWithProgress(ctx context.Context, pid string, onProgress func(CoursePDFProgress)) ([]byte, error) {
	// 1. 先查章节，用一个短 context；拿到章节数后再据此决定整体时间预算
	tasks, err := queryCourseChapters(pid)
	if err != nil {
		return nil, fmt.Errorf("query chapter tasks failed: %w", err)
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("no chapter tasks found for course %s", pid)
	}

	// 2. 时间预算：按章节数缩放，并保证严格小于 middleware.PDFTimeout。
	//    反过来写（detCtx 比 handler 短）会让 http.TimeoutHandler 先到期、
	//    响应体被丢弃 ⇒ 前端只看到空 body。
	budget := time.Duration(len(tasks)) * pdfCourseBudgetPerChapter
	if budget < pdfCourseBudgetMin {
		budget = pdfCourseBudgetMin
	}
	if budget > pdfCourseBudgetMax {
		budget = pdfCourseBudgetMax
	}
	detCtx, detCancel := context.WithTimeout(context.Background(), budget)
	defer detCancel()

	pdfPruneOnce.Do(prunePDFCache)
	// 先上报一次「总数已确定」，前端才能显示 0/N 而不是光转圈
	if onProgress != nil {
		onProgress(CoursePDFProgress{Phase: PDFJobRendering, Total: len(tasks)})
	}
	global.LOG.Info("pdf.course.start",
		zap.String("pid", pid),
		zap.Int("chapters", len(tasks)),
		zap.Duration("budget", budget))

	// 3. 整门课复用一个 Chrome 实例，每章开一个新 tab。
	//
	// ⚠️ 必须**懒创建**：全命中缓存的导出（重复导出同一门课）一个 tab 都不需要，
	// 无条件启动 Chrome 会白付~1s 启动成本，更要命的是会和上一轮遗留的
	// 异步关tab 抢CPU/进程槽位，实测把「本应秒级」的缓存命中拖到 3 分钟。
	var chrome *pdfChrome
	var chromeOnce sync.Once
	getChrome := func() *pdfChrome {
		chromeOnce.Do(func() { chrome = newPdfChrome(detCtx) })
		return chrome
	}
	defer func() {
		if chrome != nil {
			// ⚠️ 必须异步：chromedp 的 cancel 在 Windows 上同步关闭整个浏览器
			// 实测要~170s，会把响应硬生生拖住（客户端看到的是「卡住不返回」）。
			// 合并结果已在上面拿到，这里没理由阻塞调用方。
			go chrome.close()
		}
	}()

	// 单请求并发上限。16 核机器上取 8（原为 6）：再往上要留意每 tab 的内存开销 ——
	// 实测 24 个 chrome 进程 + 内存占用 80% 时会出现资源争抢，耗时反而翻几倍。
	const maxConcurrency = 8
	sem := make(chan struct{}, maxConcurrency)
	tChapters := time.Now()

	type chapterPDF struct {
		index int
		title string
		bytes []byte
	}
	pdfChan := make(chan chapterPDF, len(tasks))
	var wg sync.WaitGroup
	var doneCount int64
	var cacheHits int64

	// processed = 已结束（无论成功 / 失败 / 放弃）的章节数，专供进度上报；
	// doneCount 保持原语义（仅统计成功），两者不要混用。
	var processed int64
	reportProgress := func(phase string) {
		if onProgress == nil {
			return
		}
		onProgress(CoursePDFProgress{
			Phase:     phase,
			Total:     len(tasks),
			Done:      int(atomic.LoadInt64(&processed)),
			CacheHits: int(atomic.LoadInt64(&cacheHits)),
		})
	}

	for i, t := range tasks {
		wg.Add(1)
		go func(idx int, t *model.Task) {
			defer wg.Done()
			// 无论成功、失败还是因预算超时放弃，都计入「已处理」并上报一次进度 ——
			// 少了这条，失败章节会让进度永远停在 Total 之前，前端看起来像卡死。
			defer func() {
				atomic.AddInt64(&processed, 1)
				reportProgress(PDFJobRendering)
			}()

			// 先过全局闸门，再抢本地信号量：两者都保证 Chrome tab 总数有界
			select {
			case chapterRenderSem <- struct{}{}:
				defer func() { <-chapterRenderSem }()
			case <-detCtx.Done():
				global.LOG.Warn("pdf.chapter.aborted: waiting for global semaphore",
					zap.String("taskId", t.TaskId))
				return
			}
			sem <- struct{}{}
			defer func() { <-sem }()

			// 每章独立 context，互不影响
			chapterCtx, chapterCancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer chapterCancel()

			tChapter := time.Now()

			var articleData geek.ArticleData
			if err := json.Unmarshal(t.Raw, &articleData); err != nil {
				global.LOG.Warn("unmarshal article data failed, skip", zap.String("taskId", t.TaskId))
				return
			}

			pdfBytes, fromCache, err := generateSinglePDFLazy(chapterCtx, getChrome, t.TaskId, t.TaskName, t.OtherId, articleData)
			if err != nil {
				global.LOG.Warn("generate chapter PDF failed, skip",
					zap.String("taskId", t.TaskId), zap.Error(err))
				return
			}
			if fromCache {
				atomic.AddInt64(&cacheHits, 1)
			}
			pdfChan <- chapterPDF{index: idx, title: t.TaskName, bytes: pdfBytes}
			global.LOG.Info("pdf.chapter.done",
				zap.String("taskId", t.TaskId),
				zap.Int64("done", atomic.AddInt64(&doneCount, 1)),
				zap.Int("total", len(tasks)),
				zap.Bool("fromCache", fromCache),
				zap.Int("pdfSize", len(pdfBytes)),
				// dTotal：本章从抢到信号量到产出 PDF 的总耗时。
				// 埋点目的：全缓存命中时 dRender 应为 0，若 dTotal 仍很大，
				// 说明瓶颈在「取数 + 建 HTML」而非渲染（见 generateSinglePDFLazy 的分段日志）。
				zap.Duration("dTotal", time.Since(tChapter)))
		}(i, t)
	}

	wg.Wait()
	close(pdfChan)

	// 4. 收集结果：**按索引回填**，并让 title 与 bytes 严格配对。
	//    旧实现把 nil 过滤掉后仍用同下标去取 taskNames，导致部分章节失败时
	//    合并出来的书签（pdfcpu 用文件名做书签名）会与正文错位。
	slots := make([]chapterPDF, len(tasks))
	for p := range pdfChan {
		slots[p.index] = p
	}
	filtered := make([]chapterPDF, 0, len(slots))
	for _, s := range slots {
		if len(s.bytes) > 0 {
			filtered = append(filtered, s)
		}
	}

	global.LOG.Info("pdf.course.rendered",
		zap.String("pid", pid),
		zap.Int("ok", len(filtered)),
		zap.Int("failed", len(tasks)-len(filtered)),
		zap.Int64("cacheHits", atomic.LoadInt64(&cacheHits)),
		// dChapters：全部章节处理完的墙钟耗时（并发 6）。与 dMerge 对比即可
		// 判断时间花在「准备/渲染」还是「合并」。
		zap.Duration("dChapters", time.Since(tChapters)),
		zap.Bool("budgetExceeded", detCtx.Err() != nil))

	if len(filtered) == 0 {
		if detCtx.Err() != nil {
			return nil, fmt.Errorf(
				"PDF 生成超时：%d 章全部未完成（预算 %s）。请重试，已渲染的章节会被缓存，重试会快很多",
				len(tasks), budget)
		}
		return nil, fmt.Errorf("failed to generate any chapter PDFs")
	}
	if detCtx.Err() != nil {
		// 部分成功：仍然把已生成的章节合并返回，好过整门课前功尽弃
		global.LOG.Warn("pdf.course.partial: budget exceeded, returning partial PDF",
			zap.String("pid", pid),
			zap.Int("missing", len(tasks)-len(filtered)))
	}

	// 5. 合并所有 PDF（使用脱离的 detCtx，避免 HTTP 请求 context 被 cancel 影响文件操作）
	if onProgress != nil {
		onProgress(CoursePDFProgress{
			Phase:     PDFJobMerging,
			Total:     len(tasks),
			Done:      len(filtered),
			CacheHits: int(atomic.LoadInt64(&cacheHits)),
		})
	}
	pdfs := make([][]byte, len(filtered))
	titles := make([]string, len(filtered))
	for i, f := range filtered {
		pdfs[i] = f.bytes
		titles[i] = f.title
	}
	tMerge := time.Now()
	merged, err := mergePDFs(detCtx, pdfs, titles)
	if err != nil {
		return nil, err
	}
	global.LOG.Info("pdf.course.merged",
		zap.String("pid", pid),
		zap.Int("chapters", len(filtered)),
		zap.Int("pdfBytes", len(merged)),
		zap.Duration("dMerge", time.Since(tMerge)))
	return merged, nil
}

// generateSinglePDFLazy 与 generateSinglePDF 等价，但把 Chrome 的获取推迟到
// **确实需要渲染时**（缓存未命中）。课程级导出全命中缓存时，一个Chrome 都不用启。
//
// 之所以不能直接在调用点写 generateSinglePDF(chapterCtx, getChrome(), ...)：
// Go 的实参在调用时求值，getChrome() 会在每章都执行，等于退化成无条件启动。
func generateSinglePDFLazy(ctx context.Context, getChrome func() *pdfChrome, taskId, fallbackTitle, otherId string, articleData geek.ArticleData) ([]byte, bool, error) {
	title := articleData.Info.Title
	if title == "" && fallbackTitle != "" {
		title = fallbackTitle
	}
	content := articleData.Info.Content
	if len(content) == 0 && len(articleData.Info.Cshort) > 0 {
		content = articleData.Info.Cshort
	}

	aid := otherId
	if aid == "" {
		aid = fmt.Sprintf("%d", articleData.Info.ID)
	}
	comments, err := fetchArticleComments(ctx, aid)
	if err != nil {
		global.LOG.Warn("fetch article comments failed", zap.Error(err))
		comments = nil
	}
	processedContent, err := HtmlURLProxyReplace(content)
	if err != nil {
		global.LOG.Warn("HtmlURLProxyReplace failed, using raw content", zap.Error(err))
		processedContent = content
	}

	data := PDFData{
		Title:    title,
		Content:  template.HTML(processedContent),
		Comments: comments,
	}
	tmpl := template.Must(template.New("pdf").Funcs(template.FuncMap{
		"urlProxy": URLProxyReplace,
	}).Parse(PdfHtmlTPL))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, false, fmt.Errorf("execute template failed: %w", err)
	}
	htmlBytes := buf.Bytes()

	fingerprint := htmlFingerprint(htmlBytes)
	if cached, ok := loadCachedPDF(taskId, fingerprint); ok {
		return cached, true, nil
	}

	// 只有走到这里才真的需要浏览器
	return renderAndCache(ctx, getChrome(), taskId, fingerprint, htmlBytes)
}

// generateSinglePDF 生成单章 PDF（供 GenerateArticlePDF 和 GenerateCoursePDF 复用）
// session 为 nil 时自行开一个一次性 Chrome 实例。
// fallbackTitle 用于 Raw JSON 中缺少 info.title 时的兜底（如缓存列表 API 创建的任务）
func generateSinglePDF(ctx context.Context, session *pdfChrome, taskId, fallbackTitle, otherId string, articleData geek.ArticleData) ([]byte, bool, error) {
	title := articleData.Info.Title
	if title == "" && fallbackTitle != "" {
		title = fallbackTitle
	}
	content := articleData.Info.Content
	if len(content) == 0 && len(articleData.Info.Cshort) > 0 {
		content = articleData.Info.Cshort
	}

	// 获取评论
	aid := otherId
	if aid == "" {
		aid = fmt.Sprintf("%d", articleData.Info.ID)
	}
	comments, err := fetchArticleComments(ctx, aid)
	if err != nil {
		global.LOG.Warn("fetch article comments failed", zap.Error(err))
		comments = nil
	}

	// 将文章内容中的图片/链接 URL 替换为本机代理 URL，避免 chromedp 渲染时受防盗链限制
	processedContent, err := HtmlURLProxyReplace(content)
	if err != nil {
		global.LOG.Warn("HtmlURLProxyReplace failed, using raw content", zap.Error(err))
		processedContent = content
	}

	data := PDFData{
		Title:    title,
		Content:  template.HTML(processedContent),
		Comments: comments,
	}

	tmpl := template.Must(template.New("pdf").Funcs(template.FuncMap{
		"urlProxy": URLProxyReplace,
	}).Parse(PdfHtmlTPL))

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, false, fmt.Errorf("execute template failed: %w", err)
	}
	htmlBytes := buf.Bytes()

	// 缓存判定放在模板渲染之后、Chrome 启动之前 —— 这是省时间的关键：
	// 命中时完全不碰浏览器，重复导出 32 章的课从~15 分钟降到秒级。
	fingerprint := htmlFingerprint(htmlBytes)
	if cached, ok := loadCachedPDF(taskId, fingerprint); ok {
		return cached, true, nil
	}

	if session == nil {
		one := newPdfChrome(ctx)
		// 异步关闭：同步 cancel 在 Windows 上会阻塞 ~170s（见GenerateCoursePDF 注释）
		defer func() { go one.close() }()
		session = one
	}
	return renderAndCache(ctx, session, taskId, fingerprint, htmlBytes)
}

// renderAndCache 真正用 Chrome 渲染并写入章节缓存。缓存未命中才会走到这里。
// singleflight 保证同一 taskId + 同一指纹的并发渲染只真正跑一次。
func renderAndCache(ctx context.Context, session *pdfChrome, taskId, fingerprint string, htmlBytes []byte) ([]byte, bool, error) {
	flightKey := taskId + ":" + fingerprint
	v, err, shared := chapterFlight.Do(flightKey, func() (any, error) {
		b, rerr := renderHTMLToPDF(session, htmlBytes)
		if rerr != nil {
			return nil, rerr
		}
		storeCachedPDF(taskId, fingerprint, b)
		return b, nil
	})
	if err != nil {
		return nil, false, err
	}
	if shared {
		global.LOG.Info("pdf.chapter.dedup: joined an in-flight render",
			zap.String("taskId", taskId))
	}
	return v.([]byte), false, nil
}

// renderHTMLToPDF 在给定 Chrome 会话上开一个新 tab，把 HTML 渲染为 PDF。
// 使用临时 HTTP server 提供 HTML，避免 Windows 上 file:// URL 路径解析问题。
func renderHTMLToPDF(session *pdfChrome, htmlBytes []byte) ([]byte, error) {
	tStart := time.Now()
	// 必须最先注册 ⇒ 最后才执行（LIFO）：这里能看到 tab 关闭等收尾开销。
	// 实测依据：曾用 `defer tabCancel()` 导致每章多等 ~70s（Windows 上关闭 CDP
	// target 很慢），改成异步关闭后 dFull 才回到与函数体同量级。
	defer func() {
		global.LOG.Info("pdf.render.done", zap.Duration("dFull", time.Since(tStart)))
	}()
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "geektime-pdf-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir failed: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// 写入 HTML 文件
	htmlFile := filepath.Join(tmpDir, "index.html")
	if err := os.WriteFile(htmlFile, htmlBytes, 0644); err != nil {
		return nil, fmt.Errorf("write temp file failed: %w", err)
	}

	// 创建临时 HTTP server 提供 HTML，并代理 /v2/file/proxy 请求到后端
	//（Transport 走包级单例，理由见 pdfProxyTransport 的注释）
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(tmpDir)))
	mux.HandleFunc("/v2/file/proxy", func(w http.ResponseWriter, r *http.Request) {
		proxy := &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.URL.Scheme = "http"
				req.URL.Host = fmt.Sprintf("127.0.0.1:%d", global.CONF.Server.HTTPPort)
				req.URL.Path = "/v2/file/proxy"
				req.URL.RawQuery = r.URL.RawQuery
				req.Header.Set("X-Forwarded-For", "")
			},
			Transport: pdfProxyTransport,
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				global.LOG.Warn("reverse proxy error", zap.Error(err))
				http.Error(w, "proxy error", 502)
			},
		}
		proxy.ServeHTTP(w, r)
	})

	server := &http.Server{Handler: mux}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen failed: %w", err)
	}
	server.Addr = lis.Addr().String()

	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Serve(lis)
	}()

	serverURL := "http://" + lis.Addr().String() + "/"

	// 关键复用点：以 session 的浏览器 context 为父，新建 tab context。
	// 旧实现在这里 chromedp.NewContext(Background()) ⇒ 每章都拉起一个 Chrome 进程。
	//
	// ⚠️ tabCancel 不能写成 `defer tabCancel()`：实测在 Windows 上关闭一个 tab 要
	// ~70s（等CDP target 销毁），defer 会让**每章**白等 70s ——
	// 32 章 ÷ 并发 6 ≈ 6 分钟纯等待，且这段等待完全在渲染完成之后。
	// 正确姿势：渲染全部做完后再异步关闭，且不在渲染期间触发 cancel。
	tabCtx, tabCancel := chromedp.NewContext(session.ctx)
	tabClosed := false
	closeTabAsync := func() {
		if tabClosed {
			return
		}
		tabClosed = true
		go tabCancel()
	}
	defer closeTabAsync()

	stopServer := func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
		// 非阻塞读取 errChan，避免 Shutdown 超时后死锁
		select {
		case <-errChan:
		default:
		}
	}

	// 导航到临时 server
	if err := chromedp.Run(tabCtx,
		chromedp.Navigate(serverURL),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		// 等待所有图片加载完成
		chromedp.Evaluate(`
			Promise.all(Array.from(document.querySelectorAll('img')).map(img =>
				img.complete ? Promise.resolve() : new Promise(resolve => {
					img.onload = resolve;
					img.onerror = resolve;
				})
			))
		`, &struct{}{}),
		chromedp.Sleep(500*time.Millisecond),
	); err != nil {
		stopServer()
		return nil, fmt.Errorf("chromedp navigate failed: %w", err)
	}

	// 使用 CDP page.printToPDF 生成 PDF
	var pdfBuf []byte
	if err := chromedp.Run(tabCtx,
		chromedp.Evaluate(`document.fonts.ready`, &struct{}{}),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			pdfBuf, _, err = page.PrintToPDF().
				WithDisplayHeaderFooter(false).
				WithMarginTop(1.0).
				WithMarginBottom(1.0).
				WithMarginLeft(0.6).
				WithMarginRight(0.6).
				Do(ctx)
			return err
		}),
	); err != nil {
		stopServer()
		return nil, fmt.Errorf("chromedp printToPDF failed: %w", err)
	}

	// PDF 生成完成后关闭临时 server
	stopServer()

	if len(pdfBuf) == 0 {
		global.LOG.Error("renderHTMLToPDF", zap.String("error", "chromedp generated empty PDF (0 bytes)"))
		return nil, fmt.Errorf("chromedp generated empty PDF (0 bytes)")
	}
	global.LOG.Info("renderHTMLToPDF", zap.Int("pdfSize", len(pdfBuf)))

	return pdfBuf, nil
}

// fetchArticleComments 获取文章评论及讨论
func fetchArticleComments(ctx context.Context, aidStr string) ([]PDFComment, error) {
	aidInt64, err := strconv.ParseInt(aidStr, 10, 64)
	if err != nil {
		// aid 是字符串形式的 UUID 或其他格式，跳过评论查询
		global.LOG.Info("aid is not a number, skipping comments", zap.String("aid", aidStr))
		return nil, nil
	}

	var comments []*model.ArticleComment
	if err := global.DB.WithContext(ctx).
		Where("aid = ?", aidInt64).
		Order("id desc").
		Find(&comments).Error; err != nil {
		return nil, fmt.Errorf("query comments failed: %w", err)
	}

	result := make([]PDFComment, 0, len(comments))
	for _, c := range comments {
		var row geek.ArticleComment
		if err := json.Unmarshal(c.Raw, &row); err != nil {
			continue
		}

		pc := PDFComment{
			UserHeader:      URLProxyReplace(row.UserHeader),
			UserName:        row.UserName,
			LikeCount:       row.LikeCount,
			DiscussionCount: row.DiscussionCount,
			Content:         utils.UnescapeComment(row.CommentContent),
			Time:            time.Unix(row.CommentCtime, 0).Format("2006-01-02"),
		}

		// 获取该评论的讨论
		if row.DiscussionCount > 0 {
			discussions, err := fetchDiscussions(ctx, row.ID)
			if err != nil {
				global.LOG.Warn("fetch discussions failed",
					zap.Int64("cid", row.ID), zap.Error(err))
			} else {
				pc.Discussions = discussions
			}
		}

		result = append(result, pc)
	}

	return result, nil
}

// fetchDiscussions 获取评论的讨论列表
func fetchDiscussions(ctx context.Context, cid int64) ([]PDFDiscussion, error) {
	var discussions []*model.ArticleCommentDiscussion
	if err := global.DB.WithContext(ctx).
		Where("cid = ?", cid).
		Order("likes_number desc, ctime desc").
		Find(&discussions).Error; err != nil {
		return nil, err
	}

	result := make([]PDFDiscussion, 0, len(discussions))
	for _, d := range discussions {
		var row geek.DiscussionData
		if err := json.Unmarshal(d.Raw, &row); err != nil {
			continue
		}

		pd := PDFDiscussion{
			Avatar:      URLProxyReplace(row.Author.Avatar),
			Nickname:    row.Author.Nickname,
			Content:     utils.UnescapeComment(row.Discussion.DiscussionContent),
			Time:        time.Unix(row.Discussion.Ctime, 0).Format("2006-01-02"),
			LikesNumber: row.Discussion.LikesNumber,
		}

		if row.ReplyAuthor.Nickname != "" {
			pd.ReplyNickname = row.ReplyAuthor.Nickname
		}

		if len(row.ChildDiscussions) > 0 {
			pd.ChildDiscussions = make([]PDFChildDiscussion, 0, len(row.ChildDiscussions))
			for _, child := range row.ChildDiscussions {
				pcd := PDFChildDiscussion{
					AuthorNickname: child.Author.Nickname,
					Content:        utils.UnescapeComment(child.Discussion.DiscussionContent),
				}
				if child.ReplyAuthor.Nickname != "" {
					pcd.ReplyNickname = child.ReplyAuthor.Nickname
				}
				pd.ChildDiscussions = append(pd.ChildDiscussions, pcd)
			}
		}

		result = append(result, pd)
	}

	return result, nil
}

// mergePDFs 使用 pdfcpu 合并多个 PDF 为一个
func mergePDFs(_ context.Context, pdfs [][]byte, titles []string) ([]byte, error) {
	if len(pdfs) == 1 {
		return pdfs[0], nil
	}

	// 创建临时目录存放各章 PDF
	tmpDir, err := os.MkdirTemp("", "geektime-course-pdf-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir failed: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tWrite := time.Now()
	tempFiles := make([]string, 0, len(pdfs))
	for i, pdf := range pdfs {
		title := titles[i]
		if title == "" {
			title = fmt.Sprintf("chapter_%03d", i+1)
		} else {
			title = VerifyFileName(title)
		}
		// 临时文件名去掉 .pdf 后缀，避免 pdfcpu 用 filepath.Base(fName) 作为书签名时带上 .pdf
		title = strings.TrimSuffix(title, ".pdf")
		tmpFile := filepath.Join(tmpDir, title)
		if err := os.WriteFile(tmpFile, pdf, 0644); err != nil {
			return nil, fmt.Errorf("write temp pdf %d failed: %w", i, err)
		}
		tempFiles = append(tempFiles, tmpFile)
	}
	dWrite := time.Since(tWrite)

	// 使用 pdfcpu 合并
	outputPath := filepath.Join(tmpDir, "merged.pdf")
	config := pdfcpuModel.NewDefaultConfiguration()
	tPdfcpu := time.Now()
	if err := api.MergeCreateFile(tempFiles, outputPath, false, config); err != nil {
		return nil, fmt.Errorf("merge pdfs failed: %w", err)
	}
	dPdfcpu := time.Since(tPdfcpu)

	tRead := time.Now()
	merged, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("read merged pdf failed: %w", err)
	}
	dRead := time.Since(tRead)

	// 显式清理并计时（defer 里的那句作为异常路径兜底，二次调用无害）。
	// 目的是验证「临时目录清理」是否被 Windows 的文件句柄/索引器拖住 ——
	// 离线基准（同样 14 个文件）总耗时仅 4.35s，而服务内 dMerge 实测 60~86s。
	tClean := time.Now()
	os.RemoveAll(tmpDir)
	dClean := time.Since(tClean)

	global.LOG.Info("pdf.merge.breakdown",
		zap.Int("chapters", len(pdfs)),
		zap.Int("bytes", len(merged)),
		zap.Duration("dWrite", dWrite),
		zap.Duration("dPdfcpu", dPdfcpu),
		zap.Duration("dRead", dRead),
		zap.Duration("dClean", dClean),
		zap.Duration("dTotal", dWrite+dPdfcpu+dRead+dClean))

	return merged, nil
}
