import { useCallback, useRef, useState } from 'react'
import {
  downloadPdfBlob,
  preparePdfExport,
  getPdfExportStatus,
  type PdfExportJob,
} from '@/api/task'
import { downloadFileFromBlob } from '@/utils/request'
import { useToast } from '@/components/ui/Toast'

/**
 * 课程级 PDF 导出队列。
 *
 * 为什么不用一个页面级 boolean 锁（旧实现的 bug）：
 * `const [pdfDownloading, setPdfDownloading] = useState(false)` 是**全页共享**的，
 * 于是点第二个课程时 `if (pdfDownloading) return` 会**静默丢弃**这次点击 ——
 * 不发请求、不弹 toast，用户看到的就是「点了几个，一个都没导出」。
 *
 * 本 hook 改为：
 *   1. per-task 状态（Set<pid>），只禁用正在导出/排队的那个按钮；
 *   2. 并发上限 MAX_CONCURRENT（避免同时开太多 Chrome 把机器打爆）；
 *   3. 超出上限的进等待队列，每个都有独立 toast（排队 → 生成 → 成功/失败），
 *      排队提示会在真正开始生成时被移除，不会永远挂在屏幕上。
 *
 * 2026-10-03 起改为**作业化**：
 * 不再用一个「同步等十几分钟」的长请求拉 PDF（实测 TTFB 330s、首字节为零，
 * 期间没有任何反馈，网络一抖就 net::ERR_FAILED），而是
 *   提交作业（幂等）→ 轮询进度（toast 上显示 12/44 章）→ 完成后取文件（命中缓存、秒回）。
 */
const MAX_CONCURRENT = 2

export interface PdfExportItem {
  pid: string
  taskName: string
}

interface QueueEntry {
  item: PdfExportItem
  /** 排队提示的 toast id；开始生成时移除。null 表示不是排队进来的 */
  queueToastId: string | null
}

/** 文件名清洗：与后端 VerifyFileName 保持一致，避免 Windows 非法字符 */
const sanitizeFileName = (name: string) =>
  name
    .replace(/"/g, '-')                          // " → -
    .replace(/\|/g, '-')                         // | → -
    .replace(/｜/g, '-')                         // 全角 ｜ → -
    .replace(/:/g, '：')                         // : → ：
    .replace(/”/g, '“')                          // " → “
    .replace(/\?/g, '？')                        // ? → ？
    .replace(/&/g, '+')                          // & → +
    .replace(/\t/g, '')                          // tab → 空
    .replace(/ /g, '')                           // 空格 → 空
    .trim()

/** 轮询间隔：作业状态是内存读取，1.5s 足够顺滑又不至于刷爆后端 */
const POLL_INTERVAL = 1500
/** 轮询总上限：比后端单次作业预算（最长 55 分钟）留足余量 */
const POLL_TIMEOUT = 70 * 60 * 1000

const sleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms))

/** 轮询/下载请求的网络容错。
 *
 * 2026-10-03 实测：13 分钟的导出期间，某一次 status 轮询在连接层被重置
 * （axios "Network Error"；vite 与后端日志均无该请求 —— 连接没到 handler），
 * 而后端作业跑在 context.Background() 上照常完成并落盘。
 * 一次瞬时抖动就判死整个导出是错误的 ⇒ 把「失败」收紧为「同一条请求
 * 连续 MAX_REQUEST_ATTEMPTS 次都失败」。作业化轮询是幂等读，重试无副作用。
 */
const MAX_REQUEST_ATTEMPTS = 10
const REQUEST_RETRY_BACKOFF = 3000

const resilient = async <T>(fn: () => Promise<T>): Promise<T> => {
  for (let attempt = 1; ; attempt++) {
    try {
      return await fn()
    } catch (err) {
      if (attempt >= MAX_REQUEST_ATTEMPTS) throw err
      console.warn(
        `[pdf-export] 请求失败，${REQUEST_RETRY_BACKOFF / 1000}s 后重试（${attempt}/${MAX_REQUEST_ATTEMPTS}）`,
        err
      )
      await sleep(REQUEST_RETRY_BACKOFF)
    }
  }
}

/** 把作业状态翻译成一句人话，直接显示在 toast 上 */
const describePhase = (job: PdfExportJob): string => {
  switch (job.status) {
    case 'none':
      return '重新提交任务...'
    case 'pending':
      return '任务已受理...'
    case 'rendering':
      return job.total > 0 ? `渲染中 ${job.done}/${job.total} 章` : '渲染中...'
    case 'merging':
      return `正在合并 ${job.total} 章 PDF...`
    case 'done':
      return '准备下载...'
    default:
      return '处理中...'
  }
}

/**
 * 提交作业并轮询到完成。
 *
 * 幂等性依赖后端：同 pid 已有进行中的作业会直接复用（重复点击不会重复渲染）。
 * 若状态变成 none（后端重启导致内存状态丢失），这里会自动重新提交一次 ——
 * 章节 PDF 都在磁盘缓存里，重新提交不会白跑。
 */
const pollPdfExportJob = async (
  pid: string,
  onProgress?: (job: PdfExportJob) => void
): Promise<PdfExportJob> => {
  const deadline = Date.now() + POLL_TIMEOUT
  let job = await resilient(() => preparePdfExport(pid))
  onProgress?.(job)

  while (job.status !== 'done') {
    if (job.status === 'failed') {
      throw new Error(job.error || '服务端渲染失败')
    }
    if (Date.now() > deadline) {
      throw new Error('导出超时（已等待超过 70 分钟），请稍后重试 —— 已渲染的章节会被缓存，重试会快很多')
    }
    await sleep(POLL_INTERVAL)
    job = await resilient(() => getPdfExportStatus(pid))
    if (job.status === 'none') {
      // 后端重启等导致内存作业表丢失：重新提交即可（章节/课程 PDF 都在磁盘，命中缓存秒回）
      job = await resilient(() => preparePdfExport(pid))
    }
    onProgress?.(job)
  }
  return job
}

export const usePdfExport = () => {
  const { addToast, removeToast, updateToast } = useToast()

  // 正在导出 + 排队中的 pid 集合（用于按钮禁用态）
  const [pendingPids, setPendingPids] = useState<Set<string>>(new Set())
  const pendingRef = useRef<Set<string>>(new Set())

  const runningRef = useRef(0)
  const queueRef = useRef<QueueEntry[]>([])
  // processQueue 自身在 finally 里递归调用，用 ref 打破闭包循环依赖
  const processRef = useRef<() => void>(() => {})

  const setPending = useCallback((pid: string, on: boolean) => {
    const next = new Set(pendingRef.current)
    if (on) next.add(pid)
    else next.delete(pid)
    pendingRef.current = next
    setPendingPids(next)
  }, [])

  const startOne = useCallback((entry: QueueEntry) => {
    const { item, queueToastId } = entry
    const { pid, taskName } = item

    if (queueToastId) {
      removeToast(queueToastId)
    }

    const safeName = sanitizeFileName(taskName) || pid
    const label = safeName.length > 20 ? `${safeName.slice(0, 20)}…` : safeName

    // 一个 toast 从头用到尾，进度靠 updateToast 原地改写 ——
    // 不闪、不重放动画、不重置计时器。
    const toastId = addToast(`正在生成《${label}》PDF：提交任务...`, 'info', Infinity)

    pollPdfExportJob(pid, (job) => {
      updateToast(toastId, `正在生成《${label}》PDF：${describePhase(job)}`)
    })
      // 作业 done 之后才取文件：此时命中课程级磁盘缓存，是秒回而不是十几分钟。
      // 下载同样套 resilient：85MB 经 vite 代理转发，一次连接抖动不该废掉整个导出
      //（缓存命中 ⇒ 重试成本极低；axios 拿不到完整 blob 就不会触发浏览器下载，重试安全）
      .then(() => resilient(() => downloadPdfBlob({ pid })))
      .then((blob) => {
        downloadFileFromBlob(blob, `${safeName}.pdf`)
        removeToast(toastId)
        addToast(`《${label}》PDF 导出成功`, 'success')
      })
      .catch((error) => {
        console.error('Failed to export PDF', error)
        removeToast(toastId)
        addToast(`《${label}》PDF 导出失败：${error?.message || '未知错误'}`, 'error', 8000)
      })
      .finally(() => {
        setPending(pid, false)
        runningRef.current -= 1
        processRef.current()
      })
  }, [addToast, removeToast, updateToast, setPending])

  // 从等待队列里尽量填满并发额度
  const processQueue = useCallback(() => {
    while (runningRef.current < MAX_CONCURRENT && queueRef.current.length > 0) {
      const entry = queueRef.current.shift()!
      runningRef.current += 1
      startOne(entry)
    }
  }, [startOne])

  processRef.current = processQueue

  const enqueue = useCallback((pid: string, taskName: string) => {
    if (pendingRef.current.has(pid)) {
      addToast('该课程已在导出队列中', 'warning')
      return
    }
    setPending(pid, true)

    if (runningRef.current < MAX_CONCURRENT) {
      runningRef.current += 1
      startOne({ item: { pid, taskName }, queueToastId: null })
    } else {
      const queueToastId = addToast(
        `并发上限 ${MAX_CONCURRENT}，《${taskName.slice(0, 18)}》已排队`,
        'info',
        Infinity
      )
      queueRef.current.push({ item: { pid, taskName }, queueToastId })
    }
  }, [addToast, setPending, startOne])

  return {
    /** 该 pid 是否正在导出或排队中（用于按钮禁用态） */
    isPdfExporting: useCallback((pid: string) => pendingPids.has(pid), [pendingPids]),
    /** 是否存在任意进行中的导出 */
    hasPending: pendingPids.size > 0,
    enqueuePdfExport: enqueue,
  }
}
