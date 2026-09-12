package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zkep/my-geektime/internal/config"
	"github.com/zkep/my-geektime/internal/global"
	"github.com/zkep/my-geektime/internal/initialize"
	"github.com/zkep/my-geektime/internal/model"
	"github.com/zkep/my-geektime/internal/service"
	"github.com/zkep/my-geektime/internal/types/geek"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// EPUB 电子书导出 CLI。
//
// 与 Web 端共用 service.EpubGenerator，输出单文件 .epub，
// 图片全部内嵌，脱离本项目服务也能阅读。
//
// 用法：
//
//	my-geektime cli epub --config=config.yml --taskid=<courseId> --output=./out
//	my-geektime cli epub --config=config.yml --output=./out            # 导出全部课程
//	my-geektime cli epub --comments=hot --output=./out                 # 只取精选留言
//	my-geektime cli epub --comments=0   --output=./out                 # 不含留言

type EpubFlags struct {
	Config string `name:"config" description:"Path to config file"`
	TaskID string `name:"taskid" description:"task id, empty means all courses" default:""`
	Output string `name:"output" description:"output directory for epub files" default:"epub"`
	// comments 评论模式：all（默认，全部留言）| hot（每篇 20 条按赞）| 0（不含）
	Comments string `name:"comments" description:"comments mode: all | hot | 0" default:"all"`
}

// initEpub 读取配置并初始化 DB、Logger、Storage。
// 与 cmd/cli/docs.go 的初始化流程保持一致（epub 需要 Storage 来读写图片缓存）。
func (app *App) initEpub(configPath string) error {
	var (
		cfg       config.Config
		configRaw []byte
		err       error
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
	if err = initialize.Logger(app.ctx); err != nil {
		return err
	}
	return initialize.Storage(app.ctx)
}

// Epub 把已缓存的课程导出为 epub 电子书。
func (app *App) Epub(f *EpubFlags) error {
	if err := app.initEpub(f.Config); err != nil {
		return err
	}
	opt := service.NormalizeEpubOptions(service.EpubOptions{Comments: f.Comments})
	pids, err := epubCourseIds(f.TaskID)
	if err != nil {
		return err
	}
	if len(pids) == 0 {
		return fmt.Errorf("no cached course found, " +
			"please download a course first or check the --taskid value")
	}
	if err = os.MkdirAll(f.Output, 0755); err != nil {
		return err
	}

	generator := service.NewEpubGenerator()
	succeeded, failed := 0, 0
	for i, pid := range pids {
		out, err := epubExportOne(app, generator, pid, f.Output, opt)
		if err != nil {
			failed++
			fmt.Printf("[%d/%d] %s failed: %v\n", i+1, len(pids), pid, err)
			global.LOG.Error("epub export failed", zap.String("taskId", pid), zap.Error(err))
			continue
		}
		succeeded++
		fmt.Printf("[%d/%d] %s\n", i+1, len(pids), out)
	}
	fmt.Printf("\nepub export finished: %d succeeded, %d failed, comments=%s\n",
		succeeded, failed, opt.Comments)
	if succeeded == 0 {
		return fmt.Errorf("all %d course(s) failed to export", failed)
	}
	return nil
}

// epubExportOne 导出单门课程，返回写出的文件路径。
func epubExportOne(app *App, generator *service.EpubGenerator, pid, outputDir string,
	opt service.EpubOptions) (string, error) {
	var root model.Task
	if err := global.DB.Model(&model.Task{}).
		Where(&model.Task{TaskId: pid}).First(&root).Error; err != nil {
		return "", fmt.Errorf("query task failed: %w", err)
	}
	var product geek.ProductBase
	if err := json.Unmarshal(root.Raw, &product); err != nil {
		return "", fmt.Errorf("unmarshal product failed: %w", err)
	}
	title := strings.TrimSpace(product.Title)
	if title == "" {
		title = strings.TrimSpace(root.TaskName)
	}
	buf, err := generator.MakeEpub(app.ctx, pid, product.Title, product.IntroHTML, opt)
	if err != nil {
		return "", err
	}
	name := service.VerifyFileName(title) + ".epub"
	path := filepath.Join(outputDir, name)
	// 同名课程（不同 taskId）不要互相覆盖
	if _, statErr := os.Stat(path); statErr == nil {
		path = filepath.Join(outputDir,
			fmt.Sprintf("%s-%s.epub", service.VerifyFileName(title), pid))
	}
	if err = os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s (%d KB)", path, buf.Len()/1024), nil
}

// epubCourseIds 解析待导出的课程 task id。
//
// 只取「确有已缓存文章」的课程（即存在 task_pid 指向它的子任务），
// 避免把大量没有缓存内容的根任务也当成课程去导出。
func epubCourseIds(taskId string) ([]string, error) {
	if strings.TrimSpace(taskId) != "" {
		return []string{strings.TrimSpace(taskId)}, nil
	}
	var pids []string
	if err := global.DB.Model(&model.Task{}).
		Where("task_pid <> ?", "").
		Distinct().
		Pluck("task_pid", &pids).Error; err != nil {
		return nil, fmt.Errorf("query course list failed: %w", err)
	}
	return pids, nil
}
