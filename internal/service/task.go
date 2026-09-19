package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/zkep/my-geektime/internal/global"
	"github.com/zkep/my-geektime/internal/model"
	"github.com/zkep/my-geektime/internal/types/geek"
	"github.com/zkep/my-geektime/internal/types/task"
	"go.uber.org/zap"
)

const (
	TASK_STATUS_PENDING  = 0x01
	TASK_STATUS_RUNNING  = 0x02
	TASK_STATUS_FINISHED = 0x03
	TASK_STATUS_ERROR    = 0x04
)

const (
	TASK_TYPE_PRODUCT = "product"
	TASK_TYPE_ARTICLE = "article"
)

var ALLStatus = []int{
	TASK_STATUS_PENDING,
	TASK_STATUS_RUNNING,
	TASK_STATUS_FINISHED,
	TASK_STATUS_ERROR,
}

func SyncProductStatistics(x *model.Task) (task.TaskStatistics, int32) {
	var statistics task.TaskStatistics
	if len(x.Statistics) > 0 {
		_ = json.Unmarshal(x.Statistics, &statistics)
	}
	if statistics.Items == nil {
		statistics.Items = make(map[int]int, 5)
	}

	type statusCount struct {
		Status int `gorm:"column:status"`
		Count  int `gorm:"column:count"`
	}
	var results []statusCount
	global.DB.Model(&model.Task{}).
		Select("status, count(*) as count").
		Where("task_pid = ? AND deleted_at = 0", x.TaskId).
		Group("status").
		Scan(&results)

	for _, item := range ALLStatus {
		statistics.Items[item] = 0
	}
	var pendingCount, runningCount, errorCount int
	for _, r := range results {
		statistics.Items[r.Status] = r.Count
		switch r.Status {
		case TASK_STATUS_PENDING:
			pendingCount = r.Count
		case TASK_STATUS_RUNNING:
			runningCount = r.Count
		case TASK_STATUS_ERROR:
			errorCount = r.Count
		}
	}
	statistics.Count = pendingCount + runningCount + errorCount + statistics.Items[TASK_STATUS_FINISHED]

	status := int32(TASK_STATUS_FINISHED)
	if pendingCount > 0 {
		status = int32(TASK_STATUS_PENDING)
	}
	if runningCount > 0 {
		status = int32(TASK_STATUS_PENDING)
	}
	if errorCount > 0 {
		status = int32(TASK_STATUS_ERROR)
	}

	raw, _ := json.Marshal(statistics)
	if status != x.Status || string(raw) != string(x.Statistics) {
		global.DB.Model(&model.Task{}).Where("id = ?", x.Id).
			Updates(map[string]interface{}{
				"status":     status,
				"statistics": raw,
			})
		x.Status = status
		x.Statistics = raw
	}

	return statistics, status
}

func BatchSyncProductStatistics(tasks []*model.Task) map[string]task.TaskStatistics {
	result := make(map[string]task.TaskStatistics, len(tasks))
	if len(tasks) == 0 {
		return result
	}

	taskIds := make([]string, 0, len(tasks))
	for _, t := range tasks {
		if t.TaskType != TASK_TYPE_PRODUCT {
			continue
		}
		taskIds = append(taskIds, t.TaskId)
	}
	if len(taskIds) == 0 {
		return result
	}

	type statusCount struct {
		TaskPid string `gorm:"column:task_pid"`
		Status  int    `gorm:"column:status"`
		Count   int    `gorm:"column:count"`
	}
	var results []statusCount
	global.DB.Model(&model.Task{}).
		Select("task_pid, status, count(*) as count").
		Where("task_pid IN ? AND deleted_at = 0", taskIds).
		Group("task_pid, status").
		Scan(&results)

	countMap := make(map[string]map[int]int)
	for _, r := range results {
		if countMap[r.TaskPid] == nil {
			countMap[r.TaskPid] = make(map[int]int)
		}
		countMap[r.TaskPid][r.Status] = r.Count
	}

	type updateItem struct {
		id         int64
		status     int32
		statistics json.RawMessage
	}
	var updates []updateItem

	for _, t := range tasks {
		if t.TaskType != TASK_TYPE_PRODUCT {
			continue
		}
		var statistics task.TaskStatistics
		if len(t.Statistics) > 0 {
			_ = json.Unmarshal(t.Statistics, &statistics)
		}
		if statistics.Items == nil {
			statistics.Items = make(map[int]int, 5)
		}

		counts := countMap[t.TaskId]
		var pendingCount, runningCount, errorCount int
		for _, s := range ALLStatus {
			c := counts[s]
			statistics.Items[s] = c
			switch s {
			case TASK_STATUS_PENDING:
				pendingCount = c
			case TASK_STATUS_RUNNING:
				runningCount = c
			case TASK_STATUS_ERROR:
				errorCount = c
			}
		}
		statistics.Count = pendingCount + runningCount + errorCount + statistics.Items[TASK_STATUS_FINISHED]

		status := int32(TASK_STATUS_FINISHED)
		if pendingCount > 0 {
			status = int32(TASK_STATUS_PENDING)
		}
		if runningCount > 0 {
			status = int32(TASK_STATUS_PENDING)
		}
		if errorCount > 0 {
			status = int32(TASK_STATUS_ERROR)
		}

		raw, _ := json.Marshal(statistics)
		if status != t.Status || string(raw) != string(t.Statistics) {
			updates = append(updates, updateItem{t.Id, status, raw})
			t.Status = status
			t.Statistics = raw
		}

		result[t.TaskId] = statistics
	}

	if len(updates) > 0 {
		for _, u := range updates {
			global.DB.Model(&model.Task{}).Where("id = ?", u.id).
				Updates(map[string]interface{}{
					"status":     u.status,
					"statistics": u.statistics,
				})
		}
	}

	return result
}

var Replaces = map[string]string{
	"/":  "-",
	"|":  "-",
	"｜":  "-",
	":":  "：",
	`"`:  "“",
	"?":  "？",
	"&":  "+",
	"\t": "",
	"\b": "",
	" ":  "",
}

func VerifyFileName(name string) string {
	for k, v := range Replaces {
		name = strings.ReplaceAll(name, k, v)
	}
	return strings.TrimSpace(name)
}

type PlayMeta struct {
	Spec         []byte
	LocalSpec    []byte
	KeyPath      string
	Ciphertext   string
	CipherMethod string
	Parts        []Part
}

type PlayMetaRequest struct {
	DowloadURL string
	Dir        string
	Filename   string
	TaskId     string
	Ciphertext []byte
	Spec       []byte
}

type Part struct {
	Src   string
	Dest  string
	IsKey bool
}

func Download(ctx context.Context, x *model.Task, data geek.ArticleData) error {
	t0 := time.Now()
	var (
		source      string
		downloadURL string
		playURL     string
		err         error
	)
	fileName := VerifyFileName(data.Info.Title)
	dir := path.Join(x.TaskPid, VerifyFileName(data.Product.Title))
	if data.Info.IsVideo {
		if len(data.Info.Video.HlsMedias) == 0 && len(data.Info.VideoPreview.Medias) > 0 {
			data.Info.Video.HlsMedias = data.Info.VideoPreview.Medias
		}
		if len(data.Info.Video.HlsMedias) == 0 && len(data.Info.VideoPreview.Medias) == 0 {
			return fmt.Errorf("article info not found or no VIP product %s", x.OtherId)
		}
		sort.Slice(data.Info.Video.HlsMedias, func(i, j int) bool {
			return data.Info.Video.HlsMedias[i].Size > data.Info.Video.HlsMedias[j].Size
		})
		downloadURL = data.Info.Video.HlsMedias[0].URL
		playURL = downloadURL
	} else if data.Info.Audio.DownloadURL != "" {
		downloadURL = data.Info.Audio.DownloadURL
		playURL = data.Info.Audio.URL
	}

	if len(downloadURL) > 0 && len(playURL) > 0 {
		rewritePlayReq := PlayMetaRequest{
			DowloadURL: playURL,
			Dir:        dir,
			Filename:   fileName,
			TaskId:     x.TaskId,
			Spec:       x.RewriteHls,
		}
		if len(x.Ciphertext) > 0 {
			cipher, err1 := base64.StdEncoding.DecodeString(x.Ciphertext)
			if err1 != nil {
				global.LOG.Error("download rewritePlay", zap.Error(err1), zap.String("taskId", x.TaskId))
				return err1
			}
			rewritePlayReq.Ciphertext = cipher
		}
		meta, err1 := RewritePlay(ctx, rewritePlayReq)
		if err1 != nil {
			global.LOG.Error("download rewritePlay", zap.Error(err1), zap.String("taskId", x.TaskId))
			return err1
		}
		x.RewriteHls = meta.Spec
		x.Ciphertext = meta.Ciphertext
		if x.Bstatus > 0 {
			if data.Info.IsVideo {
				source, err = VideoWithM3u8(ctx, dir, fileName, meta)
				if err != nil {
					global.LOG.Error("download video with m3u8 lib", zap.Error(err), zap.String("taskId", x.TaskId))
					return err
				}
			} else {
				up, _ := url.Parse(downloadURL)
				if strings.Contains(up.Path, up.Host) {
					downloadURL = strings.TrimPrefix(up.Path, "/")
				}
				source, err = Audio(ctx, x, downloadURL, dir, fileName)
				if err != nil {
					global.LOG.Error("download audio", zap.Error(err), zap.String("taskId", x.TaskId))
					return err
				}
			}
		}
	}

	if x.Bstatus > 0 {
		message := task.TaskMessage{}
		if len(source) > 0 {
			message.Object = global.Storage.GetKey(source, false)
		} else {
			message.Text = "not found download url"
		}
		x.Message, _ = json.Marshal(message)
	}

	global.LOG.Info("download end", zap.String("taskId", x.TaskId),
		zap.String("url", downloadURL), zap.Duration("cost", time.Since(t0)),
	)
	return nil
}
