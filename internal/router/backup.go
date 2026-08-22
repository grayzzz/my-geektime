package router

import (
	"github.com/gin-gonic/gin"
	v2 "github.com/zkep/my-geektime/internal/api/v2"
)

func backup(_, private *gin.RouterGroup) {
	api := v2.NewBackup()
	p := private.Group("/backup")
	{
		p.GET("/export", api.Export)
		p.POST("/import", api.Import)
	}
}
