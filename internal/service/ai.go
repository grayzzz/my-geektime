package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/zkep/my-geektime/internal/model"
	"github.com/zkep/my-geektime/internal/global"
	"github.com/zkep/my-geektime/internal/types/ai"
	"github.com/zkep/my-geektime/internal/types/geek"
)

const (
	// AIRequestTimeout 单次 LLM 请求上限
	AIRequestTimeout = 180 * time.Second
	// aiMaxArticleChars 送入上下文的文章字符上限（按 rune）
	aiMaxArticleChars = 60000
	// aiMaxHistoryMsgs 问答保留的最近消息条数（不持久化）
	aiMaxHistoryMsgs = 20
)

var (
	ErrAINotConfigured  = errors.New("ai not configured")
	ErrArticleNotCached = errors.New("article not cached")
)

const aiSummarizeSystemPrompt = "你是一位专业的文章摘要助手。请基于给定的文章内容，用简体中文输出 Markdown 格式的摘要，结构如下：\n" +
	"## 核心观点\n（2-3 句话概括）\n## 要点\n（用列表列出最多 8 条关键内容）\n## 关键结论\n（1-2 句）\n" +
	"只使用文章中出现的信息，不要编造。"

const aiChatSystemPrompt = "你是文章阅读助手。请仅依据给定的文章内容回答用户问题，用简体中文；如果问题超出文章内容，明确说明文章中没有相关信息。"

// ArticleTextForAI 从本地缓存的文章原始 JSON 中提取标题与正文文本。
// 只服务已缓存内容（打开过文章详情即有缓存），不做在线拉取兜底。
func ArticleTextForAI(aid string) (title, text string, err error) {
	var art model.Article
	if e := global.DB.Where(&model.Article{Aid: aid}).First(&art).Error; e != nil {
		return "", "", ErrArticleNotCached
	}
	if len(art.Raw) == 0 {
		return "", "", ErrArticleNotCached
	}
	var resp geek.ArticleInfoResponse
	if e := json.Unmarshal(art.Raw, &resp); e != nil {
		return "", "", e
	}
	info := resp.Data.Info
	content := info.ContentMd
	if content == "" {
		content = stripHTML(info.Content)
	}
	if content == "" {
		return "", "", ErrArticleNotCached
	}
	if runes := []rune(content); len(runes) > aiMaxArticleChars {
		content = string(runes[:aiMaxArticleChars]) + "\n\n[文章内容过长已截断]"
	}
	return art.Title, content, nil
}

var (
	aiBreakRe = regexp.MustCompile(`(?i)<(br|/p)[^>]*>`)
	aiTagRe   = regexp.MustCompile(`<[^>]*>`)
	aiSpaceRe = regexp.MustCompile(`[ \t]+`)
	aiBlankRe = regexp.MustCompile(`\n{3,}`)
)

// stripHTML 去除 HTML 标签得到纯文本：块级标签转空行、去标签、反转义、压缩空白。
func stripHTML(s string) string {
	s = aiBreakRe.ReplaceAllString(s, "\n\n")
	s = aiTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(aiSpaceRe.ReplaceAllString(line, " "))
	}
	s = strings.Join(lines, "\n")
	s = aiBlankRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// chatCompletionStream 调用 OpenAI 兼容 /chat/completions（stream=true），
// 逐 delta 回调 onDelta；ctx 截止时间由调用方给定，客户端断开自动取消上游请求。
func chatCompletionStream(ctx context.Context, messages []ai.ChatMessage, onDelta func(string)) error {
	cfg := global.CONF.AI
	if cfg.BaseURL == "" || cfg.APIKey == "" || cfg.Model == "" {
		return ErrAINotConfigured
	}
	payload, err := json.Marshal(map[string]any{
		"model":    cfg.Model,
		"messages": messages,
		"stream":   true,
	})
	if err != nil {
		return err
	}
	url := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := global.HttpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("LLM API %d: %s", resp.StatusCode, body)
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "data:"))
		if line == "[DONE]" {
			break
		}
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if e := json.Unmarshal([]byte(line), &chunk); e != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" && onDelta != nil {
				onDelta(choice.Delta.Content)
			}
		}
	}
	if err = scanner.Err(); err != nil {
		return err
	}
	return ctx.Err()
}

// SummarizeStream 生成单篇文章摘要，返回累积的完整 Markdown 文本。
func SummarizeStream(ctx context.Context, title, text string, onDelta func(string)) (string, error) {
	messages := []ai.ChatMessage{
		{Role: "system", Content: aiSummarizeSystemPrompt},
		{Role: "user", Content: fmt.Sprintf("文章标题：%s\n\n文章内容：\n%s", title, text)},
	}
	var full strings.Builder
	err := chatCompletionStream(ctx, messages, func(t string) {
		full.WriteString(t)
		if onDelta != nil {
			onDelta(t)
		}
	})
	return full.String(), err
}

// ChatStream 文章问答：system（含文章全文）+ 最近 aiMaxHistoryMsgs 条历史，
// 历史仅随请求传递，不持久化。
func ChatStream(ctx context.Context, title, text string, history []ai.ChatMessage, onDelta func(string)) error {
	if len(history) > aiMaxHistoryMsgs {
		history = history[len(history)-aiMaxHistoryMsgs:]
	}
	messages := make([]ai.ChatMessage, 0, len(history)+1)
	messages = append(messages, ai.ChatMessage{
		Role:    "system",
		Content: aiChatSystemPrompt + fmt.Sprintf("\n\n文章标题：%s\n\n文章内容：\n%s", title, text),
	})
	messages = append(messages, history...)
	return chatCompletionStream(ctx, messages, onDelta)
}
