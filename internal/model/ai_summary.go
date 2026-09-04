package model

// AISummary 缓存单篇文章的 AI 摘要（Markdown）
type AISummary struct {
	Id        int64  `gorm:"primaryKey;autoIncrement;comment:id"`
	Aid       string `gorm:"uniqueIndex;size:128;comment:article id"`
	Content   string `gorm:"type:text;comment:summary markdown"`
	AIModel   string `gorm:"column:ai_model;size:128;comment:model used"`
	CreatedAt int64  `gorm:"index;comment:created at"`
	UpdatedAt int64  `gorm:"comment:updated at"`
}
