import request, { longRequest } from '@/utils/request'

export interface ProductListParams {
  page?: number
  perPage?: number
  direction?: number
  label_id?: number
  orderby?: string
  type?: string
  prev?: string
}

export interface PvipListParams {
  page?: number
  perPage?: number
  sort?: number
  direction?: number
  product_type?: number
  product_form?: number
  tag?: number
  keyword?: string
  with_articles?: boolean
}

export interface ProductItem {
  id: string
  title: string
  subtitle: string
  cover: {
    square: string
  }
  author: {
    name: string
    intro: string
  }
  article: {
    id: string
    count: number
  }
  is_audio?: boolean
  is_video?: boolean
  is_finish: boolean
  sale_type: number
  sale: number
  redirect?: string
  intro_html?: string
}

export const getProductList = (params?: ProductListParams) => {
  return request.get<any, { rows: ProductItem[]; count: number; score?: number; hasNext?: boolean }>('/product/list', { params })
}

export const getPvipList = (params?: PvipListParams) => {
  return request.get<any, { rows: ProductItem[]; count: number }>('/product/pvip/list', { params })
}

export interface ArticleItem {
  id: string
  article_title: string
  article_summary: string
  video_size?: number
  audio_size?: number
  cid: string
}

export const getProductArticles = (params: { cid: string; page?: number; perPage?: number; order?: string; sample?: boolean }) => {
  return request.get<any, { rows: ArticleItem[]; count: number }>('/product/articles', { params })
}

export const getArticleInfo = (id: string) => {
  return request.get('/product/article/info', { params: { id } })
}

export const getArticleComments = (params: { aid: string; page?: number; perPage?: number }) => {
  return request.get('/product/article/comments', { params })
}

export const getArticleDiscussions = (params: { target_id: string; target_type: number; page?: number; perPage?: number }) => {
  return request.get('/product/article/discussions', { params })
}

// 缓存整门课程到本地：后端同步执行 —— 按 pid 拉上游目录（一次 500 条），
// 再对**未入库**的章节逐个调上游取详情（串行 IO，见 internal/api/v2/product.go 的 Download）。
// 大课轻松超过默认 30s，因此走 longRequest 而不是默认实例。
export const downloadProduct = (params: { pid: number }) => {
  return longRequest.post('/product/download', params)
}
