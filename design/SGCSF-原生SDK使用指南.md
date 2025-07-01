# SGCSF原生SDK使用指南

## 1. 概述

对于愿意修改源码或开发新组件的场景，SGCSF提供原生SDK，让组件直接使用发布订阅机制进行通信，无需HTTP适配层，获得最佳性能和最丰富的功能。

## 2. SGCSF原生SDK架构

```
┌─────────────────────────────────────────────────────────────┐
│                    SGCSF原生SDK架构                          │
├─────────────────────────────────────────────────────────────┤
│ 应用层 (用户组件)                                            │
│ ┌─────────────┐ ┌─────────────┐ ┌─────────────────────────┐ │
│ │New CloudCore│ │New DataAgent│ │    Custom App           │ │
│ │             │ │             │ │                         │ │
│ └─────────────┘ └─────────────┘ └─────────────────────────┘ │
│        │                │                        │         │
├────────┼────────────────┼────────────────────────┼─────────┤
│ SGCSF原生SDK层                                               │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │              SGCSF Client SDK                           │ │
│ │  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────────┐   │ │
│ │  │Publish  │ │Subscribe│ │Stream   │ │Sync/Async   │   │ │
│ │  │API      │ │API      │ │API      │ │API          │   │ │
│ │  └─────────┘ └─────────┘ └─────────┘ └─────────────┘   │ │
│ └─────────────────────────────────────────────────────────┘ │
├─────────────────────────────────────────────────────────────┤
│ SGCSF核心层                                                  │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │              SGCSF Core Engine                          │ │
│ └─────────────────────────────────────────────────────────┘ │
├─────────────────────────────────────────────────────────────┤
│ 传输层 (QUIC)                                                │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │            QUIC Transport Layer                         │ │
│ └─────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

## 3. SGCSF Client SDK设计

### 3.1 核心接口定义

```go
// SGCSF客户端接口
type SGCSFClient interface {
    // 连接管理
    Connect(config *ClientConfig) error
    Disconnect() error
    IsConnected() bool
    
    // 异步发布订阅
    Publish(topic string, message *SGCSFMessage) error
    Subscribe(topic string, handler MessageHandler) (*Subscription, error)
    Unsubscribe(subscriptionID string) error
    
    // 同步通信
    PublishSync(topic string, message *SGCSFMessage, timeout time.Duration) (*SGCSFMessage, error)
    SendResponse(originalMessage *SGCSFMessage, response *SGCSFMessage) error
    
    // 流式传输
    CreateStream(topic string, streamType StreamType) (Stream, error)
    AcceptStream(subscription *Subscription) (Stream, error)
    
    // 广播通信
    Broadcast(topic string, message *SGCSFMessage) error
    
    // 高级功能
    CreateTopic(topic string, config *TopicConfig) error
    GetTopicInfo(topic string) (*TopicInfo, error)
    ListSubscriptions() ([]*Subscription, error)
    
    // 监控和诊断
    GetClientStats() (*ClientStats, error)
    GetConnectionInfo() (*ConnectionInfo, error)
}

// 消息处理器接口
type MessageHandler interface {
    Handle(message *SGCSFMessage) error
}

// 函数式消息处理器
type MessageHandlerFunc func(message *SGCSFMessage) error

func (f MessageHandlerFunc) Handle(message *SGCSFMessage) error {
    return f(message)
}

// 流接口
type Stream interface {
    io.ReadWriteCloser
    
    // 流控制
    SetWriteDeadline(t time.Time) error
    SetReadDeadline(t time.Time) error
    
    // 流信息
    GetStreamID() string
    GetStreamType() StreamType
    GetStreamStats() (*StreamStats, error)
    
    // 流操作
    Flush() error
    CloseWrite() error
}
```

### 3.2 客户端配置

```go
// 客户端配置
type ClientConfig struct {
    // 基础配置
    ClientID      string            `json:"client_id"`       // 客户端ID
    ServerAddr    string            `json:"server_addr"`     // 服务器地址
    Credentials   *Credentials      `json:"credentials"`     // 认证信息
    
    // 连接配置
    ConnTimeout   time.Duration     `json:"conn_timeout"`    // 连接超时
    KeepAlive     time.Duration     `json:"keep_alive"`      // 心跳间隔
    MaxStreams    int               `json:"max_streams"`     // 最大流数量
    BufferSize    int               `json:"buffer_size"`     // 缓冲区大小
    
    // 重连配置
    EnableReconnect bool            `json:"enable_reconnect"` // 启用自动重连
    ReconnectDelay  time.Duration   `json:"reconnect_delay"`  // 重连延迟
    MaxReconnects   int             `json:"max_reconnects"`   // 最大重连次数
    
    // QoS配置
    DefaultQoS    QoSLevel          `json:"default_qos"`     // 默认QoS级别
    MessageTTL    time.Duration     `json:"message_ttl"`     // 消息TTL
    
    // 安全配置
    TLSConfig     *tls.Config       `json:"tls_config"`      // TLS配置
    Compression   bool              `json:"compression"`     // 启用压缩
    
    // 调试配置
    Debug         bool              `json:"debug"`           // 调试模式
    LogLevel      string            `json:"log_level"`       // 日志级别
    
    // 扩展配置
    Metadata      map[string]string `json:"metadata"`        // 扩展元数据
}

// 认证信息
type Credentials struct {
    Type     CredentialType `json:"type"`      // 认证类型
    Username string         `json:"username"`  // 用户名
    Password string         `json:"password"`  // 密码
    Token    string         `json:"token"`     // Token
    CertFile string         `json:"cert_file"` // 证书文件
    KeyFile  string         `json:"key_file"`  // 私钥文件
}

type CredentialType int
const (
    CredentialNone CredentialType = iota
    CredentialUserPass
    CredentialToken
    CredentialCert
)
```

### 3.3 消息构建器

```go
// 消息构建器
type MessageBuilder struct {
    message *SGCSFMessage
}

// 创建消息构建器
func NewMessage() *MessageBuilder {
    return &MessageBuilder{
        message: &SGCSFMessage{
            ID:        generateMessageID(),
            Timestamp: time.Now().UnixNano(),
            Headers:   make(map[string]string),
            Metadata:  make(map[string]string),
        },
    }
}

// 链式调用方法
func (b *MessageBuilder) Topic(topic string) *MessageBuilder {
    b.message.Topic = topic
    return b
}

func (b *MessageBuilder) Type(msgType MessageType) *MessageBuilder {
    b.message.Type = msgType
    return b
}

func (b *MessageBuilder) Priority(priority Priority) *MessageBuilder {
    b.message.Priority = priority
    return b
}

func (b *MessageBuilder) QoS(qos QoSLevel) *MessageBuilder {
    b.message.QoS = qos
    return b
}

func (b *MessageBuilder) TTL(ttl time.Duration) *MessageBuilder {
    b.message.TTL = ttl.Milliseconds()
    return b
}

func (b *MessageBuilder) Header(key, value string) *MessageBuilder {
    b.message.Headers[key] = value
    return b
}

func (b *MessageBuilder) Metadata(key, value string) *MessageBuilder {
    b.message.Metadata[key] = value
    return b
}

func (b *MessageBuilder) ContentType(contentType string) *MessageBuilder {
    b.message.ContentType = contentType
    return b
}

func (b *MessageBuilder) Payload(payload interface{}) *MessageBuilder {
    switch v := payload.(type) {
    case []byte:
        b.message.Payload = v
    case string:
        b.message.Payload = []byte(v)
    default:
        data, _ := json.Marshal(v)
        b.message.Payload = data
        if b.message.ContentType == "" {
            b.message.ContentType = "application/json"
        }
    }
    return b
}

func (b *MessageBuilder) Build() *SGCSFMessage {
    return b.message
}
```

## 4. 使用示例

### 4.1 基础发布订阅示例

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "time"
    
    "github.com/sgcsf/sdk/go"
)

// 传感器数据结构
type SensorData struct {
    SensorID    string  `json:"sensor_id"`
    Temperature float64 `json:"temperature"`
    Humidity    float64 `json:"humidity"`
    Timestamp   int64   `json:"timestamp"`
}

// 传感器数据发布者（星上组件）
func sensorDataPublisher() {
    // 1. 创建SGCSF客户端
    config := &sgcsf.ClientConfig{
        ClientID:   "satellite-sat5-sensor",
        ServerAddr: "sgcsf-core:7000",
        Debug:      true,
    }
    
    client, err := sgcsf.NewClient(config)
    if err != nil {
        log.Fatal("Failed to create client:", err)
    }
    defer client.Disconnect()
    
    // 2. 连接到SGCSF服务器
    err = client.Connect()
    if err != nil {
        log.Fatal("Failed to connect:", err)
    }
    
    fmt.Println("Sensor publisher connected")
    
    // 3. 定期发布传感器数据
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        // 构建传感器数据
        data := &SensorData{
            SensorID:    "temp-001",
            Temperature: 25.0 + float64(time.Now().Second()%10),
            Humidity:    60.0 + float64(time.Now().Second()%20),
            Timestamp:   time.Now().Unix(),
        }
        
        // 构建SGCSF消息
        message := sgcsf.NewMessage().
            Topic("/satellite/sat5/sensors/temperature").
            Type(sgcsf.MessageTypeAsync).
            Priority(sgcsf.PriorityNormal).
            ContentType("application/json").
            Payload(data).
            Build()
        
        // 发布消息
        err := client.Publish(message.Topic, message)
        if err != nil {
            log.Printf("Failed to publish sensor data: %v", err)
        } else {
            fmt.Printf("Published sensor data: %.1f°C, %.1f%%\n", 
                data.Temperature, data.Humidity)
        }
    }
}

// 传感器数据订阅者（地面组件）
func sensorDataSubscriber() {
    // 1. 创建SGCSF客户端
    config := &sgcsf.ClientConfig{
        ClientID:   "ground-monitoring-system",
        ServerAddr: "sgcsf-core:7000",
        Debug:      true,
    }
    
    client, err := sgcsf.NewClient(config)
    if err != nil {
        log.Fatal("Failed to create client:", err)
    }
    defer client.Disconnect()
    
    // 2. 连接到SGCSF服务器
    err = client.Connect()
    if err != nil {
        log.Fatal("Failed to connect:", err)
    }
    
    fmt.Println("Sensor subscriber connected")
    
    // 3. 订阅传感器数据
    subscription, err := client.Subscribe(
        "/satellite/+/sensors/temperature", // 通配符订阅所有卫星的温度传感器
        sgcsf.MessageHandlerFunc(func(message *sgcsf.SGCSFMessage) error {
            // 解析传感器数据
            var data SensorData
            err := json.Unmarshal(message.Payload, &data)
            if err != nil {
                return fmt.Errorf("failed to parse sensor data: %v", err)
            }
            
            // 处理传感器数据
            fmt.Printf("Received sensor data from %s: %.1f°C, %.1f%% (topic: %s)\n", 
                data.SensorID, data.Temperature, data.Humidity, message.Topic)
            
            // 检查异常值
            if data.Temperature > 35.0 {
                fmt.Printf("WARNING: High temperature detected: %.1f°C\n", data.Temperature)
                // 可以在这里触发告警
            }
            
            return nil
        }),
    )
    
    if err != nil {
        log.Fatal("Failed to subscribe:", err)
    }
    
    fmt.Printf("Subscribed to sensor data with ID: %s\n", subscription.ID)
    
    // 4. 保持订阅状态
    select {} // 阻塞等待
}
```

### 4.2 同步通信示例

```go
// 文件下载请求处理（地面组件）
func fileDownloadClient() {
    client, _ := sgcsf.NewClient(&sgcsf.ClientConfig{
        ClientID:   "ground-data-manager",
        ServerAddr: "sgcsf-core:7000",
    })
    defer client.Disconnect()
    
    client.Connect()
    
    // 构建下载请求
    request := map[string]interface{}{
        "file_id":     "sat5-log-20240301.txt",
        "destination": "/tmp/downloads/",
        "checksum":    true,
    }
    
    message := sgcsf.NewMessage().
        Topic("/satellite/sat5/file/download/request").
        Type(sgcsf.MessageTypeSync).
        Priority(sgcsf.PriorityHigh).
        Payload(request).
        Build()
    
    // 发送同步请求，等待响应
    response, err := client.PublishSync(message.Topic, message, 30*time.Second)
    if err != nil {
        log.Printf("Download request failed: %v", err)
        return
    }
    
    // 处理响应
    var result map[string]interface{}
    json.Unmarshal(response.Payload, &result)
    
    if status, ok := result["status"].(string); ok && status == "accepted" {
        fmt.Printf("Download request accepted, stream_id: %s\n", result["stream_id"])
        // 可以开始接收文件流
    } else {
        fmt.Printf("Download request rejected: %s\n", result["error"])
    }
}

// 文件下载请求处理（星上组件）
func fileDownloadServer() {
    client, _ := sgcsf.NewClient(&sgcsf.ClientConfig{
        ClientID:   "satellite-sat5-fileserver",
        ServerAddr: "sgcsf-core:7000",
    })
    defer client.Disconnect()
    
    client.Connect()
    
    // 订阅下载请求
    client.Subscribe("/satellite/sat5/file/download/request", 
        sgcsf.MessageHandlerFunc(func(message *sgcsf.SGCSFMessage) error {
            // 解析下载请求
            var request map[string]interface{}
            json.Unmarshal(message.Payload, &request)
            
            fileID := request["file_id"].(string)
            fmt.Printf("Received download request for file: %s\n", fileID)
            
            // 检查文件是否存在
            if !fileExists(fileID) {
                // 发送错误响应
                errorResp := sgcsf.NewMessage().
                    Type(sgcsf.MessageTypeResponse).
                    Payload(map[string]string{
                        "status": "error",
                        "error":  "file not found",
                    }).
                    Build()
                
                return client.SendResponse(message, errorResp)
            }
            
            // 创建文件流
            streamID := generateStreamID()
            
            // 发送成功响应
            successResp := sgcsf.NewMessage().
                Type(sgcsf.MessageTypeResponse).
                Payload(map[string]string{
                    "status":    "accepted",
                    "stream_id": streamID,
                }).
                Build()
            
            err := client.SendResponse(message, successResp)
            if err != nil {
                return err
            }
            
            // 启动文件传输（异步）
            go startFileTransfer(client, fileID, streamID, message.Source)
            
            return nil
        }),
    )
    
    fmt.Println("File download server started")
    select {} // 保持服务运行
}
```

### 4.3 流式传输示例

```go
// 文件流传输发送方
func startFileTransfer(client sgcsf.SGCSFClient, fileID, streamID, destination string) {
    // 1. 创建数据流
    stream, err := client.CreateStream(
        fmt.Sprintf("/satellite/sat5/file/stream/%s", streamID),
        sgcsf.StreamFile,
    )
    if err != nil {
        log.Printf("Failed to create stream: %v", err)
        return
    }
    defer stream.Close()
    
    // 2. 打开文件
    file, err := os.Open(getFilePath(fileID))
    if err != nil {
        log.Printf("Failed to open file: %v", err)
        return
    }
    defer file.Close()
    
    // 3. 流式传输文件
    buffer := make([]byte, 32*1024) // 32KB缓冲区
    totalBytes := int64(0)
    
    for {
        n, err := file.Read(buffer)
        if err == io.EOF {
            break
        }
        if err != nil {
            log.Printf("Failed to read file: %v", err)
            return
        }
        
        // 写入流
        _, err = stream.Write(buffer[:n])
        if err != nil {
            log.Printf("Failed to write to stream: %v", err)
            return
        }
        
        totalBytes += int64(n)
        
        // 可以在这里报告进度
        if totalBytes%1024*1024 == 0 { // 每1MB报告一次
            fmt.Printf("Transferred %d bytes\n", totalBytes)
        }
    }
    
    fmt.Printf("File transfer completed: %d bytes\n", totalBytes)
}

// 文件流传输接收方
func receiveFileStream() {
    client, _ := sgcsf.NewClient(&sgcsf.ClientConfig{
        ClientID:   "ground-file-receiver",
        ServerAddr: "sgcsf-core:7000",
    })
    defer client.Disconnect()
    
    client.Connect()
    
    // 订阅文件流
    subscription, err := client.Subscribe(
        "/satellite/+/file/stream/+", // 订阅所有卫星的文件流
        sgcsf.MessageHandlerFunc(func(message *sgcsf.SGCSFMessage) error {
            // 这是流开始消息
            if message.IsStreamStart {
                // 接受流
                stream, err := client.AcceptStream(subscription)
                if err != nil {
                    return err
                }
                
                // 启动文件接收
                go receiveFile(stream, message.StreamID)
            }
            return nil
        }),
    )
    
    if err != nil {
        log.Fatal("Failed to subscribe to file streams:", err)
    }
    
    fmt.Println("File stream receiver started")
    select {}
}

func receiveFile(stream sgcsf.Stream, streamID string) {
    defer stream.Close()
    
    // 创建输出文件
    outputFile, err := os.Create(fmt.Sprintf("/tmp/downloads/%s", streamID))
    if err != nil {
        log.Printf("Failed to create output file: %v", err)
        return
    }
    defer outputFile.Close()
    
    // 接收文件数据
    totalBytes, err := io.Copy(outputFile, stream)
    if err != nil {
        log.Printf("Failed to receive file: %v", err)
        return
    }
    
    fmt.Printf("File received successfully: %d bytes (stream: %s)\n", totalBytes, streamID)
}
```

### 4.4 广播消息示例

```go
// 系统通知广播（地面控制中心）
func systemNotificationBroadcaster() {
    client, _ := sgcsf.NewClient(&sgcsf.ClientConfig{
        ClientID:   "ground-control-center",
        ServerAddr: "sgcsf-core:7000",
    })
    defer client.Disconnect()
    
    client.Connect()
    
    // 发送系统维护通知
    notification := map[string]interface{}{
        "type":        "maintenance",
        "title":       "Scheduled Maintenance",
        "message":     "System will be under maintenance from 02:00 to 04:00 UTC",
        "start_time":  "2024-03-01T02:00:00Z",
        "end_time":    "2024-03-01T04:00:00Z",
        "severity":    "info",
    }
    
    message := sgcsf.NewMessage().
        Topic("/broadcast/system/notification").
        Type(sgcsf.MessageTypeBroadcast).
        Priority(sgcsf.PriorityHigh).
        Payload(notification).
        Build()
    
    // 广播给所有订阅者
    err := client.Broadcast(message.Topic, message)
    if err != nil {
        log.Printf("Failed to broadcast notification: %v", err)
    } else {
        fmt.Println("System notification broadcasted successfully")
    }
}

// 系统通知接收（所有相关组件）
func systemNotificationReceiver(componentName string) {
    client, _ := sgcsf.NewClient(&sgcsf.ClientConfig{
        ClientID:   componentName,
        ServerAddr: "sgcsf-core:7000",
    })
    defer client.Disconnect()
    
    client.Connect()
    
    // 订阅系统通知
    client.Subscribe("/broadcast/system/+", 
        sgcsf.MessageHandlerFunc(func(message *sgcsf.SGCSFMessage) error {
            var notification map[string]interface{}
            json.Unmarshal(message.Payload, &notification)
            
            fmt.Printf("[%s] Received system notification: %s - %s\n", 
                componentName, 
                notification["title"], 
                notification["message"])
            
            // 根据通知类型采取相应行动
            if notification["type"] == "maintenance" {
                fmt.Printf("[%s] Preparing for maintenance mode\n", componentName)
                // 准备维护模式逻辑
            }
            
            return nil
        }),
    )
    
    fmt.Printf("[%s] Subscribed to system notifications\n", componentName)
    select {}
}
```

### 4.5 高级功能示例

```go
// 动态主题管理
func topicManagementExample() {
    client, _ := sgcsf.NewClient(&sgcsf.ClientConfig{
        ClientID:   "topic-manager",
        ServerAddr: "sgcsf-core:7000",
    })
    defer client.Disconnect()
    
    client.Connect()
    
    // 创建自定义主题
    topicConfig := &sgcsf.TopicConfig{
        Name:        "/custom/high-priority-data",
        Type:        sgcsf.TopicUnicast,
        Persistent:  true,
        Retention:   24 * time.Hour,
        MaxSize:     10 * 1024 * 1024, // 10MB
        QoS:         sgcsf.QoSExactlyOnce,
        Metadata: map[string]string{
            "description": "High priority sensor data",
            "owner":       "data-analytics-team",
        },
    }
    
    err := client.CreateTopic("/custom/high-priority-data", topicConfig)
    if err != nil {
        log.Printf("Failed to create topic: %v", err)
    } else {
        fmt.Println("Custom topic created successfully")
    }
    
    // 获取主题信息
    topicInfo, err := client.GetTopicInfo("/custom/high-priority-data")
    if err != nil {
        log.Printf("Failed to get topic info: %v", err)
    } else {
        fmt.Printf("Topic info: %+v\n", topicInfo)
    }
    
    // 列出所有订阅
    subscriptions, err := client.ListSubscriptions()
    if err != nil {
        log.Printf("Failed to list subscriptions: %v", err)
    } else {
        fmt.Printf("Active subscriptions: %d\n", len(subscriptions))
        for _, sub := range subscriptions {
            fmt.Printf("  - %s: %s\n", sub.ID, sub.Topic)
        }
    }
}

// 客户端监控
func clientMonitoringExample() {
    client, _ := sgcsf.NewClient(&sgcsf.ClientConfig{
        ClientID:   "monitoring-client",
        ServerAddr: "sgcsf-core:7000",
    })
    defer client.Disconnect()
    
    client.Connect()
    
    // 定期获取客户端统计信息
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        // 获取客户端统计
        stats, err := client.GetClientStats()
        if err != nil {
            log.Printf("Failed to get client stats: %v", err)
            continue
        }
        
        fmt.Printf("Client Stats:\n")
        fmt.Printf("  Messages Sent: %d\n", stats.MessagesSent)
        fmt.Printf("  Messages Received: %d\n", stats.MessagesReceived)
        fmt.Printf("  Bytes Sent: %d\n", stats.BytesSent)
        fmt.Printf("  Bytes Received: %d\n", stats.BytesReceived)
        fmt.Printf("  Active Subscriptions: %d\n", stats.ActiveSubscriptions)
        fmt.Printf("  Active Streams: %d\n", stats.ActiveStreams)
        
        // 获取连接信息
        connInfo, err := client.GetConnectionInfo()
        if err != nil {
            log.Printf("Failed to get connection info: %v", err)
            continue
        }
        
        fmt.Printf("Connection Info:\n")
        fmt.Printf("  Status: %s\n", connInfo.Status)
        fmt.Printf("  RTT: %v\n", connInfo.RTT)
        fmt.Printf("  Bandwidth: %s\n", connInfo.Bandwidth)
        fmt.Printf("  Packet Loss: %.2f%%\n", connInfo.PacketLoss*100)
    }
}
```

## 5. SDK配置文件示例

```yaml
# sgcsf-client.yaml
sgcsf:
  client_id: "my-application"
  server_addr: "sgcsf-core:7000"
  
  # 连接配置
  connection:
    timeout: 30s
    keep_alive: 60s
    max_streams: 1000
    buffer_size: 65536
    
  # 重连配置
  reconnect:
    enabled: true
    delay: 5s
    max_attempts: 10
    backoff_factor: 2.0
    
  # QoS配置
  qos:
    default_level: at_least_once
    message_ttl: 300s
    
  # 安全配置
  security:
    tls_enabled: true
    cert_file: "/etc/sgcsf/client.crt"
    key_file: "/etc/sgcsf/client.key"
    ca_file: "/etc/sgcsf/ca.crt"
    
  # 性能配置
  performance:
    compression: true
    batch_size: 100
    flush_interval: 100ms
    
  # 日志配置
  logging:
    level: "info"
    format: "json"
    output: "stdout"
    
  # 扩展元数据
  metadata:
    environment: "production"
    version: "1.0.0"
    region: "asia-pacific"
```

## 6. 优势对比

### 原生SDK vs HTTP适配器

| 特性 | 原生SDK | HTTP适配器 |
|------|---------|------------|
| **性能** | 最优，零开销 | 较好，有转换开销 |
| **功能** | 完整功能 | 基础功能 |
| **开发工作量** | 需要修改代码 | 零修改 |
| **流式传输** | 原生支持 | 需要特殊处理 |
| **监控能力** | 丰富的指标 | 基础指标 |
| **错误处理** | 精细控制 | 标准HTTP错误 |
| **扩展性** | 高度可扩展 | 受HTTP限制 |

## 7. 最佳实践

1. **客户端ID管理**: 使用有意义的客户端ID，便于监控和调试
2. **错误处理**: 实现完善的错误处理和重试机制
3. **资源清理**: 及时关闭不需要的订阅和流
4. **配置管理**: 使用配置文件而非硬编码
5. **监控集成**: 利用SDK提供的监控功能
6. **安全配置**: 在生产环境中启用TLS和认证
7. **性能优化**: 根据场景调整缓冲区大小和批处理参数

通过SGCSF原生SDK，开发者可以充分利用框架的所有功能，构建高性能、可靠的分布式通信应用！