/**
 * 站点图标（favicon）—— 造型「J：播放三角 + 学习进度条」
 *
 * 为什么不用静态图片：
 * 项目有 5 套可切换主题（`src/utils/theme.ts`），写死颜色的 favicon 切主题后就不搭了。
 * 这里按当前主题的 `primary[400] / primary[500]` 现场生成 SVG data-URI，
 * 在 `initFavicon()` 与 `themeChange` 事件里替换 <link rel="icon">，
 * 于是切主题标签页图标立刻跟着变色，以后新增主题也自动覆盖、无需补文件。
 *
 * 静态兜底：`public/favicon.ico`（科技蓝，多尺寸）负责首屏绘制与不支持 SVG 图标的浏览器。
 */

import { themes, getCurrentTheme } from './theme'

/** 设计稿坐标系边长，所有坐标都在这个 512 的网格里 */
const GRID = 512

/** 播放三角三个顶点（造型 J 里整体上移 0.075*GRID 后的结果） */
const PLAY_POINTS: Array<[number, number]> = [
  [180.74, 110.08],
  [180.74, 325.12],
  [350.62, 217.6],
]
/** 三角的圆角半径（对应设计稿里的 blob 圆角） */
const PLAY_CORNER = 17.9

/** 进度条：轨道 + 已学部分 */
const BAR = {
  x0: 120.32,
  y0: 358.4,
  x1: 391.68,
  y1: 396.8,
  radius: 9.2,
  /** 已学比例，对应设计稿的 62% */
  done: 0.62,
}

const SVG_LINK_ID = 'theme-svg-favicon'

const round2 = (n: number): number => Math.round(n * 100) / 100

function unit(from: [number, number], to: [number, number]): [number, number] {
  const dx = to[0] - from[0]
  const dy = to[1] - from[1]
  const len = Math.hypot(dx, dy) || 1
  return [dx / len, dy / len]
}

/**
 * 圆角多边形路径。
 * 对每个顶点，沿两条邻边各退让 d = r / tan(θ/2)，再用二次贝塞尔近似圆弧。
 */
function roundedPolygon(points: Array<[number, number]>, radius: number): string {
  const n = points.length
  const segs: string[] = []

  for (let i = 0; i < n; i++) {
    const cur = points[i]
    const prev = points[(i - 1 + n) % n]
    const next = points[(i + 1) % n]

    const v1 = unit(cur, prev)
    const v2 = unit(cur, next)
    const cos = Math.max(-1, Math.min(1, v1[0] * v2[0] + v1[1] * v2[1]))
    const half = Math.acos(cos) / 2
    const d = radius / Math.tan(half)

    const entry: [number, number] = [cur[0] + v1[0] * d, cur[1] + v1[1] * d]
    const exit: [number, number] = [cur[0] + v2[0] * d, cur[1] + v2[1] * d]

    segs.push(`${i === 0 ? 'M' : 'L'}${round2(entry[0])} ${round2(entry[1])}`)
    segs.push(`Q${round2(cur[0])} ${round2(cur[1])} ${round2(exit[0])} ${round2(exit[1])}`)
  }

  return `${segs.join(' ')}Z`
}

/** 按主题生成 favicon 的 SVG 源码 */
export function buildFaviconSvg(themeName?: string): string {
  const theme = themes[themeName ?? ''] ?? themes.default
  const gradTop = theme.primary[400]
  const gradBottom = theme.primary[500]

  const triangle = roundedPolygon(PLAY_POINTS, PLAY_CORNER)
  const barWidth = BAR.x1 - BAR.x0
  const barHeight = BAR.y1 - BAR.y0

  return (
    `<svg xmlns="http://www.w3.org/2000/svg" width="${GRID}" height="${GRID}" viewBox="0 0 ${GRID} ${GRID}">` +
    `<defs><linearGradient id="g" x1="0" y1="0" x2="0" y2="1">` +
    `<stop offset="0" stop-color="${gradTop}"/>` +
    `<stop offset="1" stop-color="${gradBottom}"/>` +
    `</linearGradient></defs>` +
    `<rect width="${GRID}" height="${GRID}" rx="112.6" fill="url(#g)"/>` +
    `<path d="${triangle}" fill="#ffffff"/>` +
    `<rect x="${BAR.x0}" y="${BAR.y0}" width="${round2(barWidth)}" height="${round2(barHeight)}" ` +
    `rx="${BAR.radius}" fill="#ffffff" fill-opacity="0.43"/>` +
    `<rect x="${BAR.x0}" y="${BAR.y0}" width="${round2(barWidth * BAR.done)}" height="${round2(barHeight)}" ` +
    `rx="${BAR.radius}" fill="#ffffff"/>` +
    `</svg>`
  )
}

/** 把 <link rel="icon"> 换成当前主题的 SVG 图标 */
export function applyFavicon(themeName?: string): void {
  if (typeof document === 'undefined') return

  const name = themeName || getCurrentTheme()
  const href = `data:image/svg+xml,${encodeURIComponent(buildFaviconSvg(name))}`

  let link = document.getElementById(SVG_LINK_ID) as HTMLLinkElement | null
  if (!link) {
    link = document.createElement('link')
    link.id = SVG_LINK_ID
    link.rel = 'icon'
    link.type = 'image/svg+xml'
    // 追加在静态 .ico 之后：支持 SVG 图标的浏览器优先用它，其余继续用 .ico 兜底
    document.head.appendChild(link)
  }

  link.setAttribute('href', href)
}

let initialized = false

/** 应用当前主题的图标，并跟随后续主题切换 */
export function initFavicon(): void {
  applyFavicon()
  if (initialized) return
  initialized = true
  window.addEventListener('themeChange', (e) => {
    applyFavicon((e as CustomEvent<string>).detail)
  })
}
