package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func TimeoutResponse(c *gin.Context) {
	c.JSON(
		http.StatusRequestTimeout,
		gin.H{"status": http.StatusRequestTimeout, "msg": "request timeout"},
	)
}

func Timeout() gin.HandlerFunc {
	return func(c *gin.Context) {
		http.TimeoutHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.Next()
		}), 30*time.Second, "").ServeHTTP(c.Writer, c.Request)
	}
}

// PDFTimeout 针对 PDF 导出的长超时中间件
// 使用 http.TimeoutHandler 而非 gin-contrib/timeout，避免覆盖已完成的响应
//
// ⚠️ 这个值必须**大于** service.GenerateCoursePDF 的 detCtx 预算
// （pdfCourseBudgetMax = 55min，见 internal/service/pdf.go）。若中间件先到期，
// http.TimeoutHandler 会写 503 空 body 且丢弃业务结果 —— 前端只拿到空响应，
// 且不会有任何错误信息，是最难排查的"静默失败"。
// 改这里的值时务必同步检查 pdf.go 里的 pdfCourseBudgetMax。
func PDFTimeout() gin.HandlerFunc {
	return func(c *gin.Context) {
		http.TimeoutHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.Next()
		}), 60*time.Minute, "").ServeHTTP(c.Writer, c.Request)
	}
}
