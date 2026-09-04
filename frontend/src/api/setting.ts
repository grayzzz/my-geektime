import request from '@/utils/request'

export interface SettingData {
  storage: {
    host: string
  }
  site: {
    cache: boolean
    download: boolean
    register: {
      type: string
    }
    play: {
      type: string
      proxy_url: string[]
    }
    proxy: {
      proxy_url: string
      urls: string[]
    }
    cookie: {
      geektime: string
    }
  }
  ai: {
    base_url: string
    api_key: string
    model: string
  }
}

export const getSetting = () => {
  return request.get<any, SettingData>('/setting/query')
}

export const updateSetting = (data: any) => {
  return request.post('/setting/update', data)
}
