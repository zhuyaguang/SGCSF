# SGCSF 文件传输演示

本演示展示SGCSF框架在卫星星座中的文件上传和下载功能，重点演示离线场景下的可靠文件传输。

## 场景设计

### 卫星星座配置
- **总计**: 12颗卫星
- **卫星1 (sat-01)**: 3个节点 (node-1, node-2, node-3)
- **卫星2 (sat-02)**: 3个节点 (node-1, node-2, node-3)  
- **卫星3-12**: 每颗1个节点 (node-1)

### 部署架构
- **地面**: 1个 Ground File Manager (文件管理器)
- **卫星**: 每个节点1个 Satellite File Agent (DaemonSet模式)
- **通信**: 通过SGCSF进行可靠的星地文件传输

### 离线模拟
- **离线率**: 卫星大部分时间(80%)处于离线状态
- **在线窗口**: 每颗卫星随机2-5分钟在线，然后离线8-15分钟
- **通信延迟**: 200-800ms(模拟真实星地距离)

## 架构图

```
地面控制中心                          卫星星座 (DaemonSet部署)
┌─────────────────────────┐          ┌─────────────────────────┐
│ Ground File Manager     │          │ Satellite File Agents  │
│                         │          │                         │
│ ┌─────────────────────┐ │          │ sat-01:                 │
│ │ • 文件上传管理       │ │◄────────►│ ├─ node-1 (agent)       │
│ │ • 文件下载管理       │ │          │ ├─ node-2 (agent)       │
│ │ • 传输进度监控       │ │   SGCSF  │ └─ node-3 (agent)       │
│ │ • 离线任务队列       │ │          │                         │
│ │ • Web管理界面        │ │          │ sat-02:                 │
│ └─────────────────────┘ │          │ ├─ node-1 (agent)       │
└─────────────────────────┘          │ ├─ node-2 (agent)       │
                                     │ └─ node-3 (agent)       │
                                     │                         │
                                     │ sat-03~12:              │
                                     │ └─ node-1 (agent each)  │
                                     └─────────────────────────┘
```

## 功能特性

### 🌍 地面文件管理器 (Ground File Manager)
- **文件上传**: 选择本地文件上传到指定卫星节点
- **文件下载**: 从指定卫星节点下载文件到本地
- **批量操作**: 同时向多个卫星节点发送文件
- **进度监控**: 实时显示传输进度和状态
- **离线队列**: 管理卫星离线期间的待处理任务
- **任务调度**: 智能调度文件传输任务
- **Web界面**: 直观的文件管理界面

### 🛰️ 卫星文件代理 (Satellite File Agent - DaemonSet)
- **文件接收**: 处理来自地面的文件上传请求
- **文件发送**: 响应地面的文件下载请求
- **离线缓存**: 存储离线期间收到的文件操作指令
- **断点续传**: 支持大文件的分片传输和恢复
- **存储管理**: 管理有限的卫星存储空间
- **状态上报**: 定期向地面汇报节点状态和文件清单

### 📡 传输特性
- **可靠传输**: 基于SGCSF的消息确认和重试机制
- **分片传输**: 大文件自动分片，适配网络限制
- **压缩传输**: 自动压缩文件减少传输时间
- **完整性校验**: MD5校验确保文件完整性
- **离线处理**: 离线期间任务自动排队，上线后执行

## 快速开始

### 1. 启动演示

```bash
cd test/file-transfer-demo
./run-demo.sh
```

演示系统将自动：
- 创建测试数据文件
- 启动地面文件管理器
- 启动所有卫星文件代理
- 创建Web管理界面
- 开始离线场景模拟

### 2. 访问管理界面

打开浏览器访问：
```
http://localhost:8082
```

### 3. 命令行交互

启动后可以使用交互式命令：

```bash
# 查看帮助
SGCSF> help

# 查看系统状态
SGCSF> status

# 查看卫星状态
SGCSF> satellites

# 上传文件
SGCSF> upload test-data/config.json sat-01 node-1

# 下载文件
SGCSF> download config.json sat-01 node-1

# 强制卫星上线/离线（测试用）
SGCSF> online sat-01
SGCSF> offline sat-01

# 控制离线模拟器
SGCSF> simulate start
SGCSF> simulate stop

# 退出
SGCSF> quit
```

### 4. HTTP API 使用

```bash
# 查看卫星状态
curl http://localhost:8082/api/satellites/status

# 查看传输状态
curl http://localhost:8082/api/transfers

# 上传文件
curl -X POST http://localhost:8082/api/transfer/upload \
  -d "local_path=test-data/config.json&remote_path=config.json&satellite_id=sat-01&node_id=node-1"

# 下载文件
curl -X POST http://localhost:8082/api/transfer/download \
  -d "remote_path=config.json&satellite_id=sat-01&node_id=node-1"

# 控制卫星状态
curl -X POST http://localhost:8082/api/satellites/sat-01/online
curl -X POST http://localhost:8082/api/satellites/sat-01/offline
```

## 测试场景

运行自动化测试场景：

```bash
# 运行所有测试场景
./test-scenarios.sh

# 运行单个场景
./test-scenarios.sh offline_upload
./test-scenarios.sh batch_transfer
./test-scenarios.sh network_interruption
./test-scenarios.sh bidirectional
./test-scenarios.sh multi_node
./test-scenarios.sh stress_test

# 查看系统状态
./test-scenarios.sh status

# 清理测试环境
./test-scenarios.sh cleanup
```

### 测试场景说明

1. **离线上传场景**: 向离线卫星上传文件，验证离线队列机制
2. **批量传输场景**: 同时向多个离线卫星传输不同文件
3. **网络中断恢复**: 模拟大文件传输过程中的网络中断和恢复
4. **双向传输场景**: 先上传文件，再下载验证完整性
5. **多节点分发**: 向多节点卫星的不同节点分发文件
6. **压力测试**: 并发传输多个文件测试系统性能

## Kubernetes 部署

### 1. 部署到 Kubernetes 集群

```bash
# 创建命名空间和基础配置
kubectl apply -f k8s/namespace.yaml

# 创建 RBAC
kubectl apply -f k8s/rbac.yaml

# 部署地面文件管理器
kubectl apply -f k8s/ground-deployment.yaml

# 部署卫星文件代理 (DaemonSet)
kubectl apply -f k8s/satellite-daemonset.yaml

# 部署监控配置
kubectl apply -f k8s/monitoring.yaml
```

### 2. 节点标签配置

为 Kubernetes 节点添加适当的标签：

```bash
# 标记地面站节点
kubectl label nodes <ground-node> node-type=ground-station

# 标记卫星节点
kubectl label nodes <satellite-node-1> node-type=satellite
kubectl label nodes <satellite-node-1> satellite-id=sat-01

# 标记多节点卫星
kubectl label nodes <multi-node-sat> satellite-type=multi-node
```

### 3. 验证部署

```bash
# 查看所有Pod状态
kubectl get pods -n sgcsf-file-transfer

# 查看DaemonSet状态
kubectl get daemonset -n sgcsf-file-transfer

# 查看服务状态
kubectl get services -n sgcsf-file-transfer

# 查看日志
kubectl logs -n sgcsf-file-transfer deployment/ground-file-manager
kubectl logs -n sgcsf-file-transfer daemonset/satellite-file-agent
```

## 监控和告警

### Prometheus 指标

系统暴露以下 Prometheus 指标：

- `sgcsf_file_transfers_total` - 文件传输总数
- `sgcsf_active_transfers` - 活跃传输数量
- `sgcsf_queued_transfers` - 队列中的传输数量
- `sgcsf_satellite_status` - 卫星在线状态
- `sgcsf_storage_used_bytes` - 存储使用字节数
- `sgcsf_storage_total_bytes` - 存储总字节数
- `sgcsf_bytes_transferred_total` - 传输字节总数
- `sgcsf_transfer_errors_total` - 传输错误总数
- `sgcsf_transfer_duration_seconds` - 传输持续时间

### Grafana 仪表板

导入提供的 Grafana 仪表板配置：

```bash
kubectl apply -f k8s/monitoring.yaml
```

### 告警规则

系统包含以下预定义告警：

- **SatelliteOffline**: 卫星离线超过5分钟
- **HighTransferFailureRate**: 文件传输失败率过高
- **StorageSpaceLow**: 存储空间不足(>90%)
- **LongQueuedTransfers**: 队列中传输任务过多
- **FileTransferTimeout**: 文件传输耗时过长

## 文件类型支持

### 预置测试文件
- **配置文件**: config.json (小文件, <1KB)
- **日志文件**: system.log (中等文件, ~100KB)  
- **数据文件**: sensor-data.csv (大文件, ~10MB)
- **软件包**: firmware-update.bin (超大文件, ~50MB)

### 支持的操作
- **上传**: 地面 → 卫星节点
- **下载**: 卫星节点 → 地面
- **删除**: 删除卫星节点上的文件
- **列表**: 查看卫星节点的文件清单
- **同步**: 多节点间文件同步 (仅sat-01, sat-02)

## 演示场景

### 场景1: 向离线卫星上传文件
1. 选择要上传的文件 (测试文件: test-data.txt)
2. 选择目标: sat-01/node-1 (当前离线)
3. 发起上传任务 → 任务进入离线队列
4. 等待卫星上线 → 自动执行上传
5. 验证文件上传成功

### 场景2: 从离线卫星下载文件
1. 请求下载: sat-02/node-2/satellite-log.txt
2. 卫星当前离线 → 请求进入待处理队列
3. 卫星上线后自动处理下载请求
4. 支持大文件的分片下载和断点续传

### 场景3: 批量文件操作
1. 同时向多个卫星上传配置文件
2. 大部分卫星离线 → 任务分别排队
3. 卫星陆续上线 → 依次执行任务
4. 监控整体任务进度

### 场景4: 网络中断恢复
1. 上传大文件过程中模拟网络中断
2. 连接恢复后自动从断点继续
3. 验证文件完整性和传输成功

## 故障排除

### 常见问题

1. **端口占用**
   ```bash
   # 检查端口占用
   lsof -i :8082
   
   # 停止演示
   ./run-demo.sh stop
   ```

2. **卫星一直离线**
   ```bash
   # 强制卫星上线
   curl -X POST http://localhost:8082/api/satellites/sat-01/online
   
   # 或使用命令行
   SGCSF> online sat-01
   ```

3. **传输失败**
   ```bash
   # 查看传输状态
   curl http://localhost:8082/api/transfers
   
   # 查看系统日志
   tail -f logs/file-transfer-demo.log
   ```

4. **文件校验失败**
   - 检查源文件是否完整
   - 验证网络连接稳定性
   - 重新启动传输

### 日志位置

- **应用日志**: `logs/file-transfer-demo.log`
- **传输日志**: `logs/transfers/`
- **错误日志**: `logs/errors/`

### 性能调优

1. **调整并发数**
   ```go
   maxConcurrentTransfers: 10  // 默认5
   ```

2. **调整分片大小**
   ```go
   chunkSize: 2048 * 1024  // 2MB, 默认1MB
   ```

3. **调整缓冲区大小**
   ```go
   bufferSize: 131072  // 128KB, 默认64KB
   ```

## 技术特性

### 离线消息持久化
- 使用本地队列存储离线期间的传输请求
- 支持请求重试和超时处理
- 自动在卫星上线时执行排队的任务

### 可靠传输保障
- 基于 SGCSF 的消息确认机制
- 自动重传丢失的消息分片
- 端到端的文件完整性校验

### 分片传输优化
- 自适应分片大小，适配网络条件
- 并行传输多个分片提高效率
- 支持断点续传，网络中断后自动恢复

### 存储管理
- 智能存储空间管理和清理
- 支持存储配额和使用统计
- 文件生命周期管理

## 扩展功能

### 文件同步
在多节点卫星上实现文件同步：

```bash
# 同步 sat-01 的所有节点
curl -X POST http://localhost:8082/api/sync/satellite/sat-01

# 同步特定文件到所有节点
curl -X POST http://localhost:8082/api/sync/file \
  -d "file_path=config.json&target_satellites=sat-01,sat-02"
```

### 文件版本管理
支持文件版本控制和回滚：

```bash
# 查看文件版本
curl http://localhost:8082/api/files/sat-01/node-1/config.json/versions

# 回滚到特定版本
curl -X POST http://localhost:8082/api/files/rollback \
  -d "file_path=config.json&satellite_id=sat-01&node_id=node-1&version=v1.0"
```

### 批量操作
支持批量文件操作：

```bash
# 批量上传
curl -X POST http://localhost:8082/api/batch/upload \
  -d '{"files": ["file1.txt", "file2.txt"], "targets": ["sat-01/node-1", "sat-02/node-1"]}'

# 批量下载
curl -X POST http://localhost:8082/api/batch/download \
  -d '{"files": ["log1.txt", "log2.txt"], "sources": ["sat-01/node-1", "sat-02/node-1"]}'
```

## 开发指南

### 添加新的文件类型支持

1. 在 `FileMetadata` 中添加新字段
2. 实现特定的序列化/反序列化逻辑
3. 更新 Web 界面支持新类型

### 自定义传输策略

1. 实现 `TransferStrategy` 接口
2. 在 `GroundFileManager` 中注册新策略
3. 配置策略选择逻辑

### 添加新的监控指标

1. 在代码中添加 Prometheus 指标
2. 更新 Grafana 仪表板配置
3. 添加相应的告警规则

## 贡献指南

1. Fork 本项目
2. 创建特性分支 (`git checkout -b feature/new-feature`)
3. 提交更改 (`git commit -am 'Add new feature'`)
4. 推送到分支 (`git push origin feature/new-feature`)
5. 创建 Pull Request

## 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。

## 联系方式

如有问题或建议，请通过以下方式联系：

- 提交 Issue: [GitHub Issues](https://github.com/sgcsf/sgcsf-go/issues)
- 发送邮件: support@sgcsf.org
- 参与讨论: [SGCSF Community](https://community.sgcsf.org)