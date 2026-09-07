package converter

import "testing"

// makeProp 构造一个可读保持寄存器属性：regs 为占用寄存器数（由 endBit 推导）。
func makeProp(name string, offset, regs int) PropMeta {
	return PropMeta{
		Name: name, DataType: "int", AccessMode: "r",
		ReadFC: 3, RegisterBase: 100,
		Offset: offset, StartBit: 0, EndBit: regs*16 - 1,
		Coefficient: 1,
	}
}

func findGroup(groups []RegGroup, name string) *RegGroup {
	for i := range groups {
		for _, m := range groups[i].Members {
			if m.Prop.Name == name {
				return &groups[i]
			}
		}
	}
	return nil
}

// TestBuildGroupsMergesAdjacentProps 验证同桶相邻属性合并为一次读请求（无回归）。
func TestBuildGroupsMergesAdjacentProps(t *testing.T) {
	groups := BuildGroups([]PropMeta{makeProp("A", 0, 1), makeProp("B", 2, 1)}, 125)
	if len(groups) != 1 {
		t.Fatalf("相邻属性应合并为 1 组: %+v", groups)
	}
	g := groups[0]
	if g.StartAddr != 100 || g.Quantity != 3 || len(g.Members) != 2 {
		t.Fatalf("合并组参数错误: %+v", g)
	}
}

// TestBuildGroupsStraddlingPropertyGetsOwnGroup 验证横跨段边界的属性
// 以自身起点独立成组，不再被静默丢弃。
func TestBuildGroupsStraddlingPropertyGetsOwnGroup(t *testing.T) {
	// maxRegs=10：B（offset 0，1 寄存器）落在第一段；A（offset 8，4 寄存器，
	// 区间 [8,12)）横跨 [0,10)/[10,20) 段边界。
	props := []PropMeta{makeProp("A", 8, 4), makeProp("B", 0, 1)}
	groups := BuildGroups(props, 10)

	gA := findGroup(groups, "A")
	if gA == nil {
		t.Fatalf("跨界属性 A 不应被丢弃: %+v", groups)
	}
	if gA.StartAddr != 108 || gA.Quantity != 4 || len(gA.Members) != 1 {
		t.Fatalf("跨界属性应独立成组（起点 108、数量 4）: %+v", gA)
	}
	if m := gA.Members[0]; m.ByteOffset != 0 || m.ByteLen != 8 {
		t.Fatalf("独立组成员定位错误: %+v", m)
	}

	gB := findGroup(groups, "B")
	if gB == nil || gB.StartAddr != 100 {
		t.Fatalf("段内属性 B 应保留在原段: %+v", groups)
	}
}

// TestBuildGroupsOverWidePropertyStillEmitted 验证单个属性宽度超过 maxRegs 时
// 仍生成独立分组（请求会被设备拒绝并在通讯监控中可见，优于静默丢失）。
func TestBuildGroupsOverWidePropertyStillEmitted(t *testing.T) {
	groups := BuildGroups([]PropMeta{makeProp("W", 0, 20)}, 10)
	g := findGroup(groups, "W")
	if g == nil {
		t.Fatalf("超宽属性不应被静默丢弃: %+v", groups)
	}
	if g.StartAddr != 100 || g.Quantity != 20 {
		t.Fatalf("超宽属性独立分组参数错误: %+v", g)
	}
}
