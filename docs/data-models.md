# 数据模型文档

本文档描述网关的两类核心配置数据——**设备模型**与**链路通道**，涵盖前端表单结构、JSON 字段、数据库存储以及引擎内部的协议映射，便于开发与对接。

---

## 一、设备模型（DeviceModel）

设备模型是设备的"模板"，定义设备的档案信息与一组属性的协议映射。一个模型可被多条链路挂载复用。

### 1.1 数据库存储

存储于 SQLite `data/config.db`，表 `device_models`，GORM 模型 `store.DeviceModel`：

| 字段 | 类型 | GORM 标签 | 说明 |
|------|------|-----------|------|
| `ID` | `string` | `primaryKey` | 模型 ID（UUID），即 `profile.profileId` |
| `ProfileIndex` | `int` | — | 档案索引，从 0 自增，用于排序 |
| `Name` | `string` | — | 模型名称 |
| `Profile` | `datatypes.JSON` | — | 档案信息（设备/协议元数据），见 §1.2 |
| `Properties` | `datatypes.JSON` | — | 属性数组，见 §1.3 |
| `CreatedAt` | `time.Time` | `json:"-"` | 创建时间（不返回前端） |
| `UpdatedAt` | `time.Time` | `json:"-"` | 更新时间（不返回前端） |

> `Profile` 与 `Properties` 整体以 JSON Blob 存储，结构灵活，无需改表结构即可扩展字段。

### 1.2 Profile（档案信息）

前端 `emptyProfile()` 定义的完整字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `profileIndex` | `int` | 档案索引（从 0 自增） |
| `profileId` | `string` | 模型 UUID（与 `DeviceModel.ID` 一致） |
| `name` | `string` | 模型名称 |
| `manufacturer` | `string` | 厂商 |
| `description` | `string` | 描述 |
| `deviceType` | `string` | 设备类型 |
| `deviceModel` | `string` | 设备型号 |
| `ratedPower` | `number\|null` | 额定功率 |
| `interfaceType` | `string` | 接口类型：`Serial` / `Network` / `CAN` |
| `protocolType` | `string` | 协议类型（如 Modbus RTU / Modbus TCP） |
| `protocolVersion` | `string` | 协议版本 |
| `maxRegisterCount` | `int` | 单次读取最大寄存器数（默认 125，即 Modbus 上限） |

### 1.3 Properties（属性数组）

每个属性描述一个采集/控制量的 Modbus 映射。默认含一个虚拟属性 `在线状态`。

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | `string` | 属性 ID（前端唯一，如 `online`） |
| `name` | `string` | 属性名称（实时数据展示用，也是缓存键） |
| `description` | `string` | 描述 |
| `dataType` | `string` | 数据类型：`bool` / `int` / `float` / `string` |
| `unit` | `string` | 单位 |
| `accessMode` | `string` | 访问模式：`r`（只读）/ `w`（只写）/ `rw`（读写） |
| `startBit` | `int` | 起始位（bit 0 = 最低位），定义数据长度的位区间起点 |
| `endBit` | `int` | 终止位（含两端），寄存器数 = `ceil((endBit + 1) / 16)` |
| `deltaValue` | `number` | 偏移量（可正负），`engineering = raw × coef + deltaValue` |
| `coefficient` | `number` | 系数（`engineering = raw × coef + deltaValue`） |
| `readFunctionCode` | `int\|null` | 读功能码：3(保持寄存器)/4(输入寄存器)/1(线圈)/2(离散量)；null=不读取 |
| `writeFunctionCode` | `int\|null` | 写功能码：5(写单线圈)/6(写单寄存器)/15(写多线圈)/16(写多寄存器) |
| `registerBase` | `int\|null` | 寄存器基址（分组依据） |
| `registerOffset` | `int\|null` | 相对基址的偏移（实际地址 = `registerBase + registerOffset`） |
| `byteOrder` | `string` | 字节序（如 `ABCD` / `DCBA` / `BADC` / `CDAB`） |

#### 寄存器寻址与分组

```
实际地址 = registerBase + registerOffset
```

引擎按 `(readFunctionCode, registerBase)` 分组，同一组内的属性合并为一次读请求，请求的寄存器数量为组内最远地址覆盖范围。

#### 数据长度与位提取

`startBit` / `endBit` 定义了属性在寄存器区间的**位级定位**（bit 0 = 最低位）：

- 寄存器数 = `ceil((endBit + 1) / 16)`（每寄存器 16 bit）
- 约束：`endBit - startBit < 寄存器数 × 16`
- `string` 类型不走位提取，按字节级处理

示例：

| 场景 | startBit | endBit | 寄存器数 | 说明 |
|------|----------|--------|---------|------|
| 单 bit 标志 | 3 | 3 | 1 | 提取第 3 位 |
| 16 位整数 | 0 | 15 | 1 | 全部 16 位 |
| 32 位浮点 | 0 | 31 | 2 | 全部 32 位（2 寄存器） |
| 位段提取 | 4 | 7 | 1 | 提取 bit4~bit7 |

#### 虚拟属性：在线状态

模型创建时默认带一个特殊属性：

```json
{
  "id": "online", "name": "在线状态", "description": "0-离线 1-在线",
  "dataType": "bool", "accessMode": "r",
  "startBit": 0, "endBit": 0,
  "readFunctionCode": null, "writeFunctionCode": null,
  "registerBase": null, "registerOffset": null
}
```

- `readFunctionCode=null` → 不参与 Modbus 读取，`BuildGroups` 自动跳过。
- 由 worker 根据通信结果写入：轮询成功=1，失败/断线=0。
- 前端实时值直接显示 `0` / `1`。

### 1.4 REST 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/models` | 列出全部模型（按 `profile_index asc`） |
| `POST` | `/api/models` | 新建/更新模型（upsert），成功后触发**引擎热重载** |
| `DELETE` | `/api/models/{id}` | 删除模型，成功后触发**引擎热重载** |

---

## 二、链路通道（Channel）

链路通道描述一条物理通信链路及其挂载的设备。

### 2.1 数据库存储

存储于 SQLite `data/config.db`，表 `channels`，GORM 模型 `store.Channel`：

| 字段 | 类型 | GORM 标签 | 说明 |
|------|------|-----------|------|
| `ID` | `int` | `primaryKey` | 通道 ID（自增） |
| `Name` | `string` | — | 链路名称 |
| `Type` | `string` | — | 链路类型：`Serial` / `Network` / `CAN` |
| `Config` | `datatypes.JSON` | — | 通信参数（见 §2.2） |
| `Devices` | `datatypes.JSON` | — | 挂载设备列表（见 §2.3） |
| `CreatedAt` | `time.Time` | `json:"-"` | 创建时间 |
| `UpdatedAt` | `time.Time` | `json:"-"` | 更新时间 |

### 2.2 Config（通信参数）

通用字段（所有链路类型）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `frameInterval` | `int\|null` | 帧间隔（毫秒），半双工总线发送后等待响应 |
| `reconnectRetries` | `int\|null` | 连接失败重试次数 |
| `resendRetries` | `int\|null` | 单帧重发次数 |
| `pollInterval` | `int\|null` | 轮询间隔（毫秒），0 表示默认 1s |

按链路类型的附加字段：

**Serial（串口）**

| 字段 | 类型 | 说明 |
|------|------|------|
| `serialName` | `string` | 串口节点（如 `/dev/ttyS1`），从 `hardware.yaml` 映射 |
| `baudRate` | `int` | 波特率 |
| `dataBits` | `int` | 数据位（7/8） |
| `parity` | `string` | 校验：`None` / `Even` / `Odd` |
| `stopBits` | `string` | 停止位：`1` / `1.5` / `2` |

**Network（网络/TCP）**

| 字段 | 类型 | 说明 |
|------|------|------|
| `deviceIp` | `string` | 设备 IP |
| `devicePort` | `int` | 设备端口（如 502） |

**CAN**

| 字段 | 类型 | 说明 |
|------|------|------|
| `canName` | `string` | CAN 节点（如 `can0`） |
| `canBaud` | `int` | CAN 波特率 |

### 2.3 Devices（挂载设备列表）

数组，每项描述一个挂载在链路上的从站设备：

| 字段 | 类型 | 说明 |
|------|------|------|
| `index` | `int` | 在链路内的序号（从 0 起） |
| `commNo` | `int` | 通信地址 / 从站号（Modbus Unit ID） |
| `modelId` | `string` | 引用的设备模型 ID |
| `modelName` | `string` | 模型名称（冗余，仅展示用） |

### 2.4 硬件接口映射

`configs/hardware.yaml` 定义面板丝印标签与实际设备节点的映射：

```yaml
Serial:        # 串口
  COM1: /dev/ttyS1
  COM2: /dev/ttyS2
Ethernet:      # 网口（网关自身多网卡）
  ETH1: eth0
CAN:           # CAN 口
  CAN1: can0
```

前端配置时选择丝印标签（如 `COM1`），保存时由 `buildChannelConfig` 自动替换为真实节点（如 `/dev/ttyS1`）。通过 `GET /api/hardware` 接口读取。

### 2.5 REST 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/channels` | 列出全部链路（按 `id asc`） |
| `POST` | `/api/channels` | 新建/更新链路（含冲突检测），成功后触发**引擎热重载** |
| `DELETE` | `/api/channels/{id}` | 删除链路，成功后触发**引擎热重载** |
| `GET` | `/api/hardware` | 读取 `hardware.yaml` 硬件接口映射 |

#### 链路冲突检测

保存链路时，按链路类型计算资源唯一键：
- Serial → `serialName`
- CAN → `canName`
- Network → `deviceIp:devicePort`

若与其他链路冲突，返回 `409 Conflict`。

---

## 三、引擎内部映射（engine/converter）

设备模型的属性 JSON 在引擎层解析为 `PropMeta`，用于构建采集分组：

```go
type PropMeta struct {
    Name         string  // 属性名
    PropID       string  // 属性 ID
    DataType     string  // bool / int / float / string
    StartBit     int     // 起始位（bit 0 = 最低位）
    EndBit       int     // 终止位（含），RegCount() = ceil((endBit+1)/16)
    Offset       int     // 相对基址偏移
    RegisterBase int     // 寄存器基址
    ReadFC       int     // 读功能码
    WriteFC      int     // 写功能码
    Coefficient  float64 // 系数
    DeltaValue   float64 // 偏移量（可正负）
    ByteOrder    string  // 字节序
    AccessMode   string  // r / w / rw
}
```

JSON 字段名与前端属性定义的映射：

| PropMeta 字段 | JSON 字段（前端/存储） |
|---------------|------------------------|
| `StartBit` | `startBit` |
| `EndBit` | `endBit` |
| `Offset` | `registerOffset` |
| `RegisterBase` | `registerBase` |
| `ReadFC` | `readFunctionCode` |
| `WriteFC` | `writeFunctionCode` |
| `DeltaValue` | `deltaValue` |

### 采集分组规则

`BuildGroups` 将属性按 `(ReadFC, RegisterBase)` 分桶：
- 跳过 `accessMode` 不含 `r` 的属性
- 跳过 `ReadFC <= 0` 的属性（如虚拟属性"在线状态"）
- 同组 `Quantity = max(offset + regCount)`，regCount 由 `ceil((endBit+1)/16)` 推导
- 一次读请求获取组内所有属性，响应按 `offset × 2` 切片解析

### 工程量变换

```
读取：engineering = raw × coefficient + deltaValue
写入：raw = (engineering - deltaValue) / coefficient
```

> 位提取在工程量变换**之前**执行：先从寄存器字节中按 `[startBit, endBit]` 提取位段，再做系数与偏移量变换。

---

## 四、热重载机制

设备模型与链路通道的增删改均会触发**引擎热重载**（`reloadEngine`）：

1. 拉取全量 `channels` + `models`
2. `BuildPlans(channels, models)` 构建采集计划
3. `Engine.Apply(plans, models)` 差量应用

差量判断基于**配置指纹**（`fingerprint = sha1(type + config)`）：
- 指纹未变的链路保持运行，不受影响
- 指纹变化或新增的链路重启
- 被删除的链路停止

> 设备模型的热重载在近期补齐——此前模型变更后需重启或改链路才生效，现已修复。
