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

// PDFTimeout 针对 PDF 导出的长超时中间件（5 分钟）
// 使用 http.TimeoutHandler 而非 gin-contrib/timeout，避免覆盖已完成的响应
func PDFTimeout() gin.HandlerFunc {
	return func(c *gin.Context) {
		http.TimeoutHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.Next()
		}), 5*time.Minute, "").ServeHTTP(c.Writer, c.Request)
	}
}
