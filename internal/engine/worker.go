package engine

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"gateway/internal/engine/connector"
	"gateway/internal/logx"
)

// workerState 描述一条链路 worker 的运行状态。
type workerState struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Target    string `json:"target"`
	Connected bool   `json:"connected"`
	LastError string `json:"lastError,omitempty"`
}

// worker 承载一条链路：独占一个 Driver，在自己的 goroutine 中管理连接生命周期。
type worker struct {
	id   int
	name string
	fp   string // 配置指纹，用于热重载时判断是否需要重启
	cfg  connector.Config
	drv  connector.Driver

	log    *slog.Logger
	cancel context.CancelFunc
	done   chan struct{}

	mu        sync.Mutex
	connected bool
	lastErr   string
}

// newWorker 构造 worker，此时尚未启动 goroutine。
func newWorker(id int, name, fp string, cfg connector.Config, drv connector.Driver) *worker {
	return &worker{
		id:   id,
		name: name,
		fp:   fp,
		cfg:  cfg,
		drv:  drv,
		log:  logx.Module("engine"),
		done: make(chan struct{}),
	}
}

// start 启动链路 goroutine，用 parent 派生的 ctx 控制生命周期。
func (w *worker) start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	go w.run(ctx)
}

// stop 请求停止并等待 goroutine 退出（阻塞直到 Close 完成）。
func (w *worker) stop() {
	if w.cancel != nil {
		w.cancel()
	}
	<-w.done
}

// run 是链路主循环：连接（失败后每 3 秒固定间隔重连）→ 保持 → ctx 取消则关闭退出。
// 第一阶段不做协议轮询，仅负责把链路建立并维持起来。
func (w *worker) run(ctx context.Context) {
	defer close(w.done)
	defer func() {
		if err := w.drv.Close(); err != nil {
			w.log.Warn("关闭链路失败", "channel", w.name, "err", err)
		}
		w.setConnected(false, "")
	}()

	const reconnectInterval = 3 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := w.drv.Open(ctx); err != nil {
			w.setConnected(false, err.Error())
			w.log.Warn("链路连接失败，稍后重试",
				"channel", w.name, "target", w.cfg.Target(), "err", err, "retryIn", reconnectInterval.String())
			if !sleepCtx(ctx, reconnectInterval) {
				return
			}
			continue
		}

		w.setConnected(true, "")
		w.log.Info("链路已连接", "channel", w.name, "type", w.cfg.Type, "target", w.cfg.Target())

		// 保持连接，直到收到取消信号。协议轮询将在后续阶段接入此处。
		<-ctx.Done()
		return
	}
}

func (w *worker) setConnected(v bool, errMsg string) {
	w.mu.Lock()
	w.connected = v
	w.lastErr = errMsg
	w.mu.Unlock()
}

// state 返回 worker 当前状态快照。
func (w *worker) state() workerState {
	w.mu.Lock()
	connected, lastErr := w.connected, w.lastErr
	w.mu.Unlock()
	return workerState{
		ID:        w.id,
		Name:      w.name,
		Type:      w.cfg.Type,
		Target:    w.cfg.Target(),
		Connected: connected,
		LastError: lastErr,
	}
}

// sleepCtx 在 ctx 可取消的前提下睡眠 d；被取消返回 false。
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
