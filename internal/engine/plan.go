package engine

import (
	"encoding/json"
	"fmt"

	"gateway/internal/engine/converter"
	"gateway/internal/store"
)

// 本文件定义采集计划的领域模型：将 store 层的扁平记录（Channel + DeviceModel）
// 转化为 engine 可直接执行的分层结构（ChannelPlan → DevicePlan → RegGroup）。
//
// 数据流向:
//
//	store.Channel ─┐
//	               ├─ BuildPlans ─► ChannelPlan
//	store.DeviceModel ─┘                │
//	                                    ├─ Devices[]: DevicePlan
//	                                    │     ├─ UnitID (从站地址)
//	                                    │     ├─ Converter (协议转换器)
//	                                    │     └─ Groups[]: RegGroup (寄存器分组)
//	                                    │           └─ Members[]: GroupMember (属性定位)
//	                                    └─ PollInterval (轮询间隔)

// DeviceMount 描述挂载在链路上的单个设备（从 store.Channel.Devices JSON 解析）。
type DeviceMount struct {
	Index   int    `json:"index"`   // 设备序号
	CommNo  int    `json:"commNo"`  // 从站地址 / 单元 ID
	ModelID string `json:"modelId"` // 对应 DeviceModel.ID
}

// DevicePlan 是一个设备的采集执行计划。
type DevicePlan struct {
	UnitID    byte                 // 从站地址
	ModelName string               // 设备名称（日志用）
	Protocol  string               // 协议类型（"Modbus RTU" / "Modbus TCP"）
	Conv      converter.FrameIO    // 协议转换器
	Groups    []converter.RegGroup // 寄存器分组
	Props     []converter.PropMeta // 完整属性表（写操作查找用）
}

// ChannelPlan 是一条链路的完整采集计划。
type ChannelPlan struct {
	ChannelID   int    // 链路 ID
	ChannelName string // 链路名称
	ChannelType string // 链路类型（Serial / Network / CAN）
	Config      []byte // 链路配置 JSON（透传给 connector.ParseConfig）
	Devices     []DevicePlan
	PollMs      int // 轮询间隔（毫秒），0 表示默认
}

// BuildPlans 将 store 层的链路 + 设备模型转化为引擎可执行的采集计划。
// 无法解析的设备/模型会被跳过并记录原因（返回的 error 仅用于日志）。
func BuildPlans(channels []store.Channel, models []store.DeviceModel) ([]ChannelPlan, []string) {
	modelMap := make(map[string]store.DeviceModel, len(models))
	for _, m := range models {
		modelMap[m.ID] = m
	}

	plans := make([]ChannelPlan, 0, len(channels))
	var warnings []string

	for _, ch := range channels {
		plan, warns := buildChannelPlan(ch, modelMap)
		warnings = append(warnings, warns...)
		if plan == nil {
			continue
		}
		plans = append(plans, *plan)
	}
	return plans, warnings
}

// buildChannelPlan 构建单条链路的采集计划。
func buildChannelPlan(ch store.Channel, modelMap map[string]store.DeviceModel) (*ChannelPlan, []string) {
	var warns []string

	// 解析链路挂载的设备列表。
	var mounts []DeviceMount
	if len(ch.Devices) > 0 {
		if err := json.Unmarshal(ch.Devices, &mounts); err != nil {
			return nil, []string{fmt.Sprintf("链路 %q 设备列表解析失败: %v", ch.Name, err)}
		}
	}

	plan := &ChannelPlan{
		ChannelID:   ch.ID,
		ChannelName: ch.Name,
		ChannelType: ch.Type,
		Config:      ch.Config,
	}

	// 从 Config JSON 中提取 pollInterval。
	var cfgRaw struct {
		PollInterval int `json:"pollInterval"`
	}
	_ = json.Unmarshal(ch.Config, &cfgRaw)
	plan.PollMs = cfgRaw.PollInterval

	for _, mt := range mounts {
		model, ok := modelMap[mt.ModelID]
		if !ok {
			warns = append(warns, fmt.Sprintf("链路 %q 设备 %d: 模型 %q 不存在，跳过", ch.Name, mt.Index, mt.ModelID))
			continue
		}

		dp, err := buildDevicePlan(model, mt.CommNo)
		if err != nil {
			warns = append(warns, fmt.Sprintf("链路 %q 设备 %d (%s): %v", ch.Name, mt.Index, model.Name, err))
			continue
		}
		plan.Devices = append(plan.Devices, *dp)
	}

	if len(plan.Devices) == 0 {
		return nil, warns
	}
	return plan, warns
}

// buildDevicePlan 从设备模型构建单个设备的采集计划。
func buildDevicePlan(model store.DeviceModel, commNo int) (*DevicePlan, error) {
	// 解析协议信息与最大寄存器数。
	var profile struct {
		ProtocolType     string `json:"protocolType"`
		MaxRegisterCount *int   `json:"maxRegisterCount"`
	}
	if err := json.Unmarshal(model.Profile, &profile); err != nil {
		return nil, fmt.Errorf("模型 Profile 解析失败: %w", err)
	}
	proto := profile.ProtocolType
	if proto == "" {
		return nil, fmt.Errorf("模型 %q 未定义协议类型", model.Name)
	}
	maxRegs := 0
	if profile.MaxRegisterCount != nil && *profile.MaxRegisterCount > 0 {
		maxRegs = *profile.MaxRegisterCount
	}

	// 创建协议转换器。
	conv, err := converter.New(proto)
	if err != nil {
		return nil, fmt.Errorf("模型 %q 协议 %q 不支持: %w", model.Name, proto, err)
	}

	// 解析属性列表为 PropMeta。
	var rawProps []converter.PropMeta
	if err := json.Unmarshal(model.Properties, &rawProps); err != nil {
		return nil, fmt.Errorf("模型 %q 属性列表解析失败: %w", model.Name, err)
	}

	// 补全默认值 + 向后兼容旧字段。
	for i := range rawProps {
		p := &rawProps[i]
		// 旧 JSON 的 "base" → DeltaValue
		if p.DeltaValue == 0 && p.LegacyBase != 0 {
			p.DeltaValue = p.LegacyBase
		}
		// 旧 JSON 的 "dataLength" → 推导 startBit/endBit
		if p.EndBit <= 0 && p.LegacyDataLength != nil && *p.LegacyDataLength > 0 {
			p.StartBit = 0
			p.EndBit = *p.LegacyDataLength*16 - 1
		}
		if p.EndBit < 0 {
			p.EndBit = 0 // 至少占 1 位 → 1 寄存器
		}
		if p.Coefficient == 0 {
			p.Coefficient = 1
		}
	}

	// 构建寄存器分组（受 maxRegisterCount 约束自动拆分）。
	groups := converter.BuildGroups(rawProps, maxRegs)

	return &DevicePlan{
		UnitID:    byte(commNo),
		ModelName: model.Name,
		Protocol:  proto,
		Conv:      conv,
		Groups:    groups,
		Props:     rawProps,
	}, nil
}

// ─── 写命令 ──────────────────────────────────────────────────

// WriteCommand 是一条设备写指令。
type WriteCommand struct {
	DeviceIndex int     // 设备在 ChannelPlan.Devices 中的序号
	PropName    string  // 属性名
	RawValue    float64 // 工程值（尚未逆变换，worker 侧做逆变换后编码）
}

// WriteResult 是写命令的执行结果。
type WriteResult struct {
	OK  bool
	Err string
}
