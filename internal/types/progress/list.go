package progress

// CourseRequest 课程进度汇总
type CourseRequest struct {
	TaskPid string `json:"task_pid" form:"task_pid"`
}

// CourseProgressResponse 课程进度汇总响应
type CourseProgressResponse struct {
	FinishedCount int64   `json:"finished_count"`
	TotalCount    int64   `json:"total_count"`
	Percent       float64 `json:"percent"`
	LastTaskId    string  `json:"last_task_id"`
	LastTaskName  string  `json:"last_task_name"`
	UpdatedAt     int64   `json:"updated_at"`
}

// MyRequest 当前用户全部进度
type MyRequest struct {
	Page    int `json:"page" form:"page"`
	PerPage int `json:"perPage" form:"perPage"`
}

// MyProgressResponse 用户全部进度响应
type MyProgressResponse struct {
	Count int64              `json:"count"`
	Rows  []ProgressResponse `json:"rows"`
}