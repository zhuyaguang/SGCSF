# HTTP集成演示测试场景

## 测试场景说明

本演示包含多个测试场景，展示SGCSF作为Prometheus和node-exporter替代方案的能力。

## 1. 基础HTTP通信测试

### 1.1 直接HTTP指标收集
```bash
# 测试卫星HTTP服务器的基本功能
curl http://localhost:9100/health
curl http://localhost:9100/info
curl http://localhost:9100/metrics
curl http://localhost:9100/metrics/prometheus
```

### 1.2 地面客户端仪表板
```bash
# 访问Web仪表板
open http://localhost:8081

# API测试
curl http://localhost:8081/api/metrics
curl http://localhost:8081/api/satellites
curl http://localhost:8081/api/realtime
```

## 2. SGCSF集成通信测试

### 2.1 SGCSF消息传递
演示将展示：
- 地面客户端通过SGCSF发送指标收集请求
- 卫星服务器通过SGCSF响应指标数据
- 双向通信的可靠性保证

### 2.2 实时告警推送
当卫星指标超出阈值时：
- CPU使用率 > 80%
- 内存使用率 > 85%  
- 卫星温度 > 30°C
- 告警会通过SGCSF自动推送到地面

## 3. 离线缓存测试

### 3.1 模拟网络中断
```bash
# 停止地面网关模拟网络中断
pkill -f ground-gateway

# 观察卫星网关的离线存储行为
tail -f logs/satellite-gateway.log
```

### 3.2 网络恢复测试
```bash
# 重启地面网关
go run ../../cmd/ground-gateway/main.go &

# 观察离线消息的自动重传
tail -f logs/ground-gateway.log
```

## 4. 性能对比测试

### 4.1 HTTP vs SGCSF延迟对比
- 直接HTTP: ~10-50ms
- SGCSF协议: ~50-200ms (含加密和路由)
- SGCSF优势: 可靠性、离线缓存、QoS保证

### 4.2 吞吐量测试
```bash
# 高频请求测试
for i in {1..100}; do
  curl -s http://localhost:9100/metrics > /dev/null &
done
wait

# 观察系统负载
curl http://localhost:9100/metrics | grep cpu_usage
```

## 5. 容错能力测试

### 5.1 服务重启测试
```bash
# 重启卫星HTTP服务器
pkill -f satellite-http-server
sleep 5
go run satellite-http-server.go &

# 观察地面客户端的自动重连
tail -f logs/ground-client.log
```

### 5.2 数据完整性测试
- 验证重启后数据收集是否正常
- 检查离线期间的数据是否正确恢复
- 确认告警机制是否正常工作

## 6. 扩展功能演示

### 6.1 多卫星支持
可以通过修改配置支持多个卫星：
```go
satellites: map[string]string{
    "sat-demo": "http://localhost:9100",
    "sat-001":  "http://localhost:9101", 
    "sat-002":  "http://localhost:9102",
}
```

### 6.2 自定义指标
卫星服务器支持自定义指标：
- 卫星温度
- 太阳能板电压
- 电池电量
- 信号强度
- 轨道高度
- 通信延迟

## 测试数据示例

### HTTP指标响应示例
```json
{
  "timestamp": 1640995200,
  "satellite_id": "sat-demo",
  "node_info": {
    "hostname": "sat-node-sat-demo",
    "os": "linux",
    "architecture": "amd64",
    "uptime": 3600
  },
  "cpu_metrics": {
    "usage_percent": 25.5,
    "core_count": 4
  },
  "memory_metrics": {
    "usage_percent": 68.2,
    "total_bytes": 8589934592
  },
  "custom_metrics": {
    "satellite_temperature_celsius": 28.5,
    "battery_level_percent": 95.2,
    "signal_strength_dbm": -75.5
  }
}
```

### SGCSF告警示例
```json
{
  "timestamp": 1640995200,
  "satellite_id": "sat-demo",
  "alerts": [
    {
      "type": "high_temperature",
      "value": 32.1,
      "threshold": 30.0,
      "severity": "warning"
    }
  ]
}
```

## 监控指标

### 关键性能指标 (KPI)
- **连接可用性**: >99.9%
- **消息延迟**: <500ms (P99)
- **数据完整性**: 100%
- **告警响应时间**: <30s
- **离线恢复时间**: <60s

### 系统资源使用
- **CPU使用率**: <20%
- **内存使用**: <512MB
- **网络带宽**: <10Mbps
- **磁盘使用**: <1GB

## 故障排除

### 常见问题
1. **端口占用**: 确保8080, 8081, 9100端口未被占用
2. **权限问题**: 确保脚本有执行权限
3. **依赖缺失**: 确保已编译SGCSF网关组件
4. **网络问题**: 检查防火墙设置

### 调试命令
```bash
# 检查进程状态
ps aux | grep -E "(ground-gateway|satellite-gateway|http-server|http-client)"

# 检查端口监听
netstat -tlnp | grep -E "(7000|8080|8081|9100)"

# 查看实时日志
tail -f logs/*.log

# 测试网络连接
curl -v http://localhost:9100/health
```