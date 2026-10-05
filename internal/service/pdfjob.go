package service

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/zkep/my-geektime/internal/global"
	"go.uber.org/zap"
)

// ============================================================================
// 课程 PDF 导出作业化
//
// 背景（2026-10-03）：一门 44 章的课冷导出要 5~15 分钟。同步 HTTP 请求会让
// 客户端「零字节干等」十几分钟（实测 TTFB 330s），期间任何网络抖动都表现为
// 失败（net::ERR_FAILED），而且用户完全看不出进度。
//
// 改造后：提交作业 → 轮询进度 → 完成后下载（命中课程 PDF 磁盘缓存）。
//
// 设计取舍：
//   - **以 pid 为键**，不生成 jobId：同一课程重复点击天然幂等（复用同一作业），
//     也省掉一份 pid→jobId 的映射。
//   - 状态只存内存，进程重启即丢。这不是问题：章节/课程 PDF 都已落盘，
//     重启后重新提交会命中缓存、直接变 done。
//   - 同一 pid 同时只会有一个作业在跑，不会重复渲染。
// ============================================================================

const (
	PDFJobPending   = "pending"
	PDFJobRendering = "rendering"
	PDFJobMerging   = "merging"
	PDFJobDone      = "done"
	PDFJobFailed    = "failed"
)

// pdfJobTTL 是作业结束后状态在内存里的保留时长。缓存文件仍在磁盘上，
// 回收状态只是让前端下次重新提交（会命中缓存、秒回）。
const pdfJobTTL = 30 * time.Minute

// courseJobSem 课程级导出的**全局**并发闸门，容量 1（即串行）。
//
// 为什么必须串行（2026-10-03 实测踩坑）：
//
// 前端 usePdfExport 的 MAX_CONCURRENT=2 只在**单个标签页内**限流，后端此前对
// 不同 pid 没有任何并发约束（SubmitCoursePDFJob 只按 pid 去重，不限同时作业数）。
// 于是「点两门课」会真的并发跑两个 GenerateCoursePDFToFileWithProgress，而
// 每个作业各自 newPdfChrome 建一个 Chrome、结束时 `go chrome.close()` 关掉整个浏览器。
// 两门课互相拖慢到 5~10 倍，实测同一时段：
//
//	课A（25章）渲染 186s → 合并 dMerge 597s（正常 3.8s）
//	课B（30章）渲染 1072s（正常 186s），单章 dTotal 高达 802s（正常 8~35s）
//
// 根因是资源争抢而非代码错误（章节全部 ok、failed=0），表现为前端
// 「导出超时」这种极难定位的失败。
//
// ⚠️ 这里用容量 1 而不是 2：chapterRenderSem 只包住**单章渲染**，合并阶段完全不占闸门，
// 所以「两个作业并发」时合并与渲染会交叉，再叠加各自 Chrome 的异步 close ⇒ 实测就是上面的数据。
// 串行化后一次只跑一门课，代价是排队（前端会显示「已排队」并给出进度反馈）。
var courseJobSem = make(chan struct{}, 1)

// PDFJob 是一次课程 PDF 导出作业的状态快照（直接 JSON 给前端）。
type PDFJob struct {
	Pid       string `json:"pid"`
	Status    string `json:"status"`
	Total     int    `json:"total"`
	Done      int    `json:"done"`
	CacheHits int    `json:"cacheHits"`
	Bytes     int64  `json:"bytes"`
	Err       string `json:"error,omitempty"`
	StartedAt int64  `json:"startedAt"`
	UpdatedAt int64  `json:"updatedAt"`

	// 产物路径，不对外暴露（前端不需要也不该知道本机路径）
	path string
}

type pdfJobStore struct {
	mu   sync.Mutex
	jobs map[string]*PDFJob
}

var coursePDFJobs = &pdfJobStore{jobs: make(map[string]*PDFJob)}

// SubmitCoursePDFJob 提交（或复用）一门课程的 PDF 导出作业，立即返回状态快照。
//
// 复用规则：
//   - 同 pid 有进行中的作业 ⇒ 直接返回它（重复点击不会重复渲染）；
//   - 同 pid 已完成且产物文件仍在 ⇒ 直接返回 done；
//   - 其它情况（失败 / 无记录 / 产物已被清理）⇒ 新建并异步执行。
func SubmitCoursePDFJob(pid string) PDFJob {
	coursePDFJobs.mu.Lock()
	if job, ok := coursePDFJobs.jobs[pid]; ok {
		switch job.Status {
		case PDFJobPending, PDFJobRendering, PDFJobMerging:
			snap := *job
			coursePDFJobs.mu.Unlock()
			return snap
		case PDFJobDone:
			if fi, err := os.Stat(job.path); err == nil && fi.Size() > 0 {
				snap := *job
				coursePDFJobs.mu.Unlock()
				return snap
			}
		}
	}
	now := time.Now().UnixMilli()
	job := &PDFJob{Pid: pid, Status: PDFJobPending, StartedAt: now, UpdatedAt: now}
	coursePDFJobs.jobs[pid] = job
	snap := *job
	coursePDFJobs.mu.Unlock()

	go runCoursePDFJob(pid)
	return snap
}

// GetCoursePDFJob 读取作业状态；没有记录时返回 ok=false。
func GetCoursePDFJob(pid string) (PDFJob, bool) {
	coursePDFJobs.mu.Lock()
	defer coursePDFJobs.mu.Unlock()
	job, ok := coursePDFJobs.jobs[pid]
	if !ok {
		return PDFJob{}, false
	}
	return *job, true
}

// runCoursePDFJob 在后台执行渲染。
//
// 用 context.Background() 而不是请求 context：HTTP 响应早就返回了，
// 作业不该被任何请求生命周期（客户端断开、超时中间件）影响。
func runCoursePDFJob(pid string) {
	tStart := time.Now()

	// 全局串行闸门（见 courseJobSem 的注释）：同一时刻只跑一门课。
	// 抢不到就阻塞在这里，作业状态保持 pending，前端会显示「任务已受理」并继续轮询 ——
	// 这是期望行为：排队信息通过 toast 的进度文案体现，不会误报失败。
	courseJobSem <- struct{}{}
	defer func() { <-courseJobSem }()

	// 抢到闸门后作业可能已被 reap 或被替换（理论上不会，见 SubmitCoursePDFJob），
	// 这里重新确认记录仍在，避免 update 静默丢弃进度。
	if _, ok := GetCoursePDFJob(pid); !ok {
		global.LOG.Warn("pdf.job.vanished", zap.String("pid", pid))
		return
	}

	path, err := GenerateCoursePDFToFileWithProgress(context.Background(), pid,
		func(p CoursePDFProgress) {
			coursePDFJobs.update(pid, func(j *PDFJob) {
				if p.Phase != "" {
					j.Status = p.Phase
				}
				if p.Total > 0 {
					j.Total = p.Total
				}
				j.Done = p.Done
				j.CacheHits = p.CacheHits
			})
		})

	coursePDFJobs.update(pid, func(j *PDFJob) {
		if err != nil {
			j.Status = PDFJobFailed
			j.Err = err.Error()
			return
		}
		fi, statErr := os.Stat(path)
		if statErr != nil || fi.Size() == 0 {
			j.Status = PDFJobFailed
			j.Err = "生成的 PDF 文件不存在或为空"
			return
		}
		j.Status = PDFJobDone
		j.path = path
		j.Bytes = fi.Size()
		if j.Total > 0 {
			j.Done = j.Total
		}
	})

	snap, _ := GetCoursePDFJob(pid)
	if snap.Status == PDFJobFailed {
		global.LOG.Error("pdf.job.failed",
			zap.String("pid", pid),
			zap.String("err", snap.Err),
			zap.Duration("dTotal", time.Since(tStart)))
	} else {
		global.LOG.Info("pdf.job.done",
			zap.String("pid", pid),
			zap.Int("chapters", snap.Total),
			zap.Int("cacheHits", snap.CacheHits),
			zap.Int64("pdfBytes", snap.Bytes),
			zap.Duration("dTotal", time.Since(tStart)))
	}

	time.AfterFunc(pdfJobTTL, func() { coursePDFJobs.reap(pid) })
}

func (s *pdfJobStore) update(pid string, fn func(*PDFJob)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[pid]
	if !ok {
		return
	}
	fn(job)
	job.UpdatedAt = time.Now().UnixMilli()
}

// reap 回收已结束作业的状态。若作业已被重新提交（状态回到进行中）则不动它。
func (s *pdfJobStore) reap(pid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[pid]
	if !ok {
		return
	}
	if job.Status == PDFJobDone || job.Status == PDFJobFailed {
		delete(s.jobs, pid)
	}
}
