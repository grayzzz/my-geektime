import React, { useEffect, useState, useRef, useCallback } from 'react'
import {
  getCollectList,
  deleteCollect,
  CollectItem,
} from '@/api/collect'
import { getDictTree } from '@/api/dict'
import { retryTask, exportTask, downloadPdfBlob } from '@/api/task'
import { getCourseProgressCached } from '@/api/progress'
import { downloadFileFromBlob } from '@/utils/request'
import { Button, Card, Spinner, Alert, Modal } from '@/components/ui'
import { LessonDrawer } from '@/components/LessonDrawer'
import { useAuthStore } from '@/store/auth'
import { useToast } from '@/components/ui/Toast'
import { useLessonList } from '@/hooks/useLessonList'
import {
  FileText,
  FileDown,
  ExternalLink,
  Trash2,
  RefreshCw,
  Rocket,
  BookOpen,
} from 'lucide-react'

const productTypeOptions = [
  { label: '全部类型', value: 0 },
  { label: '体系课', value: 1 },
  { label: '公开课', value: 4 },
  { label: '线下大会', value: 5 },
  { label: '社区课', value: 6 },
  { label: '每日一课', value: 19 },
  { label: '大厂案例', value: 20 },
]

export const CollectList: React.FC = () => {
  const [loading, setLoading] = useState(false)
  const [items, setItems] = useState<CollectItem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [perPage] = useState(6)
  const [category, setCategory] = useState(0)
  const [prevCategory, setPrevCategory] = useState<number | null>(null)
  const [collectCategory, setCollectCategory] = useState<any[]>([])
  const [geektimeDirection, setGeektimeDirection] = useState<any[]>([])
  const [showDeleteModal, setShowDeleteModal] = useState(false)
  const [deleteIds, setDeleteIds] = useState<string[]>([])
  const [showRetryModal, setShowRetryModal] = useState(false)
  const [retryTaskId, setRetryTaskId] = useState('')
  const [showBackToTop, setShowBackToTop] = useState(false)
  const [loadMoreLoading, setLoadMoreLoading] = useState(false)
  const [hasMore, setHasMore] = useState(true)
  const observerRef = useRef<IntersectionObserver | null>(null)
  const loadMoreRef = useRef<HTMLDivElement | null>(null)
  const scrollContainerRef = useRef<HTMLElement | null>(null)
  const { addToast, removeToast } = useToast()

  const {
    lessonList,
    lessonLoading,
    lessonTotal,
    lessonPage,
    lessonFilters,
    selectedTask,
    showLessonDrawer,
    setShowLessonDrawer,
    setLessonFilters,
    openLessonDrawer,
    handleLessonFilter,
    handleLessonPageChange,
  } = useLessonList()

  const user = useAuthStore((state) => state.user)
  const isAdmin = user?.role_id === 1

  useEffect(() => {
    loadDictData()
  }, [])

  useEffect(() => {
    // 只有当 category 真正发生变化时才重新加载数据
    if (prevCategory !== null && prevCategory === category) {
      return
    }
    
    setPage(1)
    setItems([])
    setHasMore(true)
    loadData(1, false)
    setPrevCategory(category)
  }, [category])

  const handleLoadMore = useCallback(() => {
    if (hasMore && !loadMoreLoading && !loading && items.length < total) {
      loadData(page + 1, true)
    }
  }, [hasMore, loadMoreLoading, loading, items.length, total, page])

  useEffect(() => {
    observerRef.current = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasMore && !loadMoreLoading && !loading) {
          handleLoadMore()
        }
      },
      { threshold: 0.1 }
    )

    if (loadMoreRef.current) {
      observerRef.current.observe(loadMoreRef.current)
    }

    return () => {
      if (observerRef.current) {
        observerRef.current.disconnect()
      }
    }
  }, [hasMore, loadMoreLoading, loading, handleLoadMore])

  useEffect(() => {
    const mainElement = document.querySelector('main') as HTMLElement
    if (mainElement) {
      scrollContainerRef.current = mainElement
      const handleScroll = () => {
        setShowBackToTop(mainElement.scrollTop > 500)
      }
      mainElement.addEventListener('scroll', handleScroll)
      return () => mainElement.removeEventListener('scroll', handleScroll)
    }
  }, [])

  const [pdfDownloading, setPdfDownloading] = useState(false)

  const scrollToTop = () => {
    if (scrollContainerRef.current) {
      scrollContainerRef.current.scrollTo({ top: 0, behavior: 'smooth' })
    }
  }

  const loadDictData = async () => {
    try {
      const res = await getDictTree('collectCategory,geektimeCategory')
      if (res) {
        setCollectCategory(res.collectCategory || [])
        setGeektimeDirection(res.geektimeDirection || [])
      }
    } catch (error) {
      console.error('Failed to load dict data', error)
    }
  }

  // 为收藏的课程项拉取学习进度，并入 items
  const attachCourseProgress = async (rows: CollectItem[]): Promise<CollectItem[]> => {
    if (rows.length === 0) return rows
    try {
      const progressResults = await Promise.all(
        rows.map(async (item) => {
          try {
            const courseProgress = await getCourseProgressCached(item.item.task_id)
            return {
              task_id: item.item.task_id,
              course_progress: {
                ...courseProgress,
                total_count: courseProgress.total_count || item.item.article?.count || 0,
              },
            }
          } catch (error) {
            // 后端暂时不可用时仍保留章节总数，卡片可以显示 0/总章节数
            return {
              task_id: item.item.task_id,
              course_progress: {
                finished_count: 0,
                total_count: item.item.article?.count || 0,
                percent: 0,
                last_task_id: '',
                last_task_name: '',
                updated_at: 0,
              },
            }
          }
        })
      )
      const progressMap = new Map(progressResults.map((p) => [p.task_id, p.course_progress]))
      return rows.map((item) => {
        const p = progressMap.get(item.item.task_id)
        return p ? { ...item, item: { ...item.item, course_progress: p } } : item
      })
    } catch (error) {
      console.error('Failed to load course progress', error)
      return rows
    }
  }

  const loadData = async (loadPage = 1, isLoadMore = false) => {
    if (isLoadMore) {
      setLoadMoreLoading(true)
    } else {
      setLoading(true)
    }
    try {
      const params: any = { page: loadPage, perPage }
      if (category) params.category = category

      const res = await getCollectList(params)
      const rowsWithProgress = await attachCourseProgress(res.rows || [])
      if (isLoadMore) {
        setItems((prev) => [...prev, ...rowsWithProgress])
      } else {
        setItems(rowsWithProgress)
      }
      setTotal(res.count || 0)
      setPage(loadPage)
      const newItems = isLoadMore ? [...items, ...rowsWithProgress] : rowsWithProgress
      setHasMore(newItems.length < (res.count || 0))
    } catch (error) {
      console.error('Failed to load collects', error)
    } finally {
      if (isLoadMore) {
        setLoadMoreLoading(false)
      } else {
        setLoading(false)
      }
    }
  }

  const handleDelete = (ids: string[]) => {
    setDeleteIds(ids)
    setShowDeleteModal(true)
  }

  const handleDeleteConfirm = async () => {
    if (deleteIds.length === 0) return
    try {
      await deleteCollect(deleteIds)
      addToast('删除成功', 'success')
      setShowDeleteModal(false)
      setDeleteIds([])
      // 直接更新本地状态，删除对应的收藏项
      setItems(prevItems => prevItems.filter(item => !deleteIds.includes(item.id)))
      // 更新总数
      setTotal(prev => Math.max(0, prev - deleteIds.length))
    } catch (error) {
      console.error('Failed to delete', error)
      addToast('删除失败', 'error')
    }
  }

  const handleRetry = (taskId: string) => {
    setRetryTaskId(taskId)
    setShowRetryModal(true)
  }

  const handleRetryConfirm = async () => {
    if (!retryTaskId) return
    try {
      await retryTask({ pid: retryTaskId, retry: true })
      addToast('下载任务已创建', 'success')
      setShowRetryModal(false)
      setRetryTaskId('')
      // 直接更新本地状态，将该任务的状态改为"处理中"
      setItems(prevItems => 
        prevItems.map(item => 
          item.item.task_id === retryTaskId 
            ? { ...item, item: { ...item.item, status: 2 } } 
            : item
        )
      )
    } catch (error) {
      console.error('Failed to retry', error)
      addToast('操作失败', 'error')
    }
  }

  const handleExport = async (taskId: string, type: string) => {
    try {
      if (type === 'markdown') {
        const blob = await exportTask({ pid: taskId, type })
        const url = window.URL.createObjectURL(blob as Blob)
        const a = document.createElement('a')
        a.href = url
        a.download = `${taskId}-${type}.tar.gz`
        a.click()
        window.URL.revokeObjectURL(url)
      } else {
        const response: any = await exportTask({ pid: taskId, type })
        addToast('文档生成成功', 'success')
        // 更新本地状态而不是重新加载整个列表
        if (response && response.doc) {
          setItems(prevItems =>
            prevItems.map(item =>
              item.item.task_id === taskId
                ? { ...item, item: { ...item.item, doc: response.doc } }
                : item
            )
          )
        }
      }
    } catch (error) {
      console.error('Failed to export', error)
      addToast('操作失败', 'error')
    }
  }

  const handleExportPdf = (taskId: string, taskName: string) => {
    if (pdfDownloading) return
    setPdfDownloading(true)
    const sanitize = (name: string) =>
      name
        .replace(/"/g, '-')
        .replace(/\|/g, '-')
        .replace(/｜/g, '-')
        .replace(/:/g, '：')
        .replace(/”/g, '“')
        .replace(/\?/g, '？')
        .replace(/&/g, '+')
        .replace(/\t/g, '')
        .replace(/ /g, '')
        .trim()
    const safeName = sanitize(taskName) || taskId
    const loadingId = addToast('正在生成PDF，请稍候...', 'info', Infinity)
    downloadPdfBlob({ pid: taskId })
      .then((blob) => {
        downloadFileFromBlob(blob, `${safeName}.pdf`)
        removeToast(loadingId)
        addToast('PDF导出成功', 'success')
      })
      .catch((error) => {
        removeToast(loadingId)
        addToast(error?.message || 'PDF导出失败', 'error')
      })
      .finally(() => setPdfDownloading(false))
  }

  const getStatusText = (status: number) => {
    const map: Record<number, string> = {
      1: '排队中',
      2: '处理中',
      3: '已缓存',
      4: '异常',
    }
    return map[status] || '-'
  }

  const getTypeText = (type: number) => {
    const item = productTypeOptions.find((o) => o.value === type)
    return item?.label || '-'
  }

  const getDirectionText = (group: number) => {
    const item = geektimeDirection.find((o) => o.value === group)
    return item?.label || '-'
  }

  

  return (
    <div>
      <Card header="我的收藏" />

      <Alert variant="info" showIcon className="mt-4">
        温馨提示：从课程列表批量收藏后，会出现在我的收藏列表
      </Alert>

      <Card className="mt-4">
        <div className="bg-gradient-to-r from-primary-50/50 to-primary-100/50 rounded-xl p-4 mb-4 border border-primary-100/50">
          <div className="text-sm font-medium text-gray-700 mb-3">收藏分类</div>
          <div className="flex flex-wrap gap-2">
            <button
              onClick={() => setCategory(0)}
              className={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
                category === 0
                  ? 'bg-primary-500/10 text-primary-600 border border-primary-300 shadow-sm'
                  : 'bg-white text-gray-600 hover:bg-gray-50 border border-gray-200'
              }`}
            >
              全部
            </button>
            {collectCategory
              .filter((item) => item.value !== 0)
              .map((item) => (
                <button
                  key={item.value}
                  onClick={() => setCategory(Number(item.value))}
                  className={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
                    category === Number(item.value)
                      ? 'bg-primary-500/10 text-primary-600 border border-primary-300 shadow-sm'
                      : 'bg-white text-gray-600 hover:bg-gray-50 border border-gray-200'
                  }`}
                >
                  {item.label}
                </button>
              ))}
          </div>
        </div>

        {loading ? (
          <div className="flex justify-center py-8">
            <Spinner />
          </div>
        ) : (
          <>
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {items.map((item) => (
                <div
                  key={item.id}
                  className="relative bg-white rounded-xl border-2 border-gray-100 hover:border-purple-300 transition-all duration-300 hover:shadow-lg"
                >
                  <div className="p-4">
                    <div className="flex items-start gap-3">
                      <div className="relative flex-shrink-0">
                        <img
                          src={item.item.cover}
                          alt={item.item.task_name}
                          className="w-20 h-20 rounded-xl object-cover shadow-md"
                        />
                        {(item.item.is_audio || item.item.is_video) && (
                          <div className="absolute -bottom-1 -right-1 flex gap-1">
                            {item.item.is_audio && (
                              <span className="px-1.5 py-0.5 bg-primary-500 text-white text-xs rounded-md">音</span>
                            )}
                            {item.item.is_video && (
                              <span className="px-1.5 py-0.5 bg-primary-600 text-white text-xs rounded-md">视</span>
                            )}
                          </div>
                        )}
                      </div>
                      <div className="flex-1 min-w-0">
                        <h3 className="font-semibold text-gray-800 truncate leading-tight">
                          {item.item.task_name}
                        </h3>
                        <p className="text-sm text-gray-500 truncate mt-1">
                          {item.item.subtitle}
                        </p>
                        <div className="flex items-center gap-2 mt-2">
                          <span className="text-xs text-gray-400">{item.item.author?.name}</span>
                          {item.item.is_finish && (
                            <span className="px-1.5 py-0.5 bg-green-100 text-green-600 text-xs rounded">已完结</span>
                          )}
                        </div>
                      </div>
                    </div>
                    <div className="mt-4 pt-3 border-t border-gray-100">
                      <div className="grid grid-cols-2 gap-2 text-xs">
                        <div className="flex items-center gap-1">
                          <span className="text-gray-400">类型:</span>
                          <span className="text-gray-600">{getTypeText(item.item.other_type)}</span>
                        </div>
                        <div className="flex items-center gap-1">
                          <span className="text-gray-400">方向:</span>
                          <span className="text-gray-600">{getDirectionText(item.item.other_group)}</span>
                        </div>
                        <div className="flex items-center gap-1">
                          <span className="text-gray-400">章节:</span>
                          <span className="text-gray-600">{item.item.article?.count || 0} 讲</span>
                        </div>
                        <div className="flex items-center gap-1">
                          <span className="text-gray-400">价格:</span>
                          <span className="text-gray-600">
                            {item.item.sale_type === 6 || item.item.sale_type === 7
                              ? '免费'
                              : `¥${(item.item.sale / 100).toFixed(2)}`}
                          </span>
                        </div>
                      </div>
                      {item.item.statistics?.items && (
                        <div className="mt-2 flex flex-wrap gap-2 text-xs">
                          {Object.entries(item.item.statistics.items).map(([key, count]) => {
                            if ((count as number) > 0) {
                              return (
                                <span key={key} className="flex items-center gap-1">
                                  <span className={`w-2 h-2 rounded-full ${
                                    key === '1' ? 'bg-yellow-500' :
                                    key === '2' ? 'bg-blue-500' :
                                    key === '3' ? 'bg-green-500' : 'bg-red-500'
                                  }`} />
                                  {getStatusText(Number(key))}({count as number})
                                </span>
                              )
                            }
                            return null
                          })}
                        </div>
                      )}
                    </div>
                    {/* 学习进度 */}
                    {item.item.course_progress && item.item.course_progress.total_count > 0 && (
                      <div className="mt-3 pt-3 border-t border-gray-100">
                        <div className="flex items-center justify-between mb-1.5">
                          <span className="text-xs text-gray-400">学习进度:</span>
                          <div className="flex items-center gap-2">
                            <span className="text-xs text-gray-600">
                              {item.item.course_progress.finished_count}/{item.item.course_progress.total_count} 讲
                            </span>
                            <span className={`text-xs font-semibold ${item.item.course_progress.percent >= 100 ? 'text-green-600' : 'text-primary-600'}`}>
                              {item.item.course_progress.percent >= 100
                                ? '100%'
                                : `${(Math.round(item.item.course_progress.percent * 10) / 10)}%`}
                            </span>
                            {item.item.course_progress.percent >= 100 && (
                              <span className="px-1.5 py-0.5 bg-green-100 text-green-600 text-xs rounded">已学完</span>
                            )}
                          </div>
                        </div>
                        <div className="w-full h-2 bg-gray-100 rounded-full overflow-hidden">
                          <div
                            className={`h-full rounded-full transition-all ${item.item.course_progress.percent >= 100 ? 'bg-green-500' : 'bg-primary-500'}`}
                            style={{ width: `${Math.min(100, Math.max(0, item.item.course_progress.percent))}%` }}
                          />
                        </div>
                        {item.item.course_progress.last_task_name ? (
                          <p className="text-xs truncate mt-1.5" title={item.item.course_progress.last_task_name}>
                            <span className="text-gray-400">上次学到:</span>{' '}
                            <span className="text-gray-600">{item.item.course_progress.last_task_name}</span>
                          </p>
                        ) : (
                          <p className="text-xs mt-1.5">
                            <span className="text-gray-400">上次学到:</span>{' '}
                            <span className="text-gray-600">尚未开始</span>
                          </p>
                        )}
                      </div>
                    )}
                    <div className="mt-3 flex flex-wrap items-center gap-2">
                      {item.item.redirect && (
                        <div className="relative group">
                          <Button
                            variant="light"
                            size="sm"
                            onClick={() => window.open(item.item.redirect, '_blank')}
                            className="!p-2"
                          >
                            <ExternalLink size={14} />
                          </Button>
                          <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[150px] text-center pointer-events-none z-20">
                            源站
                          </div>
                        </div>
                      )}
                      <div className="relative group">
                        <Button
                          variant="light"
                          size="sm"
                          onClick={() => openLessonDrawer(item.item)}
                          className="!p-2"
                        >
                          <BookOpen size={14} />
                        </Button>
                        <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[150px] text-center pointer-events-none z-20">
                          课程
                        </div>
                      </div>
                      <div className="relative group">
                        <Button
                          variant="light"
                          size="sm"
                          disabled={pdfDownloading}
                          onClick={() => handleExportPdf(item.item.task_id, item.item.task_name)}
                          className="!p-2"
                        >
                          <FileDown size={14} />
                        </Button>
                        <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[150px] text-center pointer-events-none z-20">
                          导出课程PDF
                        </div>
                      </div>
                      <div className="relative group">
                        <Button
                          variant="light"
                          size="sm"
                          onClick={() => handleExport(item.item.task_id, 'markdown')}
                          className="!p-2"
                        >
                          <FileText size={14} />
                        </Button>
                        <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[150px] text-center pointer-events-none z-20">
                          导出Markdown
                        </div>
                      </div>
                      {item.item.doc !== undefined ? (
                        <div className="relative group">
                          <Button
                            variant="success"
                            size="sm"
                            onClick={() => window.open(item.item.doc, '_blank')}
                            className="!p-2"
                          >
                            <FileText size={14} />
                          </Button>
                          <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[150px] text-center pointer-events-none z-20">
                            文档
                          </div>
                        </div>
                      ) : (
                        <div className="relative group">
                          <Button
                            variant="light"
                            size="sm"
                            onClick={() => handleExport(item.item.task_id, 'docsite')}
                            className="!p-2"
                          >
                            <FileText size={14} />
                          </Button>
                          <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[150px] text-center pointer-events-none z-20">
                            生成文档
                          </div>
                        </div>
                      )}
                      {isAdmin && (
                        <>
                          <div className="relative group">
                            <Button
                              variant="warning"
                              size="sm"
                              onClick={() => handleRetry(item.item.task_id)}
                              className="!p-2"
                            >
                              <RefreshCw size={14} />
                            </Button>
                            <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[150px] text-center pointer-events-none z-20">
                              下载资源
                            </div>
                          </div>
                          <div className="relative group">
                            <Button
                              variant="danger"
                              size="sm"
                              onClick={() => handleDelete([item.id])}
                              className="!p-2"
                            >
                              <Trash2 size={14} />
                            </Button>
                            <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[150px] text-center pointer-events-none z-20">
                              删除
                            </div>
                          </div>
                        </>
                      )}
                    </div>
                  </div>
                  </div>
              ))}
            </div>
            {items.length === 0 && !loading && (
              <div className="flex flex-col items-center justify-center py-20 text-gray-500">
                <div className="text-lg">暂无数据</div>
              </div>
            )}
            <div ref={loadMoreRef} className="h-4" />
            {loadMoreLoading && (
              <div className="flex justify-center py-6">
                <Spinner size="sm" className="mr-2" />
                <span className="text-gray-500">加载中...</span>
              </div>
            )}
            {!hasMore && items.length > 0 && (
              <div className="text-center text-gray-400 text-sm py-6">
                已加载全部 {total} 条数据
              </div>
            )}
          </>
        )}
      </Card>

      {showBackToTop && (
        <button
          onClick={scrollToTop}
          className="fixed bottom-8 right-8 w-12 h-12 rounded-full bg-white text-primary-600 border-2 border-primary-300 shadow-lg hover:bg-primary-50 hover:border-primary-400 transition-all duration-300 flex items-center justify-center z-10 hover:scale-110"
          title="返回顶部"
        >
          <Rocket size={20} />
        </button>
      )}

      <Modal
        isOpen={showDeleteModal}
        onClose={() => {
          setShowDeleteModal(false)
          setDeleteIds([])
        }}
        title="确认删除"
        size="sm"
      >
        <div className="space-y-4">
          <p className="text-sm text-gray-600">确定要删除选中的收藏吗？</p>
          <div className="flex gap-3">
            <Button
              variant="secondary"
              onClick={() => {
                setShowDeleteModal(false)
                setDeleteIds([])
              }}
              className="flex-1"
            >
              取消
            </Button>
            <Button variant="danger" onClick={handleDeleteConfirm} className="flex-1">
              确定
            </Button>
          </div>
        </div>
      </Modal>

      <Modal
        isOpen={showRetryModal}
        onClose={() => {
          setShowRetryModal(false)
          setRetryTaskId('')
        }}
        title="确认重新下载"
        size="sm"
      >
        <div className="space-y-4">
          <p className="text-sm text-gray-600">确定要重新下载音视频到本地磁盘吗？</p>
          <div className="flex gap-3">
            <Button
              variant="secondary"
              onClick={() => {
                setShowRetryModal(false)
                setRetryTaskId('')
              }}
              className="flex-1"
            >
              取消
            </Button>
            <Button onClick={handleRetryConfirm} className="flex-1">
              确定
            </Button>
          </div>
        </div>
      </Modal>

      <LessonDrawer
        showLessonDrawer={showLessonDrawer}
        selectedTask={selectedTask}
        lessonList={lessonList}
        lessonLoading={lessonLoading}
        lessonTotal={lessonTotal}
        lessonPage={lessonPage}
        lessonFilters={lessonFilters}
        setLessonFilters={setLessonFilters}
        setShowLessonDrawer={(show: boolean) => {
          setShowLessonDrawer(show)
          // 关闭课程抽屉后刷新收藏列表的课程进度
          if (!show) {
            loadData(1, false)
          }
        }}
        handleLessonPageChange={handleLessonPageChange}
        handleLessonFilter={handleLessonFilter}
      />
    </div>
  )
}
