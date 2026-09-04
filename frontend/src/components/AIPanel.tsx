import React, { useEffect, useRef, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { renderToStaticMarkup } from 'react-dom/server'
import { X, Sparkles, Send, Square, RefreshCw, FileText, MessageCircle, Download } from 'lucide-react'
import { Button, Spinner } from '@/components/ui'
import { useToast } from '@/components/ui/Toast'
import { getAISummary, streamAI, type ChatMessage } from '@/api/ai'
import { downloadFileFromBlob } from '@/utils/request'

interface AIPanelProps {
  aid: string
  title: string
  open: boolean
  onClose: () => void
}

// markdown 容器手写 Tailwind 间距类，不引入 @tailwindcss/typography
const mdClass =
  'space-y-2 break-words text-sm leading-relaxed [&_h1]:text-base [&_h2]:text-base [&_h3]:text-sm [&_h4]:text-sm [&_h1]:font-semibold [&_h2]:font-semibold [&_h3]:font-semibold [&_h4]:font-semibold [&_ul]:list-disc [&_ul]:pl-5 [&_ol]:list-decimal [&_ol]:pl-5 [&_li]:my-0.5 [&_code]:bg-gray-100 [&_code]:px-1 [&_code]:rounded [&_pre]:bg-gray-900 [&_pre]:text-gray-100 [&_pre]:p-2 [&_pre]:rounded [&_pre]:overflow-x-auto [&_a]:text-primary-500 [&_blockquote]:border-l-2 [&_blockquote]:border-gray-200 [&_blockquote]:pl-3 [&_blockquote]:text-gray-500'

type Tab = 'summary' | 'chat'

export const AIPanel: React.FC<AIPanelProps> = ({ aid, title, open, onClose }) => {
  const [tab, setTab] = useState<Tab>('summary')
  const [summary, setSummary] = useState('')
  const [summaryCached, setSummaryCached] = useState(false)
  const [summaryLoading, setSummaryLoading] = useState(false)
  const [summarizing, setSummarizing] = useState(false)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [chatInput, setChatInput] = useState('')
  const [chatting, setChatting] = useState(false)
  const abortRef = useRef<AbortController | null>(null)
  const chatBottomRef = useRef<HTMLDivElement>(null)
  const { addToast } = useToast()

  // aid 变化：重置摘要与问答历史（问答不持久化）
  useEffect(() => {
    setSummary('')
    setSummaryCached(false)
    setSummarizing(false)
    setMessages([])
    setChatting(false)
    abortRef.current?.abort()
  }, [aid])

  // 打开面板且切到摘要 tab 时查询缓存摘要
  useEffect(() => {
    if (!open || tab !== 'summary' || !aid) return
    let cancelled = false
    setSummaryLoading(true)
    getAISummary(aid)
      .then((res) => {
        if (cancelled) return
        if (res?.content) {
          setSummary(res.content)
          setSummaryCached(true)
        }
      })
      .catch(() => {
        // 查询失败不打断使用，可点击生成重新走流式
      })
      .finally(() => {
        if (!cancelled) setSummaryLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [aid, open, tab])

  // 关闭面板时中断进行中的流式请求
  useEffect(() => {
    if (!open) {
      abortRef.current?.abort()
      setSummarizing(false)
      setChatting(false)
    }
  }, [open])

  useEffect(() => {
    chatBottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, tab])

  const generateSummary = (force: boolean) => {
    if (!aid || summarizing) return
    setSummarizing(true)
    setSummary('')
    const controller = new AbortController()
    abortRef.current = controller
    void streamAI(
      '/ai/summary/generate',
      { aid, force },
      {
        onDelta: (text) => setSummary((prev) => prev + text),
        onDone: (full) => {
          setSummary(full)
          setSummaryCached(true)
          setSummarizing(false)
        },
        onError: (message) => {
          addToast(message, 'error')
          setSummarizing(false)
        },
      },
      controller.signal,
    ).finally(() => {
      setSummarizing(false)
    })
  }

  // 流式期间把增量实时刷进最后一条 assistant 占位消息
  const sendChat = () => {
    const text = chatInput.trim()
    if (!text || chatting || !aid) return
    setChatInput('')
    const userMsg: ChatMessage = { role: 'user', content: text }
    const history = [...messages, userMsg]
    setMessages([...history, { role: 'assistant', content: '' }])
    setChatting(true)
    const controller = new AbortController()
    abortRef.current = controller
    let acc = ''
    void streamAI(
      '/ai/chat',
      { aid, messages: history },
      {
        onDelta: (delta) => {
          acc += delta
          setMessages((prev) => {
            const next = [...prev]
            const last = next[next.length - 1]
            if (last?.role === 'assistant') next[next.length - 1] = { ...last, content: acc }
            return next
          })
        },
        onDone: (full) => {
          setMessages((prev) => {
            const next = [...prev]
            const last = next[next.length - 1]
            if (last?.role === 'assistant') next[next.length - 1] = { ...last, content: full || acc }
            return next
          })
          setChatting(false)
        },
        onError: (message) => {
          addToast(message, 'error')
          setMessages((prev) => {
            // 占位消息仍为空则移除，避免留下空气泡
            const last = prev[prev.length - 1]
            if (last?.role === 'assistant' && !last.content) return prev.slice(0, -1)
            return prev
          })
          setChatting(false)
        },
      },
      controller.signal,
    ).finally(() => {
      setChatting(false)
    })
  }

  const stopStream = () => {
    abortRef.current?.abort()
    setSummarizing(false)
    setChatting(false)
  }

  const safeFileName = (name: string): string =>
    name.replace(/[\\/:*?"<>|]/g, '-').replace(/\s+/g, '').trim() || '未命名'

  const escapeHtml = (s: string): string =>
    s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;')

  // 摘要 tab：导出当前 Markdown 源文本为 .md 文件
  const exportSummaryMd = () => {
    const content = `# ${title || 'AI 摘要'}\n\n${summary}\n`
    const blob = new Blob([content], { type: 'text/markdown;charset=utf-8' })
    downloadFileFromBlob(blob, `${safeFileName(title)}-AI摘要.md`)
  }

  // 问答 tab：导出当前会话为自包含 HTML（assistant 消息用与面板一致的管线渲染成 HTML）
  const exportChatHtml = () => {
    const rows = messages
      .filter((msg) => !(msg.role === 'assistant' && !msg.content))
      .map((msg) => {
        if (msg.role === 'user') {
          return `<div class="row row-user"><div class="msg msg-user">${escapeHtml(msg.content)}</div></div>`
        }
        const body = renderToStaticMarkup(
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{msg.content}</ReactMarkdown>,
        )
        return `<div class="row row-assistant"><div class="msg msg-assistant">${body}</div></div>`
      })
      .join('\n')
    const html = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${escapeHtml(title)} · AI 问答</title>
<style>
*{box-sizing:border-box}
body{margin:0;padding:24px 12px;background:#f3f4f6;color:#1f2937;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI','PingFang SC','Hiragino Sans GB','Microsoft YaHei',sans-serif}
.container{max-width:760px;margin:0 auto;background:#fff;border-radius:12px;padding:24px;box-shadow:0 1px 3px rgba(0,0,0,.08)}
.header{border-bottom:1px solid #f3f4f6;padding-bottom:16px;margin-bottom:20px}
.header h1{margin:0 0 6px;font-size:18px;color:#111827}
.header .meta{font-size:12px;color:#9ca3af}
.msg-list{display:flex;flex-direction:column;gap:12px}
.row{display:flex}
.row-user{justify-content:flex-end}
.row-assistant{justify-content:flex-start}
.msg{max-width:90%;padding:8px 12px;font-size:14px;line-height:1.6;word-break:break-word}
.msg-user{max-width:85%;background:#3b82f6;color:#fff;border-radius:16px;border-bottom-right-radius:4px;white-space:pre-wrap}
.msg-assistant{background:#f9fafb;border-radius:16px;border-bottom-left-radius:4px}
.msg-assistant>:first-child{margin-top:0}
.msg-assistant>:last-child{margin-bottom:0}
.msg-assistant p{margin:8px 0}
.msg-assistant h1,.msg-assistant h2{margin:12px 0 8px;font-size:16px;font-weight:600}
.msg-assistant h3,.msg-assistant h4{margin:10px 0 6px;font-size:14px;font-weight:600}
.msg-assistant ul{margin:8px 0;padding-left:20px;list-style:disc}
.msg-assistant ol{margin:8px 0;padding-left:20px;list-style:decimal}
.msg-assistant li{margin:2px 0}
.msg-assistant code{padding:1px 4px;border-radius:4px;background:#f3f4f6;font-size:13px;font-family:ui-monospace,SFMono-Regular,Consolas,'Liberation Mono',monospace}
.msg-assistant pre{margin:8px 0;padding:8px;border-radius:6px;background:#111827;color:#f3f4f6;overflow-x:auto}
.msg-assistant pre code{padding:0;background:transparent;color:inherit}
.msg-assistant blockquote{margin:8px 0;padding-left:12px;border-left:2px solid #e5e7eb;color:#6b7280}
.msg-assistant a{color:#3b82f6}
</style>
</head>
<body>
<div class="container">
<div class="header">
<h1>${escapeHtml(title)}</h1>
<div class="meta">导出时间：${new Date().toLocaleString('zh-CN')}</div>
</div>
<div class="msg-list">
${rows}
</div>
</div>
</body>
</html>`
    downloadFileFromBlob(new Blob([html], { type: 'text/html;charset=utf-8' }), `${safeFileName(title)}-AI问答.html`)
  }

  if (!open) return null

  return (
    <div className="fixed right-0 top-0 h-full w-[400px] max-w-[90vw] bg-white shadow-2xl z-[100] flex flex-col">
      {/* 头部 */}
      <div className="flex items-center gap-2 px-4 py-3 border-b border-gray-100 shrink-0">
        <Sparkles size={16} className="text-primary-500 shrink-0" />
        <div className="flex-1 min-w-0">
          <div className="text-sm font-semibold text-gray-800">AI 助手</div>
          <div className="text-xs text-gray-400 truncate">{title}</div>
        </div>
        <button
          onClick={onClose}
          className="p-1 rounded-full text-gray-400 hover:text-gray-600 hover:bg-gray-100 transition-colors"
          aria-label="关闭 AI 助手"
        >
          <X size={16} />
        </button>
      </div>

      {/* Tab 切换 */}
      <div className="flex border-b border-gray-100 shrink-0">
        <button
          onClick={() => setTab('summary')}
          className={`flex-1 flex items-center justify-center gap-1.5 py-2.5 text-sm transition-colors ${
            tab === 'summary'
              ? 'text-primary-500 border-b-2 border-primary-500 font-medium'
              : 'text-gray-500 hover:text-gray-700'
          }`}
        >
          <FileText size={14} />
          摘要
        </button>
        <button
          onClick={() => setTab('chat')}
          className={`flex-1 flex items-center justify-center gap-1.5 py-2.5 text-sm transition-colors ${
            tab === 'chat'
              ? 'text-primary-500 border-b-2 border-primary-500 font-medium'
              : 'text-gray-500 hover:text-gray-700'
          }`}
        >
          <MessageCircle size={14} />
          问答
        </button>
      </div>

      {/* 内容区 */}
      {tab === 'summary' ? (
        <div className="flex-1 overflow-y-auto p-4">
          {summaryLoading ? (
            <div className="flex justify-center py-8">
              <Spinner size="md" />
            </div>
          ) : summary ? (
            <div className={mdClass}>
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{summary}</ReactMarkdown>
            </div>
          ) : (
            <div className="text-center py-8 space-y-3">
              <Sparkles size={28} className="mx-auto text-gray-300" />
              <p className="text-sm text-gray-400">还没有生成过这篇文章的摘要</p>
            </div>
          )}
        </div>
      ) : (
        <div className="flex-1 overflow-y-auto p-4 space-y-3">
          {messages.length === 0 && (
            <div className="text-center py-8 space-y-3">
              <MessageCircle size={28} className="mx-auto text-gray-300" />
              <p className="text-sm text-gray-400">基于本文内容提问，历史仅保留在当前会话</p>
            </div>
          )}
          {messages.map((msg, i) =>
            msg.role === 'user' ? (
              <div key={i} className="flex justify-end">
                <div className="max-w-[85%] bg-primary-500 text-white rounded-2xl rounded-br-sm px-3 py-2 text-sm break-words whitespace-pre-wrap">
                  {msg.content}
                </div>
              </div>
            ) : (
              <div key={i} className="flex justify-start">
                <div className="max-w-[90%] bg-gray-50 rounded-2xl rounded-bl-sm px-3 py-2 w-fit">
                  {msg.content ? (
                    <div className={mdClass}>
                      <ReactMarkdown remarkPlugins={[remarkGfm]}>{msg.content}</ReactMarkdown>
                    </div>
                  ) : (
                    <Spinner size="sm" />
                  )}
                </div>
              </div>
            ),
          )}
          <div ref={chatBottomRef} />
        </div>
      )}

      {/* 底部操作区 */}
      {tab === 'summary' ? (
        <div className="p-4 border-t border-gray-100 shrink-0">
          {summarizing ? (
            <Button variant="danger" onClick={stopStream} className="w-full">
              <Square size={14} className="mr-1" />
              停止生成
            </Button>
          ) : (
            <div className="flex gap-2">
              {summary && (
                <Button variant="light" onClick={exportSummaryMd} className="flex-1">
                  <Download size={14} className="mr-1" />
                  导出MD
                </Button>
              )}
              {summaryCached ? (
                <Button variant="light" onClick={() => generateSummary(true)} className="flex-1">
                  <RefreshCw size={14} className="mr-1" />
                  重新生成
                </Button>
              ) : (
                <Button onClick={() => generateSummary(false)} className="flex-1">
                  <Sparkles size={14} className="mr-1" />
                  生成摘要
                </Button>
              )}
            </div>
          )}
        </div>
      ) : (
        <div className="p-3 border-t border-gray-100 shrink-0 space-y-2">
          {messages.length > 0 && !chatting && (
            <div className="flex justify-end">
              <Button variant="light" size="sm" onClick={exportChatHtml}>
                <Download size={14} className="mr-1" />
                导出对话
              </Button>
            </div>
          )}
          <div className="flex gap-2 items-end">
            <textarea
              value={chatInput}
              onChange={(e) => setChatInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
                  e.preventDefault()
                  sendChat()
                }
              }}
              placeholder="输入问题，Enter 发送"
              rows={2}
              disabled={chatting}
              className="flex-1 resize-none text-sm border border-gray-200 rounded-lg px-3 py-2 focus:outline-none focus:ring-1 focus:ring-primary-400 focus:border-primary-400 disabled:opacity-50"
            />
            {chatting ? (
              <Button variant="danger" size="sm" onClick={stopStream} className="shrink-0">
                <Square size={14} />
              </Button>
            ) : (
              <Button
                size="sm"
                onClick={sendChat}
                disabled={!chatInput.trim()}
                className="shrink-0"
                aria-label="发送"
              >
                <Send size={14} />
              </Button>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
