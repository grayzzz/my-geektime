package router

import (
	"github.com/gin-gonic/gin"
	v2 "github.com/zkep/my-geektime/internal/api/v2"

	mw "github.com/zkep/my-geektime/internal/middleware"
)

func task(public, private *gin.RouterGroup) {
	api := v2.NewTask()
	{
		private.GET("/task/list", api.List)
		private.GET("/task/info", api.Info)
		private.GET("/task/download", mw.PDFTimeout(), api.Download)
		// 作业化下载：提交（幂等）+ 轮询进度。两个都是快接口，不需要超时中间件。
		// 完成后前端仍走上面的 /task/download 取文件（命中课程缓存，秒回）。
		private.POST("/task/download/prepare", api.DownloadPrepare)
		private.GET("/task/download/status", api.DownloadStatus)
		private.DELETE("/task/delete", api.Delete)
		private.POST("/task/retry", mw.AccessToken(), api.Retry)
		private.GET("/task/export", api.Export)
		private.GET("/task/article/comments", api.ArticleComments)
		private.GET("/task/article/discussions", api.ArticleDiscussion)
	}
	{
		public.GET("/task/kms", api.Kms)
		public.GET("/task/play.m3u8", api.Play)
		public.GET("/task/play/part", api.PlayPart)
	}
}
