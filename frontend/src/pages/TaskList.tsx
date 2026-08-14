import React, { useEffect, useState, useRef, useCallback } from 'react'
import {
  getTaskList,
  deleteTask,
  retryTask,
  exportTask,
  downloadPdfBlob,
  TaskItem,
} from '@/api/task'
import { downloadFileFromBlob } from '@/utils/request'
import { createCollect } from '@/api/collect'
import { getDictTree } from '@/api/dict'
import { getCourseProgressCached } from '@/api/progress'
import { Button, Card, Select, Input, Spinner, Modal } from '@/components/ui'
import { TaskCard } from '@/components/TaskCard'
import { LessonDrawer } from '@/components/LessonDrawer'
import { useAuthStore } from '@/store/auth'
import { useToast } from '@/components/ui/Toast'
import { useLessonList } from '@/hooks/useLessonList'
import { Rocket, RefreshCw, Trash2, Heart, FileText } from 'lucide-react'
import { useDebounce } from '@/hooks/useDebounce'

const productTypeOptions = [
  { label: '全部类型', value: 0 },
  { label: '体系课', value: 1 },
  { label: '公开课', value: 4 },
  { label: '线下大会', value: 5 },
  { label: '社区课', value: 6 },
  { label: '每日一课', value: 19 },
  { label: '大厂案例', value: 20 },
]

const productFormOptions = [
  { label: '全部形式', value: 0 },
  { label: '图文+音频', value: 1 },
  { label: '视频', value: 2 },
]

const productStatusOptions = [
  { label: '全部状态', value: 0 },
  { label: '排队中', value: 1 },
  { label: '处理中', value: 2 },
  { label: '已缓存', value: 3 },
  { label: '异常', value: 4 },
]

export const TaskList: React.FC = () => {
  const [loading, setLoading] = useState(false)
  const [items, setItems] = useState<TaskItem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [perPage] = useState(12)
  const [filters, setFilters] = useState({
    direction: 0,
    tag: 0,
    product_type: 0,
    product_form: 0,
    xstatus: 0,
    keywords: '',
  })
  const [prevFilters, setPrevFilters] = useState<typeof filters | null>(null)

  // 为关键字搜索添加防抖
  const debouncedKeywords = useDebounce(filters.keywords, 500)
  const [selectedItems, setSelectedItems] = useState<Set<string>>(new Set())
  const [geektimeCategory, setGeektimeCategory] = useState<any[]>([])
  const [geektimeDirection, setGeektimeDirection] = useState<any[]>([])
  const [collectCategory, setCollectCategory] = useState<any[]>([])
  const [showDeleteModal, setShowDeleteModal] = useState(false)
  const [deleteIds, setDeleteIds] = useState<string[]>([])
  const [showRetryModal, setShowRetryModal] = useState(false)
  const [retryParams, setRetryParams] = useState<{ pid?: string; ids?: string[] }>({})
  const [showCollectModal, setShowCollectModal] = useState(false)
  const [collectCategoryValue, setCollectCategoryValue] = useState<number>(0)
  const [collectIds, setCollectIds] = useState<string[]>([])
  const [showAllCategories, setShowAllCategories] = useState(false)
  const [loadMoreLoading, setLoadMoreLoading] = useState(false)
  const [hasMore, setHasMore] = useState(true)
  const [showBackToTop, setShowBackToTop] = useState(false)
  const [pdfDownloading, setPdfDownloading] = useState(false)
  const observerRef = useRef<IntersectionObserver | null>(null)
  const loadMoreRef = useRef<HTMLDivElement | null>(null)
  const scrollContainerRef = useRef<HTMLElement | null>(null)
  const { addToast, removeToast } = useToast()

  const {
    lessonList,
    lessonLoading,
    lessonPage,
    lessonTotal,
    lessonFilters,
    selectedTask,
    showLessonDrawer,
    setShowLessonDrawer,
    setLessonFilters,
    openLessonDrawer,
    handleLessonPageChange,
    handleLessonFilter,
  } = useLessonList()

  const user = useAuthStore((state) => state.user)
  const isAdmin = user?.role_id === 1

  useEffect(() => {
    loadDictData()
  }, [])

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

  const scrollToTop = () => {
    if (scrollContainerRef.current) {
      scrollContainerRef.current.scrollTo({ top: 0, behavior: 'smooth' })
    }
  }

  useEffect(() => {
    // 只有当 filters 真正发生变化时才重新加载数据
    if (prevFilters && JSON.stringify(prevFilters) === JSON.stringify({ ...filters, keywords: prevFilters.keywords })) {
      return
    }
    
    setPage(1)
    setItems([])
    setHasMore(true)
    loadData(1, false)
    setPrevFilters(filters)
  }, [debouncedKeywords, filters.direction, filters.tag, filters.product_type, filters.product_form, filters.xstatus])

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

  const loadDictData = async () => {
    try {
      // 首先尝试从数据库获取字典数据
      const res = await getDictTree('geektimeCategory,collectCategory')
      if (res) {
        const categories = res.geektimeCategory || []
        const directions = categories
          .filter((item: any) => item.label !== '全部')
          .map((item: any) => ({
            label: item.label,
            value: Number(item.value),
            children: item.children
              ?.filter((c: any) => c.label !== '全部')
              .map((c: any) => ({
                label: c.label,
                value: Number(c.value),
              })) || [],
          }))
        setGeektimeCategory(directions)
        setGeektimeDirection(directions)
        setCollectCategory(res.collectCategory || [])
      }
    } catch (error) {
      console.error('Failed to load dict data from database, trying tags API', error)
    }
  }

  // 为所有课程级项拉取学习进度，并入 items。
  // 课程级 task_pid 为空时会被后端 omitempty 省略，因此使用 !item.task_pid 判断。
  const attachCourseProgress = async (rows: TaskItem[]): Promise<TaskItem[]> => {
    const courseRows = rows.filter((item) => !item.task_pid)
    if (courseRows.length === 0) return rows
    try {
      const progressResults = await Promise.all(
        courseRows.map(async (item) => {
          try {
            const courseProgress = await getCourseProgressCached(item.task_id)
            return {
              task_id: item.task_id,
              course_progress: {
                ...courseProgress,
                total_count: courseProgress.total_count || item.article?.count || 0,
              },
            }
          } catch (error) {
            // 后端暂时不可用时仍保留章节总数，卡片可以显示 0/总章节数。
            return {
              task_id: item.task_id,
              course_progress: {
                finished_count: 0,
                total_count: item.article?.count || 0,
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
        const p = progressMap.get(item.task_id)
        return p ? { ...item, course_progress: p } : item
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
      if (filters.direction) params.direction = filters.direction
      if (filters.tag) params.tag = filters.tag
      if (filters.product_type) params.product_type = filters.product_type
      if (filters.product_form) params.product_form = filters.product_form
      if (filters.xstatus) params.xstatus = filters.xstatus
      if (filters.keywords) params.keywords = filters.keywords

      const res = await getTaskList(params)
      // 批量拉取课程级进度（仅课程级、已缓存项）
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
      console.error('Failed to load tasks', error)
    } finally {
      if (isLoadMore) {
        setLoadMoreLoading(false)
      } else {
        setLoading(false)
      }
    }
  }

  const handleExport = (pid: string, type: string, mode?: string, taskName?: string) => {
    // 文件名清洗：与后端 VerifyFileName 保持一致，避免 Windows 非法字符
    const sanitizeFileName = (name: string) =>
      name
        .replace(/"/g, '-')                          // " → -
        .replace(/\|/g, '-')                         // | → -
        .replace(/｜/g, '-')                         // 全角 ｜ → -
        .replace(/:/g, '：')                         // : → ：
        .replace(/”/g, '“')                     // " (right double quote) → ""
        .replace(/\?/g, '？')                        // ? → ？
        .replace(/&/g, '+')                          // & → +
        .replace(/\t/g, '')                          // tab → 空
        .replace(/ /g, '')                           // 空格 → 空
        .trim()

    const safeName = taskName ? sanitizeFileName(taskName) : pid
    // 课程 PDF 导出（走 axios + blob 下载，携带 token）
    if (mode === 'course' && type === 'pdf') {
      if (pdfDownloading) return
      setPdfDownloading(true)
      const loadingId = addToast('正在生成PDF，请稍候...', 'info', Infinity)
      downloadPdfBlob({ pid })
        .then((blob) => {
          downloadFileFromBlob(blob, `${safeName}.pdf`)
          removeToast(loadingId)
          addToast('PDF导出成功', 'success')
        })
        .catch((error) => {
          console.error('Failed to export PDF', error)
          removeToast(loadingId)
          addToast(error?.message || 'PDF导出失败', 'error')
        })
        .finally(() => {
          setPdfDownloading(false)
        })
      return
    }

    if (type === 'markdown') {
      exportTask({ pid, type })
        .then((blob) => {
          const url = window.URL.createObjectURL(blob as Blob)
          const a = document.createElement('a')
          a.href = url
          a.download = `${safeName}-${type}.tar.gz`
          a.click()
          window.URL.revokeObjectURL(url)
        })
        .catch((error) => {
          console.error('Failed to export', error)
          addToast('导出失败', 'error')
        })
    } else {
      exportTask({ pid, type })
        .then((response: any) => {
          addToast('文档生成成功', 'success')
          // 更新本地状态而不是重新加载整个列表
          if (response && response.doc) {
            setItems(prevItems => 
              prevItems.map(item => 
                item.task_id === pid 
                  ? { ...item, doc: response.doc } 
                  : item
              )
            )
          }
        })
        .catch((error) => {
          console.error('Failed to export', error)
          addToast('文档生成失败', 'error')
        })
    }
  }

  const handleRetry = (pid?: string, ids?: string[]) => {
    setRetryParams({ pid, ids })
    setShowRetryModal(true)
  }

  const handleRetryConfirm = async () => {
    try {
      const ids = retryParams.ids || (retryParams.pid ? [retryParams.pid] : [])
      await retryTask({ pid: retryParams.pid, ids: retryParams.ids, retry: true })
      addToast('下载任务已创建', 'success')
      setShowRetryModal(false)
      setRetryParams({})
      // 直接更新本地状态，将选中任务的状态改为"处理中"
      setItems(prevItems => 
        prevItems.map(item => 
          ids.includes(item.task_id) 
            ? { ...item, status: 2 } 
            : item
        )
      )
    } catch (error) {
      console.error('Failed to retry tasks', error)
      addToast('操作失败', 'error')
    }
  }

  const handleDelete = (ids: string[]) => {
    setDeleteIds(ids)
    setShowDeleteModal(true)
  }

  const handleDeleteConfirm = async () => {
    try {
      await deleteTask(deleteIds)
      addToast('删除成功', 'success')
      setShowDeleteModal(false)
      setDeleteIds([])
      setSelectedItems(new Set())
      // 直接更新本地状态，删除对应的任务项
      setItems(prevItems => prevItems.filter(item => !deleteIds.includes(item.task_id)))
      // 更新总数
      setTotal(prev => Math.max(0, prev - deleteIds.length))
    } catch (error) {
      console.error('Failed to delete tasks', error)
      addToast('删除失败', 'error')
    }
  }

  const toggleSelect = (id: string) => {
    setSelectedItems((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const handleBatchExport = (type: string) => {
    if (selectedItems.size === 0) {
      addToast('请先选择要导出的课程', 'warning')
      return
    }
    const ids = Array.from(selectedItems)
    const selectedItem = items.find(i => i.task_id === ids[0])
    handleExport(ids[0], type, undefined, selectedItem?.task_name)
  }

  const handleBatchRetry = () => {
    if (selectedItems.size === 0) {
      addToast('请先选择要重新下载的课程', 'warning')
      return
    }
    const ids = Array.from(selectedItems)
    handleRetry(undefined, ids)
  }

  const handleBatchDelete = () => {
    if (selectedItems.size === 0) {
      addToast('请先选择要删除的课程', 'warning')
      return
    }
    const ids = Array.from(selectedItems)
    handleDelete(ids)
  }

  const handleSingleCollect = (id: string) => {
    setCollectCategoryValue(0)
    setCollectIds([id])
    setShowCollectModal(true)
  }

  const handleBatchCollect = () => {
    if (selectedItems.size === 0) {
      addToast('请先选择要收藏的课程', 'warning')
      return
    }
    setCollectCategoryValue(0)
    setCollectIds(Array.from(selectedItems))
    setShowCollectModal(true)
  }

  const handleCollectConfirm = async () => {
    if (!collectCategoryValue) {
      addToast('请选择收藏分类', 'warning')
      return
    }
    try {
      await createCollect({ ids: collectIds, collect_type: 'task', category: collectCategoryValue })
      addToast('收藏成功', 'success')
      setShowCollectModal(false)
      setCollectCategoryValue(0)
      setCollectIds([])
      setSelectedItems(new Set())
      // 更新本地状态，标记已收藏
      setItems(prevItems =>
        prevItems.map(item =>
          collectIds.includes(item.task_id)
            ? { ...item, is_collected: true }
            : item
        )
      )
    } catch (error) {
      console.error('Failed to collect tasks', error)
    }
  }

  return (
    <div>
      <Card header="我的课程" />

      <Card className="mt-4">
        <div className="bg-gradient-to-r from-primary-50/50 to-primary-100/50 rounded-xl p-4 mb-4 border border-primary-100/50">
          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">课程方向</label>
              <div className="flex flex-wrap gap-2">
                <button
                  className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-all duration-200 ${
                    filters.direction === 0
                      ? 'bg-primary-500/10 text-primary-600 border border-primary-300 shadow-sm'
                      : 'bg-white text-gray-600 border border-gray-200 hover:border-primary-300 hover:bg-primary-50'
                  }`}
                  onClick={() => setFilters({ ...filters, direction: 0, tag: 0 })}
                >
                  全部
                </button>
                {geektimeDirection.map((item) => (
                  <button
                    key={item.value}
                    className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-all duration-200 ${
                      filters.direction === item.value
                        ? 'bg-primary-500/10 text-primary-600 border border-primary-300 shadow-sm'
                        : 'bg-white text-gray-600 border border-gray-200 hover:border-primary-300 hover:bg-primary-50'
                    }`}
                    onClick={() => {
                      setFilters({ ...filters, direction: item.value, tag: 0 })
                      setGeektimeCategory(
                        geektimeDirection.map((d) =>
                          d.value === item.value ? d : { ...d, children: [] }
                        )
                      )
                    }}
                  >
                    {item.label}
                  </button>
                ))}
              </div>
            </div>

            {filters.direction !== 0 && (
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">课程分类</label>
                <div className="flex flex-wrap gap-2">
                  <button
                    className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-all duration-200 ${
                      filters.tag === 0
                        ? 'bg-primary-500/10 text-primary-600 border border-primary-300 shadow-sm'
                        : 'bg-white text-gray-600 border border-gray-200 hover:border-primary-300 hover:bg-primary-50'
                    }`}
                    onClick={() => setFilters({ ...filters, tag: 0 })}
                  >
                    全部
                  </button>
                  {geektimeCategory
                    .find((d) => d.value === filters.direction)
                    ?.children?.slice(0, showAllCategories ? undefined : 10)
                    .map((item: any) => (
                      <button
                        key={item.value}
                        className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-all duration-200 ${
                          filters.tag === item.value
                            ? 'bg-primary-500/10 text-primary-600 border border-primary-300 shadow-sm'
                            : 'bg-white text-gray-600 border border-gray-200 hover:border-primary-300 hover:bg-primary-50'
                        }`}
                        onClick={() => setFilters({ ...filters, tag: item.value })}
                      >
                        {item.label}
                      </button>
                    ))}
                  {geektimeCategory.find((d) => d.value === filters.direction)?.children?.length > 10 && (
                    <button
                      className="px-3 py-1.5 rounded-lg text-sm font-medium text-primary-600 hover:bg-primary-50 transition-colors"
                      onClick={() => setShowAllCategories(!showAllCategories)}
                    >
                      {showAllCategories ? '收起' : '展开更多'}
                    </button>
                  )}
                </div>
              </div>
            )}

            <div className="flex flex-wrap gap-3 items-center">
              <Select
                options={productTypeOptions}
                value={filters.product_type}
                onChange={(e) => setFilters({ ...filters, product_type: Number(e.target.value) })}
                className="w-32"
              />
              <Select
                options={productFormOptions}
                value={filters.product_form}
                onChange={(e) => setFilters({ ...filters, product_form: Number(e.target.value) })}
                className="w-32"
              />
              <Select
                options={productStatusOptions}
                value={filters.xstatus}
                onChange={(e) => setFilters({ ...filters, xstatus: Number(e.target.value) })}
                className="w-32"
              />
              <Input
                placeholder="搜索课程名称"
                value={filters.keywords}
                onChange={(e) => setFilters({ ...filters, keywords: e.target.value })}
                onKeyDown={(e) => e.key === 'Enter' && loadData()}
                className="w-48"
              />
            </div>
          </div>
        </div>

        {selectedItems.size > 0 && (
          <div className="bg-primary-50 rounded-lg p-3 mb-4 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-2">
            <span className="text-sm text-primary-600">
              已选择 <span className="font-semibold">{selectedItems.size}</span> 个课程
            </span>
            <div className="flex gap-2">
              <div className="relative group">
                <Button size="sm" variant="light" onClick={handleBatchCollect} className="!p-2">
                  <Heart size={14} />
                </Button>
                <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[150px] text-center pointer-events-none z-20">
                  批量收藏
                </div>
              </div>
              <div className="relative group">
                <Button size="sm" variant="light" onClick={() => handleBatchExport('markdown')} className="!p-2">
                  <FileText size={14} />
                </Button>
                <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[150px] text-center pointer-events-none z-20">
                  批量导出Markdown
                </div>
              </div>
              {isAdmin && (
                <>
                  <div className="relative group">
                    <Button size="sm" variant="light" onClick={handleBatchRetry} className="!p-2">
                      <RefreshCw size={14} />
                    </Button>
                    <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[150px] text-center pointer-events-none z-20">
                      批量重新下载
                    </div>
                  </div>
                  <div className="relative group">
                    <Button size="sm" variant="danger" onClick={handleBatchDelete} className="!p-2">
                      <Trash2 size={14} />
                    </Button>
                    <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[150px] text-center pointer-events-none z-20">
                      批量删除
                    </div>
                  </div>
                </>
              )}
            </div>
          </div>
        )}

        {loading ? (
          <div className="flex justify-center py-20">
            <Spinner />
          </div>
        ) : (
          <>
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {items.map((item, index) => (
                <TaskCard
                  key={item.task_id}
                  item={item}
                  index={index}
                  isSelected={selectedItems.has(item.task_id)}
                  isAdmin={isAdmin}
                  geektimeDirection={geektimeDirection}
                  onToggleSelect={toggleSelect}
                  onOpenLesson={openLessonDrawer}
                  onExport={handleExport}
                  onRetry={handleRetry}
                  onDelete={handleDelete}
                  onCollect={handleSingleCollect}
                  isExporting={pdfDownloading}
                  isCollected={item.is_collected}
                />
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
          <p className="text-sm text-gray-600">确定要删除选中的课程吗？</p>
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
          setRetryParams({})
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
                setRetryParams({})
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

      <Modal
        isOpen={showCollectModal}
        onClose={() => {
          setShowCollectModal(false)
          setCollectCategoryValue(0)
        }}
        title="批量收藏"
        size="sm"
      >
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">分类</label>
            <Select
              options={[{ label: '请选择分类', value: 0 }, ...collectCategory.filter((c: any) => c.value !== 0)]}
              value={collectCategoryValue}
              onChange={(e) => setCollectCategoryValue(Number(e.target.value))}
              className="w-full"
            />
          </div>
          <div className="flex gap-3">
            <Button
              variant="secondary"
              onClick={() => {
                setShowCollectModal(false)
                setCollectCategoryValue(0)
              }}
              className="flex-1"
            >
              关闭
            </Button>
            <Button onClick={handleCollectConfirm} disabled={!collectCategoryValue} className="flex-1">
              保存
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
          // 关闭课程抽屉后刷新列表课程进度
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
