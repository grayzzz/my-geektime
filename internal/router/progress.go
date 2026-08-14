package router

import (
	"github.com/gin-gonic/gin"
	v2 "github.com/zkep/my-geektime/internal/api/v2"
)

func progress(_, private *gin.RouterGroup) {
	api := v2.NewProgress()
	{
		private.POST("/progress/save", api.Save)
		private.GET("/progress/get", api.Get)
		private.GET("/progress/course", api.Course)
		private.GET("/progress/my", api.My)
	}
}