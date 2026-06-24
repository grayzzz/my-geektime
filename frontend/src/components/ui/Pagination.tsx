import React from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { clsx } from 'clsx'

interface PaginationProps {
  current: number
  total: number
  pageSize: number
  onChange: (page: number) => void
  onPageSizeChange?: (size: number) => void
  pageSizeOptions?: number[]
}

export const Pagination: React.FC<PaginationProps> = ({
  current,
  total,
  pageSize,
  onChange,
  onPageSizeChange,
  pageSizeOptions = [10, 20, 50],
}) => {
  const totalPages = Math.ceil(total / pageSize)

  const getPageNumbers = () => {
    const pages: (number | string)[] = []
    if (totalPages <= 7) {
      for (let i = 1; i <= totalPages; i++) {
        pages.push(i)
      }
    } else {
      if (current <= 4) {
        for (let i = 1; i <= 5; i++) {
          pages.push(i)
        }
        pages.push('...')
        pages.push(totalPages)
      } else if (current >= totalPages - 3) {
        pages.push(1)
        pages.push('...')
        for (let i = totalPages - 4; i <= totalPages; i++) {
          pages.push(i)
        }
      } else {
        pages.push(1)
        pages.push('...')
        for (let i = current - 1; i <= current + 1; i++) {
          pages.push(i)
        }
        pages.push('...')
        pages.push(totalPages)
      }
    }
    return pages
  }

  return (
    <div className="flex flex-col sm:flex-row items-center justify-between gap-2 px-4 py-3 bg-white border-t">
      <div className="flex items-center gap-2 shrink-0">
        <span className="text-sm text-gray-600">共 {total} 条</span>
        {onPageSizeChange && (
          <select
            value={pageSize}
            onChange={(e) => onPageSizeChange(Number(e.target.value))}
            className="border rounded px-2 py-1 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
          >
            {pageSizeOptions.map((size) => (
              <option key={size} value={size}>
                {size} 条/页
              </option>
            ))}
          </select>
        )}
      </div>
      <div className="flex items-center gap-1 overflow-x-auto max-w-full">
        <button
          onClick={() => onChange(current - 1)}
          disabled={current === 1}
          className="p-2 rounded hover:bg-gray-100 disabled:opacity-50 disabled:cursor-not-allowed flex-shrink-0"
        >
          <ChevronLeft size={20} />
        </button>
        {getPageNumbers().map((page, index) =>
          page === '...' ? (
            <span key={index} className="px-3 py-2 flex-shrink-0">
              ...
            </span>
          ) : (
            <button
              key={index}
              onClick={() => onChange(page as number)}
              className={clsx(
                'px-2 py-1 sm:px-3 py-2 rounded min-w-[32px] sm:min-w-[40px] text-xs sm:text-sm flex-shrink-0',
                current === page
                  ? 'bg-primary-500 text-white hover:bg-primary-600'
                  : 'hover:bg-gray-100'
              )}
            >
              {page}
            </button>
          )
        )}
        <button
          onClick={() => onChange(current + 1)}
          disabled={current === totalPages}
          className="p-2 rounded hover:bg-gray-100 disabled:opacity-50 disabled:cursor-not-allowed flex-shrink-0"
        >
          <ChevronRight size={20} />
        </button>
      </div>
    </div>
  )
}
