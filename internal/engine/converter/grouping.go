package converter

import (
	"fmt"
	"sort"
)

// DefaultMaxRegs 是 Modbus 协议单次读请求的寄存器数上限。
const DefaultMaxRegs = 125

// 本文件实现「寄存器分组」策略。
//
// 地址模型：实际寄存器地址 = registerBase + registerOffset
//
//   - registerBase：寄存器基址，是分组的核心依据。相同 (readFC, registerBase)
//     的属性归入同一读请求，一次 EncodeRead 从 base 开始连续读取。
//   - registerOffset：相对于基址的偏移量（寄存器个数，非字节），用于从响应帧中
//     定位该属性的起始位置：字节偏移 = registerOffset × 2。
//   - startBit / endBit：在该属性寄存器区间的位级定位（bit 0 = 最低位）。
//     占用寄存器数 = ceil((endBit + 1) / 16)，约束 endBit - startBit < regCount × 16。
//     例如 startBit=0,endBit=0 → 提取最低 1 位（1 寄存器）；
//     startBit=0,endBit=31 → 全部 32 位（2 寄存器）。
//   - string 类型不走位提取，忽略 startBit/endBit，按字节级处理。
//
// 分组规则：
//  1. 按 (readFC, registerBase) 分桶 —— 同一基址 + 同一功能码的属性合并为一次读请求。
//  2. 组内 Quantity = max(registerOffset + regCount)，覆盖所有成员的最远地址。
//  3. 组内 ByteOffset = registerOffset × 2，用于从响应数据中切片各属性。

// PropMeta 描述单个属性的协议映射元数据（来自设备模型 JSON）。
type PropMeta struct {
	Name         string  `json:"name"`
	PropID       string  `json:"propId"`
	DataType     string  `json:"dataType"`
	StartBit     int     `json:"startBit"`       // 起始位（bit 0 = 最低位）
	EndBit       int     `json:"endBit"`         // 终止位（含），regCount = ceil((endBit+1)/16)
	Offset       int     `json:"registerOffset"` // 相对基址的寄存器偏移，实际地址 = registerBase + registerOffset
	RegisterBase int     `json:"registerBase"`   // 寄存器基址（分组的依据）
	ReadFC       int     `json:"readFunctionCode"`
	WriteFC      int     `json:"writeFunctionCode"`
	Coefficient  float64 `json:"coefficient"`
	DeltaValue   float64 `json:"deltaValue"` // 偏移量（可正负），工程值 = 原始值 × coefficient + deltaValue
	ByteOrder    string  `json:"byteOrder"`
	AccessMode   string  `json:"accessMode"` // r / w / rw

	// Legacy 别名，仅用于向后兼容旧 JSON 数据（base → deltaValue, dataLength 逆向推导）。
	LegacyBase       float64 `json:"base,omitempty"`
	LegacyDataLength *int    `json:"dataLength,omitempty"`
}

// RegGroup 是一次读请求对应的寄存器组。
type RegGroup struct {
	ReadFC    int // Modbus 功能码
	StartAddr int // 实际请求的起始地址（= registerBase）
	Quantity  int // 请求的寄存器数量
	Members   []GroupMember
}

// GroupMember 是组内单个属性的定位信息。
type GroupMember struct {
	Prop       PropMeta
	ByteOffset int // 在响应数据中的字节偏移（= offset * 2）
	ByteLen    int // 该属性的字节长度（= regCount * 2）
}

// RegCount 返回该属性占用的寄存器数（由 startBit/endBit 推导）。
// string 类型或 endBit < 0 时返回 1。
func (p PropMeta) RegCount() int {
	if p.EndBit < 0 {
		return 1
	}
	return BitsToRegCount(p.EndBit)
}

// BitsToRegCount 将最高位序号转换为需要的寄存器数（每寄存器 16 bit）。
// endBit=0 → 1, endBit=15 → 1, endBit=16 → 2, endBit=31 → 2。
func BitsToRegCount(endBit int) int {
	if endBit < 0 {
		return 1
	}
	return endBit/16 + 1
}

// BuildGroups 将设备的属性列表按 (readFC, registerBase) 分组。
// 只处理可读属性（accessMode 含 r）。
// maxRegs 限制单个读请求的最大寄存器数（Modbus 协议上限 125）；
// 同一分组的覆盖范围超过该值时自动拆分为多个连续子请求，每个子请求的
// Quantity <= maxRegs。maxRegs <= 0 时取默认上限 125。
func BuildGroups(props []PropMeta, maxRegs int) []RegGroup {
	if maxRegs <= 0 {
		maxRegs = DefaultMaxRegs
	}

	// 1. 按分组键索引
	type key struct {
		fc   int
		base int
	}
	buckets := make(map[key][]PropMeta)

	for _, p := range props {
		if !canRead(p.AccessMode) {
			continue
		}
		// 跳过未配置读功能码的属性 —— FC=0 不是合法 Modbus 读码，会产生无效请求。
		if p.ReadFC <= 0 {
			continue
		}
		k := key{fc: p.ReadFC, base: p.RegisterBase}
		buckets[k] = append(buckets[k], p)
	}

	// 2. 对每个 bucket 计算覆盖范围；超过 maxRegs 则拆分为多段子请求
	groups := make([]RegGroup, 0, len(buckets))
	for k, members := range buckets {
		maxEnd := 0
		for _, m := range members {
			end := m.Offset + m.RegCount()
			if end > maxEnd {
				maxEnd = end
			}
		}

		// 按段拆分：每段最多覆盖 maxRegs 个寄存器
		for start := 0; start < maxEnd; start += maxRegs {
			end := start + maxRegs
			if end > maxEnd {
				end = maxEnd
			}
			segLen := end - start

			var segMembers []GroupMember
			for _, m := range members {
				mStart := m.Offset // 属性在原始 bucket 中的寄存器偏移
				mEnd := m.Offset + m.RegCount()
				// 属性必须完全落在当前段内
				if mStart >= start && mEnd <= end {
					rc := m.RegCount()
					segMembers = append(segMembers, GroupMember{
						Prop:       m,
						ByteOffset: (m.Offset - start) * 2, // 相对段起始的字节偏移
						ByteLen:    rc * 2,
					})
				}
			}
			if len(segMembers) == 0 {
				continue
			}
			groups = append(groups, RegGroup{
				ReadFC:    k.fc,
				StartAddr: k.base + start,
				Quantity:  segLen,
				Members:   segMembers,
			})
		}
	}

	// 3. 稳定排序：按功能码、起始地址，保证每次重建 plan 时顺序一致
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].ReadFC != groups[j].ReadFC {
			return groups[i].ReadFC < groups[j].ReadFC
		}
		return groups[i].StartAddr < groups[j].StartAddr
	})

	return groups
}

// FindWriteProp 在属性列表中查找指定属性名的可写属性。
// 返回属性元数据和 nil；未找到返回零值和错误。
func FindWriteProp(props []PropMeta, propName string) (PropMeta, error) {
	for _, p := range props {
		if p.Name == propName && canWrite(p.AccessMode) {
			return p, nil
		}
	}
	return PropMeta{}, fmt.Errorf("属性 %q 不可写或不存在", propName)
}

// canRead 判断属性是否可读。
func canRead(access string) bool {
	switch access {
	case "r", "rw", "R", "RW", "":
		return true
	}
	return false
}

// canWrite 判断属性是否可写。
func canWrite(access string) bool {
	switch access {
	case "w", "rw", "W", "RW":
		return true
	}
	return false
}
