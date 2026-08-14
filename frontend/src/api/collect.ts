import request from '@/utils/request'

export interface CollectListParams {
  page?: number
  perPage?: number
  category?: number
}

export interface CollectItem {
  id: string
  category: number
  item: {
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
    course_progress?: {
      finished_count: number
      total_count: number
      percent: number
      last_task_id: string
      last_task_name: string
      updated_at: number
    }
  }
}

export const getCollectList = (params?: CollectListParams) => {
  return request.get<any, { rows: CollectItem[]; count: number }>('/collect/list', { params })
}

export const createCollect = (params: { ids: string[]; collect_type: string; category: number }) => {
  return request.post('/collect/create', params)
}

export const deleteCollect = (ids: string[]) => {
  return request.delete('/collect/delete', { data: { ids } })
}
