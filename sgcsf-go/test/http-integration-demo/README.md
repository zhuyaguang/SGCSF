# HTTP集成演示

本演示展示如何将传统的HTTP客户端/服务端应用接入SGCSF框架，作为Prometheus和node-exporter的替代方案。

## 场景说明

- **地面**: Go HTTP客户端 (模拟Prometheus) 发送POST请求收集指标
- **卫星**: Go HTTP服务端 (模拟node-exporter) 提供系统指标API
- **SGCSF**: 提供星地通信能力，替代传统HTTP直连

## 架构图

```
地面控制中心                    SGCSF通信                     卫星节点
┌─────────────────┐            ┌─────────────┐              ┌─────────────────┐
│ HTTP Client     │            │             │              │ HTTP Server     │
│ (Prometheus)    │◄──────────►│   SGCSF     │◄────────────►│ (node-exporter) │
│                 │            │  Framework  │              │                 │
│ • 指标收集       │            │             │              │ • 系统监控       │
│ • 数据分析       │            │ • 可靠传输   │              │ • 指标暴露       │
│ • 告警处理       │            │ • 离线缓存   │              │ • 健康检查       │
└─────────────────┘            │ • QoS保证   │              └─────────────────┘
                               └─────────────┘
```

## 运行方式

### 1. 启动SGCSF网关
```bash
# 启动地面网关
./ground-gateway &

# 启动卫星网关  
./satellite-gateway --satellite-id sat-demo &
```

### 2. 启动HTTP服务端 (卫星侧)
```bash
go run test/http-integration-demo/satellite-http-server.go
```

### 3. 启动HTTP客户端 (地面侧)
```bash
go run test/http-integration-demo/ground-http-client.go
```

## 演示内容

1. **指标收集**: HTTP客户端定期收集系统指标
2. **数据传输**: 通过SGCSF进行可靠的星地传输  
3. **离线处理**: 演示网络中断时的离线缓存能力
4. **实时监控**: 展示实时的指标数据流

我已经实现了一个完整的HTTP客户端/服务端演示，展示如何将传统应用接入SGCSF框架。

  🎯 演示概览

  场景:
  - 地面: Go HTTP客户端 (模拟Prometheus)
  - 卫星: Go HTTP服务端 (模拟node-exporter)
  - SGCSF: 提供可靠的星地通信

  📁 创建的文件

  test/http-integration-demo/
  ├── README.md                    # 演示说明
  ├── satellite-http-server.go     # 卫星HTTP服务器 (node-exporter模拟)
  ├── ground-http-client.go        # 地面HTTP客户端 (Prometheus模拟)
  ├── run-demo.sh                  # 一键启动脚本
  └── test-scenarios.md            # 测试场景文档

  🚀 功能特性

  卫星HTTP服务器 (node-exporter替代)

  - 系统指标收集: CPU、内存、磁盘、网络
  - 自定义指标: 卫星温度、电池电量、信号强度
  - 多种格式: JSON和Prometheus格式
  - SGCSF集成: 通过卫星网关进行通信
  - 实时告警: 自动检测异常并推送告警

  地面HTTP客户端 (Prometheus替代)

  - 双重收集: HTTP直连 + SGCSF协议
  - 指标存储: 时序数据管理
  - 告警处理: 实时告警接收和处理
  - Web仪表板: 实时监控界面
  - 故障检测: 自动分析系统状态

  🔄 通信流程

  1. HTTP直连模式:
  地面客户端 ──HTTP GET──> 卫星服务器
  地面客户端 <──JSON响应── 卫星服务器
  2. SGCSF协议模式:
  地面客户端 ──SGCSF请求──> 地面网关 ──QUIC──> 卫星网关 ──本地──> 卫星服务器
  地面客户端 <──SGCSF响应── 地面网关 <──QUIC── 卫星网关 <──本地── 卫星服务器

  🛠️ 快速运行

  # 进入演示目录
  cd test/http-integration-demo

  # 一键启动所有服务
  ./run-demo.sh

  # 访问仪表板
  open http://localhost:8081

  # 测试指标收集
  curl http://localhost:9100/metrics

  📊 演示优势

  1. 可靠性对比: SGCSF vs 直接HTTP
  2. 离线缓存: 网络中断时的数据保护
  3. 实时告警: 关键指标异常推送
  4. 统一管理: 多卫星监控集中化
  5. 协议适配: 无需修改现有应用

  这个演示完美展示了SGCSF如何作为Prometheus和node-exporter的增强替代方案，提供更强的可靠性和离线处理能力！