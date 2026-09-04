import React, { useEffect, useState, useRef, useCallback } from 'react'
import { X, ExternalLink, FileText, FileDown, ChevronLeft, ChevronRight, Maximize2, Rocket, MessageCircle, ThumbsUp, Sparkles } from 'lucide-react'
import { getTaskInfo, getArticleComments, getCommentDiscussions, type TaskInfoResponse, downloadPdfBlob } from '@/api/task'
import { getProgress, saveProgress, type ProgressItem } from '@/api/progress'
import { downloadFileFromBlob, showErrorMessage } from '@/utils/request'
import type Hls from 'hls.js'
import { AIPanel } from './AIPanel'

interface LessonDetailProps {
  show: boolean
  taskId: string | null
  lessonList: any[]
  currentIndex: number
  onClose: () => void
  onPrev: () => void
  onNext: () => void
  hasPrev: boolean
  hasNext: boolean
}

export const LessonDetail: React.FC<LessonDetailProps> = ({
  show,
  taskId,
  onClose,
  onPrev,
  onNext,
  hasPrev,
  hasNext,
}) => {
  // ===== 学习进度上报（定义在顶部，供 handleClose / handleVideoTimeUpdate 引用）=====
  const flushProgress = useCallback(() => {
    const p = progressRef.current
    if (!p || !p.task_id) return
    const payload = {
      task_id: p.task_id,
      task_pid: p.task_pid,
      task_type: p.task_type,
      position: p.position || 0,
      scroll_position: p.scroll_position || 0,
      duration: p.duration || 0,
      is_finished: !!p.is_finished,
    }
    const doSave = (retry = true) => {
      saveProgress(payload)
        .catch(() => {
          // 网络抖动/瞬时失败时重试一次，避免进度丢失
          if (retry) {
            setTimeout(() => doSave(false), 800)
          }
        })
    }
    doSave()
    lastScrollReportRef.current = Date.now()
    lastVideoReportRef.current = Date.now()
  }, [])

  const reportScrollProgress = useCallback(() => {
    const content = contentRef.current
    if (!content) return
    const p = progressRef.current
    if (!p) return
    p.scroll_position = content.scrollTop
    // 读完判定（满足任一）：
    // 1. 内容不足一屏（无需滚动）
    // 2. 已滚动超过可滚动高度的 95%
    // 3. 距离底部 50px 内
    const clientH = content.clientHeight
    const scrollH = content.scrollHeight
    const scrollable = scrollH - clientH
    const atBottom =
      scrollable <= 0 ||
      content.scrollTop + clientH >= scrollH - 50 ||
      (scrollable > 0 && content.scrollTop >= scrollable * 0.95)
    if (atBottom) {
      // 有图文内容滚动到底即学完（不管视频播没播，也不管音频播没播）
      p.is_finished = true
      flushProgress()
      return
    }
    // 1.5s 防抖
    const now = Date.now()
    if (now - lastScrollReportRef.current >= 1500) {
      flushProgress()
    }
  }, [flushProgress])

  const reportVideoProgress = useCallback(() => {
    const video = videoRef.current
    const p = progressRef.current
    if (!video || !p) return
    p.position = Math.floor(video.currentTime)
    if (video.duration && isFinite(video.duration)) {
      p.duration = Math.floor(video.duration)
    }
    // 距上次上报 >5s 才写
    const now = Date.now()
    if (now - lastVideoReportRef.current >= 5000) {
      flushProgress()
    }
  }, [flushProgress])

  // 关闭时 flush 最后一次未上报的进度
  const handleClose = useCallback(() => {
    if (scrollDebounceRef.current) {
      clearTimeout(scrollDebounceRef.current)
      scrollDebounceRef.current = null
    }
    // 读完兜底：关闭瞬间再检查一次是否已滚到底/内容不足一屏，
    // 防止最后的 scroll 事件因时序问题没触发 atBottom 判定
    const content = contentRef.current
    const p = progressRef.current
    if (content && p && !p.is_finished) {
      const clientH = content.clientHeight
      const scrollH = content.scrollHeight
      const scrollable = scrollH - clientH
      const atBottom =
        scrollable <= 0 ||
        content.scrollTop + clientH >= scrollH - 50 ||
        (scrollable > 0 && content.scrollTop >= scrollable * 0.95)
      if (atBottom) {
        p.is_finished = true
        p.scroll_position = Math.max(p.scroll_position || 0, content.scrollTop)
      }
    }
    flushProgress()
    onClose()
  }, [flushProgress, onClose])
  const [loading, setLoading] = useState(false)
  const [aiOpen, setAiOpen] = useState(false)
  const [pdfDownloading, setPdfDownloading] = useState(false)
  const [taskInfoResponse, setTaskInfoResponse] = useState<TaskInfoResponse | null>(null)
  const taskInfo = taskInfoResponse?.task || null
  const article = taskInfoResponse?.article || null
  const playUrl = taskInfoResponse?.play_url
  const [showFloatingPlayer, setShowFloatingPlayer] = useState(false)
  const [floatingPosition, setFloatingPosition] = useState({ x: 20, y: 80 })
  const [floatingSize, setFloatingSize] = useState({ width: 300, height: 220 })
  const [isDragging, setIsDragging] = useState(false)
  const [dragOffset, setDragOffset] = useState({ x: 0, y: 0 })
  const [isResizing, setIsResizing] = useState(false)
  const [resizeStart, setResizeStart] = useState({ x: 0, y: 0, width: 0, height: 0 })
  const [drawerWidth, setDrawerWidth] = useState<number>(1024) // max-w-4xl = 1024px
  const [isMobile, setIsMobile] = useState(() => typeof window !== 'undefined' ? window.innerWidth < 768 : false)
  const isMobileRef = useRef(isMobile)
  isMobileRef.current = isMobile
  const isDrawerResizing = useRef(false)
  const drawerResizeStartX = useRef(0)
  const drawerResizeStartWidth = useRef(0)
  const [showScrollTop, setShowScrollTop] = useState(false)
  const [videoState, setVideoState] = useState({ currentTime: 0, isPlaying: false })
  const [comments, setComments] = useState<any[]>([])
  const [commentsLoading, setCommentsLoading] = useState(false)
  const [commentsPage, setCommentsPage] = useState(1)
  const [commentsHasMore, setCommentsHasMore] = useState(false)
  const [commentsTotal, setCommentsTotal] = useState(0)
  const [expandedDiscussions, setExpandedDiscussions] = useState<Set<string>>(new Set())
  const [previewImage, setPreviewImage] = useState<string | null>(null)
  const videoContainerRef = useRef<HTMLDivElement>(null)
  const floatingPlayerRef = useRef<HTMLDivElement>(null)
  const contentRef = useRef<HTMLDivElement>(null)
  const videoRef = useRef<HTMLVideoElement>(null)
  const prevTaskIdRef = useRef<string | null>(null)
  // 进度上报相关
  const progressRef = useRef<ProgressItem | null>(null)
  const lastScrollReportRef = useRef(0)
  const lastVideoReportRef = useRef(0)
  const scrollDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const progressRestoredRef = useRef(false)

  useEffect(() => {
    const handleResize = () => setIsMobile(typeof window !== 'undefined' ? window.innerWidth < 768 : false)
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [])

  const loadComments = async (aid: string, page: number) => {
    console.log('loadComments called:', aid, page)
    try {
      setCommentsLoading(true)
      const res: any = await getArticleComments({ aid, page, perPage: 5 })
      console.log('comments response:', res)
      const items = res?.rows || res?.items || res?.data?.rows || res?.data?.items || []
      const total = res?.count || res?.total || 0
      if (page === 1) {
        setComments(items)
      } else {
        setComments(prev => [...prev, ...items])
      }
      setCommentsTotal(total)
      setCommentsHasMore(items.length >= 5)
      setCommentsPage(page)
    } catch (err) {
      console.error('获取评论失败', err)
    } finally {
      setCommentsLoading(false)
    }
  }

  const toggleDiscussions = async (commentId: string) => {
    const newExpanded = new Set(expandedDiscussions)
    if (newExpanded.has(commentId)) {
      newExpanded.delete(commentId)
      setExpandedDiscussions(newExpanded)
    } else {
      newExpanded.add(commentId)
      setExpandedDiscussions(newExpanded)
      await loadDiscussionsForComment(commentId, 1)
    }
  }

  const loadDiscussionsForComment = async (commentId: string, page: number) => {
    try {
      setComments(prev => prev.map(c => {
        if (c.id === commentId) {
          return { ...c, discussionsLoading: true }
        }
        return c
      }))
      const res: any = await getCommentDiscussions({
        target_id: commentId,
        target_type: 1,
        page,
        perPage: 10,
        use_likes_order: true
      })
      const items = res?.rows || res?.items || res?.data?.rows || res?.data?.items || []
      const hasMore = items.length >= 10
      setComments(prev => prev.map(c => {
        if (c.id === commentId) {
          const existingDiscussions = page === 1 ? [] : (c.discussions || [])
          return {
            ...c,
            discussions: [...existingDiscussions, ...items],
            discussionsPage: page,
            discussionsHasMore: hasMore,
            discussionsLoading: false
          }
        }
        return c
      }))
    } catch (err) {
      console.error('获取讨论失败', err)
      setComments(prev => prev.map(c => {
        if (c.id === commentId) {
          return { ...c, discussionsLoading: false }
        }
        return c
      }))
    }
  }

  // ESC 关闭图片预览
  useEffect(() => {
    if (!previewImage) return
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setPreviewImage(null)
    }
    window.addEventListener('keydown', handleKey)
    return () => window.removeEventListener('keydown', handleKey)
  }, [previewImage])

  const handleImageClick = useCallback((e: React.MouseEvent) => {
    const target = e.target as HTMLElement
    if (target.tagName === 'IMG') {
      const src = (target as HTMLImageElement).src
      if (src) {
        e.stopPropagation()
        setPreviewImage(src)
      }
    }
  }, [])

  const loadMoreDiscussions = (commentId: string) => {
    const comment = comments.find(c => c.id === commentId)
    if (comment) {
      loadDiscussionsForComment(commentId, (comment.discussionsPage || 1) + 1)
    }
  }

  useEffect(() => {
    console.log('LessonDetail useEffect - show:', show, 'taskId:', taskId)
    if (show && taskId) {
      // 检查是否已经加载过相同的数据
      if (prevTaskIdRef.current === taskId) {
        console.log('Skipping duplicate load for taskId:', taskId)
        return
      }
      
      prevTaskIdRef.current = taskId
      setLoading(true)
      setShowFloatingPlayer(false)
      setShowScrollTop(false)
      setComments([])
      setCommentsPage(1)
      // 重置进度状态（新章节）
      progressRef.current = null
      progressRestoredRef.current = false
      lastScrollReportRef.current = 0
      lastVideoReportRef.current = 0
      if (scrollDebounceRef.current) {
        clearTimeout(scrollDebounceRef.current)
        scrollDebounceRef.current = null
      }
      if (contentRef.current) {
        contentRef.current.scrollTop = 0
      }
      if (videoRef.current) {
        videoRef.current.currentTime = 0
        videoRef.current.pause()
      }
      getTaskInfo(taskId)
        .then(async (data) => {
          setTaskInfoResponse(data)
          const otherId = data.task?.other_id || data.article?.other_id || data.article?.id
          if (otherId) {
            loadComments(otherId, 1)
          }
          // 加载并恢复学习进度
          progressRestoredRef.current = false
          try {
            const saved = await getProgress(taskId)
            if (saved?.task_id) {
              progressRef.current = {
                task_id: taskId,
                task_pid: data.task?.task_pid || saved.task_pid,
                task_type: data.task?.task_type || saved.task_type,
                position: saved.position || 0,
                scroll_position: saved.scroll_position || 0,
                duration: saved.duration || 0,
                is_finished: saved.is_finished || false,
              }
              // 恢复滚动位置：文章内容加载后
              if (saved.scroll_position && saved.scroll_position > 0) {
                const content = contentRef.current
                if (content) {
                  // 等 DOM 渲染完成后恢复
                  setTimeout(() => {
                    content.scrollTop = saved.scroll_position || 0
                  }, 50)
                }
              }
              // 恢复视频进度：在 loadedmetadata 后设置（见 video 初始化 effect）
              if (saved.position && saved.position > 0) {
                const video = videoRef.current
                const dur = saved.duration || 0
                if (video && dur > 0 && saved.position < dur - 5) {
                  video.currentTime = saved.position
                }
              }
            } else {
              progressRef.current = {
                task_id: taskId,
                task_pid: data.task?.task_pid || '',
                task_type: data.task?.task_type || '',
                position: 0,
                scroll_position: 0,
                duration: 0,
                is_finished: false,
              }
            }
            progressRestoredRef.current = true
          } catch (err) {
            console.error('获取学习进度失败', err)
            progressRef.current = {
              task_id: taskId,
              task_pid: data.task?.task_pid || '',
              task_type: data.task?.task_type || '',
              position: 0,
              scroll_position: 0,
              duration: 0,
              is_finished: false,
            }
          }
        })
        .catch((err) => {
          console.error('获取章节详情失败', err)
        })
        .finally(() => {
          setLoading(false)
        })
    }
  }, [show, taskId])

  useEffect(() => {
    const handleScroll = () => {
      if (!videoContainerRef.current || !contentRef.current) return
      
      const container = videoContainerRef.current
      const rect = container.getBoundingClientRect()
      const contentElement = contentRef.current
      
      if (isMobileRef.current) {
        setShowFloatingPlayer(false) // 手机端禁用浮动播放器
      } else if (rect.bottom < 100) {
        setShowFloatingPlayer(true)
      } else if (rect.bottom > 200) {
        setShowFloatingPlayer(false)
      }

      if (contentElement.scrollTop > 300) {
        setShowScrollTop(true)
      } else {
        setShowScrollTop(false)
      }

      // 学习进度上报（滚动位置 + 滚到底完成）
      reportScrollProgress()
    }

    const contentElement = contentRef.current
    if (contentElement) {
      contentElement.addEventListener('scroll', handleScroll)
      return () => contentElement.removeEventListener('scroll', handleScroll)
    }
  }, [taskInfo])

  // 内容渲染完成后主动检查一次完成度（短内容无 scroll 事件时也能标记读完）
  useEffect(() => {
    if (!taskInfo || !article?.content) return
    const timer = setTimeout(() => {
      reportScrollProgress()
    }, 200)
    return () => clearTimeout(timer)
  }, [taskInfo, article?.content, reportScrollProgress])

  // 读完轮询兜底：图片懒加载等会让 scrollHeight 在滚动后变大，
  // 用户滚到"旧底"时判定可能失败，且图片加载完后不再触发 scroll 事件。
  // 每 2s 检查一次是否滚到底（最长 60s），命中即标记读完。
  useEffect(() => {
    if (!taskInfo || !article?.content) return
    let stop = false
    let count = 0
    let checkTimer: ReturnType<typeof setTimeout>
    const check = () => {
      if (stop) return
      const content = contentRef.current
      const p = progressRef.current
      if (content && p) {
        const clientH = content.clientHeight
        const scrollH = content.scrollHeight
        const scrollable = scrollH - clientH
        const atBottom =
          scrollable <= 0 ||
          content.scrollTop + clientH >= scrollH - 50 ||
          (scrollable > 0 && content.scrollTop >= scrollable * 0.95)
        if (atBottom) {
          p.is_finished = true
          flushProgress()
          return // 命中后停止轮询
        }
      }
      count += 1
      if (count >= 30) return // 60s 后停止
      checkTimer = setTimeout(check, 2000)
    }
    checkTimer = setTimeout(check, 1500)
    return () => {
      stop = true
      clearTimeout(checkTimer)
    }
  }, [taskInfo, article?.content, flushProgress])

  const scrollToTop = () => {
    if (contentRef.current) {
      contentRef.current.scrollTo({
        top: 0,
        behavior: 'smooth'
      })
      if (showFloatingPlayer) {
        saveVideoState()
        setTimeout(() => {
          setShowFloatingPlayer(false)
        }, 0)
      }
    }
  }

  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    if (!floatingPlayerRef.current) return
    setIsDragging(true)
    const rect = floatingPlayerRef.current.getBoundingClientRect()
    setDragOffset({
      x: e.clientX - rect.left,
      y: e.clientY - rect.top
    })
  }, [])

  const handleMouseMove = useCallback((e: MouseEvent) => {
    if (isDragging) {
      const newX = e.clientX - dragOffset.x
      const newY = e.clientY - dragOffset.y
      
      const maxX = window.innerWidth - floatingSize.width - 20
      const maxY = window.innerHeight - floatingSize.height - 20
      
      setFloatingPosition({
        x: Math.max(0, Math.min(newX, maxX)),
        y: Math.max(0, Math.min(newY, maxY))
      })
    }
    
    if (isResizing) {
      const deltaX = e.clientX - resizeStart.x
      const deltaY = e.clientY - resizeStart.y
      
      const newWidth = Math.max(200, Math.min(600, resizeStart.width + deltaX))
      const newHeight = Math.max(150, Math.min(500, resizeStart.height + deltaY))
      
      setFloatingSize({ width: newWidth, height: newHeight })
    }
  }, [isDragging, dragOffset, isResizing, resizeStart, floatingSize])

  const handleMouseUp = useCallback(() => {
    setIsDragging(false)
    setIsResizing(false)
  }, [])

  const handleResizeStart = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setIsResizing(true)
    setResizeStart({
      x: e.clientX,
      y: e.clientY,
      width: floatingSize.width,
      height: floatingSize.height
    })
  }, [floatingSize])

  const handleDrawerResizeStart = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    isDrawerResizing.current = true
    drawerResizeStartX.current = e.clientX
    drawerResizeStartWidth.current = drawerWidth

    const handleMouseMove = (moveEvent: MouseEvent) => {
      if (!isDrawerResizing.current) return
      const deltaX = drawerResizeStartX.current - moveEvent.clientX // 从左侧拖动，所以是反向
      const newWidth = drawerResizeStartWidth.current + deltaX
      const minWidth = 600
      const maxWidth = window.innerWidth * 0.9
      setDrawerWidth(Math.min(maxWidth, Math.max(minWidth, newWidth)))
    }

    const handleMouseUp = () => {
      isDrawerResizing.current = false
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', handleMouseUp)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
    }

    document.addEventListener('mousemove', handleMouseMove)
    document.addEventListener('mouseup', handleMouseUp)
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
  }, [drawerWidth])

  useEffect(() => {
    if (isDragging || isResizing || isDrawerResizing.current) {
      window.addEventListener('mousemove', handleMouseMove)
      window.addEventListener('mouseup', handleMouseUp)
      return () => {
        window.removeEventListener('mousemove', handleMouseMove)
        window.removeEventListener('mouseup', handleMouseUp)
      }
    }
  }, [isDragging, isResizing, handleMouseMove, handleMouseUp])

  const prevFloatingRef = useRef(showFloatingPlayer)
  const hlsRef = useRef<Hls | null>(null)
  
  useEffect(() => {
    if (prevFloatingRef.current !== showFloatingPlayer) {
      const video = videoRef.current
      if (video) {
        video.currentTime = videoState.currentTime
        if (videoState.isPlaying) {
          video.play().catch(() => {})
        }
      }
      prevFloatingRef.current = showFloatingPlayer
    }
  }, [showFloatingPlayer, videoState])

  const saveVideoState = useCallback(() => {
    const video = videoRef.current
    if (video) {
      setVideoState({
        currentTime: video.currentTime,
        isPlaying: !video.paused
      })
    }
  }, [])

  const handleVideoTimeUpdate = useCallback(() => {
    const video = videoRef.current
    if (video) {
      setVideoState(prev => ({
        ...prev,
        currentTime: video.currentTime
      }))
      // 学习进度：视频播放位置（节流上报）
      reportVideoProgress()
    }
  }, [reportVideoProgress])

  const handleVideoPlay = useCallback(() => {
    setVideoState(prev => ({ ...prev, isPlaying: true }))
  }, [])

  const handleVideoPause = useCallback(() => {
    setVideoState(prev => ({ ...prev, isPlaying: false }))
  }, [])

  const scrollToVideo = () => {
    if (videoContainerRef.current && contentRef.current) {
      contentRef.current.scrollTo({
        top: 0,
        behavior: 'smooth'
      })
      handleFloatingToggle(false)
    }
  }

  const getVideoSrc = () => {
    if (!taskInfo) return null
    if (playUrl) return playUrl
    if (article?.video?.hls_medias?.length) {
      const last = article.video.hls_medias[article.video.hls_medias.length - 1]
      return last.url
    }
    if (article?.video_preview?.medias?.length) {
      const last = article.video_preview.medias[article.video_preview.medias.length - 1]
      return last.url
    }
    if (article?.audio?.url) return article.audio.url
    return null
  }

  const videoSrc = getVideoSrc()
  const poster = article?.cover?.default
  
  useEffect(() => {
    const video = videoRef.current
    if (!video || !videoSrc) {
      console.log('Video init skipped:', { hasVideo: !!video, hasSrc: !!videoSrc })
      return
    }
    
    console.log('Initializing video player with src:', videoSrc)
    
    const destroyHls = () => {
      if (hlsRef.current) {
        hlsRef.current.destroy()
        hlsRef.current = null
      }
    }
    
    destroyHls()
    
    const initHls = async () => {
      if (!videoSrc.includes('.m3u8')) {
        console.log('Non-HLS video, setting src directly')
        video.src = videoSrc
        return
      }
      
      console.log('HLS video detected')
      
      try {
        const HlsModule = await import('hls.js')
        const Hls = HlsModule.default
        
        if (Hls.isSupported()) {
          console.log('Using hls.js for playback')
          const hls = new Hls({
            enableWorker: true,
            lowLatencyMode: true,
            debug: false
          })
          hlsRef.current = hls
          
          hls.on(Hls.Events.ERROR, (_event: any, data: any) => {
            console.error('HLS error:', data.type, data.details, data)
            if (data.fatal) {
              switch (data.type) {
                case Hls.ErrorTypes.NETWORK_ERROR:
                  console.error('Fatal network error, trying to recover...')
                  hls.startLoad()
                  break
                case Hls.ErrorTypes.MEDIA_ERROR:
                  console.error('Fatal media error, trying to recover...')
                  hls.recoverMediaError()
                  break
                default:
                  console.error('Fatal error, cannot recover')
                  destroyHls()
                  break
              }
            }
          })
          
          hls.loadSource(videoSrc)
          hls.attachMedia(video)
          
          hls.on(Hls.Events.MANIFEST_PARSED, () => {
            console.log('HLS manifest loaded successfully')
          })
        } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
          // 如果 hls.js 不支持（比如 Safari），再尝试原生 HLS
          console.warn('hls.js not supported, falling back to native HLS')
          video.src = videoSrc
        } else {
          console.error('No HLS support in this browser')
        }
      } catch (err) {
        console.error('Failed to load hls.js:', err)
        // 如果 hls.js 加载失败，尝试原生 HLS
        if (video.canPlayType('application/vnd.apple.mpegurl')) {
          video.src = videoSrc
        }
      }
    }
    
    initHls()
    
    return destroyHls
  }, [videoSrc, showFloatingPlayer])

  if (!show) return null

  const renderScrollTopButton = () => {
    if (!showScrollTop) return null

    return (
      <button
        onClick={scrollToTop}
        className="fixed bottom-4 right-4 sm:bottom-8 sm:right-8 w-10 h-10 sm:w-12 sm:h-12 rounded-full bg-white text-primary-600 border-2 border-primary-300 shadow-lg hover:bg-primary-50 hover:border-primary-400 transition-all duration-300 flex items-center justify-center z-[95] hover:scale-110"
        title="返回顶部"
      >
        <Rocket size={20} />
      </button>
    )
  }

  const handleFloatingToggle = (newState: boolean) => {
    saveVideoState()
    setTimeout(() => {
      setShowFloatingPlayer(newState)
    }, 0)
  }

  const renderVideoPlayer = () => {
    if (!videoSrc) return null

    if (showFloatingPlayer) {
      return (
        <div
          ref={floatingPlayerRef}
          className="fixed z-[100] bg-white rounded-lg shadow-2xl overflow-hidden cursor-move"
          style={{
            left: floatingPosition.x,
            top: floatingPosition.y,
            width: floatingSize.width,
            height: floatingSize.height,
          }}
          onMouseDown={handleMouseDown}
        >
          <div className="bg-gray-800 text-white px-3 py-2 flex items-center justify-between">
            <span className="text-sm font-medium truncate flex-1">
              {taskInfo?.task_name || '播放中'}
            </span>
            <div className="flex items-center gap-1">
              <button
                onClick={scrollToVideo}
                className="p-1 hover:bg-white/20 rounded transition-colors"
                title="返回原位置"
              >
                <Maximize2 size={14} />
              </button>
              <button
                onClick={() => handleFloatingToggle(false)}
                className="p-1 hover:bg-white/20 rounded transition-colors"
                title="关闭"
              >
                <X size={14} />
              </button>
            </div>
          </div>
          <div className="w-full bg-black flex items-center justify-center" style={{ height: 'calc(100% - 40px)' }}>
            <video
              ref={videoRef}
              poster={poster}
              controls
              className="w-full h-full object-contain"
              onTimeUpdate={handleVideoTimeUpdate}
              onPlay={handleVideoPlay}
              onPause={handleVideoPause}
              onLoadedMetadata={() => {
                // 恢复视频进度（HLS 就绪后）
                const p = progressRef.current
                const video = videoRef.current
                if (p && p.position && p.position > 0 && video && video.duration && p.position < video.duration - 5) {
                  video.currentTime = p.position
                }
              }}
              onEnded={() => {
                // 纯视频章节（无图文）ended 即学完
                const p = progressRef.current
                if (p && !article?.content) {
                  p.is_finished = true
                  flushProgress()
                }
              }}
            />
          </div>
          <div
            className="absolute bottom-0 right-0 w-4 h-4 cursor-se-resize"
            style={{
              background: 'linear-gradient(135deg, transparent 50%, rgba(255,255,255,0.3) 50%)',
            }}
            onMouseDown={handleResizeStart}
            title="拖动调整大小"
          />
        </div>
      )
    }

    return (
      <div ref={videoContainerRef} className="bg-gray-100 rounded-lg p-4 flex justify-center">
        <div className="max-w-full max-h-[50vh] rounded-lg overflow-hidden bg-black">
          <video
            ref={videoRef}
            poster={poster}
            controls
            className="max-w-full max-h-[50vh] rounded-lg"
            onTimeUpdate={handleVideoTimeUpdate}
            onPlay={handleVideoPlay}
            onPause={handleVideoPause}
            onLoadedMetadata={() => {
              const p = progressRef.current
              const video = videoRef.current
              if (p && p.position && p.position > 0 && video && video.duration && p.position < video.duration - 5) {
                video.currentTime = p.position
              }
            }}
            onEnded={() => {
              const p = progressRef.current
              if (p && !article?.content) {
                p.is_finished = true
                flushProgress()
              }
            }}
          />
        </div>
      </div>
    )
  }

  return (
    <div className="fixed inset-0 z-[80]">
      <div
        className="fixed inset-0 bg-black/50 backdrop-blur-sm z-[80]"
        onClick={handleClose}
      />
      {renderScrollTopButton()}
      
      {videoSrc && showFloatingPlayer && !isMobileRef.current && renderVideoPlayer()}

      <div
        className="fixed right-0 top-0 h-full bg-white shadow-2xl overflow-hidden z-[90] transition-all duration-300 ease-in-out"
        style={{ width: isMobile ? '100%' : drawerWidth }}
      >
        {/* 左侧拖拽手柄（仅桌面端显示） */}
        {!isMobile && (
          <div
            className="absolute left-0 top-0 bottom-0 w-4 cursor-col-resize hover:bg-primary-500/20 transition-colors group active:cursor-col-resize z-10"
            onMouseDown={handleDrawerResizeStart}
            title="拖动调整宽度"
          >
            <div className="absolute left-1 top-1/2 -translate-y-1/2 w-1 h-8 bg-gray-300 group-hover:bg-primary-500 rounded-full transition-colors" />
          </div>
        )}
        <div className="h-full flex flex-col">
          <div className="flex items-center justify-between p-4 border-b border-primary-100 bg-gradient-to-r from-primary-50 to-primary-100">
            <div className="flex-1">
              <h3 className="font-semibold text-gray-800">
                {taskInfo?.task_name || '加载中...'}
              </h3>
              {article?.summary && (
                <p className="text-sm text-gray-500 mt-1">{article.summary}</p>
              )}
            </div>
            <button
              onClick={handleClose}
              className="p-2 hover:bg-white/50 rounded-lg transition-colors ml-4"
            >
              <X size={20} className="text-gray-500" />
            </button>
          </div>

          <div ref={contentRef} className="flex-1 overflow-y-auto p-3 sm:p-4">
            {loading ? (
              <div className="flex justify-center py-20">
                <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-purple-500" />
              </div>
            ) : taskInfo ? (
              <div className="space-y-4">
                {videoSrc && !showFloatingPlayer && renderVideoPlayer()}
                {videoSrc && showFloatingPlayer && (
                  <div className="bg-gray-100 rounded-lg p-4 flex justify-center">
                    <div className="w-full max-w-full h-36 sm:h-[250px] md:h-[300px] bg-gray-200 rounded-lg flex items-center justify-center">
                      <span className="text-gray-400 text-sm sm:text-base">视频正在浮动窗口播放</span>
                    </div>
                  </div>
                )}
                {!videoSrc && (
                  <div ref={videoContainerRef} className="bg-gray-100 rounded-lg p-4 flex justify-center">
                    <div className="w-full max-w-full h-36 sm:h-[250px] md:h-[300px] bg-gray-200 rounded-lg flex items-center justify-center">
                      <span className="text-gray-400 text-sm sm:text-base">无视频</span>
                    </div>
                  </div>
                )}

                <div className="flex flex-wrap gap-2 justify-center">
                  {taskInfo?.redirect && (
                    <div className="relative group">
                      <a
                        href={taskInfo.redirect}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="flex items-center gap-2 px-4 py-2 bg-blue-500 hover:bg-blue-600 text-white rounded-lg transition-colors text-sm"
                      >
                        <ExternalLink size={14} />
                      </a>
                      <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[200px] text-center pointer-events-none z-20">
                        查看源站
                      </div>
                    </div>
                  )}
                  <div className="relative group">
                    <a
                      href={`/v2/task/download?id=${taskId}&type=markdown`}
                      className="flex items-center gap-2 px-4 py-2 bg-primary-500 hover:bg-primary-600 text-white rounded-lg transition-colors text-sm"
                    >
                      <FileText size={14} />
                    </a>
                    <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[200px] text-center pointer-events-none z-20">
                      导出Markdown
                    </div>
                  </div>
                  <div className="relative group">
                    <button
                      disabled={pdfDownloading}
                      onClick={async () => {
                        setPdfDownloading(true)
                        try {
                          const blob = await downloadPdfBlob({ id: taskId! })
                          // 用文章标题作为文件名，与 TaskList 保持一致
                          const title = article?.title || taskInfo?.task_name || taskId
                          const safeName = title
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
                          downloadFileFromBlob(blob, `${safeName}.pdf`)
                        } catch (err: any) {
                          showErrorMessage(err?.message || '导出PDF失败')
                        } finally {
                          setPdfDownloading(false)
                        }
                      }}
                      className="flex items-center gap-2 px-4 py-2 bg-primary-500 hover:bg-primary-600 disabled:opacity-50 text-white rounded-lg transition-colors text-sm"
                    >
                      <FileDown size={14} />
                    </button>
                    <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[200px] text-center pointer-events-none z-20">
                      导出PDF
                    </div>
                  </div>
                  <div className="relative group">
                    <button
                      onClick={() => setAiOpen(true)}
                      className="flex items-center gap-2 px-4 py-2 bg-primary-500 hover:bg-primary-600 text-white rounded-lg transition-colors text-sm"
                    >
                      <Sparkles size={14} />
                    </button>
                    <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[200px] text-center pointer-events-none z-20">
                      AI 助手
                    </div>
                  </div>
                  {hasPrev && (
                    <div className="relative group">
                      <button
                        onClick={onPrev}
                        className="flex items-center gap-1 px-4 py-2 bg-gray-100 hover:bg-gray-200 text-gray-700 rounded-lg transition-colors text-sm"
                      >
                        <ChevronLeft size={14} />
                      </button>
                      <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[200px] text-center pointer-events-none z-20">
                        上一个
                      </div>
                    </div>
                  )}
                  {hasNext && (
                    <div className="relative group">
                      <button
                        onClick={onNext}
                        className="flex items-center gap-1 px-4 py-2 bg-gray-100 hover:bg-gray-200 text-gray-700 rounded-lg transition-colors text-sm"
                      >
                        <ChevronRight size={14} />
                      </button>
                      <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-800 text-white text-xs rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-normal max-w-[200px] text-center pointer-events-none z-20">
                        下一个
                      </div>
                    </div>
                  )}
                </div>

                {article?.content && (
                  <div
                    className="article-content bg-white rounded-lg p-2 sm:p-4 border border-gray-100"
                    dangerouslySetInnerHTML={{ __html: article.content }}
                    onClick={handleImageClick}
                  />
                )}

                <div className="mt-6">
                  <div className="flex items-center gap-2 mb-4">
                    <MessageCircle size={18} className="text-primary-500" />
                    <h4 className="font-semibold text-gray-800">评论列表</h4>
                    <span className="text-sm text-gray-500">({commentsTotal})</span>
                  </div>
                  
                  {commentsLoading && comments.length === 0 ? (
                    <div className="flex justify-center py-8">
                      <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-purple-500" />
                    </div>
                  ) : comments.length === 0 ? (
                    <div className="text-center py-8 text-gray-400">暂无评论</div>
                  ) : (
                    <div className="space-y-4">
                      {comments.map((comment: any) => (
                        <div key={comment.id} className="bg-gray-50 rounded-lg p-4">
                          <div className="flex items-start gap-3">
                            <img
                              src={comment.user_header}
                              alt={comment.user_name}
                              className="w-10 h-10 rounded-full object-cover"
                              onError={(e) => {
                                (e.target as HTMLImageElement).src = 'https://via.placeholder.com/40'
                              }}
                            />
                            <div className="flex-1 min-w-0">
                              <div className="flex items-center gap-2">
                                <span className="font-medium text-gray-800">{comment.user_name}</span>
                                {comment.comment_ctime ? (
                                  <span className="text-xs text-gray-500">{new Date(comment.comment_ctime * 1000).toLocaleDateString()}</span>
                                ) : ''}
                                {comment.discussion_count > 0 && (
                                  <span className="text-xs text-gray-500">{comment.discussion_count}讨论</span>
                                )}
                                {comment.like_count > 0 && (
                                  <span className="text-xs text-gray-500">{comment.like_count}赞</span>
                                )}
                              </div>
                              {comment.comment_content ? (
                                <div className="mt-2 text-sm text-gray-600 word-break whitespace-pre-wrap">
                                  {comment.comment_content}
                                </div>
                              ) : (
                                <div className="mt-2 text-sm text-gray-400 word-break">
                                  暂无内容
                                </div>
                              )}
                              {comment.discussion_count > 0 && (
                                <div className="mt-3">
                                  <button
                                    onClick={() => toggleDiscussions(comment.id)}
                                    className="text-xs text-primary-500 hover:text-primary-600 flex items-center gap-1"
                                  >
                                    {expandedDiscussions.has(comment.id) ? '收起' : '展开'} ({comment.discussion_count})
                                  </button>
                                  {expandedDiscussions.has(comment.id) && (
                                    <div className="mt-3 space-y-3 pl-2 sm:pl-4 border-l-2 border-purple-200">
                                      {comment.discussions?.map((discussion: any) => (
                                        <div key={discussion.discussion?.id || discussion.id}>
                                          <div className="flex gap-3">
                                            <img
                                              src={discussion.author?.avatar}
                                              alt={discussion.author?.nickname}
                                              className="w-8 h-8 rounded-full object-cover flex-shrink-0"
                                              onError={(e) => {
                                                (e.target as HTMLImageElement).src = 'https://via.placeholder.com/32'
                                              }}
                                            />
                                            <div className="flex-1 min-w-0 bg-white rounded-lg p-3 border border-gray-100">
                                              <div className="flex items-center gap-2 mb-2">
                                                <span className="text-sm font-medium text-gray-800">{discussion.author?.nickname}</span>
                                                <span className="text-xs text-gray-400">
                                                  {discussion.discussion?.ctime ? new Date(discussion.discussion.ctime * 1000).toLocaleDateString() : ''}
                                                </span>
                                                {discussion.discussion?.likes_number > 0 && (
                                                  <span className="flex items-center gap-1 text-xs text-gray-500 ml-auto">
                                                    <ThumbsUp size={10} />
                                                    {discussion.discussion?.likes_number}
                                                  </span>
                                                )}
                                              </div>
                                              <div className="text-sm text-gray-600 word-break whitespace-pre-wrap">
                                                {discussion.discussion?.discussion_content || ''}
                                              </div>
                                            </div>
                                          </div>
                                          {discussion.child_discussions?.length > 0 && (
                                            <div className="mt-3 ml-6 sm:ml-11 space-y-3">
                                              {discussion.child_discussions.map((child: any) => (
                                                <div key={child.discussion?.id || child.id} className="flex gap-3">
                                                  <img
                                                    src={child.author?.avatar}
                                                    alt={child.author?.nickname}
                                                    className="w-6 h-6 rounded-full object-cover flex-shrink-0"
                                                    onError={(e) => {
                                                      (e.target as HTMLImageElement).src = 'https://via.placeholder.com/24'
                                                    }}
                                                  />
                                                  <div className="flex-1 min-w-0 bg-gray-50 rounded-lg p-2 border border-gray-100">
                                                    <div className="flex items-center gap-2 mb-1">
                                                      <span className="text-xs font-medium text-gray-700">{child.author?.nickname}</span>
                                                      {child.reply_author?.nickname && (
                                                        <span className="text-xs text-gray-400">
                                                          回复 {child.reply_author.nickname}
                                                        </span>
                                                      )}
                                                      <span className="text-xs text-gray-400">
                                                        {child.discussion?.ctime ? new Date(child.discussion.ctime * 1000).toLocaleDateString() : ''}
                                                      </span>
                                                    </div>
                                                    <div className="text-xs text-gray-600 word-break whitespace-pre-wrap">
                                                      {child.discussion?.discussion_content || ''}
                                                    </div>
                                                  </div>
                                                </div>
                                              ))}
                                            </div>
                                          )}
                                        </div>
                                      ))}
                                      {comment.discussionsHasMore && (
                                        <div className="text-center pl-11">
                                          <button
                                            onClick={() => loadMoreDiscussions(comment.id)}
                                            disabled={comment.discussionsLoading}
                                            className="px-3 py-1 text-xs text-primary-500 hover:text-primary-600 disabled:opacity-50"
                                          >
                                            {comment.discussionsLoading ? '加载中...' : '加载更多'}
                                          </button>
                                        </div>
                                      )}
                                    </div>
                                  )}
                                </div>
                              )}
                            </div>
                          </div>
                        </div>
                      ))}
                      
                      {commentsHasMore && (
                        <div className="text-center pt-2">
                          <button
                            onClick={() => {
                              const aid = taskInfo?.other_id || article?.other_id || article?.id
                              if (aid) {
                                loadComments(String(aid), commentsPage + 1)
                              }
                            }}
                            disabled={commentsLoading}
                            className="px-4 py-2 text-sm text-primary-500 hover:text-primary-600 disabled:opacity-50"
                          >
                            {commentsLoading ? '加载中...' : '加载更多'}
                          </button>
                        </div>
                      )}
                    </div>
                  )}
                </div>

                {taskInfoResponse?.message?.text && (
                  <div className="mt-6 p-4 bg-yellow-50 rounded-lg border border-yellow-200">
                    <p className="text-sm text-yellow-800 whitespace-pre-wrap">
                      {taskInfoResponse.message.text.includes('failed') || taskInfoResponse.message.text.includes('error') 
                        ? '错误：' + taskInfoResponse.message.text 
                        : '提示：' + taskInfoResponse.message.text}
                    </p>
                  </div>
                )}
              </div>
            ) : null}
          </div>
        </div>
      </div>

      {/* 图片预览 */}
      {previewImage && (
        <div
          className="fixed inset-0 z-[200] flex items-center justify-center bg-black/80 backdrop-blur-sm"
          onClick={() => setPreviewImage(null)}
          role="dialog"
          aria-label="图片预览"
        >
          <button
            onClick={() => setPreviewImage(null)}
            className="absolute top-4 right-4 z-[201] p-2 rounded-full bg-white/10 hover:bg-white/20 text-white transition-colors"
            aria-label="关闭"
          >
            <X size={24} />
          </button>
          <img
            src={previewImage}
            alt="预览"
            className="max-w-[90vw] max-h-[90vh] object-contain rounded-lg shadow-2xl"
            onClick={(e) => e.stopPropagation()}
            onError={() => setPreviewImage(null)}
            draggable={false}
          />
        </div>
      )}

      {/* AI 助手面板（仅主阅读面，轻量预览抽屉不挂载） */}
      {aiOpen && (
        <AIPanel
          aid={String(taskInfo?.other_id || article?.other_id || article?.id || '')}
          title={article?.title || taskInfo?.task_name || ''}
          open={aiOpen}
          onClose={() => setAiOpen(false)}
        />
      )}
    </div>
  )
}
