# SGCSF 消息路由与数据传输架构详解

## 概述

SGCSF采用分层的消息路由架构，支持点对点、广播和流式数据传输。本文档详细说明消息如何从发布者路由到特定的订阅者，以及大文件如何通过QUIC流传输。

## 1. 消息路由架构

### 1.1 Topic层次结构

SGCSF使用层次化的topic命名空间：

```
file/                              # 根topic
├── upload/                        # 上传操作
│   ├── request                    # 上传请求
│   ├── start                      # 开始上传
│   ├── chunk                      # 分片数据
│   └── complete                   # 完成上传
├── download/                      # 下载操作
│   ├── request                    # 下载请求
│   ├── response                   # 下载响应
│   └── chunk/                     # 分片数据
│       └── {request_id}           # 特定请求的分片
└── management/                    # 文件管理
    ├── list                       # 文件列表
    ├── delete                     # 删除文件
    └── info                       # 文件信息
```

### 1.2 消息类型

```go
type MessageType int

const (
    MessageTypeAsync     MessageType = 1  // 异步消息
    MessageTypeSync      MessageType = 2  // 同步消息(需要响应)
    MessageTypeResponse  MessageType = 3  // 响应消息
    MessageTypeBroadcast MessageType = 4  // 广播消息
    MessageTypeStream    MessageType = 5  // 流消息
)
```

## 2. 消息路由机制

### 2.1 地面向卫星发送下载请求

**场景**: 地面控制中心向sat-01的node-1请求下载文件

```go
// 1. 地面发布下载请求消息
downloadMsg := core.NewMessage().
    Topic("file/download/request").           // 通用下载请求topic
    Type(types.MessageTypeSync).             // 同步消息，需要响应
    Source("ground-control").                // 发送方ID
    Destination("sat-01/node-1").            // 目标接收方
    Payload(map[string]interface{}{
        "request_id": "download-123",
        "file_path": "sensor-data.csv",
        "satellite_id": "sat-01",
        "node_id": "node-1",
    }).
    Timeout(30 * time.Second).
    Build()

// 2. 发送消息
response, err := groundClient.Send(downloadMsg)
```

**关键点**：
- **Topic**: `file/download/request` - 所有卫星节点都会订阅这个topic
- **Destination**: `sat-01/node-1` - 指定特定的目标节点
- **消息类型**: `MessageTypeSync` - 需要响应确认

### 2.2 服务器端消息路由

```go
// MessageRouter.RouteMessage() 处理流程
func (mr *messageRouter) RouteMessage(message *types.SGCSFMessage) error {
    // 1. 获取topic的所有订阅者
    subscribers, err := mr.client.topicManager.RouteMessage(message)
    
    // 2. 过滤订阅者：检查Destination字段
    var targetSubscribers []*types.Subscription
    for _, sub := range subscribers {
        // 如果消息有特定目标，只发给目标订阅者
        if message.Destination != "" {
            if sub.ClientID == message.Destination {
                targetSubscribers = append(targetSubscribers, sub)
            }
        } else {
            // 广播消息，发给所有订阅者
            targetSubscribers = append(targetSubscribers, sub)
        }
    }
    
    // 3. 投递消息到目标订阅者
    for _, subscription := range targetSubscribers {
        go func(sub *types.Subscription) {
            if sub.Handler != nil {
                sub.Handler.Handle(message)
            }
        }(subscription)
    }
    
    return nil
}
```

### 2.3 卫星节点订阅和处理

```go
// 每个卫星节点启动时订阅相关topic
func (sfa *SatelliteFileAgent) subscribeToTopics() {
    // 订阅文件下载请求topic
    sfa.sgcsfClient.Subscribe("file/download/request", sfa.handleDownloadRequest)
    // 订阅文件上传相关topic
    sfa.sgcsfClient.Subscribe("file/upload/start", sfa.handleUploadStart)
    sfa.sgcsfClient.Subscribe("file/upload/chunk", sfa.handleUploadChunk)
    // 其他订阅...
}

// 处理下载请求
func (sfa *SatelliteFileAgent) handleDownloadRequest(msg *types.SGCSFMessage) {
    // 1. 检查消息是否发给本节点
    if !sfa.isMessageForThisNode(msg) {
        return  // 不是发给本节点的，忽略
    }
    
    // 2. 解析请求
    var request map[string]interface{}
    json.Unmarshal(msg.Payload, &request)
    
    // 3. 处理下载请求
    sfa.processDownloadRequest(request)
}

// 检查消息目标
func (sfa *SatelliteFileAgent) isMessageForThisNode(msg *types.SGCSFMessage) bool {
    expectedDest := fmt.Sprintf("%s/%s", sfa.satelliteID, sfa.nodeID)
    return msg.Destination == expectedDest || msg.Destination == ""
}
```

## 3. 大文件QUIC流传输

### 3.1 下载文件的流传输过程

当卫星节点处理下载请求时，大文件通过QUIC流传输：

```go
// 1. 卫星节点发送下载就绪响应
func (sfa *SatelliteFileAgent) handleDownloadRequest(msg *types.SGCSFMessage) {
    // ... 验证文件存在等 ...
    
    // 发送就绪响应(普通消息)
    response := core.QuickResponse(msg, map[string]interface{}{
        "status": "ready",
        "request_id": requestID,
        "metadata": fileMetadata,
        "total_chunks": totalChunks,
    })
    sfa.sgcsfClient.Publish(response)
    
    // 2. 开始发送文件分片(通过独立的topic)
    go sfa.sendFileChunks(session)
}

// 3. 发送文件分片
func (sfa *SatelliteFileAgent) sendFileChunks(session *DownloadSession) {
    // 为这个下载请求创建专用的topic
    chunkTopic := fmt.Sprintf("file/download/chunk/%s", session.ID)
    
    // 读取文件并分片发送
    for chunkIndex := 0; chunkIndex < session.TotalChunks; chunkIndex++ {
        // 读取分片数据
        chunkData := readChunk(file, chunkIndex)
        
        // 创建分片消息
        chunk := &FileChunk{
            FileID:      session.ID,
            ChunkIndex:  chunkIndex,
            ChunkSize:   len(chunkData),
            TotalChunks: session.TotalChunks,
            Data:        chunkData,
            Checksum:    calculateChecksum(chunkData),
        }
        
        // 发送分片(异步消息)
        chunkMsg := core.NewMessage().
            Topic(chunkTopic).                    // 专用topic
            Type(types.MessageTypeAsync).         // 异步消息
            Source(sfa.clientID).
            Destination(session.GroundClientID).  // 发给地面
            Payload(chunk).
            QoS(types.QoSAtLeastOnce).           // 确保投递
            Build()
        
        sfa.sgcsfClient.Publish(chunkMsg)
    }
}
```

### 3.2 地面接收文件分片

```go
// 地面客户端接收下载响应
func (gfm *GroundFileManager) executeDownload(request *FileTransferRequest) error {
    // 1. 发送下载请求
    downloadMsg := createDownloadRequest(request)
    response, err := gfm.sgcsfClient.Send(downloadMsg)
    
    // 2. 解析响应，获取文件元数据
    var downloadResponse map[string]interface{}
    json.Unmarshal(response.Payload, &downloadResponse)
    
    // 3. 订阅分片topic
    chunkTopic := fmt.Sprintf("file/download/chunk/%s", request.ID)
    gfm.sgcsfClient.Subscribe(chunkTopic, func(msg *types.SGCSFMessage) {
        var chunk FileChunk
        json.Unmarshal(msg.Payload, &chunk)
        
        // 处理接收到的分片
        gfm.processReceivedChunk(&chunk)
    })
    
    // 4. 等待所有分片接收完成
    gfm.waitForAllChunks(request.ID)
}
```

## 4. 离线消息持久化

### 4.1 离线队列机制

```go
// 地面发送消息时检查目标卫星状态
func (gfm *GroundFileManager) UploadFile(localPath, remotePath, satelliteID, nodeID string) (*FileTransferRequest, error) {
    request := createUploadRequest(localPath, remotePath, satelliteID, nodeID)
    
    // 检查目标卫星是否在线
    if gfm.isSatelliteOnline(satelliteID) {
        // 在线：立即加入传输队列
        request.Status = "pending"
        gfm.addToTransferQueue(request)
    } else {
        // 离线：加入离线队列
        request.Status = "offline_queued"
        gfm.addToOfflineQueue(satelliteID, request)
    }
    
    return request, nil
}

// 离线队列监控器
func (gfm *GroundFileManager) offlineQueueMonitor() {
    ticker := time.NewTicker(5 * time.Second)
    
    for {
        select {
        case <-ticker.C:
            gfm.processOfflineQueue()
        }
    }
}

// 处理离线队列
func (gfm *GroundFileManager) processOfflineQueue() {
    for satelliteID, requests := range gfm.offlineQueue {
        if gfm.isSatelliteOnline(satelliteID) {
            // 卫星上线了，将离线队列中的请求移到正常队列
            for _, request := range requests {
                request.Status = "pending"
                gfm.transferQueue = append(gfm.transferQueue, request)
            }
            // 清空离线队列
            delete(gfm.offlineQueue, satelliteID)
        }
    }
}
```

### 4.2 服务器端消息缓存

```go
// SGCSF服务器为离线客户端缓存消息
type SGCSFServer struct {
    offlineMessageCache map[string][]*types.SGCSFMessage  // clientID -> messages
    mutex sync.RWMutex
}

// 路由消息时处理离线客户端
func (s *SGCSFServer) RouteMessage(message *types.SGCSFMessage) error {
    targetClient := message.Destination
    
    // 检查目标客户端是否在线
    if conn, exists := s.connections[targetClient]; exists && conn.IsConnected() {
        // 在线：直接发送
        return conn.SendMessage(message)
    } else {
        // 离线：缓存消息
        s.mutex.Lock()
        if s.offlineMessageCache[targetClient] == nil {
            s.offlineMessageCache[targetClient] = make([]*types.SGCSFMessage, 0)
        }
        s.offlineMessageCache[targetClient] = append(s.offlineMessageCache[targetClient], message)
        s.mutex.Unlock()
        
        return nil  // 消息已缓存，不算错误
    }
}

// 客户端重连时发送缓存的消息
func (s *SGCSFServer) OnClientReconnect(clientID string) {
    s.mutex.Lock()
    cachedMessages := s.offlineMessageCache[clientID]
    delete(s.offlineMessageCache, clientID)
    s.mutex.Unlock()
    
    // 发送缓存的消息
    if conn, exists := s.connections[clientID]; exists {
        for _, message := range cachedMessages {
            conn.SendMessage(message)
        }
    }
}
```

## 5. 完整的文件传输流程

### 5.1 上传流程

```
地面控制中心                  SGCSF服务器                 卫星节点(sat-01/node-1)
      │                          │                           │
      │ 1. 检查卫星状态            │                           │
      │ ─────────────────────────►│                           │
      │ 2. 卫星离线,加入离线队列     │                           │
      │                          │                           │
      │ 3. 卫星上线事件             │                           │
      │ ◄─────────────────────────│◄──────────────────────────│
      │ 4. 移到正常队列,开始传输     │                           │
      │                          │                           │
      │ 5. file/upload/start      │                           │
      │ ─────────────────────────►│ ──────────────────────────►│
      │                          │ 6. dest=sat-01/node-1     │
      │ 7. 准备就绪响应             │                           │
      │ ◄─────────────────────────│◄──────────────────────────│
      │                          │                           │
      │ 8. file/upload/chunk     │                           │
      │ ─────────────────────────►│ ──────────────────────────►│
      │ (发送所有分片)              │ (路由到目标节点)            │
      │                          │                           │
      │ 9. file/upload/complete  │                           │
      │ ─────────────────────────►│ ──────────────────────────►│
      │ 10. 上传成功确认            │                           │
      │ ◄─────────────────────────│◄──────────────────────────│
```

### 5.2 下载流程

```
地面控制中心                  SGCSF服务器                 卫星节点(sat-01/node-1)
      │                          │                           │
      │ 1. file/download/request │                           │
      │ ─────────────────────────►│ ──────────────────────────►│
      │ dest=sat-01/node-1       │                           │
      │                          │                           │
      │ 2. 下载就绪响应             │                           │
      │ ◄─────────────────────────│◄──────────────────────────│
      │                          │                           │
      │ 3. 订阅分片topic          │                           │
      │ file/download/chunk/req-123                          │
      │                          │                           │
      │ 4. 分片数据流              │                           │
      │ ◄─────────────────────────│◄──────────────────────────│
      │ (通过专用topic接收所有分片)  │ (路由分片消息)             │
      │                          │                           │
      │ 5. 文件重组完成             │                           │
```

## 6. 技术要点总结

### 6.1 消息路由策略

1. **Topic订阅**: 所有相关节点订阅通用topic（如`file/download/request`）
2. **目标过滤**: 通过`Destination`字段指定具体目标节点
3. **消息分发**: 服务器路由消息到匹配的订阅者
4. **节点验证**: 接收方验证消息是否发给自己

### 6.2 流数据传输

1. **控制消息**: 通过普通topic发送控制指令（开始、结束、错误等）
2. **数据流**: 通过专用topic发送实际数据分片
3. **QoS保证**: 使用`QoSAtLeastOnce`确保数据投递
4. **分片重组**: 接收方按序重组分片为完整文件

### 6.3 离线处理

1. **状态检测**: 实时监控卫星在线状态
2. **队列管理**: 离线时将请求加入离线队列
3. **自动恢复**: 卫星上线后自动处理排队的任务
4. **消息缓存**: 服务器为离线客户端缓存消息

这种架构设计实现了：
- **灵活路由**: 支持点对点、广播、多播等多种路由模式
- **可靠传输**: 通过QoS和重试机制确保消息投递
- **大文件支持**: 通过分片和流传输处理大文件
- **离线容错**: 通过队列和缓存机制处理网络中断