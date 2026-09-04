package v2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zkep/my-geektime/internal/global"
	"github.com/zkep/my-geektime/internal/model"
	"github.com/zkep/my-geektime/internal/service"
	typesai "github.com/zkep/my-geektime/internal/types/ai"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AI struct{}

func NewAI() *AI {
	return &AI{}
}

// writeSSE 写出一帧 SSE 事件（data: <json>\n\n）并立即刷新。
// 事件协议：{"type":"delta","content":"..."} 增量；
// {"type":"done","content":"<完整文本>"} 结束；{"type":"error","message":"..."} 失败。
func writeSSE(c *gin.Context, payload any) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(c.Writer, "data: %s\n\n", data)
	c.Writer.Flush()
}

// aiErrorMessage 把 service 层错误映射为面向用户的中文提示。
func aiErrorMessage(err error) string {
	switch {
	case errors.Is(err, service.ErrAINotConfigured):
		return "请先在设置页配置 AI 服务（base_url / api_key / model）"
	case errors.Is(err, service.ErrArticleNotCached):
		return "该文章尚未缓存，请先打开一次文章详情后再试"
	default:
		return err.Error()
	}
}

// Summary 查询单篇文章的缓存摘要；未命中返回 data: null。
func (a *AI) Summary(c *gin.Context) {
	aid := c.Query("aid")
	if aid == "" {
		global.FAIL(c, "fail.msg", "aid required")
		return
	}
	var item model.AISummary
	err := global.DB.Where(&model.AISummary{Aid: aid}).First(&item).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			global.FAIL(c, "fail.msg", err)
			return
		}
		global.OK(c, nil)
		return
	}
	global.OK(c, typesai.SummaryResponse{
		Aid:     item.Aid,
		Content: item.Content,
		AIModel: item.AIModel,
	})
}

// GenerateSummary 生成文章摘要（SSE 流式）。force=true 忽略缓存重新生成；
// 缓存命中时以一个 delta（全文）+ done 秒回，前端渲染逻辑与流式一致。
func (a *AI) GenerateSummary(c *gin.Context) {
	var req typesai.GenerateRequest
	if err := c.BindJSON(&req); err != nil {
		global.FAIL(c, "fail.msg", err)
		return
	}
	title, text, err := service.ArticleTextForAI(req.Aid)
	if err != nil {
		writeSSE(c, gin.H{"type": "error", "message": aiErrorMessage(err)})
		return
	}
	if !req.Force {
		var item model.AISummary
		if e := global.DB.Where(&model.AISummary{Aid: req.Aid}).First(&item).Error; e == nil {
			writeSSE(c, gin.H{"type": "delta", "content": item.Content})
			writeSSE(c, gin.H{"type": "done", "content": item.Content})
			return
		}
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), service.AIRequestTimeout)
	defer cancel()
	full, err := service.SummarizeStream(ctx, title, text, func(t string) {
		writeSSE(c, gin.H{"type": "delta", "content": t})
	})
	if err != nil {
		writeSSE(c, gin.H{"type": "error", "message": aiErrorMessage(err)})
		return
	}
	if e := global.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "aid"}},
		DoUpdates: clause.AssignmentColumns([]string{"content", "ai_model", "updated_at"}),
	}).Create(&model.AISummary{
		Aid:     req.Aid,
		Content: full,
		AIModel: global.CONF.AI.Model,
	}).Error; e != nil {
		writeSSE(c, gin.H{"type": "error", "message": e.Error()})
		return
	}
	writeSSE(c, gin.H{"type": "done", "content": full})
}

// Chat 文章问答（SSE 流式），历史随请求传递，不落库。
func (a *AI) Chat(c *gin.Context) {
	var req typesai.ChatRequest
	if err := c.BindJSON(&req); err != nil {
		global.FAIL(c, "fail.msg", err)
		return
	}
	title, text, err := service.ArticleTextForAI(req.Aid)
	if err != nil {
		writeSSE(c, gin.H{"type": "error", "message": aiErrorMessage(err)})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), service.AIRequestTimeout)
	defer cancel()
	var full strings.Builder
	err = service.ChatStream(ctx, title, text, req.Messages, func(t string) {
		full.WriteString(t)
		writeSSE(c, gin.H{"type": "delta", "content": t})
	})
	if err != nil {
		writeSSE(c, gin.H{"type": "error", "message": aiErrorMessage(err)})
		return
	}
	writeSSE(c, gin.H{"type": "done", "content": full.String()})
}
