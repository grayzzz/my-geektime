import React, { useState, useEffect, useRef, useCallback } from 'react'
import { Settings, Palette, Check, X } from 'lucide-react'
import { themes, getCurrentTheme, setTheme } from '@/utils/theme'

export const GlobalSettings: React.FC = () => {
  const [isOpen, setIsOpen] = useState(false)
  const [currentTheme, setCurrentTheme] = useState(getCurrentTheme())
  // 初始化位置为右侧中间，避免页面加载时的飞行动画
  const [position, setPosition] = useState(() => ({
    x: typeof window !== 'undefined' ? window.innerWidth - 20 : 0,
    y: typeof window !== 'undefined' ? window.innerHeight / 2 : 0,
  }))
  const [isDragging, setIsDragging] = useState(false)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const dragStartRef = useRef({ x: 0, y: 0 })
  const buttonStartRef = useRef({ x: 0, y: 0 })
  const positionRef = useRef({ x: 0, y: 0 }) // 保存最新的 position

  // 监听窗口大小变化，更新位置
  useEffect(() => {
    const updatePosition = () => {
      const x = window.innerWidth - 20 // 右侧靠边，隐藏一半（按钮宽度40px，一半是20px）
      const y = window.innerHeight / 2 // 垂直居中
      const newPos = { x, y }
      setPosition(newPos)
      positionRef.current = newPos // 同步更新 ref
    }
    window.addEventListener('resize', updatePosition)
    return () => window.removeEventListener('resize', updatePosition)
  }, [])

  // 监听主题变更事件
  useEffect(() => {
    const handleThemeChange = (event: Event) => {
      const customEvent = event as CustomEvent<string>
      setCurrentTheme(customEvent.detail)
    }

    window.addEventListener('themeChange', handleThemeChange)
    return () => window.removeEventListener('themeChange', handleThemeChange)
  }, [])

  const handleThemeChange = (themeName: string) => {
    setTheme(themeName)
    setCurrentTheme(themeName)
  }

  // 磁吸到右侧（隐藏一半）
  const snapToRight = useCallback(() => {
    const x = window.innerWidth - 20 // 隐藏一半
    const newPos = { x, y: positionRef.current.y } // 保持当前 Y 位置
    setPosition(newPos)
    positionRef.current = newPos // 同步更新 ref
  }, [])

  // 鼠标按下
  const handleMouseDown = (e: React.MouseEvent) => {
    if (isOpen) return // 打开面板时不允许拖动
    e.preventDefault()
    setIsDragging(true)
    dragStartRef.current = { x: e.clientX, y: e.clientY }
    buttonStartRef.current = { ...position }
  }

  // 鼠标移动
  useEffect(() => {
    if (!isDragging) return

    const handleMouseMove = (e: MouseEvent) => {
      const deltaX = e.clientX - dragStartRef.current.x
      const deltaY = e.clientY - dragStartRef.current.y
      
      const newX = buttonStartRef.current.x + deltaX
      const newY = Math.max(80, Math.min(window.innerHeight - 140, buttonStartRef.current.y + deltaY))
      const newPos = { x: newX, y: newY }
      
      setPosition(newPos)
      positionRef.current = newPos // 同步更新 ref
    }

    const handleMouseUp = () => {
      setIsDragging(false)
      // 如果释放时靠近右侧边缘，自动磁吸
      const currentX = positionRef.current.x
      if (currentX > window.innerWidth - 80) {
        snapToRight()
      }
    }

    window.addEventListener('mousemove', handleMouseMove)
    window.addEventListener('mouseup', handleMouseUp)
    return () => {
      window.removeEventListener('mousemove', handleMouseMove)
      window.removeEventListener('mouseup', handleMouseUp)
    }
  }, [isDragging, position, snapToRight])

  // 触摸支持
  const handleTouchStart = (e: React.TouchEvent) => {
    if (isOpen) return
    const touch = e.touches[0]
    setIsDragging(true)
    dragStartRef.current = { x: touch.clientX, y: touch.clientY }
    buttonStartRef.current = { ...position }
  }

  useEffect(() => {
    if (!isDragging) return

    const handleTouchMove = (e: TouchEvent) => {
      const touch = e.touches[0]
      const deltaX = touch.clientX - dragStartRef.current.x
      const deltaY = touch.clientY - dragStartRef.current.y
      
      const newX = buttonStartRef.current.x + deltaX
      const newY = Math.max(80, Math.min(window.innerHeight - 140, buttonStartRef.current.y + deltaY))
      const newPos = { x: newX, y: newY }
      
      setPosition(newPos)
      positionRef.current = newPos // 同步更新 ref
    }

    const handleTouchEnd = () => {
      setIsDragging(false)
      const currentX = positionRef.current.x
      if (currentX > window.innerWidth - 80) {
        snapToRight()
      }
    }

    window.addEventListener('touchmove', handleTouchMove)
    window.addEventListener('touchend', handleTouchEnd)
    return () => {
      window.removeEventListener('touchmove', handleTouchMove)
      window.removeEventListener('touchend', handleTouchEnd)
    }
  }, [isDragging, position, snapToRight])

  return (
    <>
      {/* 浮动按钮 */}
      <button
        ref={buttonRef}
        onClick={() => !isDragging && setIsOpen(!isOpen)}
        onMouseDown={handleMouseDown}
        onTouchStart={handleTouchStart}
        className={`fixed z-50 w-10 h-10 rounded-full shadow-2xl transition-all duration-300 flex items-center justify-center select-none ${
          isDragging ? 'cursor-grabbing scale-110' : 'cursor-grab'
        } ${
          isOpen 
            ? 'bg-primary-600 hover:bg-primary-700 rotate-90' 
            : 'bg-white hover:bg-primary-50 border-2 border-primary-300 hover:border-primary-400 hover:scale-110'
        }`}
        style={{
          left: `${position.x}px`,
          top: `${position.y}px`,
          transform: 'translate(-50%, -50%)',
          boxShadow: isOpen ? 'none' : isDragging 
            ? '0 12px 40px rgba(0, 0, 0, 0.4)' 
            : '0 8px 32px rgba(0, 0, 0, 0.3)',
          transition: isDragging ? 'none' : 'all 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
        }}
      >
        {isOpen ? (
          <X size={18} className={isOpen ? 'text-white' : 'text-primary-600'} />
        ) : (
          <Settings size={18} className={isOpen ? 'text-white' : 'text-primary-600'} />
        )}
      </button>

      {/* 设置面板 */}
      {isOpen && (
        <div
          className="fixed z-50 w-72 sm:w-72 max-w-[calc(100vw-48px)] bg-white/95 backdrop-blur-xl rounded-2xl shadow-2xl border border-white/20 overflow-hidden"
          style={{
            right: '24px',
            top: '50%',
            transform: 'translateY(-50%)',
            animation: 'slideIn 0.3s ease-out',
          }}
        >
          {/* 面板头部 */}
          <div className="p-4 border-b border-gray-100 bg-gradient-to-r from-primary-50 to-primary-100 flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Palette size={20} className="text-primary-600" />
              <h3 className="text-base font-semibold text-gray-800">主题设置</h3>
            </div>
            <button
              onClick={() => setIsOpen(false)}
              className="p-1.5 rounded-lg hover:bg-gray-200/50 transition-colors"
            >
              <X size={18} className="text-gray-500 hover:text-gray-700" />
            </button>
          </div>
          
          {/* 主题列表 */}
          <div className="p-3 space-y-2 max-h-96 overflow-y-auto">
            {Object.values(themes).map((theme) => (
              <button
                key={theme.name}
                onClick={() => handleThemeChange(theme.name)}
                className={`w-full flex items-center gap-3 px-4 py-3 rounded-xl transition-all duration-200 ${
                  currentTheme === theme.name
                    ? 'bg-primary-50 border-2 border-primary-300 shadow-md'
                    : 'hover:bg-gray-50 border-2 border-transparent hover:border-gray-200'
                }`}
              >
                {/* 主题颜色预览 */}
                <div className="flex gap-1 flex-shrink-0">
                  <div 
                    className="w-6 h-6 rounded-lg shadow-sm" 
                    style={{ backgroundColor: theme.primary[400] }}
                  />
                  <div 
                    className="w-6 h-6 rounded-lg shadow-sm -ml-2 border-2 border-white" 
                    style={{ backgroundColor: theme.primary[600] }}
                  />
                  <div 
                    className="w-6 h-6 rounded-lg shadow-sm -ml-2 border-2 border-white" 
                    style={{ backgroundColor: theme.primary[800] }}
                  />
                </div>
                
                <div className="flex-1 text-left">
                  <span className={`text-sm font-semibold block ${
                    currentTheme === theme.name ? 'text-primary-700' : 'text-gray-700'
                  }`}>
                    {theme.label}
                  </span>
                  {theme.description && (
                    <span className="text-xs text-gray-500 block mt-0.5">{theme.description}</span>
                  )}
                  {currentTheme === theme.name && (
                    <span className="text-xs text-primary-500 font-medium">当前主题</span>
                  )}
                </div>
                
                {currentTheme === theme.name && (
                  <div className="flex-shrink-0">
                    <Check size={20} className="text-primary-600" />
                  </div>
                )}
              </button>
            ))}
          </div>

          {/* 面板底部 */}
          <div className="p-3 border-t border-gray-100 bg-gray-50">
            <p className="text-xs text-gray-500 text-center">
              主题设置会自动保存
            </p>
          </div>
        </div>
      )}

      {/* 动画样式 */}
      <style>{`
        @keyframes slideIn {
          from {
            opacity: 0;
            transform: translateY(20px) scale(0.95);
          }
          to {
            opacity: 1;
            transform: translateY(0) scale(1);
          }
        }
      `}</style>
    </>
  )
}
