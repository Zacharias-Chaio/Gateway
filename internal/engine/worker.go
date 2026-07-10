package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"gateway/internal/engine/connector"
	"gateway/internal/engine/converter"
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

// sessionEntry 是单个属性的实时值缓存条目（内部使用，API 层看 SessionEntry）。
type sessionEntry = SessionEntry

// worker 承载一条链路：独占一个 Driver，在自己的 goroutine 中管理连接生命周期
// 与协议轮询、写命令下发。
type worker struct {
	id   int
	name string
	fp   string // 配置指纹，用于热重载时判断是否需要重启
	cfg  connector.Config
	drv  connector.Driver
	plan ChannelPlan

	log    *slog.Logger
	cancel context.CancelFunc
	done   chan struct{}

	// 写命令优先级队列（非阻塞写入，worker 侧优先消费）。
	writeCh chan WriteCommand

	mu        sync.Mutex
	connected bool
	lastErr   string

	// session 值缓存：key = "deviceIndex/propName"
	sess sync.RWMutex
	data map[string]sessionEntry
}

// newWorker 构造 worker，此时尚未启动 goroutine。
func newWorker(id int, name, fp string, cfg connector.Config, drv connector.Driver, plan ChannelPlan) *worker {
	return &worker{
		id:      id,
		name:    name,
		fp:      fp,
		cfg:     cfg,
		drv:     drv,
		plan:    plan,
		log:     logx.Module("engine"),
		done:    make(chan struct{}),
		writeCh: make(chan WriteCommand, 32),
		data:    make(map[string]sessionEntry),
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

// tryWrite 尝试非阻塞地投递一条写命令。
// 队列满时返回 false，调用方应自行处理超时。
func (w *worker) tryWrite(cmd WriteCommand) bool {
	select {
	case w.writeCh <- cmd:
		return true
	default:
		return false
	}
}

// getValues 返回所有缓存值快照（供 API realtime 查询）。
func (w *worker) getValues() map[string]SessionEntry {
	w.sess.RLock()
	defer w.sess.RUnlock()
	out := make(map[string]SessionEntry, len(w.data))
	for k, v := range w.data {
		out[k] = v
	}
	return out
}

// run 是链路主循环：连接（失败后按 reconnectRetries 策略重连）→ 采集循环 → ctx 取消则关闭退出。
func (w *worker) run(ctx context.Context) {
	defer close(w.done)
	defer func() {
		if err := w.drv.Close(); err != nil {
			w.log.Warn("关闭链路失败", "channel", w.name, "err", err)
		}
		w.setConnected(false, "")
	}()

	const reconnectInterval = 3 * time.Second
	connectAttempts := 0 // 已尝试的连接次数（用于 reconnectRetries 判断）

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := w.drv.Open(ctx); err != nil {
			connectAttempts++
			w.setConnected(false, err.Error())
			w.markAllOffline()

			// reconnectRetries == 0 表示无限重连；> 0 表示达到次数后放弃链路。
			maxRetry := w.cfg.ReconnectRetries
			if maxRetry > 0 && connectAttempts > maxRetry {
				w.log.Error("链路连接失败次数已达上限，停止重连",
					"channel", w.name, "target", w.cfg.Target(),
					"attempts", connectAttempts, "max", maxRetry)
				return
			}

			w.log.Warn("链路连接失败，稍后重试",
				"channel", w.name, "target", w.cfg.Target(), "err", err,
				"attempt", connectAttempts, "retryIn", reconnectInterval.String())
			if !sleepCtx(ctx, reconnectInterval) {
				return
			}
			continue
		}

		// 连接成功，重置计数
		connectAttempts = 0
		w.setConnected(true, "")
		w.log.Info("链路已连接，开始采集",
			"channel", w.name, "type", w.cfg.Type, "target", w.cfg.Target(),
			"devices", len(w.plan.Devices), "pollInterval", w.pollDuration().String())

		// 连接成功后进入采集循环；返回非 nil 表示需重连。
		if w.collectLoop(ctx) {
			w.setConnected(false, "")
			if !sleepCtx(ctx, reconnectInterval) {
				return
			}
			continue
		}
		return // ctx 取消，正常退出
	}
}

// collectLoop 是采集主循环，在连接已建立的前提下运行。
// 返回 true 表示链路异常需要重连，false 表示 ctx 取消正常退出。
func (w *worker) collectLoop(ctx context.Context) bool {
	pollInterval := w.pollDuration()

	// 逐设备轮询：一个 tick 轮询一个设备。
	devIdx := 0
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		// ── 写优先级检查：非阻塞查看是否有写命令待执行 ──
		select {
		case cmd := <-w.writeCh:
			if err := w.execWrite(ctx, cmd); err != nil {
				w.log.Warn("写命令执行失败", "channel", w.name, "prop", cmd.PropName, "err", err)
				if isLinkError(err) {
					return true // 需重连
				}
			}
			continue // 写完成后立即检查下一条写命令，保证写优先
		default:
			// 无写命令，继续轮询
		}

		if len(w.plan.Devices) > 0 {
			dev := &w.plan.Devices[devIdx%len(w.plan.Devices)]
			if err := w.pollDevice(ctx, dev); err != nil {
				w.log.Warn("设备轮询失败", "channel", w.name, "device", dev.ModelName, "err", err)
				w.setOnline(devIndexByName(w.plan, dev), false)
				if isLinkError(err) {
					return true
				}
			} else {
				w.setOnline(devIndexByName(w.plan, dev), true)
			}
			devIdx++
		}

		// 等待下一个 tick 或 ctx 取消
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}

// pollDuration 返回轮询间隔，默认 500ms。
func (w *worker) pollDuration() time.Duration {
	if w.cfg.PollInterval > 0 {
		return time.Duration(w.cfg.PollInterval) * time.Millisecond
	}
	return 500 * time.Millisecond
}

// pollDevice 轮询单个设备：遍历其所有寄存器分组，逐组发送读请求。
func (w *worker) pollDevice(ctx context.Context, dev *DevicePlan) error {
	for gi := range dev.Groups {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if err := w.pollOne(ctx, dev, gi); err != nil {
			return err
		}
		if w.cfg.FrameInterval > 0 {
			time.Sleep(time.Duration(w.cfg.FrameInterval) * time.Millisecond)
		}
	}
	return nil
}

// pollOne 执行一次寄存器组读取（含重发逻辑）。
// resendRetries 控制单帧发送失败后的重试次数（0 = 不重试，发一次即返回错误）。
func (w *worker) pollOne(ctx context.Context, dev *DevicePlan, gi int) error {
	g := dev.Groups[gi]

	// 组装读请求
	req, tid, err := dev.Conv.EncodeRead(dev.UnitID, g.ReadFC, g.StartAddr, g.Quantity)
	if err != nil {
		return err
	}

	maxAttempts := w.cfg.ResendRetries + 1 // resendRetries=0 → 仅发 1 次
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 发送
		w.logTX(dev, req)
		if _, err := w.drv.Send(req); err != nil {
			lastErr = err
			w.log.Debug("读请求发送失败",
				"channel", w.name, "device", dev.ModelName,
				"attempt", attempt, "err", err)
			continue
		}

		// 帧间隔（半双工总线发送后需等待响应）
		if w.cfg.FrameInterval > 0 {
			time.Sleep(time.Duration(w.cfg.FrameInterval) * time.Millisecond)
		}

		// 读取响应（渐进式帧读取）
		raw, err := w.readFrame(ctx, dev, byte(g.ReadFC), g.Quantity, tid)
		if err != nil {
			lastErr = err
			// 链路层错误（连接断开）无需重发，直接返回触发重连
			if isLinkError(err) {
				return err
			}
			w.log.Debug("读响应解析失败",
				"channel", w.name, "device", dev.ModelName,
				"attempt", attempt, "err", err)
			continue
		}

		// 解析各属性值并写入 session 缓存
		for _, m := range g.Members {
			if len(raw) < m.ByteOffset+m.ByteLen {
				continue
			}
			chunk := raw[m.ByteOffset : m.ByteOffset+m.ByteLen]
			val, err := converter.MapRegisters(chunk, m.Prop)
			if err != nil {
				continue
			}
			key := cacheKey(devIndexByName(w.plan, dev), m.Prop.Name)
			w.sess.Lock()
			w.data[key] = sessionEntry{Value: val, Timestamp: time.Now()}
			w.sess.Unlock()

			w.log.Debug("采集成功",
				"channel", w.name, "device", dev.ModelName,
				"prop", m.Prop.Name, "value", val)
		}
		return nil
	}

	// 所有重发均失败
	if lastErr != nil {
		w.log.Warn("读请求重发耗尽",
			"channel", w.name, "device", dev.ModelName,
			"attempts", maxAttempts, "lastErr", lastErr)
		return lastErr
	}
	return errors.New("读请求失败")
}

// readFrame 渐进式读取一帧响应。
// RTU 按固定长度读，TCP 先读 MBAP 头再按 length 字段读完整帧。
func (w *worker) readFrame(ctx context.Context, dev *DevicePlan, fc byte, quantity int, tid uint16) ([]byte, error) {
	const (
		readTimeout  = 1500 * time.Millisecond // 单次 Receive 超时
		maxWaitTotal = 3000 * time.Millisecond // 整帧最大等待时间
	)

	deadline := time.Now().Add(maxWaitTotal)
	var buf []byte

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		tmp := make([]byte, 256)
		n, err := w.drv.Receive(tmp, readTimeout)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}

		// 尝试解码
		data, err := dev.Conv.DecodeRead(buf, tid, dev.UnitID, fc, quantity)
		if err == nil {
			w.logRX(dev, buf)
			return data, nil
		}
		if !converter.IsShortFrame(err) {
			// 异常码或校验失败
			return nil, err
		}
		// ErrShortFrame → 继续累积字节
	}
	return nil, errors.New("读取响应帧超时")
}

// execWrite 执行一条写命令（含重发逻辑）。
// resendRetries 控制单帧发送失败后的重试次数（0 = 不重试）。
func (w *worker) execWrite(ctx context.Context, cmd WriteCommand) error {
	if cmd.DeviceIndex < 0 || cmd.DeviceIndex >= len(w.plan.Devices) {
		return errors.New("设备序号越界")
	}
	dev := &w.plan.Devices[cmd.DeviceIndex]

	// 查找属性
	prop, err := converter.FindWriteProp(dev.Props, cmd.PropName)
	if err != nil {
		return err
	}

	// 逆变换：engineering = raw×coef+delta → raw = (engineering - delta) / coef
	coef := prop.Coefficient
	if coef == 0 {
		coef = 1
	}
	rawVal := (cmd.RawValue - prop.DeltaValue) / coef

	// 编码 PDU
	pdu, err := converter.EncodeValuePDU(prop, rawVal, prop.RegisterBase+prop.Offset)
	if err != nil {
		return err
	}

	// 组帧
	frame, tid, err := dev.Conv.EncodeWrite(dev.UnitID, pdu)
	if err != nil {
		return err
	}

	maxAttempts := w.cfg.ResendRetries + 1
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 发送
		w.logTX(dev, frame)
		if _, err := w.drv.Send(frame); err != nil {
			lastErr = err
			continue
		}
		if w.cfg.FrameInterval > 0 {
			time.Sleep(time.Duration(w.cfg.FrameInterval) * time.Millisecond)
		}

		// 读响应（渐进式）
		buf := make([]byte, 0, 32)
		const writeRespTimeout = 1500 * time.Millisecond
		deadline := time.Now().Add(3000 * time.Millisecond)

		writeOK := false
		for time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			tmp := make([]byte, 64)
			n, err := w.drv.Receive(tmp, writeRespTimeout)
			if err != nil {
				lastErr = err
				break // 内层循环，尝试下一轮重发
			}
			if n > 0 {
				buf = append(buf, tmp[:n]...)
			}
			err = dev.Conv.DecodeWrite(buf, tid, dev.UnitID, byte(prop.WriteFC),
				prop.RegisterBase+prop.Offset, prop.RegCount())
			if err == nil {
				w.logRX(dev, buf)
				return nil
			}
			if !converter.IsShortFrame(err) {
				lastErr = err
				break
			}
			writeOK = true // 继续累积
		}
		if !writeOK && lastErr == nil {
			lastErr = errors.New("写响应帧超时")
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("写命令失败")
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

// ─── 辅助函数 ────────────────────────────────────────────────

// cacheKey 生成 session 缓存键。
func cacheKey(devIdx int, propName string) string {
	return fmt.Sprintf("%d/%s", devIdx, propName)
}

// logTX 记录发送报文（Debug 级），便于通信排障。
func (w *worker) logTX(dev *DevicePlan, p []byte) {
	w.log.Debug("TX 发送报文",
		"channel", w.name, "device", dev.ModelName,
		"hex", fmt.Sprintf("% x", p), "len", len(p))
}

// logRX 记录接收报文（Debug 级），便于通信排障。
func (w *worker) logRX(dev *DevicePlan, p []byte) {
	w.log.Debug("RX 接收报文",
		"channel", w.name, "device", dev.ModelName,
		"hex", fmt.Sprintf("% x", p), "len", len(p))
}

// OnlinePropName 是设备在线状态的虚拟属性名（模型默认属性，不映射实际寄存器）。
const OnlinePropName = "在线状态"

// hasOnlineProp 判断设备模型的属性表中是否包含"在线状态"虚拟属性。
func hasOnlineProp(dev *DevicePlan) bool {
	for _, p := range dev.Props {
		if p.Name == OnlinePropName {
			return true
		}
	}
	return false
}

// setOnline 更新指定设备的在线状态缓存。
// online=true → 值 1（报文交互正常）；online=false → 值 0（链路断开或设备无响应）。
func (w *worker) setOnline(devIdx int, online bool) {
	dev := &w.plan.Devices[devIdx]
	if !hasOnlineProp(dev) {
		return
	}
	val := int64(0)
	if online {
		val = 1
	}
	w.sess.Lock()
	w.data[cacheKey(devIdx, OnlinePropName)] = sessionEntry{Value: val, Timestamp: time.Now()}
	w.sess.Unlock()
}

// markAllOffline 将该链路下所有包含"在线状态"属性的设备标记为离线（0）。
func (w *worker) markAllOffline() {
	for i := range w.plan.Devices {
		w.setOnline(i, false)
	}
}

// devIndexByName 在 plan 中查找设备的序号（指针比较）。
func devIndexByName(plan ChannelPlan, dev *DevicePlan) int {
	for i := range plan.Devices {
		if &plan.Devices[i] == dev {
			return i
		}
	}
	return 0
}

// isLinkError 判断错误是否需要重连（底层连接断开）。
func isLinkError(err error) bool {
	if err == nil {
		return false
	}
	// 1) 上下文取消：不视作链路错误（正常退出）。
	if errors.Is(err, context.Canceled) {
		return false
	}
	// 2) 标准 io.EOF / net.ErrClosed：典型的对端关闭。
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	// 3) net.Error：连接重置类立即重连；超时类（设备无响应但 TCP 可能仍存活）
	//    不视作链路错误，避免频繁重连——若连接真的半开，后续 Send 会触发
	//    connection reset / EOF 再重连。
	var ne net.Error
	if errors.As(err, &ne) {
		return !ne.Timeout()
	}
	// 4) 关键字兜底：覆盖各平台底层错误文案变体（如 Windows
	//    "wsasend: An existing connection was forcibly closed by the remote host."）。
	msg := strings.ToLower(err.Error())
	for _, kw := range linkErrKeywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

// linkErrKeywords 是触发重连的错误消息关键字（统一小写匹配）。
// 注：io.EOF 由 errors.Is 精确捕获，此处不再用 "eof" 子串避免误伤。
var linkErrKeywords = []string{
	"connection reset by peer",
	"broken pipe",
	"connection forcibly closed",
	"wsasend",
	"wsaeconnreset",
	"connection refused",
	"connection aborted",
	"no connection could be made",
	"use of closed network connection",
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
