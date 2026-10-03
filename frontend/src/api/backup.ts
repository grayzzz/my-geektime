import { longRequest, TIMEOUT } from '@/utils/request'

// 备份包可能很大（数万条评论/讨论，导出耗时可达几十秒）。
// 走 longRequest，超时预算集中定义在 @/utils/request 的 TIMEOUT，不再在本地写常量。
export const exportBackup = () => {
  return longRequest.get<any, Blob>('/backup/export', {
    responseType: 'blob',
    timeout: TIMEOUT.backup,
  })
}

export const importBackup = (file: File) => {
  const fd = new FormData()
  fd.append('file', file)
  return longRequest.post('/backup/import', fd, { timeout: TIMEOUT.backup })
}
