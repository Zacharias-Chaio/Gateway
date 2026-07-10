// Package modbus 实现 Modbus RTU / TCP 协议转换器。
//
// 本包提供统一的无状态帧封装：PDU（功能码+数据）在 RTU 与 TCP 间共享，
// 仅传输层封装不同（RTU 加地址+CRC；TCP 加 MBAP 头）。
//
// 所有 Encode/Decode 方法均为纯函数，不持有任何缓冲或游标状态。
// 当输入字节不足以构成完整帧时返回 ErrShortFrame，由调用方（worker）
// 累积更多字节后重试。
package modbus

import (
	"errors"
	"sync/atomic"
)

// RTU 实现 Modbus RTU 协议转换器（无状态）。
type RTU struct{}

// NewRTU 创建 Modbus RTU 转换器。
func NewRTU() *RTU { return &RTU{} }

func (*RTU) Protocol() string { return "Modbus RTU" }

// EncodeRead 组装 RTU 读请求帧。
func (*RTU) EncodeRead(unitID byte, fc byte, startAddr, quantity int) ([]byte, error) {
	pdu := buildReadPDU(fc, startAddr, quantity)
	return rtuBuildFrame(unitID, pdu), nil
}

// EncodeWrite 组装 RTU 写请求帧。
func (*RTU) EncodeWrite(unitID byte, pdu []byte) ([]byte, error) {
	return rtuBuildFrame(unitID, pdu), nil
}

// DecodeRead 解析 RTU 读响应，返回裸数据字节。
// buf 为当前累积的全部字节；不足一帧返回 ErrShortFrame。
func (*RTU) DecodeRead(buf []byte, expectUnitID, fc byte, quantity int) ([]byte, error) {
	expectedLen := rtuReadRespLen(fc, quantity)
	if len(buf) < expectedLen {
		return nil, ErrShortFrame
	}
	frame := buf[:expectedLen]
	pdu, err := rtuExtractPDU(frame, expectUnitID)
	if err != nil {
		return nil, err
	}
	return parseReadPDU(pdu, fc, quantity)
}

// DecodeWrite 校验 RTU 写响应。
func (*RTU) DecodeWrite(buf []byte, expectUnitID, fc byte, startAddr, quantity int) error {
	expectedLen := rtuWriteRespLen(fc)
	if len(buf) < expectedLen {
		return ErrShortFrame
	}
	frame := buf[:expectedLen]
	pdu, err := rtuExtractPDU(frame, expectUnitID)
	if err != nil {
		return err
	}
	return parseWritePDU(pdu, fc, startAddr, quantity)
}

// ReadFrameLen 返回 RTU 读响应的完整帧长度。
func (*RTU) ReadFrameLen(fc byte, quantity int) int {
	return rtuReadRespLen(fc, quantity)
}

// WriteFrameLen 返回 RTU 写响应的完整帧长度。
func (*RTU) WriteFrameLen(fc byte) int {
	return rtuWriteRespLen(fc)
}

// ──────────────────────────────────────────────────────────────

// TCP 实现 Modbus TCP 协议转换器。
// 唯一可变状态是自增的事务 ID 计数器，用于请求-响应配对。
type TCP struct {
	tid atomic.Uint32
}

// NewTCP 创建 Modbus TCP 转换器。
func NewTCP() *TCP { return &TCP{} }

func (*TCP) Protocol() string { return "Modbus TCP" }

// NextTID 分配一个新的事务 ID（自动递增，回绕）。
func (t *TCP) NextTID() uint16 {
	return uint16(t.tid.Add(1))
}

// EncodeRead 组装 TCP 读请求帧，返回帧字节与分配的事务 ID。
func (t *TCP) EncodeRead(unitID byte, fc byte, startAddr, quantity int) ([]byte, uint16, error) {
	tid := t.NextTID()
	pdu := buildReadPDU(fc, startAddr, quantity)
	return tcpBuildFrame(tid, unitID, pdu), tid, nil
}

// EncodeWrite 组装 TCP 写请求帧，返回帧字节与分配的事务 ID。
func (t *TCP) EncodeWrite(unitID byte, pdu []byte) ([]byte, uint16, error) {
	tid := t.NextTID()
	return tcpBuildFrame(tid, unitID, pdu), tid, nil
}

// DecodeRead 解析 TCP 读响应。
// 由于 TCP 帧长度在 MBAP 头中，需要分两步：先判断 MBAP 头是否足够，
// 再用 length 字段计算完整帧长度。
func (*TCP) DecodeRead(buf []byte, expectTID uint16, expectUnitID, fc byte, quantity int) ([]byte, error) {
	if len(buf) < tcpMBAPHeaderLen {
		return nil, ErrShortFrame
	}
	totalLen := tcpParseFrameLen(buf)
	if len(buf) < totalLen {
		return nil, ErrShortFrame
	}
	frame := buf[:totalLen]
	pdu, err := tcpExtractPDU(frame, expectTID, expectUnitID)
	if err != nil {
		return nil, err
	}
	return parseReadPDU(pdu, fc, quantity)
}

// DecodeWrite 校验 TCP 写响应。
func (*TCP) DecodeWrite(buf []byte, expectTID uint16, expectUnitID, fc byte, startAddr, quantity int) error {
	if len(buf) < tcpMBAPHeaderLen {
		return ErrShortFrame
	}
	totalLen := tcpParseFrameLen(buf)
	if len(buf) < totalLen {
		return ErrShortFrame
	}
	frame := buf[:totalLen]
	pdu, err := tcpExtractPDU(frame, expectTID, expectUnitID)
	if err != nil {
		return err
	}
	return parseWritePDU(pdu, fc, startAddr, quantity)
}

// ReadFrameLenTCP 从已读取的 MBAP 头中计算完整帧长度。
// 若 buf 不足 MBAP 头长度返回 0。
func ReadFrameLenTCP(buf []byte) int {
	return tcpParseFrameLen(buf)
}

// MBAPHeaderLen MBAP 头固定长度（含 unitID）。
const MBAPHeaderLen = tcpMBAPHeaderLen

// AsException 尝试将 error 转为 ExceptionError，成功返回异常信息，否则返回原始错误。
func AsException(err error) (*ExceptionError, bool) {
	var exc *ExceptionError
	if errors.As(err, &exc) {
		return exc, true
	}
	return nil, false
}
