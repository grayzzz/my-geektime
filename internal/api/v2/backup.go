package v2

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zkep/my-geektime/internal/global"
	"github.com/zkep/my-geektime/internal/service"
	"github.com/zkep/my-geektime/internal/types/user"
)

// maxBackupUploadSize 上传备份文件的大小上限，防止超大文件撑爆内存/磁盘。
const maxBackupUploadSize = 2 << 30 // 2GB

type Backup struct{}

func NewBackup() *Backup {
	return &Backup{}
}

// adminOnly 校验当前登录用户是否为管理员。
func adminOnly(c *gin.Context) bool {
	roleId := c.GetFloat64(global.Role)
	if roleId != user.AdminRoleId {
		global.FAIL(c, "fail.msg", "no auth")
		return false
	}
	return true
}

// Export 导出数据库全量备份为 tar.gz 附件下载。
func (b *Backup) Export(c *gin.Context) {
	if !adminOnly(c) {
		return
	}
	buf, err := service.ExportBackup(c)
	if err != nil {
		global.FAIL(c, "fail.msg", err.Error())
		return
	}
	fileName := fmt.Sprintf("my-geektime-backup-%d.tar.gz", time.Now().Unix())
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", "attachment; filename="+url.QueryEscape(fileName))
	c.Header("Content-Transfer-Encoding", "binary")
	c.Data(200, "application/octet-stream", buf.Bytes())
}

// Import 上传备份文件并全量恢复数据库（覆盖模式）。
func (b *Backup) Import(c *gin.Context) {
	if !adminOnly(c) {
		return
	}
	// 限制上传体积（含 multipart 封装开销），超限时 FormFile 返回错误
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBackupUploadSize)
	fh, err := c.FormFile("file")
	if err != nil {
		global.FAIL(c, "fail.msg", "please upload a backup file (<=2GB)")
		return
	}
	src, err := fh.Open()
	if err != nil {
		global.FAIL(c, "fail.msg", err.Error())
		return
	}
	defer func() { _ = src.Close() }()

	if err := service.ImportBackup(c, src); err != nil {
		global.FAIL(c, "fail.msg", err.Error())
		return
	}
	global.OK(c, gin.H{"restored": true})
}
