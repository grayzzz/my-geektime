package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/zkep/my-geektime/internal/global"
	"github.com/zkep/my-geektime/internal/model"
	"github.com/zkep/my-geektime/internal/types/geek"
	"github.com/zkep/my-geektime/internal/types/sys_dict"
	"github.com/zkep/my-geektime/libs/zhttp"
	"go.uber.org/zap"
)

const (
	ArticlesURL                 = "https://time.geekbang.com/serv/v1/column/articles"
	ArticleInfoURL              = "https://time.geekbang.org/serv/v3/article/info"
	ProductListURL              = "https://time.geekbang.org/serv/v3/product/list"
	PvipProductListURL          = "https://time.geekbang.org/serv/v4/pvip/product_list"
	ArticleCommentURL           = "https://time.geekbang.org/serv/v4/comment/list"
	ArticleCommentDiscussionURL = "https://time.geekbang.org/serv/discussion/v1/root_list"
	SearchURL                   = "https://time.geekbang.org/serv/v3/search"
	ColumnInfoURL               = "https://time.geekbang.org/serv/v3/column/info"
	ProductInfoURL              = "https://time.geekbang.org/serv/v3/product/info"
)

func GetArticleInfo(ctx context.Context, accessToken string,
	req geek.ArticlesInfoRequest) (*geek.ArticleInfoResponse, error) {
	reqRaw, _ := json.Marshal(req)
	var resp geek.ArticleInfoResponse
	after := func(raw []byte) error {
		if err := json.Unmarshal(raw, &resp); err != nil {
			global.LOG.Error("GetArticleInfo", zap.Error(err))
			return err
		}
		if resp.Code != 0 {
			global.LOG.Warn("GetArticleInfo", zap.Any("error", resp.Error))
			return nil
		}
		resp.Raw = raw
		go func(ret geek.ArticleInfoResponse) {
			aid := fmt.Sprintf("%d", ret.Data.Info.ID)
			pid := fmt.Sprintf("%d", ret.Data.Info.Pid)
			if ret.Data.Info.Cover.Square != "" {
				ret.Data.Info.Cover.Default = ret.Data.Info.Cover.Square
			}
			info := model.Article{
				Aid:   aid,
				Pid:   pid,
				Title: ret.Data.Info.Title,
				Cover: ret.Data.Info.Cover.Default,
				Raw:   raw,
			}
			if err := global.DB.
				Model(&model.Article{}).
				Where(&model.Article{Aid: aid}).
				Assign(&info).
				FirstOrCreate(&info).Error; err != nil {
				global.LOG.Error("GetArticleInfo.AutoSync", zap.Error(err))
			}
		}(resp)
		return nil
	}
	err := Request(ctx, http.MethodPost, ArticleInfoURL, bytes.NewBuffer(reqRaw), accessToken, after)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func GetArticles(ctx context.Context, accessToken string,
	req geek.ArticlesListRequest) (*geek.ArticlesResponse, error) {
	raw, _ := json.Marshal(req)
	var resp geek.ArticlesResponse
	after := func(raw []byte) error {
		if err := json.Unmarshal(raw, &resp); err != nil {
			global.LOG.Error("GetArticles", zap.Error(err), zap.String("raw", string(raw)))
			return err
		}
		if resp.Code != 0 {
			global.LOG.Warn("GetArticles", zap.Any("error", resp.Error))
			return nil
		}
		go func() {
			for key, value := range resp.Data.List {
				itemRaw, _ := json.Marshal(value)
				info := model.ArticleSimple{
					Aid:   fmt.Sprintf("%d", value.ID),
					Pid:   req.Cid,
					Title: value.ArticleTitle,
					Cover: value.ArticleCover,
					Sort:  int32(key),
					Raw:   itemRaw,
				}
				if value.VideoCover != "" {
					info.Cover = value.VideoCover
				}
				if err := global.DB.
					Model(&model.ArticleSimple{}).
					Where(&model.ArticleSimple{Aid: info.Aid}).
					Assign(&info).
					FirstOrCreate(&info).Error; err != nil {
					global.LOG.Error("GetArticles.AutoSync", zap.Error(err))
				}
			}
		}()
		return nil
	}
	err := Request(ctx, http.MethodPost, ArticlesURL, bytes.NewBuffer(raw), accessToken, after)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func GetPvipProduct(ctx context.Context, accessToken string,
	req geek.PvipProductRequest) (*geek.ProductResponse, error) {
	raw, _ := json.Marshal(req)
	var resp geek.ProductResponse
	after := func(raw []byte) error {
		if err := json.Unmarshal(raw, &resp); err != nil {
			global.LOG.Error("GetPvipProduct", zap.Error(err))
			return err
		}
		if resp.Code != 0 {
			global.LOG.Warn("GetPvipProduct", zap.Any("error", resp.Error))
			return nil
		}
		go func() {
			for _, value := range resp.Data.Products {
				itemRaw, _ := json.Marshal(value)
				pid := fmt.Sprintf("%d", value.ID)
				info := model.Product{
					Pid:   pid,
					Title: value.Share.Title,
					Cover: value.Share.Cover,
					Raw:   itemRaw,
					// 类型/形式只认课程自身的布尔位，不用请求参数（见 sys_dict.ResolveProductType）
					OtherType: sys_dict.ResolveProductType(value.IsCore, value.IsOpencourse,
						value.IsMentor, value.IsDailylesson, value.IsQconp, value.IsColumn),
					OtherForm:  sys_dict.ResolveProductForm(value.ProductForm, value.IsVideo, value.IsAudio),
					OtherGroup: resolveOtherGroup(pid, value.Labels),
					Source:     value.Type,
					OtherTag:   req.Tag,
				}
				if err := global.DB.
					Model(&model.Product{}).
					Where(&model.Product{Pid: info.Pid}).
					Assign(&info).
					FirstOrCreate(&info).Error; err != nil {
					global.LOG.Error("GetPvipProduct.AutoSync", zap.Error(err))
				}
			}
		}()
		return nil
	}
	err := Request(ctx, http.MethodPost, PvipProductListURL, bytes.NewBuffer(raw), accessToken, after)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func GetProduct(ctx context.Context, accessToken string,
	req geek.DailyProductRequest) (*geek.DailyProductResponse, error) {
	raw, _ := json.Marshal(req)
	var resp geek.DailyProductResponse
	after := func(respRaw []byte) error {
		if err := json.Unmarshal(respRaw, &resp); err != nil {
			global.LOG.Error("GetProduct", zap.Error(err))
			return err
		}
		if resp.Code != 0 {
			global.LOG.Warn("GetProduct", zap.Any("error", resp.Error))
			return nil
		}
		go func() {
			for _, value := range resp.Data.List {
				itemRaw, _ := json.Marshal(value)
				pid := fmt.Sprintf("%d", value.ID)
				info := model.Product{
					Pid:   pid,
					Title: value.Share.Title,
					Cover: value.Share.Cover,
					Raw:   itemRaw,
					// 类型/形式只认课程自身的布尔位，不用请求参数（见 sys_dict.ResolveProductType）
					OtherType: sys_dict.ResolveProductType(value.IsCore, value.IsOpencourse,
						value.IsMentor, value.IsDailylesson, value.IsQconp, value.IsColumn),
					OtherForm:  sys_dict.ResolveProductForm(value.ProductForm, value.IsVideo, value.IsAudio),
					OtherGroup: resolveOtherGroup(pid, value.Labels),
					Source:     value.Type,
					OtherTag:   req.LabelID,
				}
				if err := global.DB.
					Model(&model.Product{}).
					Where(&model.Product{Pid: info.Pid}).
					Assign(info).
					FirstOrCreate(&info).Error; err != nil {
					global.LOG.Error("GetProduct.AutoSync", zap.Error(err))
				}
			}
		}()
		return nil
	}
	err := Request(ctx, http.MethodPost, ProductListURL, bytes.NewBuffer(raw), accessToken, after)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// resolveOtherGroup 解析课程的「真实方向」并写入 products.other_group。
//
// 方向只能从上游返回的 labels 里取（见 sys_dict.ResolveDirection）——
// 曾经这里写的是 req.Direction（请求的筛选参数），而该参数带 `json:"-"` 从不发给极客，
// 于是「按方向筛选」实际返回的是全量列表，再把整页课程统统标成同一个方向。
// 典型后果：用户没选方向（direction=0）时浏览过的课程，方向会被清成 0，
// 前端「我的课程」显示 `-`，且按任何方向都筛不出来。
//
// 另外，上游偶发不返回 labels 时保留库中已有值，避免把方向误清 0。
//
// ⚠️ 性能：labels 为空时这里会多打一条 SELECT（每门课一次，即 N+1）。
// 实测存量 763 行 products 中 labels 非空 695 行（不触发）、raw 无 labels 字段 68 行（触发），
// 一页 20 门最多几次，故不做批量预取。若将来列表页变大、或上游普遍不给 labels，
// 应改为调用方一次性 WHERE pid IN (...) 预取后传进来。
func resolveOtherGroup(pid string, labels []int) int32 {
	if len(labels) > 0 {
		return sys_dict.ResolveDirection(labels)
	}
	var exist model.Product
	if err := global.DB.
		Model(&model.Product{}).
		Select("other_group").
		Where(&model.Product{Pid: pid}).
		First(&exist).Error; err == nil {
		return exist.OtherGroup
	}
	return 0
}

// ResolveTypeForm 从存下来的上游原始 JSON 里还原「课程类型 / 课程形式」。
//
// 用途：products / tasks 里 2026-10-01 之前写入的行，other_type / other_form 取的都是
// 请求筛选参数（默认 0），需要按 raw 里自带的布尔位重算；Download 也会用它兜底。
// 解析不出时返回 (0, 0)，调用方应保留原值。
func ResolveTypeForm(raw []byte) (int32, int32) {
	if len(raw) == 0 {
		return 0, 0
	}
	var v struct {
		ProductForm   int32 `json:"product_form"`
		IsVideo       bool  `json:"is_video"`
		IsAudio       bool  `json:"is_audio"`
		IsCore        bool  `json:"is_core"`
		IsOpencourse  bool  `json:"is_opencourse"`
		IsMentor      bool  `json:"is_mentor"`
		IsDailylesson bool  `json:"is_dailylesson"`
		IsQconp       bool  `json:"is_qconp"`
		IsColumn      bool  `json:"is_column"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, 0
	}
	return sys_dict.ResolveProductType(v.IsCore, v.IsOpencourse, v.IsMentor,
			v.IsDailylesson, v.IsQconp, v.IsColumn),
		sys_dict.ResolveProductForm(v.ProductForm, v.IsVideo, v.IsAudio)
}

// GetProductInfo 按 pid 取课程详情，只读、不写库。
//
// 存在意义：`/serv/v3/column/info`（Download 里的兜底）**不返回 labels**，
// 也没有 is_mentor / is_column / is_qconp / product_form，所以靠它拿不到方向、
// 也判不出线下大会与社区课。而 `/serv/v3/product/info` 一次就能给全
// （labels + 全套 is_* 布尔位 + product_form，实测 2026-10-01 覆盖体系课/公开课/
// 线下大会/社区课，各类均有效；线下大会与社区课的 labels 本来就是空数组）。
//
// 契约：**拿不到数据一律返回非 nil error**（含上游的两种静默失败，见下）。
// 调用方应据此跳过兜底，不要用返回值里的零值去当"解析结果"。
func GetProductInfo(ctx context.Context, accessToken string, pid int64) (*geek.ProductInfoResponse, error) {
	raw, _ := json.Marshal(geek.ProductInfoRequest{ID: pid})
	var resp geek.ProductInfoResponse
	after := func(raw []byte) error {
		if err := json.Unmarshal(raw, &resp); err != nil {
			global.LOG.Error("GetProductInfo", zap.Error(err))
			return err
		}
		if resp.Code != 0 {
			global.LOG.Warn("GetProductInfo", zap.Any("error", resp.Error))
			// 必须把错误抛出去：调用方（Download）靠 err 判断有没有真的拿到数据。
			// 早先这里 return nil，Download 会当成成功，拿零值 info 去算方向/类型/形式（全是 0），
			// 兜底静默失效。业务错误（无权限 / cookie 过期 / 参数不对）重试也没用，
			// 故用 BreakRetryError 让 DoWithRetry 立刻返回，不做无谓重试。
			return zhttp.BreakRetryError(
				fmt.Errorf("GetProductInfo code=%d error=%v", resp.Code, resp.Error))
		}
		// 第二种静默失败：code=0 但 info 是空壳（id=0）。
		// 实测 pid=999999999 -> {"code":0,"data":{"info":{"id":0}}}，不报错。
		// 不识别它的话，调用方同样会拿零值去算方向/类型/形式。
		if resp.Data.Info.ID <= 0 {
			global.LOG.Warn("GetProductInfo empty info", zap.Int64("pid", pid))
			return zhttp.BreakRetryError(
				fmt.Errorf("GetProductInfo empty info, pid=%d", pid))
		}
		return nil
	}
	if err := Request(ctx, http.MethodPost, ProductInfoURL, bytes.NewBuffer(raw), accessToken, after); err != nil {
		return nil, err
	}
	return &resp, nil
}

func GetArticleComment(ctx context.Context, _, accessToken string,
	req geek.ArticleCommentListRequest) (*geek.ArticleCommentList, error) {
	raw, _ := json.Marshal(req)
	var resp geek.ArticleCommentList
	after := func(raw []byte) error {
		if err := json.Unmarshal(raw, &resp); err != nil {
			global.LOG.Error("GetArticleComment", zap.Error(err), zap.String("raw", string(raw)))
			return err
		}
		if resp.Code != 0 {
			global.LOG.Warn("GetArticleComment", zap.Any("error", resp.Error))
			return nil
		}
		go func() {
			for _, value := range resp.Data.List {
				itemRaw, _ := json.Marshal(value)
				info := model.ArticleComment{
					Aid:             req.Aid,
					Cid:             value.ID,
					DiscussionCount: value.DiscussionCount,
					LikeCount:       value.LikeCount,
					CommentCtime:    value.CommentCtime,
					Raw:             itemRaw,
				}
				if err := global.DB.
					Model(&model.ArticleComment{}).
					Where(&model.ArticleComment{Aid: info.Aid, Cid: info.Cid}).
					Assign(&info).
					FirstOrCreate(&info).Error; err != nil {
					global.LOG.Error("GetArticleComment.AutoSync", zap.Error(err))
				}
			}
		}()
		return nil
	}
	err := Request(ctx, http.MethodPost, ArticleCommentURL, bytes.NewBuffer(raw), accessToken, after)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func GetArticleCommentDiscussion(ctx context.Context, _, accessToken string,
	req geek.DiscussionListRequest) (*geek.DiscussionOriginListResponse, error) {
	raw, _ := json.Marshal(req)
	var resp geek.DiscussionOriginListResponse
	after := func(raw []byte) error {
		if err := json.Unmarshal(raw, &resp); err != nil {
			global.LOG.Error("GetArticleCommentDiscussion", zap.Error(err), zap.String("raw", string(raw)))
			return err
		}
		if resp.Code != 0 {
			global.LOG.Warn("GetArticleCommentDiscussion", zap.Any("error", resp.Error))
			return nil
		}
		go func() {
			for _, value := range resp.Data.List {
				itemRaw, _ := json.Marshal(value)
				info := model.ArticleCommentDiscussion{
					Cid:         req.TargetID,
					Did:         value.Discussion.ID,
					LikesNumber: value.Discussion.LikesNumber,
					Ctime:       value.Discussion.Ctime,
					Raw:         itemRaw,
				}
				if err := global.DB.
					Model(&model.ArticleCommentDiscussion{}).
					Where(&model.ArticleCommentDiscussion{Cid: info.Cid, Did: info.Did}).
					Assign(&info).
					FirstOrCreate(&info).Error; err != nil {
					global.LOG.Error("GetArticleCommentDiscussion.AutoSync", zap.Error(err))
				}
			}
		}()
		return nil
	}
	err := Request(ctx, http.MethodPost, ArticleCommentDiscussionURL, bytes.NewBuffer(raw), accessToken, after)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func ArticleAllComment(ctx context.Context, _, accessToken string, id int64) error {
	req := geek.ArticleCommentListRequest{Aid: id}
	hasMore := true
	after := func(raw []byte) error {
		var resp geek.ArticleCommentList
		if err := json.Unmarshal(raw, &resp); err != nil {
			global.LOG.Error("ArticleAllComment", zap.Error(err), zap.String("raw", string(raw)))
			return err
		}
		hasMore = resp.Data.Page.More
		if resp.Code != 0 {
			global.LOG.Warn("ArticleAllComment", zap.Any("error", resp.Error))
			return nil
		}
		for _, value := range resp.Data.List {
			itemRaw, _ := json.Marshal(value)
			info := model.ArticleComment{
				Aid:             req.Aid,
				Cid:             value.ID,
				DiscussionCount: value.DiscussionCount,
				LikeCount:       value.LikeCount,
				CommentCtime:    value.CommentCtime,
				Raw:             itemRaw,
			}
			if err := global.DB.
				Model(&model.ArticleComment{}).
				Where(&model.ArticleComment{Aid: info.Aid, Cid: info.Cid}).
				Assign(&info).
				FirstOrCreate(&info).Error; err != nil {
				global.LOG.Error("ArticleAllComment.AutoSync", zap.Error(err))
			}
			if info.DiscussionCount > 0 {
				discussionReq := geek.DiscussionListRequest{
					UseLikesOrder: true,
					TargetID:      info.Cid,
					TargetType:    1,
					PageType:      1,
					Size:          50,
				}
				hasNext := true
				discussionAfter := func(raw []byte) error {
					var discussionResp geek.DiscussionOriginListResponse
					if err := json.Unmarshal(raw, &discussionResp); err != nil {
						global.LOG.Error("ArticleAllComment", zap.Error(err), zap.String("raw", string(raw)))
						return err
					}
					hasNext = discussionResp.Data.Page.More
					if discussionResp.Code != 0 {
						global.LOG.Warn("ArticleAllComment", zap.Any("error", discussionResp.Error))
						return nil
					}
					for _, x := range discussionResp.Data.List {
						valueRaw, _ := json.Marshal(x)
						dinfo := model.ArticleCommentDiscussion{
							Cid:         discussionReq.TargetID,
							Did:         x.Discussion.ID,
							LikesNumber: x.Discussion.LikesNumber,
							Ctime:       x.Discussion.Ctime,
							Raw:         valueRaw,
						}
						if err := global.DB.
							Model(&model.ArticleCommentDiscussion{}).
							Where(&model.ArticleCommentDiscussion{Cid: dinfo.Cid, Did: dinfo.Did}).
							Assign(&dinfo).
							FirstOrCreate(&dinfo).Error; err != nil {
							global.LOG.Error("ArticleAllComment.AutoSync", zap.Error(err))
						}
					}
					return nil
				}
				for hasNext {
					discussionReq.Prev++
					discussionRaw, _ := json.Marshal(discussionReq)
					err := Request(ctx, http.MethodPost,
						ArticleCommentDiscussionURL, bytes.NewBuffer(discussionRaw), accessToken, discussionAfter)
					if err != nil {
						return err
					}
				}

			}
		}
		return nil
	}
	for hasMore {
		req.Prev++
		raw, _ := json.Marshal(req)
		err := Request(ctx, http.MethodPost, ArticleCommentURL, bytes.NewBuffer(raw), accessToken, after)
		if err != nil {
			return err
		}
	}
	return nil
}

func GeekTimeSearch(ctx context.Context, accessToken string, req geek.SearchRequest) (*geek.SearchResponse, error) {
	raw, _ := json.Marshal(req)
	var resp geek.SearchResponse
	after := func(raw []byte) error {
		if err := json.Unmarshal(raw, &resp); err != nil {
			global.LOG.Error("GeekTimeSearch", zap.Error(err), zap.String("raw", string(raw)))
			return err
		}
		if resp.Code != 0 {
			global.LOG.Warn("GeekTimeSearch", zap.Any("error", resp.Error))
			return nil
		}
		return nil
	}
	err := Request(ctx, http.MethodPost, SearchURL, bytes.NewBuffer(raw), accessToken, after)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func GetColumnInfo(ctx context.Context, accessToken string,
	req geek.ColumnRequest) (*geek.ColumnResponse, error) {
	reqRaw, _ := json.Marshal(req)
	var resp geek.ColumnResponse
	after := func(raw []byte) error {
		if err := json.Unmarshal(raw, &resp); err != nil {
			global.LOG.Error("GetArticleInfo", zap.Error(err))
			return err
		}
		if resp.Code != 0 {
			global.LOG.Warn("GetArticleInfo", zap.Any("error", resp.Error))
			return nil
		}
		value := resp.Data
		itemRaw, _ := json.Marshal(value)
		info := model.Product{
			Pid:    fmt.Sprintf("%d", value.ID),
			Title:  value.Share.Title,
			Cover:  value.Share.Cover,
			Raw:    itemRaw,
			Source: value.Type,
		}
		if err := global.DB.
			Model(&model.Product{}).
			Where(&model.Product{Pid: info.Pid}).
			Assign(&info).
			FirstOrCreate(&info).Error; err != nil {
			global.LOG.Error("GetColumnInfo.AutoSync", zap.Error(err))
		}
		return nil
	}
	err := Request(ctx, http.MethodPost, ColumnInfoURL, bytes.NewBuffer(reqRaw), accessToken, after)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}
