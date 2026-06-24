import request from '@/utils/request'

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

export const exportTask = (params: { pid: string; type: string }) => {
  if (params.type === 'markdown') {
    return request.get<any, Blob>('/task/export', { params, responseType: 'blob' })
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
  return request.get<any, Blob>('/task/download', {
    params: { ...params, type: 'pdf' },
    responseType: 'blob',
    timeout: 600000,
  })
}

export const getCommentDiscussions = (params: {
  target_id: string
  target_type: number
  page?: number
  perPage?: number
  use_likes_order?: boolean
}) => {
  return request.get('/task/article/discussions', { params })
}
