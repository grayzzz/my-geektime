package initialize

import (
	"context"

	"github.com/zkep/my-geektime/internal/global"
	"github.com/zkep/my-geektime/internal/model"
	"github.com/zkep/my-geektime/libs/db"
)

func Gorm(_ context.Context) error {
	g, err := db.NewGORM(
		global.CONF.DB.Driver,
		global.CONF.DB.Source,
		db.MaxIdleConns(global.CONF.DB.MaxIdleConns),
		db.MaxOpenConns(global.CONF.DB.MaxOpenConns),
	)()
	if err != nil {
		return err
	}
	global.DB = g
	// AutoMigrate 仅创建缺失的表和缺失的字段，不会回填已有数据的默认值。
	// 新增模型（如 Progress）随版本升级自动建表，无需手动 SQL；
	// 但未来对已有表做字段变更（新增列、改约束）时，需自行编写迁移脚本
	// 对存量数据回填默认值，AutoMigrate 不会处理数据级迁移。
	if err = g.AutoMigrate(
		&model.User{},
		&model.Task{},
		&model.Article{},
		&model.ArticleSimple{},
		&model.Product{},
		&model.ArticleComment{},
		&model.ArticleCommentDiscussion{},
		&model.Collect{},
		&model.Progress{},
		&model.SysDict{},
	); err != nil {
		return err
	}
	return nil
}
