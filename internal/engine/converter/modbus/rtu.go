package modbus

// RTU 帧封装层。
//
// RTU 帧格式:
//
//	[从站地址(1)][PDU(N)][CRC16(2)]
//
// 其中 CRC16 低字节在前。
// 本文件的所有函数都是无状态的纯函数。

// rtuBuildFrame 组装一帧完整的 RTU 报文：[addr][PDU][CRC]。
func rtuBuildFrame(unitID byte, pdu []byte) []byte {
	body := make([]byte, 0, 1+len(pdu)+2)
	body = append(body, unitID)
	body = append(body, pdu...)
	return AppendCRC(body)
}

// rtuReadRespLen 根据 PDU 内容推导 RTU 读响应的完整帧长度：
//
//	1(addr) + 1(fc) + 1(byteCount) + byteCount + 2(crc)
//
// 调用方先读 3 字节（addr+fc+byteCount）后即可调用此函数算出总长。
func rtuReadRespLen(fc byte, quantity int) int {
	byteCount := readRespByteCount(fc, quantity)
	return 1 + 1 + 1 + byteCount + 2 // addr + fc + byteCountField + data + crc
}

// rtuMinReadRespLen 返回最小的可解析帧长度（addr+fc+byteCount）= 3。
// 用于渐进式读取：先读 3 字节拿到 byteCount，再算总长。
func rtuMinReadRespLen() int { return 3 }

// rtuExtractPDU 从完整 RTU 帧中提取 PDU 数据（校验 addr/CRC/异常码）。
// 返回去掉 addr 和 CRC 后的纯 PDU 部分。
func rtuExtractPDU(frame []byte, expectUnitID byte) ([]byte, error) {
	// 最小帧: addr + fc + crc = 4 字节（异常响应），正常读响应至少 5 字节
	if len(frame) < 4 {
		return nil, ErrShortFrame
	}
	if frame[0] != expectUnitID {
		return nil, ErrUnitIDMismatch
	}
	if !CheckCRC(frame) {
		return nil, ErrCRCMismatch
	}
	// 去掉 addr(1) 和 crc(2)
	return frame[1 : len(frame)-2], nil
}

// rtuWriteRespLen 写响应帧长度。
// 单写(05/06): addr+fc+value2+crc = 6；多写(0F/10): addr+fc+addr2+qty2+crc = 8。
func rtuWriteRespLen(reqFC byte) int {
	switch reqFC {
	case FCWriteMultiRegs, FCWriteMultiCoils:
		return 8
	default:
		return 6
	}
}
