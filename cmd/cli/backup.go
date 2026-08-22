package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/zkep/my-geektime/internal/config"
	"github.com/zkep/my-geektime/internal/global"
	"github.com/zkep/my-geektime/internal/initialize"
	"github.com/zkep/my-geektime/internal/service"
	"gopkg.in/yaml.v3"
)

type BackupFlags struct {
	Config string `name:"config" description:"Path to config file"`
	Output string `name:"output" description:"output backup file path" default:"my-geektime-backup.tar.gz"`
}

type RestoreFlags struct {
	Config string `name:"config" description:"Path to config file"`
	File   string `name:"file" description:"backup file path to restore"`
	Force  bool   `name:"force" description:"overwrite all database data without confirmation" default:"false"`
}

// initBackup 读取配置并初始化 DB 与 Logger（备份功能不涉及文件存储）。
func (app *App) initBackup(configPath string) error {
	var (
		cfg        config.Config
		configRaw  []byte
		err        error
	)
	if configPath == "" {
		configRaw, err = app.assets.ReadFile("config.yml")
	} else {
		configRaw, err = os.ReadFile(configPath)
	}
	if err != nil {
		return err
	}
	if err = yaml.Unmarshal(configRaw, &cfg); err != nil {
		return err
	}
	global.CONF = &cfg
	global.ASSETS = app.assets
	if err = initialize.Gorm(app.ctx); err != nil {
		return err
	}
	return initialize.Logger(app.ctx)
}

// Backup 导出数据库全量备份到本地 tar.gz 文件。
func (app *App) Backup(f *BackupFlags) error {
	if err := app.initBackup(f.Config); err != nil {
		return err
	}
	buf, err := service.ExportBackup(app.ctx)
	if err != nil {
		return err
	}
	if err := os.WriteFile(f.Output, buf.Bytes(), 0644); err != nil {
		return err
	}
	fmt.Printf("backup successfully saved to %s\n", f.Output)
	return nil
}

// Restore 从备份文件全量恢复数据库（覆盖模式）。
func (app *App) Restore(f *RestoreFlags) error {
	if f.File == "" {
		return errors.New("please provide a backup file with --file")
	}
	if !f.Force {
		return errors.New("restore will overwrite all database data, please add --force to confirm")
	}
	if err := app.initBackup(f.Config); err != nil {
		return err
	}
	fh, err := os.Open(f.File)
	if err != nil {
		return err
	}
	defer func() { _ = fh.Close() }()
	if err := service.ImportBackup(app.ctx, fh); err != nil {
		return err
	}
	fmt.Println("restore successfully")
	return nil
}
