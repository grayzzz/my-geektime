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
