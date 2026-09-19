import React, { useEffect, useState, useRef } from 'react'
import { getPvipList, downloadProduct, ProductItem } from '@/api/product'
import { getDictTree } from '@/api/dict'
import { dedupCategories } from '@/utils/category'
import { Button, Card, Pagination, Select, Input, Spinner, Drawer, Modal } from '@/components/ui'
import { ArticleList } from '@/components/ArticleList'
import { useAuthStore } from '@/store/auth'
import { useToast } from '@/components/ui/Toast'
import { ExternalLink, Eye, Download, RefreshCw } from 'lucide-react'
import { useDebounce } from '@/hooks/useDebounce'

const productTypeOptions = [
  { label: '全部类型', value: 0 },
  { label: '体系课', value: 1 },
  { label: '公开课', value: 4 },
  { label: '线下大会', value: 5 },
  { label: '社区课', value: 6 },
]

const productFormOptions = [
  { label: '全部形式', value: 0 },
  { label: '图文+音频', value: 1 },
  { label: '视频', value: 2 },
]

const productSortOptions = [
  { label: '综合', value: 8 },
  { label: '最新', value: 1 },
  { label: '最热', value: 4 },
]

export const PvipList: React.FC = () => {
  const [loading, setLoading] = useState(false)
  const [items, setItems] = useState<ProductItem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [perPage, setPerPage] = useState(12)
  const [filters, setFilters] = useState({
    direction: 0,
    tag: 0,
    product_type: 0,
    product_form: 0,
    sort: 8,
    keyword: '',
  })
  const [prevFilters, setPrevFilters] = useState<typeof filters | null>(null)

  // 使用 useRef 来跟踪是否已经加载过数据
  const hasLoadedRef = useRef(false)
  // 用于跟踪是否是用户主动改变页码
  const isUserPageChangeRef = useRef(false)

  // 为关键字搜索添加防抖
  const debouncedKeyword = useDebounce(filters.keyword, 500)
  const [geektimeCategory, setGeektimeCategory] = useState<any[]>([])
  const [geektimeDirection, setGeektimeDirection] = useState<any[]>([])
  const [showAllCategories, setShowAllCategories] = useState(false)
  const [showCookieModal, setShowCookieModal] = useState(false)
  const [cookie, setCookie] = useState('')
  const [showConfirmModal, setShowConfirmModal] = useState(false)
  const [confirmItem, setConfirmItem] = useState<ProductItem | null>(null)
  const [showDetailModal, setShowDetailModal] = useState(false)
  const [detailItem, setDetailItem] = useState<ProductItem | null>(null)

  const geekAuth = useAuthStore((state) => state.geekAuth)
  const setGeekAuth = useAuthStore((state) => state.setGeekAuth)
  const { addToast } = useToast()

  useEffect(() => {
    loadDictData()
  }, [])

  useEffect(() => {
    // 只有当 filters 真正发生变化时才重新加载数据
    if (prevFilters && JSON.stringify(prevFilters) === JSON.stringify({ ...filters, keyword: prevFilters.keyword })) {
      return
    }
    
    // 初始化时不立即加载，避免与下面的 useEffect 冲突
    if (!hasLoadedRef.current) {
      hasLoadedRef.current = true
      // 初始化时直接加载数据
      loadData()
      setPrevFilters(filters)
      return
    }
    
    // 非初始化时，重置页码并直接加载第一页数据
    setPrevFilters(filters)
    setPage(1)
    loadData(1)
  }, [debouncedKeyword, filters.direction, filters.tag, filters.product_type, filters.product_form, filters.sort])

  useEffect(() => {
    // 初始化时由第一个 useEffect 处理，这里只处理用户主动改变页码
    if (!hasLoadedRef.current) {
      return
    }
    
    // 如果是用户主动改变页码（通过分页组件），则加载数据
    if (isUserPageChangeRef.current || page > 1) {
      isUserPageChangeRef.current = false
      loadData()
    }
  }, [page, perPage])

  const loadDictData = async () => {
    try {
      const res = await getDictTree('geektimeCategory')
      if (res) {
        const categories = res.geektimeCategory || []
        const directions = dedupCategories(categories)
        setGeektimeCategory(directions)
        setGeektimeDirection(directions)
      }
    } catch (error) {
      console.error('Failed to load dict data', error)
    }
  }

  const loadData = async (loadPage?: number) => {
    setLoading(true)
    try {
      const currentPage = loadPage || page
      const params: any = {
        page: currentPage,
        perPage,
        sort: filters.sort,
        with_articles: true,
      }
      if (filters.direction) params.direction = filters.direction
      if (filters.tag) params.tag = filters.tag
      if (filters.product_type) params.product_type = filters.product_type
      if (filters.product_form) params.product_form = filters.product_form
      if (debouncedKeyword) params.keyword = debouncedKeyword

      const res = await getPvipList(params)
      setItems(res.rows || [])
      setTotal(res.count || 0)
    } catch (error) {
      console.error('Failed to load products', error)
    } finally {
      setLoading(false)
    }
  }

  const handleDownloadClick = (item: ProductItem) => {
    setConfirmItem(item)
    setShowConfirmModal(true)
  }

  const handleDownloadConfirm = async () => {
    if (!confirmItem) return
    try {
      await downloadProduct({
        pid: Number(confirmItem.id),
      })
      addToast('缓存任务已创建', 'success')
      setShowConfirmModal(false)
      setConfirmItem(null)
    } catch (error) {
      console.error('Failed to download', error)
    }
  }

  const handleViewDetail = (item: ProductItem) => {
    setDetailItem(item)
    setShowDetailModal(true)
  }

  const handleSaveCookie = async () => {
    if (!cookie || cookie.length < 50) {
      addToast('Cookie 不少于50个字符', 'warning')
      return
    }
    try {
      const response = await fetch('/v2/base/refresh/cookie', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token')}`,
        },
        body: JSON.stringify({ cookie }),
      })
      const data = await response.json()
      if (data.status === 0) {
        setGeekAuth(true)
        setShowCookieModal(false)
        setCookie('')
        addToast('Cookie 保存成功', 'success')
      } else {
        addToast(data.msg || 'Cookie 保存失败', 'error')
      }
    } catch (error) {
      console.error('Failed to save cookie', error)
      addToast('Cookie 保存失败，请重试', 'error')
    }
  }

  return (
    <div>
      <Card header="体系/公开/线下大会" />

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
                    onClick={() => setFilters({ ...filters, direction: item.value, tag: 0 })}
                  >
                    {item.label}
                  </button>
                ))}
              </div>
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">课程分类</label>
              <div className="flex flex-wrap gap-2 items-center">
                {(() => {
                  const categoryOptions = filters.direction
                    ? geektimeCategory.find((c) => c.value === filters.direction)?.children || []
                    : geektimeCategory.flatMap((c) => c.children || [])
                  const allOptions = [{ label: '全部', value: 0 }, ...categoryOptions]
                  const displayCount = 12
                  const shouldShowMore = allOptions.length > displayCount
                  const visibleOptions = showAllCategories ? allOptions : allOptions.slice(0, displayCount)

                  return (
                    <>
                      {visibleOptions.map((item: any) => (
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
                      {shouldShowMore && (
                        <button
                          className="px-3 py-1.5 rounded-lg text-sm font-medium text-primary-600 bg-primary-50 border border-primary-200 hover:bg-primary-100 transition-all duration-200"
                          onClick={() => setShowAllCategories(!showAllCategories)}
                        >
                          {showAllCategories ? '收起' : `更多 (${allOptions.length - displayCount})`}
                        </button>
                      )}
                    </>
                  )
                })()}
              </div>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-3">
              <Select
                label="排序"
                options={productSortOptions}
                value={filters.sort}
                onChange={(e) =>
                  setFilters({ ...filters, sort: Number(e.target.value) })
                }
              />
              <Select
                label="课程类型"
                options={productTypeOptions}
                value={filters.product_type}
                onChange={(e) =>
                  setFilters({ ...filters, product_type: Number(e.target.value) })
                }
              />
              <Select
                label="课程形式"
                options={productFormOptions}
                value={filters.product_form}
                onChange={(e) =>
                  setFilters({ ...filters, product_form: Number(e.target.value) })
                }
              />
              <Input
                label="搜索关键字"
                placeholder="搜索关键字"
                value={filters.keyword}
                onChange={(e) =>
                  setFilters({ ...filters, keyword: e.target.value })
                }
                onKeyDown={(e: React.KeyboardEvent<HTMLInputElement>) => {
                  if (e.key === 'Enter') {
                    // 按回车键时立即搜索，不等待防抖
                    setPrevFilters(filters)
                    setPage(1)
                    loadData(1)
                  }
                }}
              />
            </div>
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
                  className="border rounded-lg p-4 hover:border-gray-300 transition-colors"
                >
                  <div className="flex items-start gap-3">
                    <img
                      src={item.cover?.square}
                      alt={item.title}
                      className="w-16 h-16 rounded object-cover"
                    />
                    <div className="flex-1 min-w-0">
                      <div 
                        className="font-semibold text-gray-800 truncate search-highlight"
                        dangerouslySetInnerHTML={{ __html: item.title }}
                      />
                      <p className="text-sm text-gray-500 truncate">
                        {item.subtitle}
                      </p>
                    </div>
                  </div>
                  <div className="mt-3 grid grid-cols-2 gap-2 text-sm">
                    <div>
                      <span className="text-gray-500">作者: </span>
                      {item.author?.name}
                    </div>
                    <div>
                      <span className="text-gray-500">ID: </span>
                      {item.id}
                    </div>
                    <div>
                      <span className="text-gray-500">课程数: </span>
                      {item.article?.count} 讲
                    </div>
                    <div>
                      <span className="text-gray-500">价格: </span>
                      {item.sale_type === 6 || item.sale_type === 7
                        ? '免费'
                        : item.sale
                        ? `¥${(item.sale / 100).toFixed(2)}`
                        : '未知'}
                    </div>
                    <div>
                      <span className="text-gray-500">完结: </span>
                      {item.is_finish ? '是' : '否'}
                    </div>
                  </div>
                  <div className="mt-3 flex gap-2">
                    {item.redirect && (
                      <Button
                        variant="light"
                        size="sm"
                        onClick={() => window.open(item.redirect, '_blank')}
                      >
                        <ExternalLink size={14} className="mr-1" />
                        源站
                      </Button>
                    )}
                    <Button variant="light" size="sm" onClick={() => handleViewDetail(item)}>
                      <Eye size={14} className="mr-1" />
                      详情
                    </Button>
                    {geekAuth ? (
                      <Button
                        variant="primary"
                        size="sm"
                        onClick={() => handleDownloadClick(item)}
                      >
                        <Download size={14} className="mr-1" />
                        缓存
                      </Button>
                    ) : (
                      <Button
                        variant="light"
                        size="sm"
                        onClick={() => setShowCookieModal(true)}
                      >
                        <RefreshCw size={14} className="mr-1" />
                        缓存
                      </Button>
                    )}
                  </div>
                </div>
              ))}
            </div>
            {items.length === 0 && !loading && (
              <div className="flex flex-col items-center justify-center py-20 text-gray-500">
                <div className="text-lg">暂无数据</div>
              </div>
            )}
            <Pagination
              current={page}
              total={total}
              pageSize={perPage}
              onChange={setPage}
              onPageSizeChange={setPerPage}
              pageSizeOptions={[12, 24, 48]}
            />
          </>
        )}
      </Card>

      <Drawer
        isOpen={showCookieModal}
        onClose={() => setShowCookieModal(false)}
        title="Cookie登录"
        size="md"
      >
        <div className="space-y-4">
          <p className="text-sm text-gray-600">
            请先保存【极客时间VIP登录凭据】，该登录凭据是全站共享的基础，下载期间避免失效，失效后下载会找不到下载链接
          </p>
          <a
            href="https://zkep.github.io/my-geektime/guide/data_geektime/"
            target="_blank"
            className="text-blue-600 text-sm hover:underline"
          >
            查看详细文档
          </a>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Cookie
            </label>
            <textarea
              value={cookie}
              onChange={(e) => setCookie(e.target.value)}
              placeholder="请输入极客时间Cookie"
              className="w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
              rows={5}
            />
          </div>
          <Button onClick={handleSaveCookie} className="w-full">
            保存Cookie
          </Button>
        </div>
      </Drawer>

      <Modal
        isOpen={showConfirmModal}
        onClose={() => {
          setShowConfirmModal(false)
          setConfirmItem(null)
        }}
        title="确认缓存"
        size="sm"
      >
        <div className="space-y-4">
          <p className="text-sm text-gray-600">
              缓存 [{confirmItem?.title}]后请在[我的课程]查看详情
          </p>
          <div className="flex gap-3">
            <Button
              variant="secondary"
              onClick={() => {
                setShowConfirmModal(false)
                setConfirmItem(null)
              }}
              className="flex-1"
            >
              取消
            </Button>
            <Button onClick={handleDownloadConfirm} className="flex-1">
              确定
            </Button>
          </div>
        </div>
      </Modal>

      <Drawer
        isOpen={showDetailModal}
        onClose={() => {
          setShowDetailModal(false)
          setDetailItem(null)
        }}
        title="课程详情"
        size="xl"
      >
        {detailItem && (
          <div className="space-y-6">
            {/* Course Header */}
            <div className="flex items-start gap-4 pb-4 border-b">
              <img
                src={detailItem.cover?.square}
                alt={detailItem.title}
                className="w-32 h-32 rounded-lg object-cover shadow-md"
              />
              <div className="flex-1">
                <h3 className="text-2xl font-bold text-gray-800 mb-2">{detailItem.title}</h3>
                <p className="text-gray-600 mb-3">{detailItem.subtitle}</p>
                <div className="grid grid-cols-2 gap-2 text-sm">
                  <div>
                    <span className="text-gray-500">作者: </span>
                    <span className="font-medium">{detailItem.author?.name}</span>
                  </div>
                  <div>
                    <span className="text-gray-500">ID: </span>
                    <span className="font-medium">{detailItem.id}</span>
                  </div>
                  <div>
                    <span className="text-gray-500">课程数: </span>
                    <span className="font-medium">{detailItem.article?.count} 讲</span>
                  </div>
                  <div>
                    <span className="text-gray-500">价格: </span>
                    <span className="font-medium">
                      {detailItem.sale_type === 6 || detailItem.sale_type === 7
                        ? '免费'
                        : `¥${(detailItem.sale / 100).toFixed(2)}`}
                    </span>
                  </div>
                  <div>
                    <span className="text-gray-500">完结: </span>
                    <span className="font-medium">{detailItem.is_finish ? '是' : '否'}</span>
                  </div>
                  <div>
                    <span className="text-gray-500">音频: </span>
                    <span className="font-medium">{detailItem.is_audio ? '支持' : '不支持'}</span>
                  </div>
                  <div>
                    <span className="text-gray-500">视频: </span>
                    <span className="font-medium">{detailItem.is_video ? '支持' : '不支持'}</span>
                  </div>
                </div>
              </div>
            </div>

            {/* Author Introduction */}
            {detailItem.author?.intro && (
              <div className="bg-gradient-to-r from-primary-50 to-primary-100 rounded-lg p-4">
                <h4 className="font-semibold text-gray-700 mb-2 flex items-center">
                  <span className="w-1 h-5 bg-primary-400 rounded mr-2"></span>
                  作者简介
                </h4>
                <p className="text-gray-600 text-sm leading-relaxed">{detailItem.author.intro}</p>
              </div>
            )}

            {/* Course Introduction */}
            {detailItem.intro_html && (
              <div>
                <h4 className="font-semibold text-gray-700 mb-3 flex items-center">
                  <span className="w-1 h-5 bg-primary-400 rounded mr-2"></span>
                  课程介绍
                </h4>
                <div 
                  className="text-sm text-gray-600 leading-relaxed space-y-2"
                  dangerouslySetInnerHTML={{ __html: detailItem.intro_html }}
                />
              </div>
            )}

            {/* Articles List */}
            <ArticleList productId={detailItem.id} />

            {/* Action Buttons */}
            <div className="flex gap-3 pt-4 border-t">
              {detailItem.redirect && (
                <Button
                  variant="secondary"
                  onClick={() => window.open(detailItem.redirect, '_blank')}
                >
                  <ExternalLink size={16} className="mr-2" />
                  访问源站
                </Button>
              )}
              {geekAuth ? (
                <Button
                  variant="primary"
                  onClick={() => {
                    setShowDetailModal(false)
                    handleDownloadClick(detailItem)
                  }}
                >
                  <Download size={16} className="mr-2" />
                  缓存课程
                </Button>
              ) : (
                <Button
                  variant="light"
                  onClick={() => {
                    setShowDetailModal(false)
                    setShowCookieModal(true)
                  }}
                >
                  <RefreshCw size={16} className="mr-2" />
                  设置Cookie后缓存
                </Button>
              )}
            </div>
          </div>
        )}
      </Drawer>
    </div>
  )
}
