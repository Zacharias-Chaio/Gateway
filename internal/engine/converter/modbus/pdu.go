package modbus

import "encoding/binary"

// PDU（Protocol Data Unit）是 Modbus RTU 与 TCP 共享的协议层核心：
//
//	[功能码(1)][数据(N)]
//
// 本文件提供 PDU 级的组装与解析函数，不含任何传输层封装（地址/CRC/MBAP）。

// ─── 读取请求 PDU ───────────────────────────────────────────
//
// PDU 格式: [FC(1)][起始地址 2B 大端][数量 2B 大端]，共 5 字节。

func buildReadPDU(fc byte, startAddr, quantity int) []byte {
	pdu := make([]byte, 5)
	pdu[0] = fc
	binary.BigEndian.PutUint16(pdu[1:3], uint16(startAddr))
	binary.BigEndian.PutUint16(pdu[3:5], uint16(quantity))
	return pdu
}

// ─── 读取响应 PDU ───────────────────────────────────────────
//
// PDU 格式: [FC(1)][字节计数(1)][数据(N)]
// 其中 N = 2 × quantity（寄存器）或 ceil(quantity/8)（线圈/离散输入）。

// parseReadPDU 从 PDU（不含地址/CRC/MBAP）中提取裸数据。
// 返回的 data 是去掉 FC 与 byteCount 后的纯数据字节。
func parseReadPDU(pdu []byte, fc byte, expectedQuantity int) ([]byte, error) {
	if len(pdu) < 2 {
		return nil, ErrShortFrame
	}
	respFC := pdu[0]
	if isExceptionFC(respFC) {
		// 异常响应: [异常FC(1)][异常码(1)]
		if len(pdu) < 2 {
			return nil, ErrShortFrame
		}
		return nil, &ExceptionError{FunctionCode: respFC, ExceptionCode: pdu[1]}
	}
	if respFC != fc {
		return nil, ErrFunctionCodeMismatch
	}
	byteCount := int(pdu[1])
	if len(pdu) < 2+byteCount {
		return nil, ErrShortFrame
	}
	return pdu[2 : 2+byteCount], nil
}

// readRespLen 根据 FC 和 quantity 计算响应数据区的字节数。
// 寄存器类(03/04): 每寄存器 2 字节；位类(01/02): 每 8 位 1 字节。
func readRespByteCount(fc byte, quantity int) int {
	switch fc {
	case FCReadCoils, FCReadDiscreteInputs:
		return (quantity + 7) / 8
	default:
		return quantity * 2
	}
}

// ─── 写请求 PDU ─────────────────────────────────────────────

// BuildWriteSingleRegPDU 组装「写单个寄存器」(FC=06) PDU: [FC][addr 2B][value 2B]。
func BuildWriteSingleRegPDU(startAddr int, value uint16) []byte {
	pdu := make([]byte, 5)
	pdu[0] = FCWriteSingleReg
	binary.BigEndian.PutUint16(pdu[1:3], uint16(startAddr))
	binary.BigEndian.PutUint16(pdu[3:5], value)
	return pdu
}

// BuildWriteMultiRegsPDU 组装「写多个寄存器」(FC=10) PDU:
// [FC][addr 2B][quantity 2B][byteCount 1][data N×2B]。
func BuildWriteMultiRegsPDU(startAddr, quantity int, data []byte) []byte {
	pdu := make([]byte, 6+len(data))
	pdu[0] = FCWriteMultiRegs
	binary.BigEndian.PutUint16(pdu[1:3], uint16(startAddr))
	binary.BigEndian.PutUint16(pdu[3:5], uint16(quantity))
	pdu[5] = byte(len(data))
	copy(pdu[6:], data)
	return pdu
}

// BuildWriteSingleCoilPDU 组装「写单个线圈」(FC=05) PDU: [FC][addr 2B][value 2B]。
// value 为 true 时写入 0xFF00，false 时写入 0x0000。
func BuildWriteSingleCoilPDU(startAddr int, on bool) []byte {
	pdu := make([]byte, 5)
	pdu[0] = FCWriteSingleCoil
	binary.BigEndian.PutUint16(pdu[1:3], uint16(startAddr))
	if on {
		binary.BigEndian.PutUint16(pdu[3:5], 0xFF00)
	} else {
		binary.BigEndian.PutUint16(pdu[3:5], 0x0000)
	}
	return pdu
}

// ─── 写响应 PDU 校验 ────────────────────────────────────────

// parseWritePDU 校验写应答 PDU。
// 正常写应答是请求的回显（单写）或 [FC][addr][quantity]（多写）。
// 异常则返回 ExceptionError。
func parseWritePDU(pdu []byte, reqFC byte, startAddr, quantity int) error {
	if len(pdu) < 1 {
		return ErrShortFrame
	}
	respFC := pdu[0]
	if isExceptionFC(respFC) {
		if len(pdu) < 2 {
			return ErrShortFrame
		}
		return &ExceptionError{FunctionCode: respFC, ExceptionCode: pdu[1]}
	}
	if respFC != reqFC {
		return ErrFunctionCodeMismatch
	}
	switch reqFC {
	case FCWriteSingleReg, FCWriteSingleCoil:
		// 回显: [FC][addr 2B][value 2B]，共 5 字节
		if len(pdu) < 5 {
			return ErrShortFrame
		}
		return nil
	case FCWriteMultiRegs, FCWriteMultiCoils:
		// [FC][addr 2B][quantity 2B]，共 5 字节
		if len(pdu) < 5 {
			return ErrShortFrame
		}
		return nil
	}
	return nil
}
