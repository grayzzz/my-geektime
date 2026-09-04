package ai

// GenerateRequest 生成摘要请求（SSE 流式响应）
type GenerateRequest struct {
	Aid   string `json:"aid" form:"aid" binding:"required"`
	Force bool   `json:"force" form:"force"`
}

// SummaryResponse 摘要缓存响应
type SummaryResponse struct {
	Aid     string `json:"aid"`
	Content string `json:"content"`
	AIModel string `json:"ai_model"`
}
