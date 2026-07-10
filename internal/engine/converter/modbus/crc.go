package modbus

// CRC-16/MODBUS 查表法实现。
// 多项式 0xA001（即 0x8005 的位反转），初始值 0xFFFF，结果不取反。
// 这是 Modbus RTU 帧尾的标准校验算法。

var crcTable [256]uint16

func init() {
	for i := 0; i < 256; i++ {
		crc := uint16(i)
		for j := 0; j < 8; j++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
		crcTable[i] = crc
	}
}

// CRC16 计算给定字节的 Modbus CRC-16，返回低字节在前（Little-Endian）。
// 这与 RTU 帧中 [crcLo][crcHi] 的字节序一致。
func CRC16(data []byte) (lo, hi byte) {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc = (crc >> 8) ^ crcTable[(crc^uint16(b))&0xFF]
	}
	return byte(crc & 0xFF), byte((crc >> 8) & 0xFF)
}

// AppendCRC 在 data 末尾追加 Modbus CRC-16（低字节在前）。
func AppendCRC(data []byte) []byte {
	lo, hi := CRC16(data)
	return append(data, lo, hi)
}

// CheckCRC 校验整帧（含末尾两字节 CRC）是否正确。
func CheckCRC(frame []byte) bool {
	if len(frame) < 3 {
		return false
	}
	lo, hi := CRC16(frame[:len(frame)-2])
	return lo == frame[len(frame)-2] && hi == frame[len(frame)-1]
}
