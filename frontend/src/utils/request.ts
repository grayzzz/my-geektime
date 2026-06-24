import axios from 'axios'
import { useLoadingStore } from '@/store/loading'

// 创建自定义事件用于显示 Toast 消息
export const showErrorToastEvent = new EventTarget()

export const showErrorMessage = (message: string) => {
  showErrorToastEvent.dispatchEvent(new CustomEvent('showError', { detail: message }))
}

const request = axios.create({
  baseURL: '/v2',
  timeout: 30000,
})

request.interceptors.request.use(
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

request.interceptors.response.use(
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
