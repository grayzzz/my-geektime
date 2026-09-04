package ai

// ChatMessage 问答消息（role: user | assistant）
type ChatMessage struct {
	Role    string `json:"role" form:"role"`
	Content string `json:"content" form:"content"`
}

// ChatRequest 文章问答请求（SSE 流式响应）
type ChatRequest struct {
	Aid      string        `json:"aid" form:"aid" binding:"required"`
	Messages []ChatMessage `json:"messages" form:"messages" binding:"required"`
}
