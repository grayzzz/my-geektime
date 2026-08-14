package router

import (
	"embed"
	"net/http"
	"path"
	"strings"

	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/zkep/my-geektime/internal/config"
	"github.com/zkep/my-geektime/internal/global"
	mw "github.com/zkep/my-geektime/internal/middleware"
)

func NewRouter(assets embed.FS) (*gin.Engine, error) {
	e := gin.Default()

	e.Use(mw.Cors())

	// 禁用前端静态资源缓存，确保更新后立即生效
	e.Use(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/assets/") || strings.HasSuffix(path, ".html") {
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
		}
		c.Next()
	})

	ef, err := static.EmbedFolder(assets, "web")
	if err != nil {
		return e, err
	}

	e.Use(static.Serve("/", ef))

	e.NoRoute(func(c *gin.Context) {
		reqPath := c.Request.URL.Path
		lastSlash := strings.LastIndex(reqPath, "/")
		fileName := reqPath[lastSlash+1:]
		hasExt := strings.Contains(fileName, ".")
		if !hasExt {
			c.FileFromFS("/", ef)
			return
		}
		c.Status(http.StatusNotFound)
	})

	if global.CONF.Storage.Driver == config.StorageLocal {
		e.StaticFS(path.Join("/", global.CONF.Storage.Bucket),
			gin.Dir(global.CONF.Storage.Directory, true))
	}

	public := e.Group("v2")
	private := e.Group("v2", mw.JWTMiddleware())

	base(public, private)

	dict(public, private)

	product(public, private)

	task(public, private)

	user(public, private)

	file(public, private)

	setting(public, private)

	collect(public, private)

	progress(public, private)

	return e, nil
}
