import React, { useEffect, useState, useRef } from 'react'
import { Button } from '@/components/ui'
import { TaskItem } from '@/api/task'
import { Trash2, List, Download, Book, Sparkles, ExternalLink, Heart, FileText, FileDown } from 'lucide-react'

interface TaskCardProps {
  item: TaskItem
  index: number
  isSelected: boolean
  isAdmin: boolean
  geektimeDirection: any[]
  onToggleSelect: (id: string) => void
  onOpenLesson: (item: TaskItem) => void
  onExport: (pid: string, type: string, mode?: string, taskName?: string) => void
  onRetry: (pid?: string, ids?: string[]) => void
  onDelete: (ids: string[]) => void
  onCollect: (id: string) => void
  isExporting?: boolean
  isCollected?: boolean
}

const productTypeOptions = [
  { label: '全部类型', value: 0 },
  { label: '体系课', value: 1 },
  { label: '公开课', value: 4 },
  { label: '线下大会', value: 5 },
  { label: '社区课', value: 6 },
  { label: '每日一课', value: 19 },
  { label: '大厂案例', value: 20 },
]

const getTypeText = (type: number) => {
  const item = productTypeOptions.find((o) => o.value === type)
  return item?.label || '-'
}

const getStatusText = (status: number) => {
  const map: Record<number, string> = {
    0: '未开始',
    1: '排队中',
    2: '处理中',
    3: '已缓存',
    4: '异常',
  }
  return map[status] || '-'
}

export const TaskCard: React.FC<TaskCardProps> = ({
  item,
  index: _index,
  isSelected,
  isAdmin,
  geektimeDirection,
  onToggleSelect,
  onOpenLesson,
  onExport,
  onRetry,
  onDelete,
  onCollect,
  isExporting,
  isCollected,
}) => {
  const prevCollectedRef = useRef(isCollected)
  const [heartAnimating, setHeartAnimating] = useState(false)

  // 依赖 [isCollected]：需要在每次状态变化时检测 false→true 过渡以触发动画
  useEffect(() => {
    if (isCollected && !prevCollectedRef.current) {
      setHeartAnimating(true)
      const timer = setTimeout(() => setHeartAnimating(false), 400)
      return () => clearTimeout(timer)
    }
    prevCollectedRef.current = isCollected
  }, [isCollected])

  const getDirectionText = (group: number) => {
    const dirItem = geektimeDirection.find((o: any) => Number(o.value) === group)
    return dirItem?.label || '-'
  }

  return (
    <div
      key={item.task_id}
      className={`relative bg-white rounded-xl border-2 transition-all duration-300 cursor-pointer hover:shadow-lg ${
        isSelected
          ? 'border-purple-500 shadow-lg shadow-purple-500/20'
          : 'border-gray-100 hover:border-purple-300'
      }`}
      onClick={() => onToggleSelect(item.task_id)}
    >
      {isSelected && (
        <div className="absolute top-2 right-2 w-6 h-6 bg-primary-500 rounded-full flex items-center justify-center z-10">
          <svg className="w-4 h-4 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
          </svg>
        </div>
      )}
      <div className="p-4">
        <div className="flex items-start gap-3">
          <div className="relative flex-shrink-0">
            <img
              src={item.cover}
              alt={item.task_name}
              className="w-20 h-20 rounded-xl object-cover shadow-md"
            />
            {(item.is_audio || item.is_video) && (
              <div className="absolute -bottom-1 -right-1 flex gap-1">
                {item.is_audio && (
                  <span className="px-1.5 py-0.5 bg-primary-500 text-white text-xs rounded-md">音</span>
                )}
                {item.is_video && (
                  <span className="px-1.5 py-0.5 bg-primary-600 text-white text-xs rounded-md">视</span>
                )}
              </div>
            )}
          </div>
          <div className="flex-1 min-w-0">
            <h3 className="font-semibold text-gray-800 truncate leading-tight">
              {item.task_name}
            </h3>
            <p className="text-sm text-gray-500 truncate mt-1">
              {item.subtitle}
            </p>
            <div className="flex items-center gap-2 mt-2">
              <span className="text-xs text-gray-400">{item.author?.name}</span>
              {item.is_finish && (
                <span className="px-1.5 py-0.5 bg-green-100 text-green-600 text-xs rounded">已完结</span>
              )}
            </div>
          </div>
        </div>
        <div className="mt-4 pt-3 border-t border-gray-100">
          <div className="grid grid-cols-2 gap-2 text-xs">
            <div className="flex items-center gap-1">
              <span className="text-gray-400">类型:</span>
              <span className="text-gray-600">{getTypeText(item.other_type)}</span>
            </div>
            <div className="flex items-center gap-1">
              <span className="text-gray-400">方向:</span>
              <span className="text-gray-600">{getDirectionText(item.other_group)}</span>
            </div>
            <div className="flex items-center gap-1">
              <span className="text-gray-400">章节:</span>
              <span className="text-gray-600">{item.article?.count || 0} 讲</span>
            </div>
            <div className="flex items-center gap-1">
              <span className="text-gray-400">状态:</span>
              <span className={`px-1.5 py-0.5 rounded text-xs ${
                item.status === 3 ? 'bg-green-100 text-green-600' :
                item.status === 4 ? 'bg-red-100 text-red-600' :
                item.status === 2 ? 'bg-blue-100 text-blue-600' :
                item.status === 1 ? 'bg-yellow-100 text-yellow-600' :
                'bg-gray-100 text-gray-600'
              }`}>
                {getStatusText(item.status)}
              </span>
            </div>
          </div>
          {item.statistics?.items && (
            <div className="mt-2 flex flex-wrap gap-2 text-xs">
              {Object.entries(item.statistics.items).map(([key, count]) => {
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
        {item.course_progress && item.course_progress.total_count > 0 && (
          <div className="mt-3 pt-3 border-t border-gray-100">
            <div className="flex items-center justify-between mb-1.5">
              <span className="text-xs text-gray-400">学习进度:</span>
              <div className="flex items-center gap-2">
                <span className="text-xs text-gray-600">
                  {item.course_progress.finished_count}/{item.course_progress.total_count} 讲
                </span>
                <span className={`text-xs font-semibold ${item.course_progress.percent >= 100 ? 'text-green-600' : 'text-primary-600'}`}>
                  {item.course_progress.percent >= 100
                    ? '100%'
                    : `${(Math.round(item.course_progress.percent * 10) / 10)}%`}
                </span>
                {item.course_progress.percent >= 100 && (
                  <span className="px-1.5 py-0.5 bg-green-100 text-green-600 text-xs rounded">已学完</span>
                )}
              </div>
            </div>
            <div className="w-full h-2 bg-gray-100 rounded-full overflow-hidden">
              <div
                className={`h-full rounded-full transition-all ${item.course_progress.percent >= 100 ? 'bg-green-500' : 'bg-primary-500'}`}
                style={{ width: `${Math.min(100, Math.max(0, item.course_progress.percent))}%` }}
              />
            </div>
            {item.course_progress.last_task_name ? (
              <p className="text-xs truncate mt-1.5" title={item.course_progress.last_task_name}>
                <span className="text-gray-400">上次学到:</span>{' '}
                <span className="text-gray-600">{item.course_progress.last_task_name}</span>
              </p>
            ) : (
              <p className="text-xs mt-1.5">
                <span className="text-gray-400">上次学到:</span>{' '}
                <span className="text-gray-600">尚未开始</span>
              </p>
            )}
          </div>
        )}
        <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <div className="relative group">
              <Button
                variant="light"
                size="sm"
                onClick={(e) => {
                  e.stopPropagation()
                  onOpenLesson(item)
                }}
                className="!p-2"
              >
                <List size={14} />
              </Button>
              <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[150px] text-center pointer-events-none z-20">
                课程
              </div>
            </div>
            <div className="relative group">
              <Button
                variant="light"
                size="sm"
                disabled={isExporting}
                onClick={(e) => {
                  e.stopPropagation()
                  onExport(item.task_id, 'pdf', 'course', item.task_name)
                }}
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
                onClick={(e) => {
                  e.stopPropagation()
                  onExport(item.task_id, 'markdown', undefined, item.task_name)
                }}
                className="!p-2"
              >
                <FileText size={14} />
              </Button>
              <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[150px] text-center pointer-events-none z-20">
                导出Markdown
              </div>
            </div>
            {item.doc !== undefined ? (
              <div className="relative group">
                <Button
                  variant="success"
                  size="sm"
                  onClick={(e) => {
                    e.stopPropagation()
                    window.open(item.doc, '_blank')
                  }}
                  className="!p-2"
                >
                  <Book size={14} />
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
                  onClick={(e) => {
                    e.stopPropagation()
                    onExport(item.task_id, 'docsite', undefined, item.task_name)
                  }}
                  className="!p-2"
                >
                  <Sparkles size={14} />
                </Button>
                <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[150px] text-center pointer-events-none z-20">
                  生成文档
                </div>
              </div>
            )}
            {isAdmin && (
              <div className="relative group">
                <Button
                  variant="light"
                  size="sm"
                  onClick={(e) => {
                    e.stopPropagation()
                    onRetry(item.task_id)
                  }}
                  className="!p-2"
                >
                  <Download size={14} />
                </Button>
                <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[150px] text-center pointer-events-none z-20">
                  下载资源
                </div>
              </div>
            )}
            {item.redirect !== undefined && (
              <div className="relative group">
                <Button
                  variant="light"
                  size="sm"
                  onClick={(e) => {
                    e.stopPropagation()
                    window.open(item.redirect, '_blank')
                  }}
                  className="!p-2"
                >
                  <ExternalLink size={14} />
                </Button>
                <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[150px] text-center pointer-events-none z-20">
                  查看源站
                </div>
              </div>
            )}
            <div className="relative group">
              <Button
                variant={isCollected ? "light" : "light"}
                size="sm"
                disabled={isCollected}
                onClick={(e) => {
                  e.stopPropagation()
                  onCollect(item.task_id)
                }}
                aria-label={isCollected ? '已收藏' : '收藏'}
                title={isCollected ? '已收藏' : '收藏'}
                className={`!p-2 transition-all duration-200 ${heartAnimating ? 'heart-pop' : ''} ${isCollected
                  ? '!bg-rose-50 !text-rose-600 !border-rose-200 hover:!bg-rose-100 hover:!border-rose-300'
                  : 'hover:-translate-y-0.5 hover:shadow-sm hover:!border-gray-300'
                }`}
              >
                <Heart
                  size={14}
                  fill={isCollected ? "currentColor" : "none"}
                  className="transition-transform duration-200 group-hover:scale-110"
                />
              </Button>
              <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[150px] text-center pointer-events-none z-20">
                {isCollected ? '已收藏' : '收藏'}
              </div>
            </div>
          </div>
          {isAdmin && (
            <div className="relative group">
              <Button
                variant="danger"
                size="sm"
                onClick={(e) => {
                  e.stopPropagation()
                  onDelete([item.task_id])
                }}
                className="!p-2"
              >
                <Trash2 size={14} />
              </Button>
              <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[150px] text-center pointer-events-none z-20">
                删除
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
