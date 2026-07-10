package modbus

import "errors"

// Modbus 功能码常量。
const (
	FCReadCoils          = 0x01 // 读线圈（位，可读写）
	FCReadDiscreteInputs = 0x02 // 读离散输入（位，只读）
	FCReadHoldingRegs    = 0x03 // 读保持寄存器（16位，可读写）
	FCReadInputRegs      = 0x04 // 读输入寄存器（16位，只读）
	FCWriteSingleCoil    = 0x05 // 写单个线圈
	FCWriteSingleReg     = 0x06 // 写单个寄存器
	FCWriteMultiCoils    = 0x0F // 写多个线圈
	FCWriteMultiRegs     = 0x10 // 写多个寄存器
)

// 异常码（Modbus Exception Code）。
const (
	ExcIllegalFunction        = 0x01 // 非法功能码
	ExcIllegalDataAddress     = 0x02 // 非法数据地址
	ExcIllegalDataValue       = 0x03 // 非法数据值
	ExcSlaveDeviceFailure     = 0x04 // 从站设备故障
	ExcAcknowledge            = 0x05 // 确认
	ExcSlaveDeviceBusy        = 0x06 // 从站设备忙
	ExcMemoryParityError      = 0x08 // 存储器奇偶校验错误
	ExcGatewayPathUnavailable = 0x0A // 网关路径不可用
	ExcGatewayTargetNoResp    = 0x0B // 网关目标设备无响应
)

// exceptionText 返回异常码的中文描述。
func exceptionText(code byte) string {
	switch code {
	case ExcIllegalFunction:
		return "非法功能码"
	case ExcIllegalDataAddress:
		return "非法数据地址"
	case ExcIllegalDataValue:
		return "非法数据值"
	case ExcSlaveDeviceFailure:
		return "从站设备故障"
	case ExcAcknowledge:
		return "从站确认（需稍后重试）"
	case ExcSlaveDeviceBusy:
		return "从站设备忙"
	case ExcMemoryParityError:
		return "存储器奇偶校验错误"
	case ExcGatewayPathUnavailable:
		return "网关路径不可用"
	case ExcGatewayTargetNoResp:
		return "网关目标设备无响应"
	default:
		return "未知异常码"
	}
}

// ExceptionError 表示 Modbus 异常响应。
type ExceptionError struct {
	FunctionCode  byte // 含最高位异常标志（0x80+原始FC）
	ExceptionCode byte
}

func (e *ExceptionError) Error() string {
	return "Modbus 异常: FC=0x" + btoHex(e.FunctionCode) + " code=0x" + btoHex(e.ExceptionCode) + "(" + exceptionText(e.ExceptionCode) + ")"
}

// isExceptionFC 判断功能码是否设置了异常标志位（最高位为1）。
func isExceptionFC(fc byte) bool { return fc&0x80 != 0 }

// ErrShortFrame 帧数据不完整，需要更多字节。
// 转换器为无状态设计：每次 Decode 调用都从 buf 开头解析，
// 不足一帧时返回此错误，由 worker 负责累积更多字节后重试。
var ErrShortFrame = errors.New("Modbus 帧数据不完整")

// ErrCRCMismatch CRC 校验失败。
var ErrCRCMismatch = errors.New("Modbus CRC 校验失败")

// ErrUnitIDMismatch 响应的从站地址与请求不符。
var ErrUnitIDMismatch = errors.New("Modbus 响应从站地址不匹配")

// ErrFunctionCodeMismatch 响应的功能码与请求不符。
var ErrFunctionCodeMismatch = errors.New("Modbus 响应功能码不匹配")

// ErrTransactionIDMismatch TCP 响应的事务 ID 与请求不符。
var ErrTransactionIDMismatch = errors.New("Modbus TCP 事务 ID 不匹配")

// ErrProtocolIDMismatch TCP 响应的协议 ID 非 0。
var ErrProtocolIDMismatch = errors.New("Modbus TCP 协议 ID 不匹配")

func btoHex(b byte) string {
	const hexDigits = "0123456789ABCDEF"
	return string([]byte{hexDigits[b>>4], hexDigits[b&0x0F]})
}
