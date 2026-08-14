import request from '@/utils/request'

export interface SaveProgressParams {
  task_id: string
  task_pid?: string
  task_type?: string
  position?: number
  scroll_position?: number
  duration?: number
  is_finished?: boolean
  /** 显式请求重置完成标记（仅管理端/修复场景使用，普通阅读上报不要传） */
  reset_progress?: boolean
}

export interface ProgressItem {
  id?: number
  uid?: string
  task_id: string
  task_pid?: string
  task_type?: string
  position?: number
  scroll_position?: number
  duration?: number
  is_finished?: boolean
  created_at?: number
  updated_at?: number
}

export interface CourseProgress {
  finished_count: number
  total_count: number
  percent: number
  last_task_id: string
  last_task_name: string
  updated_at: number
}

export interface MyProgressParams {
  page?: number
  perPage?: number
}

export interface MyProgressResponse {
  count: number
  rows: ProgressItem[]
}

export const saveProgress = (params: SaveProgressParams) => {
  return request.post('/progress/save', params)
}

export const getProgress = (taskId: string) => {
  return request.get<any, ProgressItem>('/progress/get', { params: { task_id: taskId } })
}

export const getCourseProgress = (taskPid: string) => {
  return request.get<any, CourseProgress>('/progress/course', { params: { task_pid: taskPid } })
}

// ---- 课程进度缓存（60s TTL）----
// 列表页（TaskList/CollectList）每页会对每门课程调用 getCourseProgress，
// 同一课程在短时间内被多次请求（分页加载、切换分类、刷新）时，
// 这里做模块级去重，避免 N+1 请求风暴。错误不缓存，允许下次重试。
const courseProgressCache = new Map<string, { promise: Promise<CourseProgress>; expiresAt: number }>()
const COURSE_PROGRESS_TTL = 60_000

export const getCourseProgressCached = (taskPid: string): Promise<CourseProgress> => {
  const now = Date.now()
  const hit = courseProgressCache.get(taskPid)
  if (hit && hit.expiresAt > now) {
    return hit.promise
  }
  let promise: Promise<CourseProgress>
  promise = getCourseProgress(taskPid).then((res) => {
    // 成功才缓存；失败时删除缓存项让下次可重试
    courseProgressCache.set(taskPid, { promise, expiresAt: Date.now() + COURSE_PROGRESS_TTL })
    return res
  })
  promise.catch(() => {
    courseProgressCache.delete(taskPid)
  })
  courseProgressCache.set(taskPid, { promise, expiresAt: Date.now() + COURSE_PROGRESS_TTL })
  return promise
}

export const getMyProgress = (params?: MyProgressParams) => {
  return request.get<any, MyProgressResponse>('/progress/my', { params })
}
