package engine

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"log/slog"
	"sort"
	"sync"

	"gateway/internal/engine/connector"
	"gateway/internal/logx"
	"gateway/internal/store"
)

// Engine 是链路的 supervisor：按配置差量启停 worker，支持热重载。
// 对外方法均为并发安全。
type Engine struct {
	ctx context.Context
	log *slog.Logger

	mu      sync.Mutex
	workers map[int]*worker // key: channel ID
}

// New 创建 Engine。ctx 作为所有 worker 的父上下文，取消即触发全部链路关闭。
func New(ctx context.Context) *Engine {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Engine{
		ctx:     ctx,
		log:     logx.Module("engine"),
		workers: make(map[int]*worker),
	}
}

// Apply 以目标 channels 为期望状态，对运行中的 worker 做差量调谐：
//   - 期望有、当前无        → 启动
//   - 期望无、当前有        → 停止
//   - 两者都有但配置指纹变化 → 重启（先停后启）
//
// 可重复调用，用于配置保存 / 删除后的热重载。
func (e *Engine) Apply(channels []store.Channel) {
	desired := make(map[int]store.Channel, len(channels))
	for _, ch := range channels {
		desired[ch.ID] = ch
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// 停止不再期望存在，或配置已变化的 worker。
	for id, w := range e.workers {
		ch, keep := desired[id]
		if !keep {
			e.log.Info("停止链路（已删除）", "channel", w.name, "id", id)
			w.stop()
			delete(e.workers, id)
			continue
		}
		if fingerprint(ch) != w.fp {
			e.log.Info("重启链路（配置变更）", "channel", w.name, "id", id)
			w.stop()
			delete(e.workers, id)
		}
	}

	// 启动新增的、或刚因变更被移除的 worker。
	for id, ch := range desired {
		if _, running := e.workers[id]; running {
			continue
		}
		e.startChannel(ch)
	}
}

// startChannel 解析配置、构造驱动并启动一个 worker。调用方须持有 e.mu。
func (e *Engine) startChannel(ch store.Channel) {
	cfg, err := connector.ParseConfig(ch)
	if err != nil {
		e.log.Warn("链路配置解析失败，跳过", "channel", ch.Name, "id", ch.ID, "err", err)
		return
	}
	drv, err := connector.NewDriver(cfg)
	if err != nil {
		e.log.Warn("链路驱动创建失败，跳过", "channel", ch.Name, "id", ch.ID, "type", ch.Type, "err", err)
		return
	}
	w := newWorker(ch.ID, ch.Name, fingerprint(ch), cfg, drv)
	w.start(e.ctx)
	e.workers[ch.ID] = w
	e.log.Info("启动链路", "channel", ch.Name, "id", ch.ID, "type", ch.Type, "target", cfg.Target())
}

// Stop 停止所有链路并等待其 goroutine 退出。
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id, w := range e.workers {
		w.stop()
		delete(e.workers, id)
	}
	e.log.Info("引擎已停止，所有链路关闭")
}

// Status 返回全部链路的运行状态快照，按链路 ID 升序。
func (e *Engine) Status() []workerState {
	e.mu.Lock()
	out := make([]workerState, 0, len(e.workers))
	for _, w := range e.workers {
		out = append(out, w.state())
	}
	e.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// fingerprint 计算链路配置指纹：类型 + 归一化后的 Config JSON。
// 指纹不变则热重载时无需重启，避免无谓抖动。
func fingerprint(ch store.Channel) string {
	h := sha1.New()
	h.Write([]byte(ch.Type))
	h.Write([]byte{0})
	h.Write(ch.Config)
	return hex.EncodeToString(h.Sum(nil))
}
