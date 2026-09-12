package service

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/zkep/my-geektime/internal/global"
	"github.com/zkep/my-geektime/internal/model"
	"github.com/zkep/my-geektime/internal/types/geek"
	"github.com/zkep/my-geektime/libs/utils"
	"go.uber.org/zap"
)

// EPUB 评论与讨论渲染。
//
// 说明：评论是从极客时间抓取并缓存到本地的平台留言（下载课程时随 ArticleAllComment 一起抓取），
// 现有「在线文档站」导出已经带评论（docsite.go 的 getCommentsHTML），epub 对齐这一行为。
//
// 隔离要求：本文件不得改动 docsite.go 的 getCommentsHTML（在线/本地文档站共用），
// 宁可重复少量渲染逻辑，也不制造回归风险。
//
// 表关联关系（易错点）：
//
//	article_comments.aid                 → 文章 id（task.other_id）
//	article_comments.cid                 → 极客评论 id（不是主键 id）
//	article_comment_discussions.cid      → article_comments.cid
//
// 见 internal/service/product.go: 写入讨论时用的是 discussionReq.TargetID = info.Cid。

const (
	// 评论模式
	epubCommentsAll = "all"
	epubCommentsHot = "hot"
	epubCommentsOff = "0"

	// hot 模式下每篇最多保留的评论条数（按点赞倒序）
	epubHotCommentLimit = 20
	// hot 模式下每条评论最多保留的讨论条数
	epubHotReplyLimit = 3

	// IN 查询分批大小，避免语句过长
	epubCommentBatchSize = 500
)

// epubComment 一条评论及其讨论。
type epubComment struct {
	User string
	// Content 是反转义后的 HTML 片段，最终统一走 epubConverter 清洗
	Content   string
	LikeCount int64
	Ctime     int64
	Replies   []epubReply
}

// epubReply 评论下的一条讨论。
type epubReply struct {
	User      string
	Content   string
	LikeCount int64
	Ctime     int64
}

// epubCommentEntry 加载过程中的中间结构：cid 用于关联讨论。
type epubCommentEntry struct {
	cid     int64
	comment epubComment
}

// loadEpubComments 按文章 id 批量加载评论与讨论。
//
// 全库 3600+ 篇文章，若逐篇查询会产生 7000+ 次查询（N+1），
// 所以这里一律「按 aid 批量查 + 内存分组」。
func loadEpubComments(ctx context.Context, aids []int64, opt EpubOptions) (map[int64][]epubComment, error) {
	result := make(map[int64][]epubComment)
	if opt.Comments == epubCommentsOff || len(aids) == 0 {
		return result, nil
	}
	byAid, cids, err := loadEpubCommentEntries(ctx, aids, opt)
	if err != nil {
		return nil, err
	}
	if len(cids) > 0 {
		replies, err := loadEpubReplies(ctx, cids, opt)
		if err != nil {
			return nil, err
		}
		for _, entries := range byAid {
			for i := range entries {
				entries[i].comment.Replies = replies[entries[i].cid]
			}
		}
	}
	for aid, entries := range byAid {
		list := make([]epubComment, 0, len(entries))
		for i := range entries {
			list = append(list, entries[i].comment)
		}
		result[aid] = list
	}
	return result, nil
}

// loadEpubCommentEntries 分批查询评论，返回按 aid 分组的结果以及需要查询讨论的 cid 列表。
func loadEpubCommentEntries(ctx context.Context, aids []int64, opt EpubOptions) (
	map[int64][]*epubCommentEntry, []int64, error) {
	byAid := make(map[int64][]*epubCommentEntry)
	cids := make([]int64, 0, len(aids)*4)
	order := "aid asc, id asc"
	if opt.Comments == epubCommentsHot {
		order = "aid asc, like_count desc, id asc"
	}
	for start := 0; start < len(aids); start += epubCommentBatchSize {
		end := start + epubCommentBatchSize
		if end > len(aids) {
			end = len(aids)
		}
		var rows []*model.ArticleComment
		if err := global.DB.WithContext(ctx).Model(&model.ArticleComment{}).
			Where("aid in ?", aids[start:end]).
			Order(order).
			Find(&rows).Error; err != nil {
			return nil, nil, fmt.Errorf("query article comments failed: %w", err)
		}
		for _, row := range rows {
			// hot 模式下结果已按点赞倒序，逐 aid 截断即可
			if opt.Comments == epubCommentsHot && len(byAid[row.Aid]) >= epubHotCommentLimit {
				continue
			}
			var raw geek.ArticleComment
			if err := json.Unmarshal(row.Raw, &raw); err != nil {
				global.LOG.Warn("epub comment unmarshal failed",
					zap.Int64("aid", row.Aid), zap.Error(err))
				continue
			}
			content := strings.TrimSpace(utils.UnescapeComment(raw.CommentContent))
			user := strings.TrimSpace(raw.UserName)
			if content == "" && user == "" {
				continue
			}
			byAid[row.Aid] = append(byAid[row.Aid], &epubCommentEntry{
				cid: row.Cid,
				comment: epubComment{
					User:      user,
					Content:   content,
					LikeCount: row.LikeCount,
					Ctime:     row.CommentCtime,
				},
			})
			if row.Cid > 0 {
				cids = append(cids, row.Cid)
			}
		}
	}
	return byAid, cids, nil
}

// loadEpubReplies 按 cid 批量加载讨论，返回 cid → 讨论列表。
func loadEpubReplies(ctx context.Context, cids []int64, opt EpubOptions) (map[int64][]epubReply, error) {
	result := make(map[int64][]epubReply, len(cids))
	order := "cid asc, likes_number desc, id asc"
	for start := 0; start < len(cids); start += epubCommentBatchSize {
		end := start + epubCommentBatchSize
		if end > len(cids) {
			end = len(cids)
		}
		var rows []*model.ArticleCommentDiscussion
		if err := global.DB.WithContext(ctx).Model(&model.ArticleCommentDiscussion{}).
			Where("cid in ?", cids[start:end]).
			Order(order).
			Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("query comment discussions failed: %w", err)
		}
		for _, row := range rows {
			if opt.Comments == epubCommentsHot && len(result[row.Cid]) >= epubHotReplyLimit {
				continue
			}
			var raw geek.DiscussionData
			if err := json.Unmarshal(row.Raw, &raw); err != nil {
				global.LOG.Warn("epub discussion unmarshal failed",
					zap.Int64("cid", row.Cid), zap.Error(err))
				continue
			}
			// 讨论正文的字段名是 discussion.discussion_content，不是 discussion.content
			content := strings.TrimSpace(utils.UnescapeComment(raw.Discussion.DiscussionContent))
			user := strings.TrimSpace(raw.Author.Nickname)
			if content == "" && user == "" {
				continue
			}
			result[row.Cid] = append(result[row.Cid], epubReply{
				User:      user,
				Content:   content,
				LikeCount: raw.Discussion.LikesNumber,
				Ctime:     raw.Discussion.Ctime,
			})
		}
	}
	return result, nil
}

// buildCommentsHTML 把评论渲染成 HTML 片段。
//
// 注意：用户名的文本转义在这里完成；评论正文本身是 HTML（需保留排版），
// 统一交由 epubConverter 清洗，因此这里不做转义。
func buildCommentsHTML(list []epubComment, mode string) string {
	if len(list) == 0 {
		return ""
	}
	label := "全部留言"
	if mode == epubCommentsHot {
		label = "精选留言"
	}
	var b strings.Builder
	b.Grow(len(list) * 400)
	b.WriteString(`<section class="comments">`)
	fmt.Fprintf(&b, `<h3 class="comments-title">%s（%d）</h3>`, label, len(list))
	for i := range list {
		item := &list[i]
		b.WriteString(`<div class="comment">`)
		b.WriteString(`<p class="comment-meta">` + epubCommentMeta(item.User, item.LikeCount, item.Ctime) + `</p>`)
		if item.Content != "" {
			b.WriteString(`<div class="comment-body">` + item.Content + `</div>`)
		}
		if len(item.Replies) > 0 {
			b.WriteString(`<div class="comment-replies">`)
			for j := range item.Replies {
				reply := &item.Replies[j]
				b.WriteString(`<div class="reply">`)
				b.WriteString(`<p class="comment-meta">` + epubCommentMeta(reply.User, reply.LikeCount, reply.Ctime) + `</p>`)
				if reply.Content != "" {
					b.WriteString(`<div class="comment-body">` + reply.Content + `</div>`)
				}
				b.WriteString(`</div>`)
			}
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</section>`)
	return b.String()
}

// epubCommentMeta 渲染「用户名 · 赞 N · 日期」。
// 不使用 emoji：电纸书与部分阅读器缺字形会显示成方框。
func epubCommentMeta(user string, likes, ctime int64) string {
	user = strings.TrimSpace(user)
	if user == "" {
		user = "匿名用户"
	}
	parts := []string{html.EscapeString(user)}
	if likes > 0 {
		parts = append(parts, fmt.Sprintf("赞 %d", likes))
	}
	if ctime > 0 {
		parts = append(parts, time.Unix(ctime, 0).Format(time.DateOnly))
	}
	return strings.Join(parts, " · ")
}
