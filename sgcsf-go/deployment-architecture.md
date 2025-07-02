# SGCSF 正确的部署架构

## 🎯 您的原始设计是对的！

基于实际卫星通信需求，正确的部署架构应该是：

```
地面控制中心                           卫星节点
┌─────────────────────────────────┐   ┌─────────────────────────────────┐
│     Ground Gateway              │   │    Satellite Gateway            │
│   (地面消息网关)                 │◄──┤   (星上消息网关)                 │
│                                 │   │                                 │
│  ┌─────────────────────────────┐│   │ ┌─────────────────────────────┐ │
│  │ • 消息路由中心               ││   │ │ • 本地消息代理               │ │
│  │ • 订阅管理                  ││   │ │ • 离线消息存储               │ │
│  │ • 卫星连接管理               ││   │ │ • 自动重试机制               │ │
│  │ • 负载均衡                  ││   │ │ • HTTP API服务               │ │
│  │ • HTTP API                  ││   │ │ • 应用集成接口               │ │
│  └─────────────────────────────┘│   │ └─────────────────────────────┘ │
└─────────────────────────────────┘   └─────────────────────────────────┘
               ▲                                      ▲
               │                                      │
     ┌─────────▼─────────┐                  ┌─────────▼─────────┐
     │   Ground Apps     │                  │  Satellite Apps   │
     │                   │                  │                   │
     │ • Mission Control │                  │ • node-exporter   │
     │ • Data Analysis   │                  │ • Sensor Apps     │
     │ • Web Dashboard   │                  │ • Camera Control  │
     │ • Alert System    │                  │ • Legacy Systems  │
     │ • Monitoring      │                  │ • Custom Apps     │
     └───────────────────┘                  └───────────────────┘
```

## 🚀 解决的核心问题

### 1. **离线数据问题**
卫星离线时产生的数据通过**Satellite Gateway**本地存储：

```go
// 卫星应用发布数据到本地网关
curl -X POST http://localhost:8080/api/publish \
  -H "Content-Type: application/json" \
  -d '{
    "topic": "/satellite/sat-01/sensors/temperature",
    "payload": {"temperature": 25.5, "timestamp": 1234567890}
  }'

// 网关响应：数据已存储，等待连接恢复
{
  "status": "accepted",
  "message_id": "msg_123",
  "queued": true  // 表示离线存储
}
```

**离线存储和重试机制：**
- 本地持久化存储 (文件系统)
- 连接恢复后自动重传
- 消息去重和顺序保证
- 存储空间限制保护

### 2. **开源应用集成问题**
对于**node-exporter**等无法修改的开源应用：

#### 方案A: HTTP API代理
```bash
# node-exporter 通过 HTTP 发送指标到卫星网关
curl -X POST http://satellite-gateway:8080/api/publish \
  -H "Content-Type: application/json" \
  -d '{
    "topic": "/satellite/sat-01/metrics",
    "payload": {
      "metrics": "node_cpu_usage 0.85\nnode_memory_usage 0.72"
    }
  }'
```

#### 方案B: Prometheus集成
```yaml
# prometheus.yml (在卫星网关上)
global:
  external_labels:
    satellite_id: 'sat-01'

scrape_configs:
  - job_name: 'node-exporter'
    static_configs:
      - targets: ['localhost:9100']
    
remote_write:
  - url: 'http://localhost:8080/api/prometheus/write'
    headers:
      X-Satellite-ID: 'sat-01'
```

### 3. **多应用统一管理**
通过**Satellite Gateway**统一管理所有应用通信：

```yaml
# satellite-gateway.yaml
applications:
  - name: "node-exporter"
    type: "http_scrape"
    endpoint: "http://localhost:9100/metrics"
    interval: "30s"
    topic: "/satellite/sat-01/metrics/node"
    
  - name: "temperature-sensor"
    type: "tcp_listener"
    port: 9001
    topic: "/satellite/sat-01/sensors/temperature"
    
  - name: "camera-controller"
    type: "command_handler"
    topic: "/satellite/sat-01/camera/commands"
    executable: "/opt/camera/control.sh"
```

## 📦 部署示例

### 地面控制中心
```bash
# 启动地面网关
./ground-gateway \
  --listen-addr :7000 \
  --http-port :8080 \
  --max-satellites 100

# 访问管理界面
curl http://ground-control:8080/api/satellites
curl http://ground-control:8080/api/status
```

### 卫星节点部署
```bash
# 启动卫星网关
./satellite-gateway \
  --satellite-id sat-01 \
  --ground-server ground-control:7000 \
  --local-http-port :8080 \
  --offline-dir /data/sgcsf-offline

# 应用通过本地API通信
# node-exporter -> HTTP API -> Satellite Gateway -> Ground
# 传感器应用 -> HTTP API -> Satellite Gateway -> Ground  
# 遗留系统 -> TCP/UDP -> Satellite Gateway -> Ground
```

## 🔄 消息流示例

### 传感器数据上报
```
1. 传感器应用 -> POST /api/publish -> Satellite Gateway
2. Satellite Gateway -> 本地存储 (如果离线) 或 -> Ground Gateway
3. Ground Gateway -> 路由到订阅的地面应用
4. 地面应用接收数据进行处理
```

### 地面指令下发
```
1. 地面应用 -> POST /api/command -> Ground Gateway
2. Ground Gateway -> 路由到目标卫星 -> Satellite Gateway  
3. Satellite Gateway -> 转发给本地应用
4. 应用执行指令并返回结果
```

## 💾 离线处理能力

**Satellite Gateway** 的离线能力：
- **本地缓存**: 存储无法发送的消息
- **智能重试**: 连接恢复后按优先级重传
- **存储管理**: 自动清理过期消息，防止存储溢出
- **压缩优化**: 消息压缩减少存储空间

## 🛠️ 开发和部署优势

1. **应用无感知**: 现有应用无需修改，通过HTTP API即可接入
2. **统一管理**: 一个卫星网关管理所有应用通信
3. **可靠性**: 离线存储确保数据不丢失
4. **扩展性**: 支持多种应用接入方式(HTTP、TCP、UDP)
5. **监控友好**: 提供Prometheus指标和健康检查

这个架构完美解决了您提到的所有问题，是卫星-地面通信的最佳实践方案！