package model

// Progress 记录用户对某章节/文章的学习进度
//
// position 语义：恒为视频播放秒数（无视频为 0）
// scroll_position 语义：恒为文章滚动高度 px（无图文为 0）
// duration 语义：视频总时长秒数（无视频为 0）
// 字段语义由前端按章节实际形态填充，后端只存不解释。
type Progress struct {
	Id            int64  `gorm:"primaryKey;autoIncrement;comment:id"`
	Uid           string `gorm:"index:,unique,composite:user_progress;size:255;comment:uid"`
	TaskId        string `gorm:"index:,unique,composite:user_progress;size:128;comment:task id"`
	TaskPid       string `gorm:"index;size:128;comment:task pid"`
	TaskType      string `gorm:"size:50;comment:task type"`
	Position      int64  `gorm:"comment:position"`
	Duration      int64  `gorm:"comment:duration"`
	ScrollPosition int64 `gorm:"comment:scroll position"`
	IsFinished    bool   `gorm:"comment:is finished"`
	CreatedAt     int64  `gorm:"index;comment:created at"`
	UpdatedAt     int64  `gorm:"index;comment:updated at"`
	DeletedAt     int64  `gorm:"index;comment:deleted at"`
}