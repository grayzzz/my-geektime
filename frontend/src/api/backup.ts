import request from '@/utils/request'

// 备份包可能很大（数万条评论/讨论，导出耗时可达几十秒），需覆盖默认 30s 超时
const BACKUP_TIMEOUT = 600000

export const exportBackup = () => {
  return request.get<any, Blob>('/backup/export', {
    responseType: 'blob',
    timeout: BACKUP_TIMEOUT,
  })
}

export const importBackup = (file: File) => {
  const fd = new FormData()
  fd.append('file', file)
  return request.post('/backup/import', fd, { timeout: BACKUP_TIMEOUT })
}
