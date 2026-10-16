import React, { useEffect, lazy, Suspense } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import { useAuthStore } from '@/store/auth'
import { ProtectedRoute } from '@/components/ProtectedRoute'
import { Layout } from '@/components/Layout'
import { ToastProvider } from '@/components/ui/Toast'
import { GlobalLoading } from '@/components/GlobalLoading'
import { GlobalSettings } from '@/components/GlobalSettings'
import { initTheme } from '@/utils/theme'
import { initFavicon } from '@/utils/favicon'
import { Login } from '@/pages/Login'
import { Register } from '@/pages/Register'
import GitHubCallback from '@/pages/GitHubCallback'
import { ThemeTest } from '@/pages/ThemeTest'

const TaskList = lazy(() => import('@/pages/TaskList').then(m => ({ default: m.TaskList })))
const CollectList = lazy(() => import('@/pages/CollectList').then(m => ({ default: m.CollectList })))
const PvipList = lazy(() => import('@/pages/PvipList').then(m => ({ default: m.PvipList })))
const LessonList = lazy(() => import('@/pages/LessonList').then(m => ({ default: m.LessonList })))
const CaseList = lazy(() => import('@/pages/CaseList').then(m => ({ default: m.CaseList })))
const Setting = lazy(() => import('@/pages/Setting').then(m => ({ default: m.Setting })))
const UserList = lazy(() => import('@/pages/UserList').then(m => ({ default: m.UserList })))
const DictList = lazy(() => import('@/pages/DictList').then(m => ({ default: m.DictList })))

const LoadingFallback = () => (
  <div className="flex items-center justify-center h-full">
    <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-white" />
  </div>
)

const App: React.FC = () => {
  const init = useAuthStore((state) => state.init)

  useEffect(() => {
    init()
    // 初始化主题
    initTheme()
    // 站点图标跟随主题
    initFavicon()
  }, [init])

  return (
    <ToastProvider>
      <GlobalLoading />
      <GlobalSettings />
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="/register" element={<Register />} />
        <Route path="/oauth/github/callback" element={<GitHubCallback />} />
        <Route
          path="/*"
          element={
            <ProtectedRoute>
              <Layout />
            </ProtectedRoute>
          }
        >
          <Route index element={<Navigate to="/task/list" replace />} />
          <Route path="task/list" element={
            <Suspense fallback={<LoadingFallback />}>
              <TaskList />
            </Suspense>
          } />
          <Route path="collect/list" element={
            <Suspense fallback={<LoadingFallback />}>
              <CollectList />
            </Suspense>
          } />
          <Route path="product/pvip" element={
            <Suspense fallback={<LoadingFallback />}>
              <PvipList />
            </Suspense>
          } />
          <Route path="product/lesson" element={
            <Suspense fallback={<LoadingFallback />}>
              <LessonList />
            </Suspense>
          } />
          <Route path="product/case" element={
            <Suspense fallback={<LoadingFallback />}>
              <CaseList />
            </Suspense>
          } />
          <Route path="setting" element={
            <Suspense fallback={<LoadingFallback />}>
              <Setting />
            </Suspense>
          } />
          <Route path="theme-test" element={
            <ThemeTest />
          } />
          <Route path="user/list" element={
            <Suspense fallback={<LoadingFallback />}>
              <UserList />
            </Suspense>
          } />
          <Route path="sys/dict/list" element={
            <Suspense fallback={<LoadingFallback />}>
              <DictList />
            </Suspense>
          } />
        </Route>
      </Routes>
    </ToastProvider>
  )
}

export default App
