import axios, { type AxiosInstance } from 'axios'
import { useLoadingStore } from '@/store/loading'

// 创建自定义事件用于显示 Toast 消息
export const showErrorToastEvent = new EventTarget()

export const showErrorMessage = (message: string) => {
  showErrorToastEvent.dispatchEvent(new CustomEvent('showError', { detail: message }))
}

/**
 * 超时预算（毫秒）。集中定义，避免魔法数字散落在各 api 文件里。
 *
 * ⚠️ 后端不同路由挂不同的超时中间件（internal/middleware/timeout.go）。
 *    前端超时必须【严格大于】对应后端超时，否则会出现
 *    「前端先超时、后端其实还在正常干活」的假失败 —— 这是历史上一批
 *    「导出没反应」问题的根因，改任何一个值前先看一眼后端。
 */
export const TIMEOUT = {
  /** 默认值：绝大多数接口只是查本地 DB，30s 足够。调大会推迟故障报错，别动 */
  default: 30000,
  /** 长请求默认值：不确定耗时的导出 / 同步 / 备份 */
  long: 300000,
  /** EPUB 导出：要下载并内嵌图片，明显慢于 markdown */
  epub: 900000,
  /** PDF 导出：必须大于后端 PDFTimeout 的 60min（internal/middleware/timeout.go） */
  pdf: 3900000,
  /** 备份导入 / 导出：可能含数万条评论，耗时可达几十秒 */
  backup: 600000,
} as const

const BASE_URL = '/v2'

const request = axios.create({
  baseURL: BASE_URL,
  timeout: TIMEOUT.default,
})

/**
 * 长耗时请求专用实例（默认 TIMEOUT.long）。
 *
 * 新增「生成 / 导出 / 全量同步 / 备份」这类慢接口时，从这里发起请求，
 * 而不是用默认的 request + 在调用点手写 timeout —— 手写那条路一旦漏掉，
 * 请求会静默落到 30s，表现为「操作失败」而后端仍在正常工作。
 */
export const longRequest = axios.create({
  baseURL: BASE_URL,
  timeout: TIMEOUT.long,
})

const applyInterceptors = (instance: AxiosInstance) => {
  instance.interceptors.request.use(
    (config) => {
      // 显示 loading
      useLoadingStore.getState().showLoading()

      const token = localStorage.getItem('token')
      if (token) {
        config.headers.Authorization = `Bearer ${token}`
      }
      return config
    },
    (error) => {
      // 请求错误时隐藏 loading
      useLoadingStore.getState().hideLoading()
      return Promise.reject(error)
    }
  )

  instance.interceptors.response.use(
    async (response) => {
      // 响应成功时隐藏 loading
      useLoadingStore.getState().hideLoading()

      if (response.config.responseType === 'blob') {
        return response.data
      }
      const res = response.data
      // 检查业务状态码，如果为400表示token过期
      if (res.status === 400 || res.code === 400) {
        // Token过期，清除本地存储并跳转到登录页
        localStorage.clear()
        window.location.href = '/login'
        return Promise.reject(new Error('Token expired'))
      }
      // 对所有 status != 0 的响应，显示错误提示
      if (res.status !== 0) {
        const errorMsg = res.msg || '请求失败'
        showErrorMessage(errorMsg)
        // 创建一个包含完整错误信息的错误对象
        const error = new Error(errorMsg)
        ;(error as any).response = res
        return Promise.reject(error)
      }
      if (res.data !== undefined) {
        return res.data
      }
      return res
    },
    (error) => {
      // 响应错误时隐藏 loading
      useLoadingStore.getState().hideLoading()

      if (error.response?.status === 401) {
        localStorage.clear()
        window.location.href = '/login'
      }
      // 确保错误信息能被正确捕获
      return Promise.reject(error)
    }
  )
}

applyInterceptors(request)
applyInterceptors(longRequest)

export const downloadFileFromBlob = (blob: Blob, filename: string) => {
  const url = window.URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  // 延迟 revoke，给浏览器足够时间发起下载请求
  // 立即 revoke 会导致部分浏览器下载失败（blob URL 在 download 完成前被撤销）
  setTimeout(() => {
    window.URL.revokeObjectURL(url)
  }, 100)
}

export default request
