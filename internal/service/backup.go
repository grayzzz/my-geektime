package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"time"

	"github.com/zkep/my-geektime/internal/global"
	"github.com/zkep/my-geektime/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	backupFormatVersion = 1
	backupManifestFile  = "manifest.json"
	backupTablesDir     = "tables"
	// maxBackupEntrySize 单个解压条目大小上限，防止解压炸弹（zip bomb）撑爆内存
	maxBackupEntrySize = 2 << 30 // 2GB
	// maxBackupEntryCount 归档条目数上限
	maxBackupEntryCount = 64
)

// BackupManifest 备份包元信息，用于导入时校验格式兼容性。
type BackupManifest struct {
	FormatVersion int      `json:"format_version"`
	AppVersion    string   `json:"app_version"`
	CreatedAt     int64    `json:"created_at"`
	Tables        []string `json:"tables"`
}

// backupTable 描述一张参与备份的表。
type backupTable struct {
	name string // 逻辑名（GORM 模型名，也是 manifest 中的标识）
	rows any    // 指向切片，如 &[]*model.User{}
	dst  any    // 模型实例，用于 Delete
}

// backupTables 返回全部参与备份的表，顺序与 initialize/gorm.go AutoMigrate 一致。
func backupTables() []backupTable {
	return []backupTable{
		{name: "User", rows: &[]*model.User{}, dst: &model.User{}},
		{name: "Task", rows: &[]*model.Task{}, dst: &model.Task{}},
		{name: "Article", rows: &[]*model.Article{}, dst: &model.Article{}},
		{name: "ArticleSimple", rows: &[]*model.ArticleSimple{}, dst: &model.ArticleSimple{}},
		{name: "Product", rows: &[]*model.Product{}, dst: &model.Product{}},
		{name: "ArticleComment", rows: &[]*model.ArticleComment{}, dst: &model.ArticleComment{}},
		{name: "ArticleCommentDiscussion", rows: &[]*model.ArticleCommentDiscussion{}, dst: &model.ArticleCommentDiscussion{}},
		{name: "Collect", rows: &[]*model.Collect{}, dst: &model.Collect{}},
		{name: "Progress", rows: &[]*model.Progress{}, dst: &model.Progress{}},
		{name: "SysDict", rows: &[]*model.SysDict{}, dst: &model.SysDict{}},
	}
}

// tableName 通过 GORM NamingStrategy 动态获取实际表名，规避复数化差异。
func tableName(name string) string {
	return global.DB.NamingStrategy.TableName(name)
}

// ExportBackup 导出全部数据库内容为 tar.gz 备份包。
// 只包含数据库元数据，不包含下载的音视频/md/pdf 文件本体。
func ExportBackup(ctx context.Context) (*bytes.Buffer, error) {
	tables := backupTables()
	manifest := BackupManifest{
		FormatVersion: backupFormatVersion,
		AppVersion:    appVersion(),
		CreatedAt:     time.Now().Unix(),
		Tables:        make([]string, 0, len(tables)),
	}

	// 逐表读取全量数据
	data := make(map[string][]byte, len(tables))
	for _, tb := range tables {
		if err := global.DB.WithContext(ctx).Model(tb.dst).Find(tb.rows).Error; err != nil {
			return nil, fmt.Errorf("query %s failed: %w", tb.name, err)
		}
		raw, err := json.Marshal(tb.rows)
		if err != nil {
			return nil, fmt.Errorf("marshal %s failed: %w", tb.name, err)
		}
		data[tb.name] = raw
		manifest.Tables = append(manifest.Tables, tb.name)
	}

	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest failed: %w", err)
	}

	// 打包 tar.gz
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	if err := writeTarFile(tw, backupManifestFile, manifestRaw); err != nil {
		return nil, err
	}
	for _, tb := range tables {
		if err := writeTarFile(tw,
			fmt.Sprintf("%s/%s.json", backupTablesDir, tableName(tb.name)),
			data[tb.name]); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}

	global.LOG.Info("backup exported", zap.Int("size", buf.Len()))
	return &buf, nil
}

// ImportBackup 从 tar.gz 备份包全量恢复数据库（覆盖模式）。
// 先完整读取并校验 manifest + 每张表 JSON，校验失败在开启事务前拒绝，不会改动数据库。
func ImportBackup(ctx context.Context, r io.Reader) error {
	files, err := readTarGz(r)
	if err != nil {
		return err
	}

	manifestRaw, ok := files[backupManifestFile]
	if !ok {
		return errors.New("backup file is invalid: manifest.json not found")
	}
	var manifest BackupManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return errors.New("backup file is invalid: parse manifest failed")
	}
	if manifest.FormatVersion != backupFormatVersion {
		return fmt.Errorf("backup format version %d is not supported (expect %d)",
			manifest.FormatVersion, backupFormatVersion)
	}

	// 事务开启前校验每张表 JSON 可解析
	tables := backupTables()
	for _, tb := range tables {
		raw, ok := files[fmt.Sprintf("%s/%s.json", backupTablesDir, tableName(tb.name))]
		if !ok {
			return fmt.Errorf("backup file is invalid: table %s not found", tb.name)
		}
		if err := json.Unmarshal(raw, tb.rows); err != nil {
			return fmt.Errorf("backup file is invalid: parse %s failed: %w", tb.name, err)
		}
	}

	// 有正在运行的下载任务时拒绝，避免任务/数据状态不一致
	var running int64
	if err := global.DB.WithContext(ctx).Model(&model.Task{}).
		Where("status = ?", TASK_STATUS_RUNNING).Count(&running).Error; err != nil {
		return err
	}
	if running > 0 {
		return errors.New("there are running download tasks, please wait or stop them before restore")
	}

	return global.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 全量清空所有表
		for _, tb := range tables {
			if err := tx.Where("1=1").Delete(tb.dst).Error; err != nil {
				return fmt.Errorf("clear %s failed: %w", tb.name, err)
			}
		}
		// 写入备份数据
		for _, tb := range tables {
			if err := tx.CreateInBatches(tb.rows, 500).Error; err != nil {
				return fmt.Errorf("insert %s failed: %w", tb.name, err)
			}
		}
		return nil
	})
}

func writeTarFile(tw *tar.Writer, name string, content []byte) error {
	if err := tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0644,
		Size: int64(len(content)),
	}); err != nil {
		return err
	}
	if _, err := tw.Write(content); err != nil {
		return err
	}
	return nil
}

// readTarGz 解压 tar.gz 为 文件名 -> 内容 的映射。
// 对单个条目大小和条目总数设上限，防御解压炸弹。
func readTarGz(r io.Reader) (map[string][]byte, error) {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("open backup file failed: %w", err)
	}
	defer func() { _ = gzr.Close() }()
	tr := tar.NewReader(gzr)
	files := make(map[string][]byte)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read backup file failed: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if len(files) >= maxBackupEntryCount {
			return nil, errors.New("backup file is invalid: too many entries")
		}
		if hdr.Size > maxBackupEntrySize {
			return nil, fmt.Errorf("backup file is invalid: entry %s too large", hdr.Name)
		}
		// 用 LimitReader 兜底，防止 tar 头声明的 size 与实际内容不符
		content, err := io.ReadAll(io.LimitReader(tr, maxBackupEntrySize+1))
		if err != nil {
			return nil, fmt.Errorf("read backup file failed: %w", err)
		}
		if int64(len(content)) > maxBackupEntrySize {
			return nil, fmt.Errorf("backup file is invalid: entry %s too large", hdr.Name)
		}
		files[hdr.Name] = content
	}
	return files, nil
}

func appVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "unknown"
}
