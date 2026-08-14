package progress

// SaveRequest 保存单章进度
type SaveRequest struct {
	TaskId         string `json:"task_id" form:"task_id"`
	TaskPid        string `json:"task_pid" form:"task_pid"`
	TaskType       string `json:"task_type" form:"task_type"`
	Position       int64  `json:"position" form:"position"`
	ScrollPosition int64  `json:"scroll_position" form:"scroll_position"`
	Duration       int64  `json:"duration" form:"duration"`
	IsFinished     bool   `json:"is_finished" form:"is_finished"`
	// ResetProgress 显式请求重置完成标记（仅管理端/修复场景使用）；
	// 为 true 时允许把 is_finished 从 true 回退为 false。
	ResetProgress bool `json:"reset_progress" form:"reset_progress"`
}

// GetRequest 获取单章进度
type GetRequest struct {
	TaskId string `json:"task_id" form:"task_id"`
}

// ProgressResponse 单章进度响应（含模型字段）
type ProgressResponse struct {
	Id             int64  `json:"id"`
	Uid            string `json:"uid"`
	TaskId         string `json:"task_id"`
	TaskPid        string `json:"task_pid"`
	TaskType       string `json:"task_type"`
	Position       int64  `json:"position"`
	ScrollPosition int64  `json:"scroll_position"`
	Duration       int64  `json:"duration"`
	IsFinished     bool   `json:"is_finished"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}
