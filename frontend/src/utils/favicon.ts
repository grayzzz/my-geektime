/**
 * 站点图标（favicon）—— 造型「J：播放三角 + 学习进度条」
 *
 * 为什么不用静态图片：
 * 项目有 5 套可切换主题（`src/utils/theme.ts`），写死颜色的 favicon 切主题后就不搭了。
 * 这里按当前主题的 `primary[400] / primary[500]` 现场生成 SVG data-URI，
 * 在 `initFavicon()` 与 `themeChange` 事件里替换 <link rel="icon">，
 * 于是切主题标签页图标立刻跟着变色，以后新增主题也自动覆盖、无需补文件。
 *
 * ⚠️ 关键约束：整个文档**任何时候只能有一条** `link[rel~="icon"]`。
 * head 里如果同时存在 `index.html` 的静态 `/favicon.ico` 和这里的 SVG，
 * 浏览器会择优只采纳一条，实测 Chromium 会挑走 `.ico`（它声明了 `sizes="any"`、
 * 又排在前面），SVG 那条连解码都不会发生 —— 表现为「切主题图标纹丝不动」。
 * 所以这里不是「新增一条」，而是**接管** head 里已有的那条（见 `ensureIconLink`）。
 *
 * 验证方式（无头浏览器不下载 favicon，测不出来）：有头跑一次，再读 profile 里的
 * `Default/Favicons` SQLite，比对 `favicon_bitmaps.image_data` 的主色。
 * 脚本见 `.workbuddy/favicon/probe_real_favicon.py`。
 *
 * 静态兜底：`public/favicon.ico`（科技蓝，多尺寸）负责 JS 接管前的首屏绘制。
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

/** 记住上次写入的 href，避免同一主题重复重建节点 */
const appliedHref = new Map<string, string>()

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

/**
 * 拿到「那一条」icon link。
 *
 * 优先复用 `index.html` 里已有的静态 `.ico` 节点（`id` 换成 SVG_LINK_ID 就地接管），
 * 没有再自己建。目的是保证 head 里始终只有一条 `rel="icon"`，
 * 避免浏览器在 `.ico` 与 SVG 之间择优时把 SVG 挤掉。
 * 顺带清掉可能由 HMR／重复挂载留下的同名多余节点。
 */
function ensureIconLink(): HTMLLinkElement {
  let link = document.getElementById(SVG_LINK_ID) as HTMLLinkElement | null
  if (link) return link

  // `rel~="icon"` 是空白分隔的整词匹配，不会误命中 apple-touch-icon
  const candidates = Array.from(
    document.querySelectorAll<HTMLLinkElement>('link[rel~="icon"]')
  )
  link = candidates.shift() ?? document.createElement('link')

  // 多余的也一并移除，否则又变回多条竞争
  candidates.forEach((extra) => extra.remove())

  link.id = SVG_LINK_ID
  link.rel = 'icon'
  if (!link.isConnected) document.head.appendChild(link)
  return link
}

/** 把 <link rel="icon"> 换成当前主题的 SVG 图标 */
export function applyFavicon(themeName?: string): void {
  if (typeof document === 'undefined') return

  const name = themeName || getCurrentTheme()
  const href = `data:image/svg+xml,${encodeURIComponent(buildFaviconSvg(name))}`
  if (appliedHref.get(SVG_LINK_ID) === href) return

  const link = ensureIconLink()
  link.type = 'image/svg+xml'
  // 矢量图标声明 sizes="any" 是规范做法；同时抹掉 .ico 时代残留的尺寸声明
  link.setAttribute('sizes', 'any')

  // 重建节点而不是原地改 href：部分浏览器不认「改 href」，只认新节点出现。
  // 全程仍然只有这一条，不会重新引入竞争。
  const parent = link.parentNode ?? document.head
  link.remove()
  link.setAttribute('href', href)
  parent.appendChild(link)

  appliedHref.set(SVG_LINK_ID, href)
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
