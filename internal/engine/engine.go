package engine

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

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

// Apply 以目标 plans 为期望状态，对运行中的 worker 做差量调谐：
//   - 期望有、当前无        → 启动
//   - 期望无、当前有        → 停止
//   - 两者都有但配置指纹变化 → 重启（先停后启）
//
// 可重复调用，用于配置保存 / 删除后的热重载。
func (e *Engine) Apply(plans []ChannelPlan, _ []store.DeviceModel) {
	desired := make(map[int]ChannelPlan, len(plans))
	for _, p := range plans {
		desired[p.ChannelID] = p
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// 停止不再期望存在，或配置已变化的 worker。
	for id, w := range e.workers {
		p, keep := desired[id]
		if !keep {
			e.log.Info("停止链路（已删除）", "channel", w.name, "id", id)
			w.stop()
			delete(e.workers, id)
			continue
		}
		if planFingerprint(p) != w.fp {
			e.log.Info("重启链路（配置变更）", "channel", w.name, "id", id)
			w.stop()
			delete(e.workers, id)
		}
	}

	// 启动新增的、或刚因变更被移除的 worker。
	for id, p := range desired {
		if _, running := e.workers[id]; running {
			continue
		}
		e.startChannel(p)
	}
}

// startChannel 解析配置、构造驱动并启动一个 worker。调用方须持有 e.mu。
func (e *Engine) startChannel(p ChannelPlan) {
	ch := store.Channel{ID: p.ChannelID, Name: p.ChannelName, Type: p.ChannelType, Config: p.Config}
	cfg, err := connector.ParseConfig(ch)
	if err != nil {
		e.log.Warn("链路配置解析失败，跳过", "channel", p.ChannelName, "id", p.ChannelID, "err", err)
		return
	}
	drv, err := connector.NewDriver(cfg)
	if err != nil {
		e.log.Warn("链路驱动创建失败，跳过", "channel", p.ChannelName, "id", p.ChannelID, "type", p.ChannelType, "err", err)
		return
	}
	w := newWorker(p.ChannelID, p.ChannelName, planFingerprint(p), cfg, drv, p)
	w.start(e.ctx)
	e.workers[p.ChannelID] = w
	e.log.Info("启动链路", "channel", p.ChannelName, "id", p.ChannelID, "type", p.ChannelType,
		"target", cfg.Target(), "devices", len(p.Devices))
}

// Submit 向指定链路投递一条写命令（非阻塞）。
// channelID 不存在或队列已满时返回 false。
func (e *Engine) Submit(channelID int, cmd WriteCommand) bool {
	e.mu.Lock()
	w, ok := e.workers[channelID]
	e.mu.Unlock()
	if !ok {
		return false
	}
	return w.tryWrite(cmd)
}

// SessionEntry 是单个属性的实时值缓存条目（API 可见）。
type SessionEntry struct {
	Value     any       `json:"value"`     // 工程值
	Timestamp time.Time `json:"timestamp"` // 最后更新时间
}

// Values 返回指定链路的所有缓存实时值快照。
// 不存在返回 nil。
func (e *Engine) Values(channelID int) map[string]SessionEntry {
	e.mu.Lock()
	w, ok := e.workers[channelID]
	e.mu.Unlock()
	if !ok {
		return nil
	}
	return w.getValues()
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

// planFingerprint 计算采集计划的配置指纹：
// 链路类型 + 设备协议 + 设备数量 + 每个设备的分组摘要。
// 指纹不变则热重载时无需重启，避免无谓抖动。
func planFingerprint(p ChannelPlan) string {
	h := sha1.New()
	h.Write([]byte(p.ChannelType))
	h.Write([]byte{0})
	h.Write([]byte(p.ChannelName))
	h.Write([]byte{0})

	// 轮询间隔
	h.Write([]byte{byte(p.PollMs >> 8), byte(p.PollMs)})

	// 传输层配置（IP/端口/串口参数等），变化时需重启
	h.Write(p.Config)
	h.Write([]byte{0})

	// 设备列表
	for _, dev := range p.Devices {
		h.Write([]byte(dev.Protocol))
		h.Write([]byte{0})
		h.Write([]byte{dev.UnitID})
		h.Write([]byte(dev.ModelName))
		h.Write([]byte{0})

		// 分组指纹：功能码 + 起始地址 + 数量
		for _, g := range dev.Groups {
			h.Write([]byte{byte(g.ReadFC)})
			writeUint16(h, g.StartAddr)
			writeUint16(h, g.Quantity)
		}
		h.Write([]byte{0xFF})

		// 属性元数据指纹：只哈希影响采集/写操作结果的字段，排除 Legacy 兼容字段，
		// 避免 LegacyBase/LegacyDataLength 转换后指纹抖动导致不必要的 worker 重启。
		for _, prop := range dev.Props {
			h.Write([]byte(prop.Name))
			h.Write([]byte{0})
			h.Write([]byte(prop.DataType))
			h.Write([]byte{0})
			writeUint16(h, prop.StartBit)
			writeUint16(h, prop.EndBit)
			writeUint16(h, prop.Offset)
			writeUint16(h, prop.RegisterBase)
			h.Write([]byte{byte(prop.ReadFC)})
			h.Write([]byte{byte(prop.WriteFC)})
			// float64 二进制表示保证精确匹配
			int64Bits(prop.Coefficient, h)
			int64Bits(prop.DeltaValue, h)
			h.Write([]byte(prop.ByteOrder))
			h.Write([]byte(prop.AccessMode))
			h.Write([]byte{0xFD})
		}
		h.Write([]byte{0xFE})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func writeUint16(h hasher, v int) {
	h.Write([]byte{byte(v >> 8), byte(v)})
}

// int64Bits 将 float64 的 IEEE 754 二进制表示写入指纹哈希，
// 保证浮点值的精确匹配（避免字符串格式化的精度丢失）。
func int64Bits(f float64, h hasher) {
	bits := math.Float64bits(f)
	for i := 7; i >= 0; i-- {
		h.Write([]byte{byte(bits >> (i * 8))})
	}
}

// hasher 是 sha1.hash 实现的最小写入接口，便于测试桩接。
type hasher interface {
	Write([]byte) (int, error)
}
