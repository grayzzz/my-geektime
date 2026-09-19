package cli

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/zkep/my-geektime/internal/types/sys_dict"
)

type Flags struct {
	Pid int64   `name:"pid"  description:"product id"`
	Id  []int64 `name:"id"  description:"article id"`
	Dir string  `name:"dir" description:"output directory" default:"./"`
}

type ConfigFlags struct {
	Config string `name:"config" default:"config_templete.yml" description:"generate config file"`
}

type App struct {
	ctx    context.Context
	quit   <-chan os.Signal
	assets embed.FS
}

func NewApp(ctx context.Context, quit <-chan os.Signal, assets embed.FS) *App {
	return &App{ctx, quit, assets}
}

func (app *App) Config(f *ConfigFlags) error {
	fi, err := app.assets.Open("config.yml")
	if err != nil {
		return err
	}
	defer func() { _ = fi.Close() }()
	fs, err := os.Create(f.Config)
	if err != nil {
		return err
	}
	defer func() { _ = fs.Close() }()
	_, err = io.Copy(fs, fi)
	if err != nil {
		return err
	}
	fmt.Printf("successfully created %s\n", f.Config)
	return nil
}

func (app *App) loadTagData() (sys_dict.TagData, error) {
	var tagData sys_dict.TagData
	if tagRaw, err := os.ReadFile("web/pages/tags.json"); err == nil {
		if err = json.Unmarshal(tagRaw, &tagData); err != nil {
			return tagData, err
		}
	} else if tagRaw, err := app.assets.ReadFile("web/tags.json"); err != nil {
		return tagData, err
	} else {
		var tags []sys_dict.Tag
		if err := json.Unmarshal(tagRaw, &tags); err != nil {
			return tagData, err
		}
		tagData.Data = tags
	}
	return tagData, nil
}
