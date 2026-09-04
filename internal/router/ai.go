package router

import (
	"github.com/gin-gonic/gin"
	v2 "github.com/zkep/my-geektime/internal/api/v2"
)

// ai 不挂 timeout 中间件——超时由 service 内 ctx 截止时间控制，
// 避免 http.TimeoutHandler 半途写 503 破坏 SSE 帧。
func ai(_, private *gin.RouterGroup) {
	api := v2.NewAI()
	p := private.Group("ai")
	{
		p.GET("/summary", api.Summary)
		p.POST("/summary/generate", api.GenerateSummary)
		p.POST("/chat", api.Chat)
	}
}
