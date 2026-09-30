package sys_dict

import "fmt"

type Tag struct {
	Option
	Options []Option `json:"options"`
}

type Option struct {
	Label string `json:"label"`
	Value int32  `json:"value"`
}

type TagData struct {
	Status int    `json:"status"`
	Msg    string `json:"msg"`
	Data   []Tag  `json:"data,omitempty"`
}

const (
	IsColumnCore  = 1
	IsOpencourse  = 4
	IsColumn      = 5
	IsMentor      = 6
	IsDailylesson = 19
	IsQconp       = 20
)

var (
	ProductTypes = map[string]Option{
		"1": {Label: "体系课", Value: IsColumnCore},
		"4": {Label: "公开课", Value: IsOpencourse},
		"5": {Label: "线下大会", Value: IsColumn},
		"6": {Label: "社区课", Value: IsMentor},
		"d": {Label: "每日一课", Value: IsDailylesson},
		"q": {Label: "大厂案例", Value: IsQconp},
	}

	OriginTypes = map[int32]string{
		IsColumnCore:  "1",
		IsOpencourse:  "4",
		IsColumn:      "5",
		IsMentor:      "6",
		IsDailylesson: "d",
		IsQconp:       "q",
	}

	ProductForms = []Option{
		{Label: "图文+音频", Value: 1},
		{Label: "视频", Value: 2},
	}

	// DirectionValues 是「课程方向」的一级分类值，取自 sys_dicts 中
	// rkey='geektimeCategory' 且 pkey='geektimeCategory' 的条目（见 service.GeektimeCategory）。
	// 极客接口返回的 labels 里混着一级方向和二级标签（如 10185），
	// 用本集合求交集即可得到课程的真实方向。
	DirectionValues = []int32{
		3,   // 后端-架构
		5,   // 前端-移动
		6,   // 运维-测试
		9,   // 计算机基础
		101, // 人工智能
		123, // 大数据
		131, // 安全
		146, // 技术领导力
		155, // 产品-增长
		163, // 多元成长
	}
)

// ResolveDirection 从极客课程 labels 中解析出「课程方向」。
//
// ⚠️ 必须用它来填 other_group，不要用请求里的筛选参数（direction）：
// direction 只是「用户当前筛选条件」，且该参数带 `json:"-"` 从不发给极客，
// 拿它当课程属性会把整页课程错标成同一个方向（详见 product.go 的 GetPvipProduct）。
//
// 语义约定：一门课挂多个一级方向时，取 **labels 中最早出现的那一个**
// （上游同一 pid 的 labels 顺序稳定，故结果可复现）。
// 举例 labels=[101,10,3,...] 取 101；labels=[3,101,...] 取 3。
// 上游没有「主方向」字段，所以这不是 bug，只是需要明确的取值规则：
// 若改成按 DirectionValues 顺序取，会得到不同的值，且需同步重跑全量回填。
func ResolveDirection(labels []int) int32 {
	for _, label := range labels {
		for _, direction := range DirectionValues {
			if int32(label) == direction {
				return direction
			}
		}
	}
	return 0
}

// ResolveProductType 从极客课程对象自带的布尔位解析「课程类型」。
//
// ⚠️ 不要用请求里的筛选参数（product_type）当课程属性 —— 它只是「用户当前筛选条件」，
// 默认 0 会把整批课的类型抹成 0（2026-10-01 修复前实测：tasks 顶层 96 门课全是 0）。
//
// 判定优先级不可调换：体系课与社区课都同时带 is_column，只有「线下大会」才是仅有
// is_column 的那一类。抽样 368 门实测的组合分布：
//
//	is_core                189 门 -> 1 体系课
//	is_opencourse           82 门 -> 4 公开课
//	is_mentor(+is_column)   55 门 -> 6 社区课
//	仅 is_column            42 门 -> 5 线下大会
func ResolveProductType(core, opencourse, mentor, dailylesson, qconp, isColumn bool) int32 {
	switch {
	case core:
		return IsColumnCore
	case opencourse:
		return IsOpencourse
	case mentor:
		return IsMentor
	case dailylesson:
		return IsDailylesson
	case qconp:
		return IsQconp
	case isColumn:
		return IsColumn
	}
	return 0
}

// ResolveProductForm 解析「课程形式」。极客直接给出 product_form（1 图文+音频 / 2 视频），
// 少数社区课给 0，此时用 is_video / is_audio 兜底。
func ResolveProductForm(form int32, isVideo, isAudio bool) int32 {
	if form == 1 || form == 2 {
		return form
	}
	if isVideo {
		return 2
	}
	if isAudio {
		return 1
	}
	return 0
}

func ProductURLWithType(productType string, productID int) string {
	redirect := ""
	switch productType {
	case "c1", "x49", "x50":
		redirect = fmt.Sprintf("https://time.geekbang.org/column/intro/%d", productID)
	case "c3", "c6":
		redirect = fmt.Sprintf("https://time.geekbang.org/course/intro/%d", productID)
	case "p29":
		redirect = fmt.Sprintf("https://time.geekbang.org/opencourse/intro/%d", productID)
	case "p30", "p35":
		redirect = fmt.Sprintf("https://time.geekbang.org/opencourse/videointro/%d", productID)
	case "d":
		redirect = fmt.Sprintf("https://time.geekbang.org/dailylesson/detail/%d", productID)
	case "q":
		redirect = fmt.Sprintf("https://time.geekbang.org/qconplus/detail/%d", productID)
	default:
	}
	return redirect
}

func ProductDetailURLWithType(productType string, productID, articleID int) string {
	redirect := fmt.Sprintf("https://time.geekbang.org/course/detail/%d-%d", productID, articleID)
	switch productType {
	case "d":
		redirect = fmt.Sprintf("https://time.geekbang.org/dailylesson/detail/%d", productID)
	case "q":
		redirect = fmt.Sprintf("https://time.geekbang.org/qconplus/detail/%d", productID)
	default:
	}
	return redirect
}
