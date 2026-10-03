package initialize

import (
	"context"

	"github.com/zkep/my-geektime/internal/global"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger 初始化全局日志。
//
// ⚠️ 不要用 zap.NewExample()：它**不输出时间戳**，导致排查耗时问题时
// 无法把日志行和请求时间对上（2026-10-03 排查「课程全缓存命中仍要 4~5 分钟」
// 时就卡在这里，只能靠 [GIN] 行反推阶段耗时）。这里显式配置 ISO8601 时间。
func Logger(_ context.Context) error {
	cfg := zap.NewProductionConfig()
	cfg.Encoding = "json"
	cfg.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	cfg.Sampling = nil // 关掉采样：排查问题时不能丢日志
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}

	l, err := cfg.Build()
	if err != nil {
		// 日志配置失败不该让服务起不来：退回 example logger 保底
		l = zap.NewExample()
	}
	global.LOG = l
	zap.ReplaceGlobals(global.LOG)
	return nil
}
