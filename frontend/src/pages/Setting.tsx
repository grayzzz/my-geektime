import React, { useEffect, useRef, useState } from 'react'
import { getSetting, updateSetting, SettingData } from '@/api/setting'
import { exportBackup, importBackup } from '@/api/backup'
import { downloadFileFromBlob } from '@/utils/request'
import { useAuthStore } from '@/store/auth'
import { Button, Card, Input, Switch, Spinner } from '@/components/ui'
import { useToast } from '@/components/ui/Toast'

// 与后端 user.AdminRoleId 保持一致
const ADMIN_ROLE_ID = 1
// 与后端 maxBackupUploadSize 保持一致
const MAX_BACKUP_SIZE = 2 * 1024 * 1024 * 1024

export const Setting: React.FC = () => {
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [backing, setBacking] = useState(false)
  const [restoring, setRestoring] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const isAdmin = useAuthStore((s) => s.user?.role_id === ADMIN_ROLE_ID)
  const [settings, setSettings] = useState<SettingData>({
    storage: { host: '' },
    site: {
      cache: false,
      download: false,
      register: { type: 'name' },
      play: { type: 'origin', proxy_url: [] },
      proxy: { proxy_url: '', urls: [] },
      cookie: { geektime: '' },
    },
    ai: { base_url: '', api_key: '', model: '' },
  })

  const { addToast } = useToast()

  useEffect(() => {
    loadSettings()
  }, [])

  const loadSettings = async () => {
    setLoading(true)
    try {
      const res = await getSetting()
      if (res.storage && res.site) {
        setSettings(res)
      }
    } catch (error) {
      console.error('Failed to load settings', error)
    } finally {
      setLoading(false)
    }
  }

  const handleSave = async () => {
    setSaving(true)
    try {
      await updateSetting({
        storageHost: settings.storage.host,
        siteDownload: settings.site.download,
        siteCache: settings.site.cache,
        siteProxyUrl: settings.site.proxy.proxy_url,
        siteProxyUrls: settings.site.proxy.urls,
        sitePlayUrls: settings.site.play.proxy_url,
        cookie: settings.site.cookie.geektime,
        ai: settings.ai,
      })
      addToast('保存成功', 'success')
    } catch (error) {
      addToast('保存失败，请重试', 'error')
      console.error('Failed to save settings', error)
    } finally {
      setSaving(false)
    }
  }

  const handleProxyUrlAdd = () => {
    setSettings((prev) => ({
      ...prev,
      site: {
        ...prev.site,
        proxy: {
          ...prev.site.proxy,
          urls: [...prev.site.proxy.urls, ''],
        },
      },
    }))
  }

  const handleProxyUrlRemove = (index: number) => {
    setSettings((prev) => ({
      ...prev,
      site: {
        ...prev.site,
        proxy: {
          ...prev.site.proxy,
          urls: prev.site.proxy.urls.filter((_, i) => i !== index),
        },
      },
    }))
  }

  const handleProxyUrlChange = (index: number, value: string) => {
    setSettings((prev) => ({
      ...prev,
      site: {
        ...prev.site,
        proxy: {
          ...prev.site.proxy,
          urls: prev.site.proxy.urls.map((url, i) => (i === index ? value : url)),
        },
      },
    }))
  }

  const handlePlayUrlAdd = () => {
    setSettings((prev) => ({
      ...prev,
      site: {
        ...prev.site,
        play: {
          ...prev.site.play,
          proxy_url: [...prev.site.play.proxy_url, ''],
        },
      },
    }))
  }

  const handlePlayUrlRemove = (index: number) => {
    setSettings((prev) => ({
      ...prev,
      site: {
        ...prev.site,
        play: {
          ...prev.site.play,
          proxy_url: prev.site.play.proxy_url.filter((_, i) => i !== index),
        },
      },
    }))
  }

  const handlePlayUrlChange = (index: number, value: string) => {
    setSettings((prev) => ({
      ...prev,
      site: {
        ...prev.site,
        play: {
          ...prev.site.play,
          proxy_url: prev.site.play.proxy_url.map((url, i) => (i === index ? value : url)),
        },
      },
    }))
  }

  const handleBackupExport = async () => {
    setBacking(true)
    try {
      const blob = await exportBackup()
      downloadFileFromBlob(blob, `my-geektime-backup-${Date.now()}.tar.gz`)
      addToast('备份已导出', 'success')
    } catch (error) {
      addToast('导出失败，请重试', 'error')
      console.error('Failed to export backup', error)
    } finally {
      setBacking(false)
    }
  }

  const handleBackupImportClick = () => {
    fileInputRef.current?.click()
  }

  const handleBackupFileSelected = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return
    if (file.size > MAX_BACKUP_SIZE) {
      addToast('备份文件超过 2GB 上限', 'error')
      return
    }
    if (!window.confirm('导入将清空现有全部数据并覆盖为备份内容，确认继续？')) return
    setRestoring(true)
    try {
      await importBackup(file)
      addToast('导入成功，正在刷新', 'success')
      // 全量覆盖后页面数据已失效，刷新重新加载
      setTimeout(() => window.location.reload(), 800)
    } catch (error) {
      addToast('导入失败，请检查备份文件', 'error')
      console.error('Failed to import backup', error)
    } finally {
      setRestoring(false)
    }
  }

  if (loading) {
    return (
      <div className="flex justify-center py-8">
        <Spinner />
      </div>
    )
  }

  return (
    <div>
      <Card header="系统配置" />

      <Card className="mt-4 pb-12">
        <div className="pl-4 pr-4 pt-4">

          <div className="space-y-4">
          <div>
            <h3 className="text-lg font-medium mb-2">Cookie设置</h3>
            <Input
              label="极客时间Cookie"
              placeholder="请输入极客时间的Cookie"
              value={settings.site.cookie.geektime}
              onChange={(e) =>
                setSettings((prev) => ({
                  ...prev,
                  site: {
                    ...prev.site,
                    cookie: { ...prev.site.cookie, geektime: e.target.value },
                  },
                }))
              }
            />
          </div>

          <div className="border-t pt-3">
            <h3 className="text-lg font-medium mb-2">缓存设置</h3>
            <Input
              label="URL"
              placeholder="缓存后音视频本地播放的URL"
              value={settings.storage.host}
              onChange={(e) =>
                setSettings((prev) => ({
                  ...prev,
                  storage: { ...prev.storage, host: e.target.value },
                }))
              }
            />
          </div>

          <div className="border-t pt-3 flex gap-6">
            <div className="flex-1">
              <Switch
                label="下载音视频"
                checked={settings.site.download}
                onChange={(checked) =>
                  setSettings((prev) => ({
                    ...prev,
                    site: { ...prev.site, download: checked },
                  }))
                }
                onText="开启"
                offText="关闭"
              />
              <p className="text-xs text-gray-500 mt-1">
                缓存时自动下载音视频到本地
              </p>
            </div>
            <div className="flex-1">
              <Switch
                label="下载资源"
                checked={settings.site.cache}
                onChange={(checked) =>
                  setSettings((prev) => ({
                    ...prev,
                    site: { ...prev.site, cache: checked },
                  }))
                }
                onText="开启"
                offText="关闭"
              />
              <p className="text-xs text-gray-500 mt-1">
                根据被代理的源站URL配置规则，缓存任务时自动下载内容中的资源到本地，添加缓存任务后触发
              </p>
            </div>
          </div>

          <div className="border-t pt-3">
            <h3 className="text-lg font-medium mb-2">资源代理</h3>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                被代理的URL
              </label>
              {settings.site.proxy.urls.map((url, index) => (
                <div key={index} className="flex gap-2 mb-2 min-w-0">
                  <Input
                    placeholder="被代理的源站URL"
                    value={url}
                    onChange={(e) => handleProxyUrlChange(index, e.target.value)}
                    className="flex-1 min-w-0"
                  />
                  <Button
                    variant="danger"
                    size="sm"
                    onClick={() => handleProxyUrlRemove(index)}
                    className="shrink-0"
                  >
                    删除
                  </Button>
                </div>
              ))}
              <Button variant="light" size="sm" onClick={handleProxyUrlAdd}>
                添加URL
              </Button>
            </div>

            <div className="mt-3">
              <label className="block text-sm font-medium text-gray-700 mb-2">
                被代理的播放URL
              </label>
              {settings.site.play.proxy_url.map((url, index) => (
                <div key={index} className="flex gap-2 mb-2 min-w-0">
                  <Input
                    placeholder="被代理的源站播放URL"
                    value={url}
                    onChange={(e) => handlePlayUrlChange(index, e.target.value)}
                    className="flex-1 min-w-0"
                  />
                  <Button
                    variant="danger"
                    size="sm"
                    onClick={() => handlePlayUrlRemove(index)}
                    className="shrink-0"
                  >
                    删除
                  </Button>
                </div>
              ))}
              <Button variant="light" size="sm" onClick={handlePlayUrlAdd}>
                添加URL
              </Button>
            </div>
          </div>

          <div className="border-t pt-3">
            <h3 className="text-lg font-medium mb-2">AI 服务</h3>
            <div className="space-y-3">
              <Input
                label="Base URL"
                placeholder="OpenAI 兼容 API 地址，如 https://api.deepseek.com/v1"
                value={settings.ai.base_url}
                onChange={(e) =>
                  setSettings((prev) => ({
                    ...prev,
                    ai: { ...prev.ai, base_url: e.target.value },
                  }))
                }
              />
              <Input
                label="API Key"
                type="password"
                placeholder="请输入 API Key"
                value={settings.ai.api_key}
                onChange={(e) =>
                  setSettings((prev) => ({
                    ...prev,
                    ai: { ...prev.ai, api_key: e.target.value },
                  }))
                }
              />
              <Input
                label="模型"
                placeholder="如 deepseek-chat / qwen-plus / glm-4"
                value={settings.ai.model}
                onChange={(e) =>
                  setSettings((prev) => ({
                    ...prev,
                    ai: { ...prev.ai, model: e.target.value },
                  }))
                }
              />
            </div>
          </div>

          <div className="border-t pt-6 flex justify-center gap-4 mt-12 mb-6">
            <Button onClick={handleSave} disabled={saving} size="md" className="px-8 py-2.5">
              {saving ? '保存中...' : '保存'}
            </Button>
            <Button variant="light" onClick={loadSettings} size="md" className="px-8 py-2.5">
              重置
            </Button>
          </div>
        </div>
        </div>
      </Card>

      {isAdmin && (
        <Card header="数据备份" className="mt-4">
          <div className="pl-4 pr-4 pt-4 pb-4">
            <p className="text-sm text-gray-500 mb-4">
              导出当前数据库全部内容（课程/文章/任务/收藏/进度/评论/用户），不含已下载的音视频文件。
              导入将以备份内容覆盖现有全部数据，请谨慎操作。
            </p>
            <div className="flex items-center gap-4">
              <Button onClick={handleBackupExport} disabled={backing} size="md" className="px-8 py-2.5">
                {backing ? '导出中...' : '导出备份'}
              </Button>
              <Button variant="danger" onClick={handleBackupImportClick} disabled={restoring} size="md" className="px-8 py-2.5">
                {restoring ? '导入中...' : '导入备份'}
              </Button>
              <input
                ref={fileInputRef}
                type="file"
                accept=".tar.gz"
                className="hidden"
                onChange={handleBackupFileSelected}
              />
            </div>
          </div>
        </Card>
      )}
    </div>
  )
}
