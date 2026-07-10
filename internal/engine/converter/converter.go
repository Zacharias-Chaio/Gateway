// Package converter 定义协议转换层的统一抽象。
//
// 转换器（Converter）负责协议知识：PDU 组帧、CRC/MBAP 校验、异常码解析。
// 转换器为无状态设计（TCP 仅持有自增事务 ID 计数器），可被安全复用。
//
// 与 connector（传输层）和 engine（编排层）的关系：
//
//	Worker ──持有──► Driver(connector)     传输层：字节收发
//	         ──持有──► Converter(converter) 协议层：帧组装/解析
//	         ──持有──► DevicePlan(engine)   编排层：分组/调度/值缓存
package converter

import (
	"errors"

	"gateway/internal/engine/converter/modbus"
)

// ErrShortFrame 帧数据不完整，需要更多字节。
// 统一暴露给 worker 做渐进式帧读取。
var ErrShortFrame = errors.New("协议帧数据不完整")

// ErrUnsupportedProtocol 不支持的协议类型。
var ErrUnsupportedProtocol = errors.New("不支持的协议类型")

// Protocol 标识协议类型，与设备模型 profile.protocolType 对应。
type Protocol string

const (
	ModbusRTU Protocol = "Modbus RTU"
	ModbusTCP Protocol = "Modbus TCP"
)

// FrameIO 是协议转换器的统一接口。
// 无论 RTU 还是 TCP，对 worker 而言暴露的都是统一的组帧/解帧方法。
//
// 设计说明：
//   - EncodeRead/EncodeWrite 返回完整请求帧 + 可选的事务标识符（TCP 用）。
//   - DecodeRead/DecodeWrite 接收累积的字节缓冲，不足返回 ErrShortFrame。
//   - 工作器负责字节累积与重试，转换器始终保持无状态。
type FrameIO interface {
	// Protocol 返回协议标识。
	Protocol() string

	// EncodeRead 组装读取请求帧。
	//   unitID: 从站地址 / 单元 ID
	//   fc:     Modbus 功能码
	//   startAddr, quantity: 起始地址与数量
	//   返回: 完整帧字节 + 事务标识符（TCP 用，RTU 为 0）
	EncodeRead(unitID byte, fc, startAddr, quantity int) ([]byte, uint16, error)

	// EncodeWrite 组装写入请求帧（pdu 已由 value.go 序列化好）。
	EncodeWrite(unitID byte, pdu []byte) ([]byte, uint16, error)

	// DecodeRead 解析读取响应帧，返回裸寄存器数据。
	//   tid: 请求时分配的事务标识符（TCP 用于配对校验，RTU 忽略）
	//   buf 不足一帧时返回 ErrShortFrame。
	DecodeRead(buf []byte, tid uint16, expectUnitID, fc byte, quantity int) ([]byte, error)

	// DecodeWrite 校验写入响应帧。
	DecodeWrite(buf []byte, tid uint16, expectUnitID, fc byte, startAddr, quantity int) error
}

// ─── RTU 适配器 ──────────────────────────────────────────────

// rtuAdapter 将 modbus.RTU 包装为 FrameIO 接口。
type rtuAdapter struct{ inner *modbus.RTU }

func (a *rtuAdapter) Protocol() string { return a.inner.Protocol() }

func (a *rtuAdapter) EncodeRead(unitID byte, fc, startAddr, quantity int) ([]byte, uint16, error) {
	frame, err := a.inner.EncodeRead(unitID, byte(fc), startAddr, quantity)
	return frame, 0, err // RTU 无事务 ID
}

func (a *rtuAdapter) EncodeWrite(unitID byte, pdu []byte) ([]byte, uint16, error) {
	frame, err := a.inner.EncodeWrite(unitID, pdu)
	return frame, 0, err
}

func (a *rtuAdapter) DecodeRead(buf []byte, _ uint16, expectUnitID, fc byte, quantity int) ([]byte, error) {
	data, err := a.inner.DecodeRead(buf, expectUnitID, fc, quantity)
	return data, wrapShort(err)
}

func (a *rtuAdapter) DecodeWrite(buf []byte, _ uint16, expectUnitID, fc byte, startAddr, quantity int) error {
	return wrapShort(a.inner.DecodeWrite(buf, expectUnitID, fc, startAddr, quantity))
}

// ─── TCP 适配器 ──────────────────────────────────────────────

// tcpAdapter 将 modbus.TCP 包装为 FrameIO 接口。
type tcpAdapter struct{ inner *modbus.TCP }

func (a *tcpAdapter) Protocol() string { return a.inner.Protocol() }

func (a *tcpAdapter) EncodeRead(unitID byte, fc, startAddr, quantity int) ([]byte, uint16, error) {
	return a.inner.EncodeRead(unitID, byte(fc), startAddr, quantity)
}

func (a *tcpAdapter) EncodeWrite(unitID byte, pdu []byte) ([]byte, uint16, error) {
	return a.inner.EncodeWrite(unitID, pdu)
}

func (a *tcpAdapter) DecodeRead(buf []byte, tid uint16, expectUnitID, fc byte, quantity int) ([]byte, error) {
	data, err := a.inner.DecodeRead(buf, tid, expectUnitID, fc, quantity)
	return data, wrapShort(err)
}

func (a *tcpAdapter) DecodeWrite(buf []byte, tid uint16, expectUnitID, fc byte, startAddr, quantity int) error {
	return wrapShort(a.inner.DecodeWrite(buf, tid, expectUnitID, fc, startAddr, quantity))
}

// wrapShort 将 modbus.ErrShortFrame 转换为本包的 ErrShortFrame，
// 保持 worker 侧只依赖 converter 包的错误定义。
func wrapShort(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, modbus.ErrShortFrame) {
		return ErrShortFrame
	}
	return err
}

// New 根据协议标识创建对应的 FrameIO 实现。
func New(proto string) (FrameIO, error) {
	switch Protocol(proto) {
	case ModbusRTU:
		return &rtuAdapter{inner: modbus.NewRTU()}, nil
	case ModbusTCP:
		return &tcpAdapter{inner: modbus.NewTCP()}, nil
	}
	return nil, ErrUnsupportedProtocol
}

// IsShortFrame 判断错误是否为「帧数据不完整」。
func IsShortFrame(err error) bool { return errors.Is(err, ErrShortFrame) }
