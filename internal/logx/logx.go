// Package logx 是网关的统一日志管理器：以标准库 log/slog 为核心，
// 将一条日志同时扇出到三路出口——终端窗口、滚动文件、前端出口接口。
//
//	业务代码 logx.Module("channel").Info(...)
//	          └─ fanout ─┬─ TextHandler  → 终端(os.Stdout)
//	                     ├─ JSONHandler  → 滚动文件(大小 + 每日轮转)
//	                     └─ sinkHandler  → 环形缓冲 + SSE（/api/syslog）
package logx

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// Options 日志管理器初始化参数。
type Options struct {
	Level       string // debug | info | warn | error（大小写不敏感）
	Console     bool   // 终端窗口输出
	File        string // 文件输出路径（空则不落文件）
	MaxSizeMB   int    // 单文件大小上限（MB），大小轮转阈值
	MaxBackups  int    // 保留历史文件份数
	MaxAgeDays  int    // 历史文件保留天数
	Compress    bool   // 历史文件 gzip 压缩
	DailyRotate bool   // 每日 00:00 轮转
	BufferSize  int    // 前端出口环形缓冲条数
}

var (
	mu   sync.RWMutex
	root *slog.Logger
	sink *ringSink // 前端出口的环形缓冲
)

// Init 按配置装配三路输出并设置为全局默认日志器。可重复调用（热重载配置）。
func Init(opt Options) {
	level := ParseLevel(opt.Level)
	var handlers []slog.Handler

	// 1) 终端窗口：文本格式，便于人读。
	if opt.Console {
		handlers = append(handlers, slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	}

	// 2) 滚动文件：JSON 格式，便于后续采集/检索；大小 + 每日轮转。
	if opt.File != "" {
		handlers = append(handlers, slog.NewJSONHandler(newRotator(opt), &slog.HandlerOptions{Level: level}))
	}

	// 3) 前端出口：写入环形缓冲，供 /api/syslog 拉取与 SSE 推送。
	s := newRingSink(opt.BufferSize)
	handlers = append(handlers, s.handler(level))

	logger := slog.New(&fanout{handlers: handlers})

	mu.Lock()
	root = logger
	sink = s
	mu.Unlock()

	slog.SetDefault(logger) // 让散落的 slog.Info / 桥接的标准库 log 也走这里
}

// Module 返回带模块标签的“分支”日志器；name 会作为 mod 字段附加到每条日志。
// 未初始化时回退到 slog 默认器，保证任何时刻调用都安全。
func Module(name string) *slog.Logger {
	mu.RLock()
	r := root
	mu.RUnlock()
	if r == nil {
		return slog.Default().With("mod", name)
	}
	return r.With("mod", name)
}

// ParseLevel 把级别字符串解析为 slog.Level，未知值回退 Info。
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// fanout 是一个把单条记录分发到多个子 handler 的 slog.Handler。
type fanout struct {
	handlers []slog.Handler
}

func (f *fanout) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range f.handlers {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (f *fanout) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range f.handlers {
		if h.Enabled(ctx, r.Level) {
			// 每个子 handler 拿到独立副本，避免 Attrs 迭代相互影响。
			_ = h.Handle(ctx, r.Clone())
		}
	}
	return nil
}

func (f *fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return &fanout{handlers: next}
}

func (f *fanout) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		next[i] = h.WithGroup(name)
	}
	return &fanout{handlers: next}
}

// 确保实现接口。
var _ slog.Handler = (*fanout)(nil)
