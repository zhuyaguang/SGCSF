# SGCSF 部署指南

## 部署架构选择

### 方案一：直接集成模式 (推荐)
卫星组件直接集成SGCSF SDK，无需独立Agent进程。

**优势:**
- 资源消耗最小
- 延迟最低
- 部署简单
- 故障点少

**卫星端部署:**
```go
// 在您的卫星应用中直接使用
client := core.SatelliteClient("sat-sensor-01", "ground-server:7000")
client.Connect(ctx)

// 发布传感器数据
client.Publish("/satellite/sensors/temperature", message)
```

### 方案二：独立Agent模式
在资源充足的卫星节点上运行独立的SGCSF Agent。

**适用场景:**
- 需要统一管理多个应用的通信
- 需要本地缓存和重试机制
- 应用无法直接修改

## 生产环境部署架构

### 地面控制中心
```yaml
# docker-compose.yml
version: '3.8'
services:
  sgcsf-server:
    image: sgcsf:latest
    ports:
      - "7000:7000"
    environment:
      - SGCSF_LISTEN_ADDR=:7000
      - SGCSF_MAX_CONNECTIONS=10000
    volumes:
      - ./config:/config
      - ./logs:/logs
    restart: unless-stopped
    
  # 可选: HTTP适配器服务
  http-adapter:
    image: sgcsf-http-adapter:latest
    ports:
      - "8080:8080"
    environment:
      - SGCSF_SERVER=sgcsf-server:7000
    depends_on:
      - sgcsf-server
```

### 卫星节点集成

#### 选项1: SDK直接集成
```go
package main

import (
    "github.com/sgcsf/sgcsf-go/pkg/core"
    "your-satellite-app/sensors"
)

func main() {
    // 创建SGCSF客户端
    client := core.SatelliteClient("sat-01", "ground-control:7000")
    defer client.Disconnect()
    
    // 启动传感器数据采集
    sensor := sensors.NewTemperatureSensor()
    
    for data := range sensor.DataStream() {
        message := core.NewMessage().
            Topic("/satellite/sat-01/sensors/temperature").
            Payload(data).
            Build()
            
        client.Publish(message.Topic, message)
    }
}
```

#### 选项2: 独立Agent进程
```bash
# 卫星上运行
./sgcsf-agent --server ground-control:7000 --id sat-01 --config agent.yaml
```

```yaml
# agent.yaml
satellite_id: "sat-01"
server_address: "ground-control:7000"
local_services:
  - name: "temperature-sensor"
    port: 9001
    topics: ["/satellite/sat-01/sensors/temperature"]
  - name: "camera-controller" 
    port: 9002
    topics: ["/satellite/sat-01/camera/commands"]
```

## 网络拓扑

### 单地面站模式
```
Ground Control Center
    ┌─────────────────┐
    │  SGCSF Server   │
    │    :7000        │
    └─────────────────┘
           │
    ┌──────┴──────┐
    │   QUIC/UDP  │
    │   Satellite │
    │   Network   │
    └──────┬──────┘
           │
    ┌─────────────────┐
    │   Satellite     │
    │   SGCSF Client  │
    └─────────────────┘
```

### 多地面站冗余模式
```
Primary Ground Station     Secondary Ground Station
┌─────────────────┐       ┌─────────────────┐
│  SGCSF Server   │◄─────►│  SGCSF Server   │
│  (Primary)      │       │  (Backup)       │
└─────────────────┘       └─────────────────┘
         │                         │
         └─────────┬─────────────────┘
                   │
            ┌─────────────────┐
            │   Satellite     │
            │   (Auto-switch) │
            └─────────────────┘
```

## 资源需求

### 地面SGCSF服务器
- **CPU**: 4 cores
- **内存**: 8GB
- **网络**: 1Gbps
- **存储**: 100GB (消息缓存和日志)

### 卫星SGCSF客户端
- **CPU**: 100-200MHz
- **内存**: 64-128MB
- **存储**: 10MB (可执行文件 + 配置)

## 部署步骤

### 1. 地面服务器部署
```bash
# 构建Docker镜像
docker build -t sgcsf:latest .

# 启动服务
docker run -d \
  --name sgcsf-server \
  -p 7000:7000 \
  -v $(pwd)/config:/config \
  sgcsf:latest

# 验证服务
curl http://localhost:8080/health
```

### 2. 卫星端部署

#### 方案A: 直接集成
1. 将SGCSF SDK集成到现有卫星应用
2. 配置地面服务器地址
3. 重新编译并部署应用

#### 方案B: 独立Agent
```bash
# 交叉编译for卫星平台
GOOS=linux GOARCH=arm64 go build -o sgcsf-agent cmd/agent/main.go

# 上传到卫星
scp sgcsf-agent satellite-host:/opt/sgcsf/
scp agent.yaml satellite-host:/opt/sgcsf/

# 在卫星上启动
ssh satellite-host "cd /opt/sgcsf && ./sgcsf-agent"
```

## 配置管理

### 地面服务器配置
```yaml
# config/server.yaml
server:
  listen_addr: ":7000"
  max_connections: 10000
  
quic:
  keep_alive: 30s
  idle_timeout: 5m
  max_packet_size: 1350

logging:
  level: "info"
  file: "/logs/sgcsf.log"
```

### 卫星客户端配置
```yaml
# config/satellite.yaml
satellite:
  id: "sat-01"
  server_addr: "ground-control:7000"
  
connection:
  retry_interval: 10s
  max_retries: 3
  heartbeat: 30s

topics:
  - "/satellite/sat-01/sensors/*"
  - "/satellite/sat-01/telemetry/*"
```

## 监控和维护

### 健康检查
```bash
# 地面服务器状态
curl http://ground-server:8080/health

# 卫星连接状态  
curl http://ground-server:8080/api/connections
```

### 日志收集
```bash
# 实时日志监控
docker logs -f sgcsf-server

# 卫星日志收集
ssh satellite "journalctl -u sgcsf-agent -f"
```

## 安全配置

### TLS证书
```bash
# 生成证书
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 365

# 配置服务器
export SGCSF_TLS_CERT=/certs/server.crt
export SGCSF_TLS_KEY=/certs/server.key
```

### 访问控制
```yaml
# 卫星白名单
satellites:
  - id: "sat-01" 
    fingerprint: "sha256:abc123..."
  - id: "sat-02"
    fingerprint: "sha256:def456..."
```