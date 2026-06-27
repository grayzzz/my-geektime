package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
	Content        string `json:"discussion_content,omitempty"`
}

// PDFData PDF 生成数据
type PDFData struct {
	Title    string
	Content  template.HTML
	Comments []PDFComment
}

// GenerateArticlePDF 生成单篇文章 PDF
func GenerateArticlePDF(ctx context.Context, taskId string) ([]byte, error) {
	// 脱离 HTTP 请求上下文，使用独立超时避免被客户端断开影响
	detCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
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
	pdfBytes, err := generateSinglePDF(detCtx, l.TaskId, l.TaskName, l.OtherId, articleData)
	if err != nil {
		return nil, err
	}
	global.LOG.Info("GenerateArticlePDF", zap.String("taskId", taskId), zap.Int("pdfSize", len(pdfBytes)))
	return pdfBytes, nil
}

// fetchArticleComments 获取文章评论及讨论
func fetchArticleComments(ctx context.Context, aidStr string) ([]PDFComment, error) {
	var aidInt64 int64
	var err error

	// 尝试解析 aid 为 int64（Point 1: 用 strconv.ParseInt 替代 fmt.Sscanf）
	aidInt64, err = strconv.ParseInt(aidStr, 10, 64)
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

// renderHTMLToPDF 使用 chromedp 将 HTML 渲染为 PDF
// 使用临时 HTTP server 提供 HTML，避免 Windows 上 file:// URL 路径解析问题
func renderHTMLToPDF(ctx context.Context, htmlBytes []byte) ([]byte, error) {
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
	addr := "127.0.0.1:0" // 随机端口
	urlPath := "/"

	proxyTransport := &http.Transport{}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(tmpDir)))
	mux.HandleFunc("/v2/file/proxy", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/v2/file/proxy"
		r.Host = "127.0.0.1:" + strconv.FormatInt(int64(global.CONF.Server.HTTPPort), 10)
		proxy := &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.URL.Scheme = "http"
				req.URL.Host = fmt.Sprintf("127.0.0.1:%d", global.CONF.Server.HTTPPort)
				req.URL.Path = "/v2/file/proxy"
				req.URL.RawQuery = r.URL.RawQuery
				req.Header.Set("X-Forwarded-For", "")
			},
			Transport: proxyTransport,
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				global.LOG.Warn("reverse proxy error", zap.Error(err))
				http.Error(w, "proxy error", 502)
			},
		}
		proxy.ServeHTTP(w, r)
	})

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen failed: %w", err)
	}
	server.Addr = lis.Addr().String()

	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Serve(lis)
	}()

	// 获取 server 实际监听的 URL
	serverURL := "http://" + lis.Addr().String() + urlPath

	// 脱离可能已被 cancel 的父 context，使用独立的 chromedp context
	// 如果父 context 仍有效，则用 WithoutCancel 剥离取消信号；否则直接用 Background
	var chromedpParent context.Context
	if ctx.Err() != nil {
		chromedpParent = context.Background()
	} else {
		chromedpParent = context.WithoutCancel(ctx)
	}
	chromeCtx, chromeCancel := chromedp.NewContext(chromedpParent)
	defer chromeCancel()

	// 导航到临时 server
	if err := chromedp.Run(chromeCtx,
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
		_ = server.Shutdown(context.Background())
		select {
		case <-errChan:
		default:
		}
		return nil, fmt.Errorf("chromedp navigate failed: %w", err)
	}

	// 使用 CDP page.printToPDF 生成 PDF
	var pdfBuf []byte
	if err := chromedp.Run(chromeCtx,
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
		_ = server.Shutdown(context.Background())
		select {
		case <-errChan:
		default:
		}
		return nil, fmt.Errorf("chromedp printToPDF failed: %w", err)
	}

	// PDF 生成完成后关闭临时 server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	// 非阻塞读取 errChan，避免 Shutdown 超时后死锁
	select {
	case <-errChan:
	default:
	}

	if len(pdfBuf) == 0 {
		global.LOG.Error("renderHTMLToPDF", zap.String("error", "chromedp generated empty PDF (0 bytes)"))
		return nil, fmt.Errorf("chromedp generated empty PDF (0 bytes)")
	}
	global.LOG.Info("renderHTMLToPDF", zap.Int("pdfSize", len(pdfBuf)))

	return pdfBuf, nil
}

// GenerateCoursePDF 生成整门课程 PDF（所有章节合并为一个 PDF）
func GenerateCoursePDF(ctx context.Context, pid string) ([]byte, error) {
	// 脱离 HTTP 请求上下文，避免客户端断开影响 PDF 生成
	detCtx, detCancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer detCancel()

	// 1. 查询课程下所有章节任务
	var tasks []*model.Task
	if err := global.DB.WithContext(detCtx).
		Where("task_pid = ? AND task_type = ? AND deleted_at = 0", pid, TASK_TYPE_ARTICLE).
		Order("id asc").
		Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("query chapter tasks failed: %w", err)
	}

	if len(tasks) == 0 {
		return nil, fmt.Errorf("no chapter tasks found for course %s", pid)
	}

	// 预先收集章节标题（按 id 排序后的顺序）
	taskNames := make([]string, len(tasks))
	for i, t := range tasks {
		taskNames[i] = t.TaskName
	}

	// 2. 并发逐章生成 PDF（限制最大并发数为 3）
	const maxConcurrency = 3
	sem := make(chan struct{}, maxConcurrency)
	type chapterPDF struct {
		index int
		title string
		bytes []byte
	}
	pdfChan := make(chan chapterPDF, len(tasks))
	var wg sync.WaitGroup

	for i, t := range tasks {
		wg.Add(1)
		go func(idx int, t *model.Task) {
			defer wg.Done()
			sem <- struct{}{} // acquire
			defer func() { <-sem }() // release

			// 每个 goroutine 使用独立 context，互不影响
			chapterCtx, chapterCancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer chapterCancel()

			var articleData geek.ArticleData
			if err := json.Unmarshal(t.Raw, &articleData); err != nil {
				global.LOG.Warn("unmarshal article data failed, skip", zap.String("taskId", t.TaskId))
				return
			}

			pdfBytes, err := generateSinglePDF(chapterCtx, t.TaskId, t.TaskName, t.OtherId, articleData)
			if err != nil {
				global.LOG.Warn("generate chapter PDF failed, skip", zap.String("taskId", t.TaskId), zap.Error(err))
				return
			}
			pdfChan <- chapterPDF{index: idx, title: t.TaskName, bytes: pdfBytes}
		}(i, t)
	}

	wg.Wait()
	close(pdfChan)

	// 收集结果：预分配固定大小切片，按索引直接赋值
	pdfs := make([][]byte, len(tasks))
	for p := range pdfChan {
		pdfs[p.index] = p.bytes
	}
	// 过滤失败的章节（nil）
	filtered := make([][]byte, 0, len(pdfs))
	for _, p := range pdfs {
		if p != nil {
			filtered = append(filtered, p)
		}
	}
	pdfs = filtered

	if len(pdfs) == 0 {
		return nil, fmt.Errorf("failed to generate any chapter PDFs")
	}

	// 3. 合并所有 PDF（使用脱离的 detCtx，避免 HTTP 请求 context 被 cancel 影响文件操作）
	return mergePDFs(detCtx, pdfs, taskNames)
}

// generateSinglePDF 生成单章 PDF（供 GenerateArticlePDF 和 GenerateCoursePDF 复用）
// fallbackTitle 用于 Raw JSON 中缺少 info.title 时的兜底（如缓存列表 API 创建的任务）
func generateSinglePDF(ctx context.Context, taskId, fallbackTitle, otherId string, articleData geek.ArticleData) ([]byte, error) {
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
		return nil, fmt.Errorf("execute template failed: %w", err)
	}

	return renderHTMLToPDF(ctx, buf.Bytes())
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

	// 使用 pdfcpu 合并
	outputPath := filepath.Join(tmpDir, "merged.pdf")
	config := pdfcpuModel.NewDefaultConfiguration()
	if err := api.MergeCreateFile(tempFiles, outputPath, false, config); err != nil {
		return nil, fmt.Errorf("merge pdfs failed: %w", err)
	}

	merged, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("read merged pdf failed: %w", err)
	}

	return merged, nil
}
