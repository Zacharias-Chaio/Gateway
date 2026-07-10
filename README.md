# PowerPulse Gateway

IoT 网关配置服务 —— 单一 Go 二进制，内嵌 Web 前端与 SQLite，浏览器访问即可完成设备模型、链路通道、实时数据与日志的配置与监控。

> 当前已实现配置服务与链路运行框架：链路配置保存后会热重载到后端 engine，每条链路一个 goroutine，已具备 TCP/串口连接器抽象；协议转换、寄存器轮询采集、真实实时数据仍为后续阶段。

---

## 一、运行环境

| 项 | 要求 |
|----|------|
| 操作系统 | 嵌入式 **aarch64 / Ubuntu**（部署目标）；Windows / Linux / macOS 均可开发 |
| Go 版本 | **Go 1.23** 及以上（`go.mod` 指定 `go 1.23`） |
| 数据库 | 内嵌 SQLite（纯 Go 驱动 `glebarez/sqlite` + `modernc.org/sqlite`，**无需 CGO**） |
| 串口 | `go.bug.st/serial v1.6.2`，纯 Go，配合 `golang.org/x/sys v0.28.0` 保持 Go 1.23 兼容 |
| 浏览器 | 现代浏览器（访问 Web 配置界面） |
| 依赖 | 无外部运行时依赖，单文件即可运行 |

主要技术栈：`go-chi/chi` 路由、`gorm` ORM、`lumberjack` 日志轮转、`yaml.v3` 配置解析，前端通过 `embed.FS` 编译进二进制。

---

## 二、目录结构

```
Gateway/
├── main.go                  # 程序入口，解析命令行参数并启动 HTTP 服务
├── go.mod / go.sum
├── configs/
│   ├── app.yaml             # 应用配置（日志级别/轮转/输出路径等）
│   └── hardware.yaml        # 硬件接口映射（COM/ETH/CAN 丝印 → 设备节点）
├── data/                    # 运行时生成：SQLite 数据库（config.db）
├── logs/                    # 运行时生成：gateway.log 及轮转归档
├── internal/
│   ├── api/                 # REST API 处理器（model/channel/realtime/hardware）
│   ├── config/              # 配置加载
│   ├── engine/              # 链路运行引擎：热重载、worker、connector
│   │   ├── engine.go         # supervisor：按通道配置差量启停 worker
│   │   ├── worker.go         # 每条链路一个 goroutine，失败后每 3 秒重连
│   │   └── connector/        # Driver 接口与 TCP/Serial/CAN 连接器
│   │       ├── driver.go     # Driver/Config/NewDriver/ParseConfig
│   │       ├── tcp.go        # TCP 连接器
│   │       ├── serial.go     # 串口连接器
│   │       └── can.go        # CAN 占位实现（暂不支持 SocketCAN）
│   ├── logx/                # 统一日志：终端 + 文件 + 前端 SSE 出口
│   ├── store/               # GORM 数据模型与数据库连接
│   └── web/                 # chi 路由 + 内嵌静态前端
│       └── static/          # Web 前端（index.html / js / css，编译时 embed）
└── 方案设计说明书v1.0.md
```

> `data/` 与 `logs/` 为运行时自动创建，无需手动建目录，也不入库（见 `.gitignore`）。

---

## 三、编译

### 1. 获取源码与依赖

```bash
git clone <仓库地址> Gateway
cd Gateway
go mod download
```

### 2. 本机直接编译

```bash
go build -o gateway .
```

Windows PowerShell 下：

```powershell
go build -o gateway.exe .
```

### 3. 交叉编译到 aarch64 / Ubuntu（部署目标）

本项目为纯 Go（无 CGO），可直接静态交叉编译：

```bash
# Linux / macOS shell
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o gateway_arm64 .

# Windows PowerShell
$env:GOOS="linux"; $env:GOARCH="arm64"; $env:CGO_ENABLED="0"
go build -trimpath -ldflags="-s -w" -o gateway_arm64 .
Remove-Item Env:\GOOS, Env:\GOARCH, Env:\CGO_ENABLED
```

- `-trimpath`：去除本地路径信息；
- `-ldflags="-s -w"`：去除调试符号，缩小体积；
- 产物 `gateway_arm64` 拷贝到目标设备即可运行，无需安装 Go。

### 4. 校验产物架构（可选）

```bash
file gateway_arm64   # 期望: ELF 64-bit LSB executable, ARM aarch64
```

---

## 四、部署

### 1. 拷贝产物到目标设备

将以下文件部署到设备（示例目录 `/opt/gateway`）：

```
/opt/gateway/
├── gateway_arm64          # 可执行文件（赋予执行权限）
├── configs/
│   ├── app.yaml
│   └── hardware.yaml      # 按设备实际丝印/节点修改
```

```bash
chmod +x gateway_arm64
```

### 2. 配置文件说明

**`configs/app.yaml`** —— 应用（日志）配置：

| 字段 | 说明 |
|------|------|
| `log.level` | 日志级别 `debug / info / warn / error` |
| `log.console` | 是否输出到终端 |
| `log.file` | 日志文件路径（留空不落盘） |
| `log.maxSizeMB` | 单文件大小上限，触发轮转 |
| `log.maxBackups` | 历史文件份数 |
| `log.maxAgeDays` | 历史文件保留天数 |
| `log.compress` | 历史文件是否 gzip 压缩 |
| `log.dailyRotate` | 是否每日 00:00 轮转 |
| `log.bufferSize` | 前端 SSE 日志出口环形缓冲条数 |

**`configs/hardware.yaml`** —— 硬件接口映射，描述面板丝印与实际设备节点的对应关系：

```yaml
Serial:        # 串口
  COM1: /dev/ttyS1
Ethernet:      # 以太网
  ETH1: eth0
CAN:           # CAN 总线
  CAN1: can0
```

> 串口/CAN 链路配置时按通道类型选择丝印 key，导出的链路 JSON 会自动填充为对应 value（真实节点）。网络链路当前只配置目标 IP 与目标端口，不再选择网卡名称。

---

## 五、运行

### 1. 启动命令

```bash
./gateway_arm64 \
  -addr :8080 \
  -db data/config.db \
  -hardware configs/hardware.yaml \
  -config configs/app.yaml
```

### 2. 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-addr` | `:8080` | HTTP 监听地址 |
| `-db` | `data/config.db` | SQLite 配置数据库路径 |
| `-hardware` | `configs/hardware.yaml` | 硬件接口配置文件 |
| `-config` | `configs/app.yaml` | 应用配置文件 |

所有参数均可省略，使用上述默认值。首次启动会自动创建 `data/`、`logs/` 目录与数据库。

### 3. 访问 Web 界面

启动后终端会输出：

```
网关微服务启动 addr=:8080 url=http://localhost:8080
```

浏览器打开：

```
http://<设备IP>:8080
```

即可进入配置向导，覆盖 **设备模型 / 链路通道 / 实时数据 / 日志监控** 四大模块。当前链路通道保存后会同步到后端 engine 并按配置热重载；实时数据、业务日志接口仍为 mock 占位。

### 4. 链路运行行为

- 启动时从 SQLite 加载已保存的链路配置，并交给 engine 启动。
- 新建、更新、删除链路后，后端会拉取全量链路并执行差量热重载。
- 每条链路由一个 goroutine 独占管理连接生命周期。
- TCP/串口驱动已具备 `Open / Send / Receive / Refresh / Close` 抽象；CAN 当前为占位实现，打开时返回暂不支持。
- 链路打开失败后固定每 `3s` 重连一次。
- 当前阶段不做应用层心跳和协议轮询；若 TCP 建连后完全没有报文交互，无法可靠感知“半开连接”断线。

### 5. 优雅退出

按 `Ctrl+C`（发送 `SIGINT` / `SIGTERM`）触发优雅关闭，服务会在 10 秒内完成正在处理的请求后退出；**再次** `Ctrl+C` 可强制退出。

---

## 六、REST API 速览

前端通过以下接口与服务端交互（前缀 `/api`）：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/models` | 列出设备模型 |
| POST | `/api/models` | 新建/更新设备模型 |
| DELETE | `/api/models/{id}` | 删除设备模型 |
| GET | `/api/channels` | 列出链路通道 |
| POST | `/api/channels` | 新建/更新通道 |
| DELETE | `/api/channels/{id}` | 删除通道 |
| GET | `/api/realtime` | 实时数据快照 |
| POST | `/api/set` | 下发/设置值 |
| GET | `/api/logs` | 业务日志 |
| GET | `/api/hardware` | 硬件接口映射 |
| GET | `/api/engine/status` | 链路 engine 运行状态快照 |
| GET | `/api/syslog` | 系统日志快照 |
| GET | `/api/syslog/stream` | 系统日志 SSE 实时推送 |

---

## 七、作为系统服务（可选，systemd）

在目标 Ubuntu 设备上可注册为开机自启服务，`/etc/systemd/system/gateway.service`：

```ini
[Unit]
Description=PowerPulse Gateway Config Service
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/gateway
ExecStart=/opt/gateway/gateway_arm64 -addr :8080
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
```

启用：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now gateway
sudo systemctl status gateway
# 查看实时日志
journalctl -u gateway -f
```

---

## 八、常见问题

| 现象 | 排查 |
|------|------|
| 启动报「打开数据库失败」 | 检查 `-db` 路径所在目录是否存在写权限；通常让其自动创建 `data/` |
| 端口 8080 被占用 | 用 `-addr :其他端口` 指定，或释放占用进程 |
| 浏览器无法访问 | 确认设备防火墙放行对应端口，且用设备实际 IP 而非 `localhost` |
| 交叉编译体积偏大 | 加 `-ldflags="-s -w" -trimpath` 去除符号与路径 |
| 配置文件加载失败 | 服务会回退默认日志配置并打印警告，检查 YAML 缩进与路径 |
| 链路状态显示未连接 | 检查目标 IP/端口、串口设备节点、权限、设备是否在线；失败后 engine 每 3 秒重连 |

---

## 九、当前已知限制与后续问题

以下是基于当前代码状态整理出的主要问题/限制，后续阶段建议按优先级逐项处理：

| 优先级 | 问题 | 影响 / 建议 |
|--------|------|-------------|
| 高 | 协议转换器尚未实现 | 当前没有真正的 Modbus/IEC/DLT645 协议解析，无法按设备属性生成请求或解析响应值；建议下一阶段定义 `Converter` 接口并先实现 Modbus TCP/RTU。 |
| 高 | 无采集轮询与应用层心跳 | engine 只负责建连和保持 goroutine，不会主动发送心跳；TCP 半开连接在无报文交互时无法可靠发现。建议在 converter/采集层加入周期轮询与连续失败判定。 |
| 高 | 实时数据/业务日志仍为 mock | `/api/realtime`、`/api/set`、`/api/logs` 还未接入真实采集数据；前端展示仍是占位。 |
| 中 | CAN 连接器为占位 | `connector/can.go` 目前返回暂不支持；Linux 目标可后续基于 SocketCAN 实现。 |
| 中 | 链路运行状态只提供快照 | `/api/engine/status` 当前为 HTTP 快照，前端尚未做专门展示；可后续加入 SSE 或页面状态提示。 |
| 中 | TCP 断线检测依赖后续报文 | 当前建连成功后不读写就不会发现网线拔出/对端掉电；可结合 TCP keepalive 与应用层轮询失败计数。 |
| 中 | 写值下发未实现 | `/api/set` 仅返回 accepted，占位未真正调用 connector/converter。 |
| 低 | 测试覆盖集中在 connector/engine | 目前覆盖配置解析、TCP 基础收发、热重载；后续实现协议层后需补充帧解析、异常响应、数据类型转换、边界值测试。 |
