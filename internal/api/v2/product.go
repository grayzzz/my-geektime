package v2

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/zkep/my-geektime/internal/global"
	"github.com/zkep/my-geektime/internal/model"
	"github.com/zkep/my-geektime/internal/service"
	"github.com/zkep/my-geektime/internal/types/geek"
	"github.com/zkep/my-geektime/internal/types/sys_dict"
	"github.com/zkep/my-geektime/internal/types/task"
	"github.com/zkep/my-geektime/libs/utils"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Product struct{}

func NewProduct() *Product {
	return &Product{}
}

func (p *Product) Download(c *gin.Context) {
	var req geek.DowloadRequest
	if err := c.BindJSON(&req); err != nil {
		global.FAIL(c, "fail.msg", err)
		return
	}
	accessToken := c.GetString(global.AccessToken)
	if accessToken == "" {
		global.FAIL(c, "product.no_cookie")
		return
	}
	articlesMap := make(map[int64]*model.Article, 10)
	ids := make([]int64, 0, 1)
	if req.Pid <= 0 {
		global.FAIL(c, "product.no_exists_pid")
		return
	}
	resp, err := service.GetArticles(c, accessToken,
		geek.ArticlesListRequest{
			Cid:   fmt.Sprintf("%d", req.Pid),
			Order: "earliest",
			Prev:  1,
			Size:  500,
		})
	if err != nil {
		global.FailWithError(c, err)
		return
	}
	if len(resp.Data.List) == 0 {
		global.FAIL(c, "product.api_busy")
		return
	}
	for _, v := range resp.Data.List {
		if v.ID <= 0 || v.ArticleTitle == "" {
			continue
		}
		ids = append(ids, v.ID)
		itemRaw, _ := json.Marshal(v)
		info := &model.Article{
			Aid:   fmt.Sprintf("%d", v.ID),
			Pid:   fmt.Sprintf("%d", req.Pid),
			Title: v.ArticleTitle,
			Cover: v.ArticleCover,
			Raw:   itemRaw,
		}
		if v.VideoCover != "" && info.Cover == "" {
			info.Cover = v.VideoCover
		}
		articlesMap[v.ID] = info
	}
	var product model.Product
	if err := global.DB.Model(&model.Product{}).
		Where(&model.Product{Pid: fmt.Sprintf("%d", req.Pid)}).Find(&product).Error; err != nil {
		global.FAIL(c, "fail.msg", err.Error())
		return
	}
	if product.Pid == "" {
		ret, err := service.GetColumnInfo(c, accessToken,
			geek.ColumnRequest{ProductID: req.Pid, WithRecommendArticle: true})
		if err != nil {
			global.FAIL(c, "fail.msg", err.Error())
			return
		}
		product.Title = ret.Data.Title
		product.Cover = ret.Data.Cover.Square
		product.Raw, _ = json.Marshal(ret.Data)
		// Pid 必须补上：GetColumnInfo 内部（service/product.go 的 AutoSync）其实已按 pid
		// 建好了 products 行，而下面回写的守卫是 `if product.Pid != ""`。早先只补
		// Title/Cover/Raw，会让回写在这条「products 表里本来没有这门课」——也正是最需要
		// 它的路径上永远不执行（注释里承诺的「免得每次缓存都多打一次上游」根本没兑现）。
		product.Pid = fmt.Sprintf("%d", req.Pid)
	}
	// productWriteback 登记需要回写的 products 行，真正的落库放到下面的同一个事务里。
	var productWriteback *model.Product
	// 类型/形式兜底：products.other_type / other_form 为 0 时（历史脏数据，写库时取的是
	// 请求筛选参数而非课程属性），从存下来的上游原始数据里重算。
	//
	// ⚠️ 覆盖边界：三层兜底一律「只在值为 0 时触发」，**非 0 的历史脏值不会被逐步修正**。
	// 存量已由 2026-10-01 的一次性回填脚本重算过（.workbuddy/backup/fix_type_form.py，
	// 其口径是「raw 能解析出非 0 值就覆盖」，不是只补 0），实测 products 763 行里
	// other_group / other_type 与 raw 不一致均为 0 行。这里只负责给增量数据兜底。
	if (product.OtherType == 0 || product.OtherForm == 0) && len(product.Raw) > 0 {
		ot, of := service.ResolveTypeForm(product.Raw)
		if product.OtherType == 0 {
			product.OtherType = ot
		}
		if product.OtherForm == 0 {
			product.OtherForm = of
		}
	}
	// 方向兜底：products.other_group 为 0 时，尝试从存下来的上游原始数据里解 labels 还原。
	// products 表若已按 labels 同步过则此处不会生效；仅在历史脏数据（未同步过方向）时补一刀。
	if product.OtherGroup == 0 && len(product.Raw) > 0 {
		var labels struct {
			Labels []int `json:"labels"`
		}
		if err := json.Unmarshal(product.Raw, &labels); err == nil {
			product.OtherGroup = sys_dict.ResolveDirection(labels.Labels)
		}
	}
	// 终极兜底：仍缺方向/类型/形式时，按 pid 调一次 /serv/v3/product/info 补全。
	//
	// 关键场景：课程是从**关键字搜索**里点进来的。那条分支走 /serv/v3/search，
	// 返回的 product 对象只有 12 个字段，既没有 labels 也没有 is_* 布尔位，
	// 而且不写 products 表；于是上面两层兜底都拿不到方向，缓存进「我的课程」后
	// 方向会显示 `-`（类型 / 形式同理）。
	if product.OtherGroup == 0 || product.OtherType == 0 || product.OtherForm == 0 {
		if detail, err := service.GetProductInfo(c, accessToken, req.Pid); err != nil {
			global.LOG.Warn("Download.GetProductInfo", zap.Error(err))
		} else {
			info := detail.Data.Info
			if product.OtherGroup == 0 {
				product.OtherGroup = sys_dict.ResolveDirection(info.Labels)
			}
			if product.OtherType == 0 {
				product.OtherType = sys_dict.ResolveProductType(info.IsCore, info.IsOpencourse,
					info.IsMentor, info.IsDailylesson, info.IsQconp, info.IsColumn)
			}
			if product.OtherForm == 0 {
				product.OtherForm = sys_dict.ResolveProductForm(info.ProductForm, info.IsVideo, info.IsAudio)
			}
			// 顺手把补好的值写回 products 行，免得同一门课每次缓存都要多打一次上游。
			// 这里只登记；落库放到下面同一个事务里，避免「products 已改、任务没建」的半截状态。
			if product.Pid != "" {
				productWriteback = &model.Product{
					Pid:        product.Pid,
					Title:      product.Title,
					Cover:      product.Cover,
					Raw:        product.Raw,
					Source:     product.Source,
					OtherGroup: product.OtherGroup,
					OtherType:  product.OtherType,
					OtherForm:  product.OtherForm,
					OtherTag:   product.OtherTag,
				}
			}
		}
	}
	if len(articlesMap) == 0 {
		var articles []*model.Article
		if err := global.DB.Model(&model.Article{}).
			Where("aid IN ?", ids).Find(&articles).Error; err != nil {
			global.FAIL(c, "fail.msg", err.Error())
			return
		}
		for _, v := range articles {
			articlesMap[v.Id] = v
		}
	}
	jobId := utils.HalfUUID()
	job := &model.Task{
		TaskId:     jobId,
		TaskName:   product.Title,
		TaskType:   service.TASK_TYPE_PRODUCT,
		OtherId:    fmt.Sprintf("%d", req.Pid),
		Cover:      product.Cover,
		Raw:        product.Raw,
		OtherType:  product.OtherType,
		OtherForm:  product.OtherForm,
		OtherGroup: product.OtherGroup,
		OtherTag:   product.OtherTag,
	}
	tasks := make([]*model.Task, 0, len(ids))
	for _, id := range ids {
		var (
			raw      []byte
			otherId  string
			taskName string
			cover    string
		)
		if article, ok := articlesMap[id]; !ok {
			info, err := service.GetArticleInfo(c, accessToken, geek.ArticlesInfoRequest{Id: id})
			if err != nil {
				global.FAIL(c, "fail.msg", err.Error())
				return
			}
			var m geek.ArticleInfoRaw
			if err = json.Unmarshal(info.Raw, &m); err != nil {
				global.FAIL(c, "fail.msg", err.Error())
				return
			}
			raw = m.Data
			otherId = fmt.Sprintf("%d", info.Data.Info.ID)
			taskName = info.Data.Info.Title
			cover = info.Data.Info.Cover.Default
		} else {
			raw = article.Raw
			otherId = article.Aid
			taskName = article.Title
			cover = article.Cover
		}
		item := model.Task{
			TaskPid:    jobId,
			TaskId:     utils.HalfUUID(),
			OtherId:    otherId,
			TaskName:   taskName,
			TaskType:   service.TASK_TYPE_ARTICLE,
			Cover:      cover,
			Raw:        raw,
			OtherType:  product.OtherType,
			OtherForm:  product.OtherForm,
			OtherGroup: product.OtherGroup,
			OtherTag:   product.OtherTag,
		}
		if global.CONF.Site.Download {
			item.Bstatus = service.TASK_STATUS_PENDING
		}
		tasks = append(tasks, &item)
	}
	count := len(tasks)
	statistics := task.TaskStatistics{
		Count: count,
		Items: map[int]int{
			service.TASK_STATUS_PENDING:  count,
			service.TASK_STATUS_RUNNING:  0,
			service.TASK_STATUS_FINISHED: 0,
			service.TASK_STATUS_ERROR:    0,
		},
	}
	job.Statistics, _ = json.Marshal(statistics)
	if global.CONF.Site.Download {
		job.Bstatus = service.TASK_STATUS_PENDING
	}
	err = global.DB.Transaction(func(tx *gorm.DB) error {
		// products 回写与 job/tasks 同一事务：任一步失败整体回滚，不会留下「元数据已改、任务没建」。
		// 回写自身失败只记日志、不让请求失败 —— 它只是「省一次上游请求」的优化，
		// 不该因此让用户的缓存动作报错。
		if productWriteback != nil {
			if err := tx.Model(&model.Product{}).
				Where(&model.Product{Pid: productWriteback.Pid}).
				Assign(*productWriteback).
				FirstOrCreate(productWriteback).Error; err != nil {
				global.LOG.Error("Download.UpdateProduct", zap.Error(err))
			}
		}
		if err := tx.Create(job).Error; err != nil {
			return err
		}
		for _, x := range tasks {
			if err := tx.Create(x).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		global.FAIL(c, "fail.msg", err.Error())
		return
	}
	result := geek.DowloadResponse{JobID: jobId}
	global.OK(c, result)
}

func (p *Product) ProductList(c *gin.Context) {
	var req geek.DailyProductRequest
	if err := c.ShouldBind(&req); err != nil {
		global.FAIL(c, "fail.msg", err.Error())
		return
	}
	if req.Size <= 0 {
		req.Size = 20
	}
	if req.Prev <= 0 {
		req.Prev = 1
	}
	req.Prev = req.Prev - 1
	accessToken := global.CONF.Site.Cookie.Geektime
	resp, err := service.GetProduct(c, accessToken, req)
	if err != nil {
		global.FAIL(c, "fail.msg", err.Error())
		return
	}
	ret := geek.ProductListResponse{Rows: make([]geek.ProductListRow, 0)}
	ret.Score = resp.Data.Page.Score
	ret.Count = resp.Data.Page.Count
	if resp.Data.Page.Count == 0 {
		ret.HasNext = resp.Data.Page.More
	}
	for _, v := range resp.Data.List {
		row := geek.ProductListRow{
			ID:            v.ID,
			Title:         v.Title,
			Subtitle:      v.Subtitle,
			Intro:         v.Intro,
			IntroHTML:     v.IntroHTML,
			Ucode:         v.Ucode,
			IsFinish:      v.IsFinish,
			IsVideo:       v.IsVideo,
			IsAudio:       v.IsAudio,
			IsColumn:      v.IsColumn,
			IsCore:        v.IsCore,
			IsDailylesson: v.IsDailylesson,
			IsUniversity:  v.IsUniversity,
			IsOpencourse:  v.IsOpencourse,
			IsQconp:       v.IsQconp,
			IsMentor:      v.IsMentor,
			IsSale:        v.IsSale,
			Sale:          v.Price.Sale,
			SaleType:      v.Price.SaleType,
			Share:         v.Share,
			Author:        v.Author,
			Cover:         v.Cover,
			Article:       v.Article,
		}
		row.Cover.Square = service.URLProxyReplace(row.Cover.Square)
		row.Author.Avatar = service.URLProxyReplace(row.Author.Avatar)
		if len(row.IntroHTML) > 0 {
			if introHTML, err1 := service.HtmlURLProxyReplace(row.IntroHTML); err1 == nil {
				row.IntroHTML = introHTML
			}
		}
		row.Redirect = sys_dict.ProductURLWithType(v.Type, v.ID)
		ret.Rows = append(ret.Rows, row)
	}
	global.OK(c, ret)
}

func (p *Product) PvipProductList(c *gin.Context) {
	var req geek.PvipProductRequest
	if err := c.ShouldBind(&req); err != nil {
		global.FAIL(c, "fail.msg", err.Error())
		return
	}
	if req.Tag > 0 {
		req.TagIds = []int32{req.Tag}
	} else if req.Direction > 0 {
		// 「课程方向」必须借 tag_ids 传给上游：实测（2026-10-01）
		// POST https://time.geekbang.org/serv/v4/pvip/product_list 对 direction /
		// category_id / label_ids 等参数一律忽略（返回结果与不带参数完全相同），
		// 而方向值本身就是标签体系里的一级标签 id，用 tag_ids 传即生效。
		// 曾用写法是把 direction 塞进 other_group 写库，既筛不出课又污染了数据。
		req.TagIds = []int32{req.Direction}
	}
	req.Size = req.PerPage
	req.Prev = req.Page
	accessToken := global.CONF.Site.Cookie.Geektime
	ret := geek.ProductListResponse{Rows: make([]geek.ProductListRow, 0)}
	if len(req.Keyword) > 0 {
		searchReq := geek.SearchRequest{
			Keyword:  req.Keyword,
			Category: "product",
			Platform: "pc",
			Prev:     req.Prev,
			Size:     req.Size + 1,
		}
		searchRet, err := service.GeekTimeSearch(c, accessToken, searchReq)
		if err != nil {
			global.FAIL(c, "fail.msg", err.Error())
			return
		}
		ret.Count = searchRet.Data.Page.Count
		if len(searchRet.Data.List) > req.Size {
			ret.HasNext = true
			searchRet.Data.List = searchRet.Data.List[:req.Size]
		}
		for _, v := range searchRet.Data.List {
			if v.ItemType != "product" {
				continue
			}
			item := v.Product
			row := geek.ProductListRow{
				ID:       item.ID,
				Title:    item.Title,
				Subtitle: item.Subtitle,
				IsVideo:  item.Type == "c3",
				IsAudio:  item.Type == "c1",
			}
			row.Article.Count = item.TotalLesson
			row.Author.Name = item.AuthorName
			row.Author.Info = item.AuthorIntro
			row.Cover.Square = item.Cover
			row.Cover.Square = service.URLProxyReplace(row.Cover.Square)
			if len(row.IntroHTML) > 0 {
				if introHTML, err1 := service.HtmlURLProxyReplace(row.IntroHTML); err1 == nil {
					row.IntroHTML = introHTML
				}
			}
			ret.Rows = append(ret.Rows, row)
		}

		global.OK(c, ret)
		return
	}
	resp, err := service.GetPvipProduct(c, accessToken, req)
	if err != nil {
		global.FAIL(c, "fail.msg", err.Error())
		return
	}
	ret.Count = resp.Data.Page.Total
	if resp.Data.Page.Total == 0 {
		ret.HasNext = resp.Data.Page.More
	}
	for _, v := range resp.Data.Products {
		row := geek.ProductListRow{
			ID:            v.ID,
			Title:         v.Title,
			Subtitle:      v.Subtitle,
			Intro:         v.Intro,
			IntroHTML:     v.IntroHTML,
			Ucode:         v.Ucode,
			IsFinish:      v.IsFinish,
			IsVideo:       v.IsVideo,
			IsAudio:       v.IsAudio,
			IsColumn:      v.IsColumn,
			IsCore:        v.IsCore,
			IsDailylesson: v.IsDailylesson,
			IsUniversity:  v.IsUniversity,
			IsOpencourse:  v.IsOpencourse,
			IsQconp:       v.IsQconp,
			IsMentor:      v.IsMentor,
			IsSale:        v.IsSale,
			Sale:          v.Price.Sale,
			SaleType:      v.Price.SaleType,
			Share:         v.Share,
			Author:        v.Author,
			Cover:         v.Cover,
			Article:       v.Article,
		}
		row.Cover.Square = service.URLProxyReplace(row.Cover.Square)
		row.Author.Avatar = service.URLProxyReplace(row.Author.Avatar)
		if len(row.IntroHTML) > 0 {
			if introHTML, err1 := service.HtmlURLProxyReplace(row.IntroHTML); err1 == nil {
				row.IntroHTML = introHTML
			}
		}
		row.Redirect = sys_dict.ProductURLWithType(v.Type, v.ID)
		ret.Rows = append(ret.Rows, row)
	}
	global.OK(c, ret)
}
