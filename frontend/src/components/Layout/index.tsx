import React, { useState } from 'react'
import { Outlet } from 'react-router-dom'
import { Github, Star } from 'lucide-react'
import { Sidebar } from './Sidebar'
import { Header } from './Header'
import { Drawer } from './Drawer'

const Bubble = ({ className, style }: { className: string; style?: React.CSSProperties }) => (
  <div className={`absolute rounded-full bg-white/30 backdrop-blur-3xl ${className}`} style={style} />
)

export const Layout: React.FC = () => {
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [openMenus, setOpenMenus] = useState<Set<string>>(new Set(['我的课程']))

  const toggleMenu = (label: string) => {
    setOpenMenus((prev) => {
      const next = new Set(prev)
      if (next.has(label)) {
        next.delete(label)
      } else {
        next.add(label)
      }
      return next
    })
  }

  return (
    <div className="flex h-[100dvh] bg-gradient-to-br from-primary-50 via-primary-100 to-primary-50 relative overflow-hidden">
      <Bubble className="w-72 h-72 -top-36 -left-36 animate-pulse" />
      <Bubble className="w-48 h-48 top-1/3 -right-24 animate-bounce" style={{ animationDuration: '4s' }} />
      <Bubble className="w-64 h-64 -bottom-32 left-1/4 animate-pulse" style={{ animationDuration: '5s' }} />
      
      <Drawer
        isOpen={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        openMenus={openMenus}
        onToggleMenu={toggleMenu}
      />

      <div className="flex flex-col h-[100dvh] w-full">
        <Sidebar />
        <div className="flex-1 flex flex-col overflow-hidden">
          <Header onMenuClick={() => setDrawerOpen(true)} />
          <main className="flex-1 overflow-auto p-6">
            <Outlet />
          </main>
          <footer className="text-center py-4 text-primary-800/60 text-sm flex items-center justify-center gap-4">
            <span>© 2025 我的极客时间, Inc.</span>
            <a
              href="https://github.com/zkep/my-geektime"
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-1 hover:text-primary-600 transition-colors"
            >
              <Github size={14} />
              <span>Star 支持</span>
              <Star size={14} className="fill-yellow-400 text-yellow-400" />
            </a>
          </footer>
        </div>
      </div>
    </div>
  )
}
