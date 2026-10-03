import request, { longRequest, TIMEOUT } from '@/utils/request'

/**
 * 二进制下载的内容守卫。
 *
 * 为什么必须校验：`responseType: 'blob'` 会在响应拦截器里提前 return，
 * 跳过 `res.status !== 0` 的业务错误检查（见 @/utils/request 的 applyInterceptors）。
 * 所以后端出错走 global.FAIL 返回 JSON 错误体 `{"status":...}` 时，
 * axios 视为成功、blob 里装的是这段文本 —— 不校验就会被存成 .pdf / .epub
 * 并提示「导出成功」，是极难察觉的静默失败。
 *
 * size === 0 挡不住这种情况（错误体有几十字节），必须校验魔数。
 *
 * @param magic 文件魔数，如 PDF 的 `%PDF`、epub 的 `PK`
 */
async function guardBlob(blob: Blob | null | undefined, magic: number[], label: string): Promise<Blob> {
  if (!blob || blob.size === 0) {
    throw new Error(`${label}导出失败：服务端返回了空响应，请确认后端已重新编译并重启`)
  }
  const head = new Uint8Array(await blob.slice(0, magic.length).arrayBuffer())
  if (!magic.every((b, i) => head[i] === b)) {
    throw new Error(`${label}导出失败：服务端返回的内容不是有效的 ${label.toUpperCase()} 文件`)
  }
  return blob
}

export interface TaskListParams {
  page?: number
  perPage?: number
  direction?: number
  tag?: number
  product_type?: number
  product_form?: number
  xstatus?: number
  keywords?: string
}

export interface TaskItem {
  task_id: string
  task_pid?: string
  task_name: string
  subtitle: string
  cover: string
  author: {
    name: string
    intro: string
  }
  is_audio?: boolean
  is_video?: boolean
  is_finish: boolean
  sale_type: number
  sale: number
  other_type: number
  other_group: number
  status: number
  article: {
    count: number
  }
  statistics: {
    items: Record<number, number>
  }
  doc?: string
  redirect?: string
  dir?: string
  is_collected?: boolean
  course_progress?: {
    finished_count: number
    total_count: number
    percent: number
    last_task_id: string
    last_task_name: string
    updated_at: number
  }
}
export const getTaskList = (params?: TaskListParams) => {
  return request.get<any, { rows: TaskItem[]; count: number }>('/task/list', { params })
}

export const deleteTask = (ids: string[]) => {
  return request.delete('/task/delete', { data: { ids } })
}

export const retryTask = (params: { pid?: string; ids?: string[]; retry?: boolean }) => {
  return request.post('/task/retry', params)
}

export const exportTask = (params: { pid: string; type: string; comments?: string }) => {
  // epub 与 markdown 一样返回二进制流，但生成过程需要下载并内嵌图片，耗时明显更长。
  // 两者都属于长耗时导出，统一走 longRequest（超时预算见 @/utils/request 的 TIMEOUT）
  if (params.type === 'epub') {
    return longRequest
      .get<any, Blob>('/task/export', {
        params,
        responseType: 'blob',
        timeout: TIMEOUT.epub,
      })
      .then((blob) => guardBlob(blob, [0x50, 0x4b], 'epub'))
  }
  if (params.type === 'markdown') {
    return longRequest.get<any, Blob>('/task/export', { params, responseType: 'blob' })
  }
  return request.get('/task/export', { params })
}

export interface TaskInfo {
  task_id: string
  task_pid?: string
  other_id?: string
  task_name: string
  task_type?: string
  other_type?: number
  other_tag?: number
  other_form?: number
  other_group?: number
  cover: string
  status?: number
  statistics?: {
    count: number
    items: Record<number, number>
  }
  subtitle?: string
  intro_html?: string
  dir?: string
  doc?: string
  object?: string
  is_video?: boolean
  is_audio?: boolean
  is_finish?: boolean
  sale?: number
  sale_type?: number
  share?: any
  author?: {
    name: string
    intro: string
    avatar?: string
    brief_html?: string
    brief?: string
  }
  article?: {
    id?: number
    other_id?: string
    title?: string
    summary?: string
    content?: string
    count?: number
    count_req?: number
    count_pub?: number
    total_length?: number
    cover?: {
      default?: string
      square?: string
    }
    video?: {
      hls_medias?: { url: string }[]
      cover?: string
    }
    video_preview?: {
      medias?: { url: string }[]
    }
    audio?: {
      url?: string
      download_url?: string
    }
  }
  redirect?: string
}

export interface TaskInfoResponse {
  task: TaskInfo
  article?: any
  message?: any
  play_url?: string
}

export const getTaskInfo = (id: string) => {
  return request.get<any, TaskInfoResponse>('/task/info', { params: { id } })
}

export const getArticleComments = (params: { aid: string; page?: number; perPage?: number }) => {
  return request.get('/task/article/comments', { params })
}

export const downloadPdfBlob = (params: { id?: string; pid?: string }) => {
  return longRequest
    .get<any, Blob>('/task/download', {
      params: { ...params, type: 'pdf' },
      responseType: 'blob',
      // ⚠️ 必须大于后端 middleware.PDFTimeout 的 60min（见 internal/middleware/timeout.go）。
      // 之前这里是 600000（10min），远小于后端 20min，导致大课程必然前端先超时，
      // 而后端还在闷头渲染 —— 两边时间对不上是这类"导不出"问题的主要根因。
      //
      // 注：自作业化改造后，走这个接口时课程 PDF 通常已在磁盘缓存里（作业已完成），
      // 它是**秒回**的；这里的 65min 只作为极端情况下的兜底。
      timeout: TIMEOUT.pdf,
    })
    // %PDF 魔数（0x25 0x50 0x44 0x46）。课程 PDF 可达 99MB，
    // 后端出错时这里会拿到 JSON 错误体的 blob，必须拦掉，
    // 否则会被存成 .pdf 并提示「导出成功」。单章 LessonDetail 也走这个函数。
    .then((blob) => guardBlob(blob, [0x25, 0x50, 0x44, 0x46], 'pdf'))
}

/**
 * 课程 PDF 导出作业状态，对应后端 service.PDFJob。
 *
 * status 语义：
 *   none      —— 后端没有这个 pid 的作业记录（例如后端重启过），应重新提交
 *   pending   —— 已受理，尚未开始逐章渲染
 *   rendering —— 逐章渲染中（done/total 有进度）
 *   merging   —— 全部章节就绪，正在合并成单个 PDF
 *   done      —— 产物就绪，此时调 downloadPdfBlob 即可拿到文件（命中缓存、秒回）
 *   failed    —— 失败，原因见 error
 */
export interface PdfExportJob {
  pid: string
  status: 'none' | 'pending' | 'rendering' | 'merging' | 'done' | 'failed'
  total: number
  done: number
  cacheHits: number
  bytes: number
  error?: string
  startedAt: number
  updatedAt: number
}

/**
 * 提交课程 PDF 导出作业（异步，立即返回）。
 *
 * 幂等：同一 pid 已有进行中的作业会直接复用，重复点击不会重复渲染。
 * 之所以不像以前那样「一个请求同步等到 PDF 生成」——冷导出要 5~15 分钟，
 * 同步请求会让浏览器零字节干等十几分钟（实测 TTFB 330s），既无进度反馈，
 * 又容易被网络层中途重置成 net::ERR_FAILED。
 */
export const preparePdfExport = (pid: string) =>
  request.post<any, PdfExportJob>('/task/download/prepare', { pid })

/** 查询课程 PDF 导出作业进度（快接口，供轮询） */
export const getPdfExportStatus = (pid: string) =>
  request.get<any, PdfExportJob>('/task/download/status', { params: { pid } })

export const getCommentDiscussions = (params: {
  target_id: string
  target_type: number
  page?: number
  perPage?: number
  use_likes_order?: boolean
}) => {
  return request.get('/task/article/discussions', { params })
}
