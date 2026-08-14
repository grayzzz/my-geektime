package v2

import (
	"math"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zkep/my-geektime/internal/global"
	"github.com/zkep/my-geektime/internal/model"
	"github.com/zkep/my-geektime/internal/types/progress"
	"gorm.io/gorm"
)

type Progress struct{}

func NewProgress() *Progress {
	return &Progress{}
}

// Save 保存单章进度（upsert，uid+task_id 唯一）
// is_finished 不回退：曾为 true 则不再置 false
func (p *Progress) Save(c *gin.Context) {
	var req progress.SaveRequest
	if err := c.ShouldBind(&req); err != nil {
		global.FAIL(c, "fail.msg", err.Error())
		return
	}
	if req.TaskId == "" {
		global.FAIL(c, "fail.msg", "task_id is required")
		return
	}
	// task_pid 为空时反查（防御：前端旧版本可能没传 task_pid，
	// 但课程进度汇总按 task_pid 统计，缺失会导致章节白学）
	if req.TaskPid == "" {
		var t model.Task
		if err := global.DB.Model(&model.Task{}).
			Where("task_id = ? AND deleted_at = ?", req.TaskId, 0).
			First(&t).Error; err == nil {
			req.TaskPid = t.TaskPid
		}
	}
	identity := c.GetString(global.Identity)
	now := time.Now().Unix()

	var item model.Progress
	err := global.DB.Where("uid = ? AND task_id = ? AND deleted_at = ?", identity, req.TaskId, 0).
		First(&item).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		global.FAIL(c, "fail.msg", err.Error())
		return
	}

	if err == gorm.ErrRecordNotFound {
		// 插入新记录
		item = model.Progress{
			Uid:            identity,
			TaskId:         req.TaskId,
			TaskPid:        req.TaskPid,
			TaskType:       req.TaskType,
			Position:       req.Position,
			ScrollPosition: req.ScrollPosition,
			Duration:       req.Duration,
			IsFinished:     req.IsFinished,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := global.DB.Create(&item).Error; err != nil {
			global.FAIL(c, "fail.msg", err.Error())
			return
		}
		global.OK(c, nil)
		return
	}

	// 存在则更新。
	// is_finished 默认不回退（防误改/防滚动抖动把已完成覆盖掉）。
	// 仅当请求显式 ResetProgress=true 时，允许清除已完成标记（修复场景）。
	updates := map[string]any{
		"task_pid":        req.TaskPid,
		"task_type":       req.TaskType,
		"position":        req.Position,
		"scroll_position": req.ScrollPosition,
		"duration":        req.Duration,
		"updated_at":      now,
	}
	switch {
	case req.IsFinished:
		updates["is_finished"] = true
	case item.IsFinished && req.ResetProgress:
		updates["is_finished"] = false
	}
	if err := global.DB.Model(&model.Progress{}).
		Where("id = ?", item.Id).
		Updates(updates).Error; err != nil {
		global.FAIL(c, "fail.msg", err.Error())
		return
	}
	global.OK(c, nil)
}

// Get 获取单章进度（无则返回空对象）
func (p *Progress) Get(c *gin.Context) {
	var req progress.GetRequest
	if err := c.ShouldBind(&req); err != nil {
		global.FAIL(c, "fail.msg", err.Error())
		return
	}
	if req.TaskId == "" {
		global.FAIL(c, "fail.msg", "task_id is required")
		return
	}
	identity := c.GetString(global.Identity)

	var item model.Progress
	err := global.DB.Where("uid = ? AND task_id = ? AND deleted_at = ?", identity, req.TaskId, 0).
		First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			global.OK(c, progress.ProgressResponse{})
			return
		}
		global.FAIL(c, "fail.msg", err.Error())
		return
	}
	global.OK(c, toResponse(&item))
}

// Course 课程进度汇总：完成章节数 / 总章节数 + 上次学习信息
func (p *Progress) Course(c *gin.Context) {
	var req progress.CourseRequest
	if err := c.ShouldBind(&req); err != nil {
		global.FAIL(c, "fail.msg", err.Error())
		return
	}
	if req.TaskPid == "" {
		global.FAIL(c, "fail.msg", "task_pid is required")
		return
	}
	identity := c.GetString(global.Identity)

	ret := progress.CourseProgressResponse{}

	var finishedCount int64
	if err := global.DB.Model(&model.Progress{}).
		Where("uid = ? AND task_pid = ? AND is_finished = ? AND deleted_at = ?", identity, req.TaskPid, true, 0).
		Count(&finishedCount).Error; err != nil {
		global.FAIL(c, "fail.msg", err.Error())
		return
	}
	ret.FinishedCount = finishedCount

	// 总章节数 = 该课程 pid 下的 article 任务数（status 无关，含未缓存章节？只统计已存在的）
	var totalCount int64
	if err := global.DB.Model(&model.Task{}).
		Where("task_pid = ? AND task_type = ? AND deleted_at = ?", req.TaskPid, "article", 0).
		Count(&totalCount).Error; err != nil {
		global.FAIL(c, "fail.msg", err.Error())
		return
	}
	ret.TotalCount = totalCount
	if totalCount > 0 {
		ret.Percent = math.Round(float64(finishedCount)*10000/float64(totalCount)) / 100
	}

	// 该课程下最新 updated_at 的章节（上次学到哪）
	var last model.Progress
	err := global.DB.Where("uid = ? AND task_pid = ? AND deleted_at = ?", identity, req.TaskPid, 0).
		Order("updated_at DESC").First(&last).Error
	if err == nil {
		ret.UpdatedAt = last.UpdatedAt
		ret.LastTaskId = last.TaskId
		var t model.Task
		if err2 := global.DB.Model(&model.Task{}).
			Where("task_id = ?", last.TaskId).First(&t).Error; err2 == nil {
			ret.LastTaskName = t.TaskName
		}
	} else if err != gorm.ErrRecordNotFound {
		global.FAIL(c, "fail.msg", err.Error())
		return
	}

	global.OK(c, ret)
}

// My 当前用户全部进度（分页）
func (p *Progress) My(c *gin.Context) {
	var req progress.MyRequest
	if err := c.ShouldBind(&req); err != nil {
		global.FAIL(c, "fail.msg", err.Error())
		return
	}
	if req.PerPage <= 0 || req.PerPage > 200 {
		req.PerPage = 10
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	identity := c.GetString(global.Identity)

	ret := progress.MyProgressResponse{
		Rows: make([]progress.ProgressResponse, 0),
	}
	var ls []*model.Progress
	tx := global.DB.Model(&model.Progress{}).
		Where("uid = ? AND deleted_at = ?", identity, 0)
	if err := tx.Count(&ret.Count).
		Offset((req.Page - 1) * req.PerPage).
		Limit(req.PerPage).
		Order("updated_at DESC").
		Find(&ls).Error; err != nil {
		global.FAIL(c, "fail.msg", err.Error())
		return
	}
	for _, l := range ls {
		ret.Rows = append(ret.Rows, toResponse(l))
	}
	global.OK(c, ret)
}

func toResponse(m *model.Progress) progress.ProgressResponse {
	return progress.ProgressResponse{
		Id:             m.Id,
		Uid:            m.Uid,
		TaskId:         m.TaskId,
		TaskPid:        m.TaskPid,
		TaskType:       m.TaskType,
		Position:       m.Position,
		ScrollPosition: m.ScrollPosition,
		Duration:       m.Duration,
		IsFinished:     m.IsFinished,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}
