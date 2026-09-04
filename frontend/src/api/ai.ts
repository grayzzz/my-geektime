import request from '@/utils/request'

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

export interface AISummaryInfo {
  aid: string
  content: string
  ai_model: string
}

export interface AIStreamHandlers {
  onDelta: (text: string) => void
  onDone: (full: string) => void
  onError: (message: string) => void
}

// SSE 事件载荷（delta / done / error 三类）
interface SSEEvent {
  type: unknown
  content?: unknown
  message?: unknown
}

const isSSEEvent = (value: unknown): value is SSEEvent =>
  typeof value === 'object' && value !== null && 'type' in value

const isAbortError = (err: unknown): boolean =>
  typeof err === 'object' && err !== null && 'name' in err && err.name === 'AbortError'

const errorMessage = (err: unknown, fallback: string): string =>
  err instanceof Error && err.message ? err.message : fallback

// 获取单篇文章的缓存摘要；无缓存时后端返回 null
export const getAISummary = (aid: string): Promise<AISummaryInfo | null> => {
  return request.get<unknown, AISummaryInfo | null>('/ai/summary', { params: { aid } })
}

// streamAI 用原生 fetch 消费 SSE 流（axios 实例有 30s 超时且不便处理流）。
// 事件协议：{"type":"delta","content":"..."} / {"type":"done","content":"..."} / {"type":"error","message":"..."}
export async function streamAI(
  path: string,
  body: unknown,
  handlers: AIStreamHandlers,
  signal?: AbortSignal,
): Promise<void> {
  let res: Response
  try {
    res = await fetch('/v2' + path, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: 'Bearer ' + (localStorage.getItem('token') || ''),
      },
      body: JSON.stringify(body),
      signal,
    })
  } catch (err: unknown) {
    if (isAbortError(err)) return
    handlers.onError(errorMessage(err, '网络请求失败'))
    return
  }
  if (!res.ok || !res.body) {
    let message = `请求失败（${res.status}）`
    try {
      const data: unknown = JSON.parse(await res.text())
      if (isSSEEvent(data) && typeof data.message === 'string') message = data.message
    } catch {
      // 非 JSON 响应体，保留默认提示
    }
    handlers.onError(message)
    return
  }
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  const dispatch = (block: string) => {
    for (const line of block.split('\n')) {
      const trimmed = line.trim()
      if (!trimmed.startsWith('data:')) continue
      const payload = trimmed.slice(5).trim()
      if (!payload) continue
      try {
        const evt: unknown = JSON.parse(payload)
        if (!isSSEEvent(evt)) continue
        if (evt.type === 'delta' && typeof evt.content === 'string') {
          handlers.onDelta(evt.content)
        } else if (evt.type === 'done') {
          handlers.onDone(typeof evt.content === 'string' ? evt.content : '')
        } else if (evt.type === 'error') {
          handlers.onError(typeof evt.message === 'string' ? evt.message : 'AI 服务出错')
        }
      } catch {
        // 忽略无法解析的事件块
      }
    }
  }
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      let idx: number
      while ((idx = buffer.indexOf('\n\n')) >= 0) {
        const block = buffer.slice(0, idx)
        buffer = buffer.slice(idx + 2)
        dispatch(block)
      }
    }
    buffer += decoder.decode()
    if (buffer.trim()) dispatch(buffer)
  } catch (err: unknown) {
    if (!isAbortError(err)) handlers.onError(errorMessage(err, '流式读取中断'))
  }
}
