# SGCSF Topic配置文件管理使用指南

## 概述

SGCSF Topic配置文件管理系统提供了基于YAML配置文件的Topic管理方案，支持：

- **配置文件管理**: 所有Topic在YAML文件中定义
- **自动同步**: 配置文件变更自动重载
- **命令行工具**: 提供完整的CLI管理工具
- **访问控制**: 基于规则的权限管理
- **动态Topic**: 支持模板化的动态Topic创建
- **分组管理**: Topic逻辑分组和批量操作

## 配置文件使用

### 1. 基本Topic定义

```yaml
topics:
  - name: "file/upload/request"
    namespace: "file"
    description: "文件上传请求"
    type: "unicast"
    qos: "at_least_once"
    persistent: false
    retention_seconds: 300
    max_size_mb: 10
    compression: true
    encryption: false
    subscribers:
      - "satellite/*"
    publishers:
      - "ground-*"
```

### 2. 动态Topic模式

```yaml
dynamic_topics:
  patterns:
    - pattern: "file/download/chunk/{request_id}"
      description: "文件下载分片topic，按请求ID动态创建"
      max_instances: 1000
      ttl_seconds: 3600
      auto_cleanup: true
```

### 3. 访问控制规则

```yaml
access_control:
  rules:
    - pattern: "system/command"
      allow_publish: ["ground-control", "admin-*"]
      allow_subscribe: ["satellite/*", "ground-gateway"]
    
    - pattern: "file/*"
      allow_publish: ["ground-*", "satellite/*"]
      allow_subscribe: ["ground-*", "satellite/*"]
```

## 集成到现有系统

### 1. 修改Client初始化

```go
// 原来的方式
func NewClient(config *types.ClientConfig) *Client {
    client := &Client{
        topicManager: NewTopicManager(),
        // ...
    }
    return client
}

// 使用配置文件的方式
func NewClientWithTopicConfig(config *types.ClientConfig, topicConfigPath string) (*Client, error) {
    client := &Client{
        topicManager: NewTopicManager(),
        // ...
    }
    
    // 创建Topic配置管理器
    topicConfigManager, err := NewTopicConfigManager(topicConfigPath, client.topicManager)
    if err != nil {
        return nil, err
    }
    
    // 初始化所有配置的Topic
    if err := topicConfigManager.InitializeTopics(); err != nil {
        return nil, err
    }
    
    client.topicConfigManager = topicConfigManager
    return client, nil
}
```

### 2. 在文件传输Demo中使用

```go
// 修改 GroundFileManager 初始化
func NewGroundFileManager(clientID, serverAddr, topicConfigPath string) *GroundFileManager {
    // 使用Topic配置文件创建客户端
    client, err := core.NewClientWithTopicConfig(
        &types.ClientConfig{
            ClientID:   clientID,
            ServerAddr: serverAddr,
        },
        topicConfigPath,
    )
    if err != nil {
        log.Fatalf("Failed to create client with topic config: %v", err)
    }
    
    manager := &GroundFileManager{
        clientID:    clientID,
        sgcsfClient: client,
        // ...
    }
    
    return manager
}

// 修改 SatelliteFileAgent 初始化
func NewSatelliteFileAgent(satelliteID, nodeID, serverAddr, topicConfigPath string) *SatelliteFileAgent {
    clientID := fmt.Sprintf("%s-%s-agent", satelliteID, nodeID)
    
    client, err := core.NewClientWithTopicConfig(
        &types.ClientConfig{
            ClientID:   clientID,
            ServerAddr: serverAddr,
        },
        topicConfigPath,
    )
    if err != nil {
        log.Fatalf("Failed to create client with topic config: %v", err)
    }
    
    agent := &SatelliteFileAgent{
        clientID:    clientID,
        sgcsfClient: client,
        // ...
    }
    
    return agent
}
```

### 3. 权限检查集成

```go
// 在订阅时检查权限
func (c *Client) Subscribe(topic string, handler types.MessageHandler) (*types.Subscription, error) {
    // 检查订阅权限
    if c.topicConfigManager != nil {
        if !c.topicConfigManager.CheckSubscribePermission(c.config.ClientID, topic) {
            return nil, fmt.Errorf("subscribe permission denied for topic: %s", topic)
        }
    }
    
    return c.topicManager.Subscribe(c.config.ClientID, topic, handler)
}

// 在发布时检查权限
func (c *Client) Publish(topic string, message *types.SGCSFMessage) error {
    // 检查发布权限
    if c.topicConfigManager != nil {
        if !c.topicConfigManager.CheckPublishPermission(c.config.ClientID, topic) {
            return fmt.Errorf("publish permission denied for topic: %s", topic)
        }
    }
    
    return c.connection.SendMessage(message)
}
```

## 命令行工具使用

### 1. 编译CLI工具

```bash
cd config
go build -o topic-cli topic_cli.go
```

### 2. 基本操作

```bash
# 列出所有Topic
./topic-cli list

# 按命名空间过滤
./topic-cli list --namespace file

# 显示详细信息
./topic-cli list --details

# 显示特定Topic
./topic-cli show file/upload/request

# 添加新Topic
./topic-cli add "monitoring/cpu/usage" \
  --namespace monitoring \
  --type multicast \
  --qos at_most_once \
  --description "CPU使用率监控" \
  --subscribers "monitoring-*,dashboard-*" \
  --publishers "satellite/*"

# 删除Topic
./topic-cli remove "old/deprecated/topic" --force

# 验证配置文件
./topic-cli validate

# 输出为JSON格式
./topic-cli list --output json

# 输出为YAML格式
./topic-cli show file/upload/request --output yaml
```

### 3. 组管理

```bash
# 列出所有Topic组
./topic-cli group list

# 显示组详情
./topic-cli group show file_transfer

# 创建新组
./topic-cli group create monitoring_topics \
  --description "监控相关Topic" \
  --topics "monitoring/cpu,monitoring/memory,monitoring/disk"
```

### 4. 访问控制

```bash
# 列出访问控制规则
./topic-cli access list

# 检查权限
./topic-cli access check file/upload/request \
  --client "ground-station-1" \
  --operation publish
```

### 5. 动态Topic

```bash
# 列出动态Topic模式
./topic-cli dynamic list

# 创建动态Topic实例
./topic-cli dynamic create "file/download/chunk/req-12345" \
  --vars "request_id=req-12345,satellite_id=sat-01"
```

### 6. 统计信息

```bash
# 显示Topic统计
./topic-cli stats
```

## 实际使用场景

### 场景1：添加新的监控Topic

```bash
# 1. 添加CPU监控Topic
./topic-cli add "monitoring/satellite/cpu" \
  --namespace monitoring \
  --type broadcast \
  --qos at_most_once \
  --description "卫星CPU使用率" \
  --retention 60 \
  --max-size 1 \
  --compression true \
  --subscribers "ground-*,monitoring-*" \
  --publishers "satellite/*"

# 2. 添加内存监控Topic
./topic-cli add "monitoring/satellite/memory" \
  --namespace monitoring \
  --type broadcast \
  --qos at_most_once \
  --description "卫星内存使用率" \
  --retention 60 \
  --max-size 1 \
  --compression true \
  --subscribers "ground-*,monitoring-*" \
  --publishers "satellite/*"

# 3. 创建监控Topic组
./topic-cli group create satellite_monitoring \
  --description "卫星监控相关Topic" \
  --topics "monitoring/satellite/cpu,monitoring/satellite/memory"

# 4. 验证配置
./topic-cli validate
```

### 场景2：修改现有Topic权限

```yaml
# 直接编辑 topics.yaml 文件
access_control:
  rules:
    - pattern: "monitoring/satellite/*"
      allow_publish: ["satellite/*"]
      allow_subscribe: ["ground-*", "monitoring-*", "dashboard-*"]
      
    - pattern: "file/emergency/*"
      allow_publish: ["ground-control", "emergency-*"]
      allow_subscribe: ["satellite/*", "ground-*"]
```

### 场景3：批量管理Topic

```bash
# 列出文件传输相关Topic
./topic-cli list --namespace file --output json > file_topics.json

# 显示文件传输组的所有Topic
./topic-cli group show file_transfer

# 按模式批量检查权限
for topic in $(./topic-cli group show file_transfer | grep "  -" | cut -d' ' -f4); do
  ./topic-cli access check "$topic" --client "satellite-agent" --operation subscribe
done
```

## 配置文件热重载

系统支持配置文件的热重载，当`topics.yaml`文件发生变化时：

1. **自动检测**: 系统每30秒检查配置文件修改时间
2. **重新加载**: 自动重新加载配置文件
3. **增量更新**: 只更新变化的Topic配置
4. **错误处理**: 如果新配置有错误，保持旧配置继续运行

```go
// 手动触发重载
topicConfigManager.ReloadConfig()

// 监听配置变化
go func() {
    for {
        select {
        case <-configChangeEvent:
            if err := topicConfigManager.ReloadConfig(); err != nil {
                log.Printf("Failed to reload config: %v", err)
            } else {
                log.Println("Topic configuration reloaded successfully")
            }
        }
    }
}()
```

## 最佳实践

### 1. 命名规范

```yaml
# 推荐的Topic命名规范
namespaces:
  file: "文件操作"
  system: "系统管理"
  monitoring: "监控遥测"
  satellite: "卫星专用"
  ground: "地面专用"

# Topic命名示例
topics:
  - name: "file/upload/request"        # 文件上传请求
  - name: "system/command/reboot"      # 系统重启命令
  - name: "monitoring/telemetry/basic" # 基础遥测数据
  - name: "satellite/sat-01/private"   # 卫星私有通道
  - name: "ground/emergency/alert"     # 地面紧急告警
```

### 2. 权限设计

```yaml
# 最小权限原则
access_control:
  rules:
    # 系统命令只允许地面控制中心发布
    - pattern: "system/command/*"
      allow_publish: ["ground-control"]
      allow_subscribe: ["satellite/*"]
    
    # 文件传输允许双向通信
    - pattern: "file/*"
      allow_publish: ["ground-*", "satellite/*"]
      allow_subscribe: ["ground-*", "satellite/*"]
    
    # 监控数据只允许查看，不允许普通客户端发布
    - pattern: "monitoring/*"
      allow_publish: ["satellite/*", "monitoring-agent"]
      allow_subscribe: ["ground-*", "dashboard-*"]
```

### 3. 性能优化

```yaml
# 大数据量Topic使用压缩
- name: "file/upload/chunk"
  compression: true
  max_size_mb: 2

# 高频Topic使用低QoS
- name: "monitoring/heartbeat"
  qos: "at_most_once"
  retention_seconds: 30

# 重要控制消息使用高QoS和持久化
- name: "system/command/critical"
  qos: "exactly_once"
  persistent: true
  retention_seconds: 3600
```

### 4. 监控和维护

```bash
# 定期检查配置有效性
./topic-cli validate

# 监控Topic使用统计
./topic-cli stats

# 定期清理不用的动态Topic
./topic-cli dynamic list | grep expired

# 备份配置文件
cp topics.yaml topics.yaml.backup.$(date +%Y%m%d)
```

## 总结

基于配置文件的Topic管理方案提供了：

1. **集中管理**: 所有Topic在一个配置文件中定义
2. **版本控制**: 配置文件可以纳入Git管理
3. **权限控制**: 细粒度的访问权限控制
4. **动态创建**: 支持模板化的动态Topic
5. **工具支持**: 完整的CLI管理工具
6. **热重载**: 运行时配置变更

这种方案完美解决了你提出的问题：可以手动添加删除Topic，同时保持了系统的灵活性和可管理性！

我为你设计了一个完整的基于配置文件的Topic管理系统，这样就可以手动添加删除Topic了！

  🎯 核心特性

  1. YAML配置文件: 所有Topic在 topics.yaml 中定义
  2. 自动同步: 配置文件变更自动重载到系统
  3. 命令行工具: 提供完整的CLI管理工具
  4. 权限控制: 基于规则的发布/订阅权限管理
  5. 动态Topic: 支持模板化的运行时Topic创建
  6. 分组管理: Topic逻辑分组和批量操作

  📋 使用方式

  1. 通过配置文件管理

  # 直接编辑 config/topics.yaml
  topics:
    - name: "monitoring/new-sensor"
      namespace: "monitoring"
      description: "新增传感器数据"
      type: "multicast"
      qos: "at_least_once"
      subscribers: ["ground-*", "dashboard-*"]
      publishers: ["satellite/*"]

  2. 通过命令行工具

  # 添加新Topic
  ./topic-cli add "monitoring/temperature" \
    --namespace monitoring \
    --type broadcast \
    --qos at_most_once \
    --description "温度监控数据"

  # 删除Topic
  ./topic-cli remove "old/deprecated/topic"

  # 列出所有Topic
  ./topic-cli list --namespace file

  # 验证配置
  ./topic-cli validate

  3. 系统自动同步

  - 热重载: 修改配置文件后自动重载
  - 权限检查: 发布/订阅时自动验证权限
  - 动态创建: 支持运行时创建临时Topic

  🔧 集成到现有系统

  只需要在初始化客户端时指定配置文件：

  // 地面文件管理器
  groundClient, err := core.NewClientWithTopicConfig(config, "./config/topics.yaml")

  // 卫星文件代理
  satelliteClient, err := core.NewClientWithTopicConfig(config, "./config/topics.yaml")

  🚀 实际效果

  - 添加新Topic: 编辑配置文件或使用CLI，系统自动同步
  - 权限管理: 基于模式匹配的细粒度权限控制
  - 版本控制: 配置文件可以纳入Git管理
  - 运维友好: 提供完整的管理工具和统计信息

  这个方案完美解决了你的需求：可以手动添加删除Topic，同时保持系统的自动化和一致性！🎉
