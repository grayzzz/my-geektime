import React, { useState, useEffect, useRef } from 'react'
import { Button, Input, Select, Spinner, Drawer, Pagination } from '@/components/ui'
import { TaskItem, TaskInfo, getTaskInfo } from '@/api/task'
import { ExternalLink } from 'lucide-react'
import { LessonDetail } from './LessonDetail'

const productStatusOptions = [
  { label: '全部状态', value: 0 },
  { label: '排队中', value: 1 },
  { label: '处理中', value: 2 },
  { label: '已缓存', value: 3 },
  { label: '异常', value: 4 },
]

interface LessonFilters {
  xstatus: number
  keywords: string
}

interface LessonDrawerProps {
  showLessonDrawer: boolean
  selectedTask: TaskItem | null
  lessonList: any[]
  lessonLoading: boolean
  lessonTotal: number
  lessonPage: number
  lessonFilters: LessonFilters
  setLessonFilters: (filters: LessonFilters) => void
  setShowLessonDrawer: (show: boolean) => void
  handleLessonPageChange: (page: number) => void
  handleLessonFilter: () => void
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

export const LessonDrawer: React.FC<LessonDrawerProps> = ({
  showLessonDrawer,
  selectedTask,
  lessonList,
  lessonLoading,
  lessonTotal,
  lessonPage,
  lessonFilters,
  setLessonFilters,
  setShowLessonDrawer,
  handleLessonPageChange,
  handleLessonFilter,
}) => {
  const [showDetail, setShowDetail] = useState(false)
  const [selectedLessonId, setSelectedLessonId] = useState<string | null>(null)
  const [currentLessonGlobalIndex, setCurrentLessonGlobalIndex] = useState(0)
  const [taskInfo, setTaskInfo] = useState<TaskInfo | null>(null)
  const [loadingInfo, setLoadingInfo] = useState(false)
  
  // 使用 useRef 来跟踪是否已经加载过数据
  const prevTaskIdRef = useRef<string | null>(null)

  useEffect(() => {
    if (showLessonDrawer && selectedTask?.task_id) {
      // 检查是否已经加载过相同的数据
      if (prevTaskIdRef.current === selectedTask.task_id) {
        return
      }
      
      prevTaskIdRef.current = selectedTask.task_id
      loadTaskInfo()
    } else {
      setTaskInfo(null)
      prevTaskIdRef.current = null
    }
  }, [showLessonDrawer, selectedTask?.task_id])

  const loadTaskInfo = async () => {
    if (!selectedTask?.task_id) return
    setLoadingInfo(true)
    try {
      const res = await getTaskInfo(selectedTask.task_id)
      setTaskInfo(res.task)
    } catch (error) {
      console.error('Failed to load task info', error)
    } finally {
      setLoadingInfo(false)
    }
  }

  const handleLessonClick = (lessonId: string, globalIndex: number) => {
    setSelectedLessonId(lessonId)
    setCurrentLessonGlobalIndex(globalIndex)
    setShowDetail(true)
  }

  const handlePrev = async () => {
    const prevGlobalIndex = currentLessonGlobalIndex - 1
    if (prevGlobalIndex >= 0) {
      // 计算上一页的页码
      const prevPage = Math.floor(prevGlobalIndex / 10) + 1
      
      // 如果上一个章节不在当前页，需要先加载对应页的数据
      if (prevPage !== lessonPage) {
        handleLessonPageChange(prevPage)
        // 等待数据加载完成后，找到对应的章节
        setTimeout(() => {
          const indexInPage = prevGlobalIndex % 10
          if (lessonList[indexInPage]) {
            setSelectedLessonId(lessonList[indexInPage].task_id)
            setCurrentLessonGlobalIndex(prevGlobalIndex)
          }
        }, 500)
      } else {
        // 在当前页内切换
        const indexInPage = prevGlobalIndex % 10
        if (lessonList[indexInPage]) {
          setSelectedLessonId(lessonList[indexInPage].task_id)
          setCurrentLessonGlobalIndex(prevGlobalIndex)
        }
      }
    }
  }

  const handleNext = async () => {
    const nextGlobalIndex = currentLessonGlobalIndex + 1
    if (nextGlobalIndex < lessonTotal) {
      // 计算下一页的页码
      const nextPage = Math.floor(nextGlobalIndex / 10) + 1
      
      // 如果下一个章节不在当前页，需要先加载对应页的数据
      if (nextPage !== lessonPage) {
        handleLessonPageChange(nextPage)
        // 等待数据加载完成后，找到对应的章节
        setTimeout(() => {
          const indexInPage = nextGlobalIndex % 10
          if (lessonList[indexInPage]) {
            setSelectedLessonId(lessonList[indexInPage].task_id)
            setCurrentLessonGlobalIndex(nextGlobalIndex)
          }
        }, 500)
      } else {
        // 在当前页内切换
        const indexInPage = nextGlobalIndex % 10
        if (lessonList[indexInPage]) {
          setSelectedLessonId(lessonList[indexInPage].task_id)
          setCurrentLessonGlobalIndex(nextGlobalIndex)
        }
      }
    }
  }

  if (!showLessonDrawer || !selectedTask) return null

  return (
    <>
      <Drawer
        isOpen={showLessonDrawer}
        onClose={() => setShowLessonDrawer(false)}
        title="课程详情"
        size="xl"
      >
        <div className="space-y-6">
          {/* Course Header */}
          {taskInfo && (
            <div className="flex items-start gap-4 pb-4 border-b">
              <img
                src={taskInfo.cover || selectedTask?.cover}
                alt={taskInfo.task_name || selectedTask?.task_name}
                className="w-32 h-32 rounded-lg object-cover shadow-md"
              />
              <div className="flex-1">
                <h3 className="text-2xl font-bold text-gray-800 mb-2">{taskInfo.task_name || selectedTask?.task_name}</h3>
                <p className="text-gray-600 mb-3">{taskInfo.subtitle || selectedTask?.subtitle}</p>
                <div className="grid grid-cols-2 gap-2 text-sm">
                  <div>
                    <span className="text-gray-500">作者: </span>
                    <span className="font-medium">{taskInfo.author?.name || selectedTask?.author?.name || '-'}</span>
                  </div>
                  <div>
                    <span className="text-gray-500">ID: </span>
                    <span className="font-medium">{taskInfo.task_id || selectedTask?.task_id}</span>
                  </div>
                  <div>
                    <span className="text-gray-500">课程数: </span>
                    <span className="font-medium">{taskInfo.article?.count || selectedTask?.article?.count || 0} 讲</span>
                  </div>
                  <div>
                    <span className="text-gray-500">完结: </span>
                    <span className="font-medium">{taskInfo.is_finish ? '是' : '否'}</span>
                  </div>
                  <div>
                    <span className="text-gray-500">音频: </span>
                    <span className="font-medium">{taskInfo.is_audio ? '支持' : '-'}</span>
                  </div>
                  <div>
                    <span className="text-gray-500">视频: </span>
                    <span className="font-medium">{taskInfo.is_video ? '支持' : '-'}</span>
                  </div>
                </div>
              </div>
            </div>
          )}

          {/* Author Introduction */}
          {(taskInfo?.author?.intro || selectedTask?.author?.intro) && (
            <div className="bg-gradient-to-r from-primary-50 to-primary-100 rounded-lg p-4">
              <h4 className="font-semibold text-gray-700 mb-2 flex items-center">
                <span className="w-1 h-5 bg-primary-400 rounded mr-2"></span>
                作者简介
              </h4>
              <p className="text-gray-600 text-sm leading-relaxed">{taskInfo?.author?.intro || selectedTask?.author?.intro}</p>
            </div>
          )}

          {/* Course Introduction */}
          {(taskInfo?.intro_html || loadingInfo) && (
            <div>
              <h4 className="font-semibold text-gray-700 mb-3 flex items-center">
                <span className="w-1 h-5 bg-primary-400 rounded mr-2"></span>
                课程介绍
              </h4>
              {loadingInfo ? (
                <div className="flex justify-center py-4">
                  <Spinner size="sm" />
                </div>
              ) : (
                <div 
                  className="text-sm text-gray-600 leading-relaxed space-y-2"
                  dangerouslySetInnerHTML={{ __html: taskInfo?.intro_html || '' }}
                />
              )}
            </div>
          )}

          {/* Filter Bar */}
          <div className="bg-gray-50 rounded-lg p-4">
            <div className="flex flex-wrap gap-3 items-center">
              <Input
                placeholder="搜索关键字"
                value={lessonFilters.keywords}
                onChange={(e) => setLessonFilters({ ...lessonFilters, keywords: e.target.value })}
                onKeyDown={(e) => e.key === 'Enter' && handleLessonFilter()}
                className="w-36 sm:w-48"
              />
              <Select
                options={productStatusOptions}
                value={lessonFilters.xstatus}
                onChange={(e) => setLessonFilters({ ...lessonFilters, xstatus: Number(e.target.value) })}
                className="w-28 sm:w-36"
              />
              <Button size="sm" onClick={handleLessonFilter}>
                搜索
              </Button>
              {selectedTask.redirect && (
                <Button
                  size="sm"
                  variant="light"
                  onClick={() => window.open(selectedTask.redirect, '_blank')}
                >
                  <ExternalLink size={14} className="mr-1" />
                  源站
                </Button>
              )}
            </div>
          </div>

          {/* Lessons List */}
          <div>
            {lessonLoading ? (
              <div className="flex justify-center py-8">
                <Spinner />
              </div>
            ) : lessonList.length > 0 ? (
              <>
                <div className="space-y-3">
                  <h4 className="font-semibold text-gray-700 mb-3">课程章节 ({lessonTotal} 讲)</h4>
                  <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3 max-h-96 overflow-y-auto">
                  {lessonList.map((lesson, index) => {
                    const globalIndex = (lessonPage - 1) * 10 + index
                    return (
                      <div
                        key={lesson.task_id}
                        onClick={() => handleLessonClick(lesson.task_id, globalIndex)}
                        className="border rounded-lg p-3 hover:border-primary-300 hover:bg-primary-50 transition-all cursor-pointer"
                      >
                        <div className="flex items-start gap-2">
                          <div className="w-8 h-8 rounded bg-primary-100 flex items-center justify-center text-primary-600 text-xs font-bold flex-shrink-0">
                            {String(globalIndex + 1).padStart(2, '0')}
                          </div>
                          <div className="flex-1 min-w-0">
                            <h5 className="text-sm font-medium text-gray-800 truncate">
                              {lesson.task_name}
                            </h5>
                            {lesson.subtitle && lesson.subtitle !== '无' && (
                              <p className="text-xs text-gray-500 mt-1 line-clamp-2">
                                {lesson.subtitle}
                              </p>
                            )}
                            <div className="flex items-center gap-2 mt-2 text-xs text-gray-400">
                              {lesson.statistics?.items && Object.entries(lesson.statistics.items).map(([key, count]) => {
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
                          </div>
                        </div>
                      </div>
                    )
                  })}
                </div>
                </div>
                {lessonTotal > 10 && (
                  <div className="flex justify-center pt-4">
                    <Pagination
                      total={lessonTotal}
                      pageSize={10}
                      current={lessonPage}
                      onChange={handleLessonPageChange}
                    />
                  </div>
                )}
              </>
            ) : (
              <div className="flex flex-col items-center justify-center py-20 text-gray-500">
                <div className="text-lg">暂无数据</div>
              </div>
            )}
          </div>
        </div>
      </Drawer>

      <LessonDetail
        key={selectedLessonId}
        show={showDetail}
        taskId={selectedLessonId}
        lessonList={lessonList}
        currentIndex={currentLessonGlobalIndex % 10}
        onClose={() => setShowDetail(false)}
        onPrev={handlePrev}
        onNext={handleNext}
        hasPrev={currentLessonGlobalIndex > 0}
        hasNext={currentLessonGlobalIndex < lessonTotal - 1}
      />
    </>
  )
}
