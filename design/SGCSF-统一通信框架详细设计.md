# SGCSF星地统一通信框架详细设计方案

## 1. 核心设计理念

SGCSF不是简单的通信优化，而是一个**统一通信框架**，所有星地组件都必须通过这个框架进行通信。核心设计理念：

1. **统一接入**: 所有组件都通过SGCSF通信，替代现有的HTTP/WebSocket等协议
2. **发布订阅**: 基于主题(Topic)的消息发布订阅机制
3. **透明集成**: 现有组件无需大量修改即可接入
4. **流式传输**: 支持大数据流的高效传输
5. **同步异步**: 统一支持同步和异步消息模式

## 2. 整体架构设计

```
┌─────────────────────────────────────────────────────────────┐
│                SGCSF统一通信框架架构                          │
├─────────────────────────────────────────────────────────────┤
│ 应用层 (现有组件)                                            │
│ ┌─────────────┐ ┌─────────────┐ ┌─────────────────────────┐ │
│ │  CloudCore  │ │  DataAgent  │ │    Prometheus           │ │
│ │  (HTTP)     │ │ (WebSocket) │ │    (HTTP)               │ │
│ └─────────────┘ └─────────────┘ └─────────────────────────┘ │
│        │                │                        │         │
├────────┼────────────────┼────────────────────────┼─────────┤
│ 协议适配层 (Protocol Adapter Layer)                          │
│ ┌──────▼──────┐ ┌──────▼──────┐ ┌──────▼──────────────────┐ │
│ │HTTP Adapter │ │WS Adapter   │ │  Custom Adapter         │ │
│ │             │ │             │ │                         │ │
│ └─────────────┘ └─────────────┘ └─────────────────────────┘ │
│        │                │                        │         │
├────────┼────────────────┼────────────────────────┼─────────┤
│ SGCSF核心层 (SGCSF Core)                                     │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │              统一消息总线 (Message Bus)                  │ │
│ │  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────────┐   │ │
│ │  │Topic    │ │Message  │ │Stream   │ │Sync/Async   │   │ │
│ │  │Manager  │ │Router   │ │Manager  │ │Handler      │   │ │
│ │  └─────────┘ └─────────┘ └─────────┘ └─────────────┘   │ │
│ └─────────────────────────────────────────────────────────┘ │
├─────────────────────────────────────────────────────────────┤
│ 传输层 (QUIC Transport)                                      │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │            星地QUIC连接 (多路复用)                       │ │
│ │  地面站 ◄──────────────────────────► 卫星节点           │ │
│ └─────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

## 3. 协议适配层设计

### 3.1 HTTP适配器设计

现有组件使用HTTP协议，需要透明转换为SGCSF消息。

```go
// HTTP适配器
type HTTPAdapter struct {
    sgcsfClient    *SGCSFClient     // SGCSF客户端
    httpServer     *http.Server     // HTTP服务器
    routeMapping   map[string]Topic // 路由映射
    syncTimeout    time.Duration    // 同步超时时间
}

// HTTP路由到SGCSF主题的映射
type RouteMapping struct {
    HTTPMethod  string `json:"http_method"`  // GET/POST/PUT/DELETE
    HTTPPath    string `json:"http_path"`    // /api/v1/pods
    SGCSFTopic  string `json:"sgcsf_topic"`  // /k8s/pods/operations
    MessageType string `json:"message_type"` // sync/async
}

// 默认路由映射配置
var DefaultRouteMappings = []RouteMapping{
    // Kubernetes API映射
    {HTTPMethod: "GET", HTTPPath: "/api/v1/pods", SGCSFTopic: "/k8s/pods/query", MessageType: "sync"},
    {HTTPMethod: "POST", HTTPPath: "/api/v1/pods", SGCSFTopic: "/k8s/pods/create", MessageType: "sync"},
    {HTTPMethod: "PUT", HTTPPath: "/api/v1/pods/*", SGCSFTopic: "/k8s/pods/update", MessageType: "sync"},
    {HTTPMethod: "DELETE", HTTPPath: "/api/v1/pods/*", SGCSFTopic: "/k8s/pods/delete", MessageType: "sync"},
    
    // Prometheus指标映射
    {HTTPMethod: "GET", HTTPPath: "/metrics", SGCSFTopic: "/monitoring/metrics/collect", MessageType: "sync"},
    
    // 数据传输映射
    {HTTPMethod: "POST", HTTPPath: "/upload/*", SGCSFTopic: "/data/upload/start", MessageType: "async"},
    {HTTPMethod: "GET", HTTPPath: "/download/*", SGCSFTopic: "/data/download/start", MessageType: "async"},
}

// HTTP请求转换为SGCSF消息
func (h *HTTPAdapter) ConvertHTTPToSGCSF(req *http.Request) (*SGCSFMessage, error) {
    // 1. 根据路径和方法查找主题映射
    topic, msgType := h.findTopicMapping(req.Method, req.URL.Path)
    
    // 2. 构建SGCSF消息
    message := &SGCSFMessage{
        ID:          generateMessageID(),
        Topic:       topic,
        Type:        msgType,
        Source:      h.getClientID(req),
        Timestamp:   time.Now().UnixNano(),
        Headers:     h.convertHTTPHeaders(req.Header),
        ContentType: req.Header.Get("Content-Type"),
    }
    
    // 3. 读取HTTP Body
    if req.ContentLength > 0 {
        body, err := ioutil.ReadAll(req.Body)
        if err != nil {
            return nil, err
        }
        message.Payload = body
    }
    
    // 4. 提取URL参数
    message.Parameters = h.extractURLParameters(req.URL)
    
    return message, nil
}

// SGCSF响应转换为HTTP响应
func (h *HTTPAdapter) ConvertSGCSFToHTTP(message *SGCSFMessage, w http.ResponseWriter) error {
    // 1. 设置HTTP状态码
    statusCode := h.extractStatusCode(message.Headers)
    w.WriteHeader(statusCode)
    
    // 2. 设置响应头
    for key, value := range message.Headers {
        w.Header().Set(key, value)
    }
    
    // 3. 写入响应体
    if len(message.Payload) > 0 {
        w.Write(message.Payload)
    }
    
    return nil
}

// HTTP服务器处理逻辑
func (h *HTTPAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // 1. 转换HTTP请求为SGCSF消息
    sgcsfMsg, err := h.ConvertHTTPToSGCSF(r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    
    // 2. 根据消息类型处理
    switch sgcsfMsg.Type {
    case "sync":
        // 同步消息，等待响应
        resp, err := h.sgcsfClient.PublishSync(sgcsfMsg.Topic, sgcsfMsg, h.syncTimeout)
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        h.ConvertSGCSFToHTTP(resp, w)
        
    case "async":
        // 异步消息，立即返回
        err := h.sgcsfClient.Publish(sgcsfMsg.Topic, sgcsfMsg)
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        w.WriteHeader(http.StatusAccepted)
        w.Write([]byte(`{"status":"accepted"}`))
    }
}
```

### 3.2 WebSocket适配器设计

```go
// WebSocket适配器
type WebSocketAdapter struct {
    sgcsfClient    *SGCSFClient               // SGCSF客户端
    connections    map[string]*WSConnection   // WebSocket连接池
    topicMapping   map[string]string          // WebSocket路径到主题映射
    mutex          sync.RWMutex
}

type WSConnection struct {
    conn           *websocket.Conn    // WebSocket连接
    clientID       string             // 客户端ID
    subscribedTopics []string         // 订阅的主题
    sendChan       chan *SGCSFMessage // 发送通道
    closeChan      chan struct{}      // 关闭通道
}

// WebSocket消息类型
type WebSocketMessage struct {
    Type    string      `json:"type"`    // subscribe/unsubscribe/publish/message
    Topic   string      `json:"topic"`   // 主题
    ID      string      `json:"id"`      // 消息ID
    Payload interface{} `json:"payload"` // 消息内容
}

// 处理WebSocket连接
func (w *WebSocketAdapter) HandleWebSocket(conn *websocket.Conn, clientID string) {
    wsConn := &WSConnection{
        conn:             conn,
        clientID:         clientID,
        subscribedTopics: make([]string, 0),
        sendChan:         make(chan *SGCSFMessage, 100),
        closeChan:        make(chan struct{}),
    }
    
    // 注册连接
    w.mutex.Lock()
    w.connections[clientID] = wsConn
    w.mutex.Unlock()
    
    // 启动消息处理协程
    go w.handleSend(wsConn)
    go w.handleReceive(wsConn)
    
    // 等待连接关闭
    <-wsConn.closeChan
    
    // 清理连接
    w.cleanup(wsConn)
}

// 处理WebSocket接收消息
func (w *WebSocketAdapter) handleReceive(wsConn *WSConnection) {
    defer close(wsConn.closeChan)
    
    for {
        var wsMsg WebSocketMessage
        err := wsConn.conn.ReadJSON(&wsMsg)
        if err != nil {
            return
        }
        
        switch wsMsg.Type {
        case "subscribe":
            // 订阅主题
            w.subscribe(wsConn, wsMsg.Topic)
            
        case "unsubscribe":
            // 取消订阅
            w.unsubscribe(wsConn, wsMsg.Topic)
            
        case "publish":
            // 发布消息
            sgcsfMsg := &SGCSFMessage{
                ID:        wsMsg.ID,
                Topic:     wsMsg.Topic,
                Source:    wsConn.clientID,
                Timestamp: time.Now().UnixNano(),
                Payload:   w.serializePayload(wsMsg.Payload),
            }
            w.sgcsfClient.Publish(wsMsg.Topic, sgcsfMsg)
        }
    }
}

// 处理WebSocket发送消息
func (w *WebSocketAdapter) handleSend(wsConn *WSConnection) {
    for {
        select {
        case msg := <-wsConn.sendChan:
            wsMsg := WebSocketMessage{
                Type:    "message",
                Topic:   msg.Topic,
                ID:      msg.ID,
                Payload: w.deserializePayload(msg.Payload),
            }
            wsConn.conn.WriteJSON(wsMsg)
            
        case <-wsConn.closeChan:
            return
        }
    }
}
```

## 4. 发布订阅机制设计

### 4.1 主题层次结构设计

```go
// 主题管理器
type TopicManager struct {
    topics        map[string]*Topic          // 主题映射
    subscriptions map[string][]*Subscription // 订阅映射
    wildcardSubs  []*WildcardSubscription    // 通配符订阅
    mutex         sync.RWMutex
}

// 主题定义
type Topic struct {
    Name        string            `json:"name"`         // 主题名称
    Type        TopicType         `json:"type"`         // 主题类型
    Retention   time.Duration     `json:"retention"`    // 消息保留时间
    MaxSize     int64             `json:"max_size"`     // 最大消息大小
    Persistent  bool              `json:"persistent"`   // 是否持久化
    Metadata    map[string]string `json:"metadata"`     // 元数据
}

type TopicType int
const (
    TopicUnicast   TopicType = iota // 单播主题
    TopicBroadcast                  // 广播主题
    TopicStream                     // 流主题
)

// 主题层次结构
/*
星地通信主题层次结构：

/satellite/{sat_id}/command         - 卫星命令控制
/satellite/{sat_id}/status          - 卫星状态上报
/satellite/{sat_id}/data/upload     - 卫星数据上传
/satellite/{sat_id}/data/download   - 卫星数据下载
/satellite/{sat_id}/logs            - 卫星日志上报

/ground/{station_id}/command        - 地面站命令
/ground/{station_id}/monitoring     - 地面站监控数据
/ground/{station_id}/data/storage   - 地面站数据存储

/cluster/k8s/pods                   - K8s Pod管理
/cluster/k8s/nodes                  - K8s Node管理
/cluster/k8s/services              - K8s Service管理

/monitoring/metrics                 - 监控指标收集
/monitoring/alerts                  - 告警通知
/monitoring/health                  - 健康检查

/data/file/transfer                 - 文件传输
/data/stream/video                  - 视频流传输
/data/stream/sensor                 - 传感器数据流

/broadcast/emergency               - 紧急广播
/broadcast/maintenance             - 维护通知
/broadcast/system                  - 系统通知
*/

// 订阅定义
type Subscription struct {
    ID          string              `json:"id"`           // 订阅ID
    ClientID    string              `json:"client_id"`    // 客户端ID
    Topic       string              `json:"topic"`        // 主题
    QoS         QoSLevel            `json:"qos"`          // QoS级别
    Handler     MessageHandler      `json:"-"`            // 消息处理器
    Filter      *MessageFilter      `json:"filter"`       // 消息过滤器
    CreateTime  time.Time           `json:"create_time"`  // 创建时间
}

// 通配符订阅
type WildcardSubscription struct {
    Pattern     string              `json:"pattern"`      // 通配符模式
    Subscription *Subscription      `json:"subscription"` // 订阅信息
}

// QoS级别
type QoSLevel int
const (
    QoSAtMostOnce  QoSLevel = iota // 最多一次
    QoSAtLeastOnce                 // 至少一次
    QoSExactlyOnce                 // 恰好一次
)

// 订阅主题
func (t *TopicManager) Subscribe(clientID, topic string, qos QoSLevel, handler MessageHandler) (*Subscription, error) {
    t.mutex.Lock()
    defer t.mutex.Unlock()
    
    subscription := &Subscription{
        ID:         generateSubscriptionID(),
        ClientID:   clientID,
        Topic:      topic,
        QoS:        qos,
        Handler:    handler,
        CreateTime: time.Now(),
    }
    
    // 检查是否是通配符订阅
    if strings.Contains(topic, "+") || strings.Contains(topic, "*") {
        wildcard := &WildcardSubscription{
            Pattern:      topic,
            Subscription: subscription,
        }
        t.wildcardSubs = append(t.wildcardSubs, wildcard)
    } else {
        // 普通订阅
        t.subscriptions[topic] = append(t.subscriptions[topic], subscription)
    }
    
    return subscription, nil
}

// 发布消息
func (t *TopicManager) Publish(topic string, message *SGCSFMessage) error {
    t.mutex.RLock()
    defer t.mutex.RUnlock()
    
    // 查找订阅者
    subscribers := t.findSubscribers(topic)
    
    // 分发消息
    for _, sub := range subscribers {
        go t.deliverMessage(sub, message)
    }
    
    return nil
}

// 查找订阅者
func (t *TopicManager) findSubscribers(topic string) []*Subscription {
    var subscribers []*Subscription
    
    // 精确匹配订阅
    if subs, exists := t.subscriptions[topic]; exists {
        subscribers = append(subscribers, subs...)
    }
    
    // 通配符匹配订阅
    for _, wildcard := range t.wildcardSubs {
        if t.matchWildcard(wildcard.Pattern, topic) {
            subscribers = append(subscribers, wildcard.Subscription)
        }
    }
    
    return subscribers
}

// 通配符匹配
func (t *TopicManager) matchWildcard(pattern, topic string) bool {
    // + 匹配单级
    // * 匹配多级
    // 例如: /satellite/+/status 匹配 /satellite/sat5/status
    // 例如: /satellite/* 匹配 /satellite/sat5/status/detailed
    
    patternParts := strings.Split(pattern, "/")
    topicParts := strings.Split(topic, "/")
    
    return t.matchParts(patternParts, topicParts, 0, 0)
}
```

### 4.2 消息分发机制

```go
// 消息分发器
type MessageDistributor struct {
    topicManager    *TopicManager
    messageQueue    chan *PendingMessage    // 待分发消息队列
    workers         []*DistributorWorker    // 分发工作者
    loadBalancer    LoadBalancer            // 负载均衡器
    retryManager    *RetryManager           // 重试管理器
}

type PendingMessage struct {
    Topic       string        `json:"topic"`
    Message     *SGCSFMessage `json:"message"`
    Subscribers []*Subscription `json:"subscribers"`
    Attempts    int           `json:"attempts"`
    NextRetry   time.Time     `json:"next_retry"`
}

// 分发工作者
type DistributorWorker struct {
    id          int
    distributor *MessageDistributor
    workChan    chan *DeliveryTask
    stopChan    chan struct{}
}

type DeliveryTask struct {
    Subscription *Subscription  `json:"subscription"`
    Message      *SGCSFMessage  `json:"message"`
    QoS          QoSLevel       `json:"qos"`
}

// 启动消息分发
func (d *MessageDistributor) Start() {
    // 启动工作者
    for i := 0; i < d.getWorkerCount(); i++ {
        worker := &DistributorWorker{
            id:          i,
            distributor: d,
            workChan:    make(chan *DeliveryTask, 100),
            stopChan:    make(chan struct{}),
        }
        d.workers[i] = worker
        go worker.run()
    }
    
    // 启动消息分发循环
    go d.distributionLoop()
}

// 分发循环
func (d *MessageDistributor) distributionLoop() {
    for pendingMsg := range d.messageQueue {
        // 为每个订阅者创建分发任务
        for _, sub := range pendingMsg.Subscribers {
            task := &DeliveryTask{
                Subscription: sub,
                Message:      pendingMsg.Message,
                QoS:          sub.QoS,
            }
            
            // 选择工作者
            worker := d.loadBalancer.SelectWorker(d.workers)
            
            // 异步分发
            select {
            case worker.workChan <- task:
                // 分发成功
            default:
                // 工作者忙，重新入队
                d.requeueMessage(pendingMsg)
            }
        }
    }
}

// 工作者运行
func (w *DistributorWorker) run() {
    for {
        select {
        case task := <-w.workChan:
            w.deliverMessage(task)
        case <-w.stopChan:
            return
        }
    }
}

// 消息投递
func (w *DistributorWorker) deliverMessage(task *DeliveryTask) {
    switch task.QoS {
    case QoSAtMostOnce:
        // 最多一次，直接投递，不管成功失败
        task.Subscription.Handler.Handle(task.Message)
        
    case QoSAtLeastOnce:
        // 至少一次，投递失败需要重试
        err := task.Subscription.Handler.Handle(task.Message)
        if err != nil {
            w.distributor.retryManager.ScheduleRetry(task)
        }
        
    case QoSExactlyOnce:
        // 恰好一次，需要去重和确认机制
        w.deliverExactlyOnce(task)
    }
}
```

## 5. 数据流传输机制

### 5.1 流式传输架构

对于大文件下载等数据流场景，不能将整个文件放在单个消息中，需要设计流式传输机制：

```go
// 流管理器
type StreamManager struct {
    streams       map[string]*DataStream    // 活跃流
    streamFactory *StreamFactory            // 流工厂
    flowControl   *FlowController           // 流控制器
    chunkSize     int                       // 分块大小
    maxStreams    int                       // 最大流数量
    mutex         sync.RWMutex
}

// 数据流定义
type DataStream struct {
    ID            string              `json:"id"`             // 流ID
    Topic         string              `json:"topic"`          // 关联主题
    Type          StreamType          `json:"type"`           // 流类型
    Source        string              `json:"source"`         // 数据源
    Destination   string              `json:"destination"`    // 目标
    TotalSize     int64               `json:"total_size"`     // 总大小
    ChunkSize     int                 `json:"chunk_size"`     // 分块大小
    CurrentChunk  int                 `json:"current_chunk"`  // 当前块
    TotalChunks   int                 `json:"total_chunks"`   // 总块数
    Status        StreamStatus        `json:"status"`         // 状态
    CreatedAt     time.Time           `json:"created_at"`     // 创建时间
    UpdatedAt     time.Time           `json:"updated_at"`     // 更新时间
    
    // 流控制
    sendChan      chan *StreamChunk   `json:"-"`             // 发送通道
    recvChan      chan *StreamChunk   `json:"-"`             // 接收通道
    ackChan       chan *ChunkAck      `json:"-"`             // 确认通道
    errorChan     chan error          `json:"-"`             // 错误通道
    closeChan     chan struct{}       `json:"-"`             // 关闭通道
    
    // 元数据
    Metadata      map[string]string   `json:"metadata"`       // 流元数据
    Checksum      string              `json:"checksum"`       // 整体校验和
}

type StreamType int
const (
    StreamFile StreamType = iota   // 文件流
    StreamVideo                    // 视频流
    StreamSensor                   // 传感器流
    StreamLog                      // 日志流
)

type StreamStatus int
const (
    StreamPending StreamStatus = iota  // 等待中
    StreamActive                       // 活跃
    StreamPaused                       // 暂停
    StreamCompleted                    // 完成
    StreamFailed                       // 失败
    StreamCanceled                     // 取消
)

// 流数据块
type StreamChunk struct {
    StreamID    string    `json:"stream_id"`    // 流ID
    ChunkIndex  int       `json:"chunk_index"`  // 块索引
    ChunkSize   int       `json:"chunk_size"`   // 块大小
    Data        []byte    `json:"data"`         // 数据
    Checksum    string    `json:"checksum"`     // 块校验和
    IsLast      bool      `json:"is_last"`      // 是否最后一块
    Timestamp   int64     `json:"timestamp"`    // 时间戳
}

// 块确认
type ChunkAck struct {
    StreamID    string `json:"stream_id"`    // 流ID
    ChunkIndex  int    `json:"chunk_index"`  // 块索引
    Status      string `json:"status"`       // 状态(ack/nack)
    ErrorMsg    string `json:"error_msg"`    // 错误信息
}

// 创建数据流
func (s *StreamManager) CreateStream(topic, source, destination string, streamType StreamType, totalSize int64) (*DataStream, error) {
    s.mutex.Lock()
    defer s.mutex.Unlock()
    
    // 检查流数量限制
    if len(s.streams) >= s.maxStreams {
        return nil, fmt.Errorf("max streams exceeded")
    }
    
    streamID := generateStreamID()
    totalChunks := int(math.Ceil(float64(totalSize) / float64(s.chunkSize)))
    
    stream := &DataStream{
        ID:           streamID,
        Topic:        topic,
        Type:         streamType,
        Source:       source,
        Destination:  destination,
        TotalSize:    totalSize,
        ChunkSize:    s.chunkSize,
        TotalChunks:  totalChunks,
        Status:       StreamPending,
        CreatedAt:    time.Now(),
        UpdatedAt:    time.Now(),
        
        sendChan:     make(chan *StreamChunk, 10),
        recvChan:     make(chan *StreamChunk, 10),
        ackChan:      make(chan *ChunkAck, 10),
        errorChan:    make(chan error, 1),
        closeChan:    make(chan struct{}),
        
        Metadata:     make(map[string]string),
    }
    
    s.streams[streamID] = stream
    
    // 启动流处理协程
    go s.handleStream(stream)
    
    return stream, nil
}

// 流处理逻辑
func (s *StreamManager) handleStream(stream *DataStream) {
    defer s.closeStream(stream)
    
    stream.Status = StreamActive
    stream.UpdatedAt = time.Now()
    
    for {
        select {
        case chunk := <-stream.sendChan:
            // 发送数据块
            err := s.sendChunk(stream, chunk)
            if err != nil {
                stream.errorChan <- err
                return
            }
            
        case chunk := <-stream.recvChan:
            // 接收数据块
            err := s.receiveChunk(stream, chunk)
            if err != nil {
                stream.errorChan <- err
                return
            }
            
            // 检查是否完成
            if chunk.IsLast {
                stream.Status = StreamCompleted
                return
            }
            
        case ack := <-stream.ackChan:
            // 处理确认
            s.handleAck(stream, ack)
            
        case err := <-stream.errorChan:
            // 处理错误
            stream.Status = StreamFailed
            s.handleStreamError(stream, err)
            return
            
        case <-stream.closeChan:
            // 流关闭
            stream.Status = StreamCanceled
            return
        }
    }
}

// 发送数据块
func (s *StreamManager) sendChunk(stream *DataStream, chunk *StreamChunk) error {
    // 1. 计算校验和
    chunk.Checksum = calculateChecksum(chunk.Data)
    
    // 2. 构建SGCSF消息
    message := &SGCSFMessage{
        ID:        generateMessageID(),
        Topic:     stream.Topic + "/chunk",
        Type:      "stream_chunk",
        Source:    stream.Source,
        Timestamp: time.Now().UnixNano(),
        Headers: map[string]string{
            "Stream-ID":     stream.ID,
            "Chunk-Index":   strconv.Itoa(chunk.ChunkIndex),
            "Chunk-Size":    strconv.Itoa(chunk.ChunkSize),
            "Is-Last":       strconv.FormatBool(chunk.IsLast),
            "Checksum":      chunk.Checksum,
        },
        Payload: chunk.Data,
    }
    
    // 3. 发布消息
    return s.publishMessage(message)
}

// 接收数据块
func (s *StreamManager) receiveChunk(stream *DataStream, chunk *StreamChunk) error {
    // 1. 验证校验和
    expectedChecksum := calculateChecksum(chunk.Data)
    if expectedChecksum != chunk.Checksum {
        return fmt.Errorf("checksum mismatch for chunk %d", chunk.ChunkIndex)
    }
    
    // 2. 写入数据块
    err := s.writeChunkData(stream, chunk)
    if err != nil {
        return err
    }
    
    // 3. 发送确认
    ack := &ChunkAck{
        StreamID:   stream.ID,
        ChunkIndex: chunk.ChunkIndex,
        Status:     "ack",
    }
    
    return s.sendAck(stream, ack)
}
```

### 5.2 流控制机制

```go
// 流控制器
type FlowController struct {
    windowSize      int                    // 滑动窗口大小
    congestionCtrl  *CongestionController  // 拥塞控制
    rateLimiter     *RateLimiter          // 速率限制器
    backpressure    *BackpressureHandler  // 背压处理器
}

// 滑动窗口控制
type SlidingWindow struct {
    size          int                 // 窗口大小
    inFlight      map[int]*StreamChunk // 在途数据块
    nextExpected  int                 // 下一个期望的块号
    lastAcked     int                 // 最后确认的块号
    mutex         sync.RWMutex
}

// 检查是否可以发送
func (w *SlidingWindow) CanSend(chunkIndex int) bool {
    w.mutex.RLock()
    defer w.mutex.RUnlock()
    
    return len(w.inFlight) < w.size && chunkIndex < w.lastAcked + w.size
}

// 发送数据块
func (w *SlidingWindow) Send(chunk *StreamChunk) {
    w.mutex.Lock()
    defer w.mutex.Unlock()
    
    w.inFlight[chunk.ChunkIndex] = chunk
}

// 确认数据块
func (w *SlidingWindow) Ack(chunkIndex int) {
    w.mutex.Lock()
    defer w.mutex.Unlock()
    
    delete(w.inFlight, chunkIndex)
    
    // 更新窗口
    if chunkIndex == w.lastAcked + 1 {
        w.lastAcked = chunkIndex
        
        // 尝试滑动窗口
        for {
            nextIndex := w.lastAcked + 1
            if _, exists := w.inFlight[nextIndex]; !exists {
                break
            }
            delete(w.inFlight, nextIndex)
            w.lastAcked = nextIndex
        }
    }
}

// 背压处理器
type BackpressureHandler struct {
    threshold     float64   // 背压阈值
    currentLoad   float64   // 当前负载
    strategy      BackpressureStrategy // 背压策略
}

type BackpressureStrategy int
const (
    BackpressureDrop BackpressureStrategy = iota  // 丢弃策略
    BackpressureBlock                              // 阻塞策略
    BackpressureThrottle                          // 限流策略
)

// 处理背压
func (b *BackpressureHandler) HandleBackpressure(stream *DataStream) error {
    if b.currentLoad < b.threshold {
        return nil // 无需处理
    }
    
    switch b.strategy {
    case BackpressureDrop:
        return fmt.Errorf("dropping stream due to backpressure")
        
    case BackpressureBlock:
        // 阻塞直到负载降低
        for b.currentLoad >= b.threshold {
            time.Sleep(time.Millisecond * 100)
        }
        return nil
        
    case BackpressureThrottle:
        // 限制发送速率
        delay := time.Duration(b.currentLoad * 1000) * time.Millisecond
        time.Sleep(delay)
        return nil
    }
    
    return nil
}
```

## 6. 同步异步消息处理

### 6.1 消息类型定义

```go
// SGCSF消息定义
type SGCSFMessage struct {
    // 基础字段
    ID              string            `json:"id"`               // 消息ID
    Type            MessageType       `json:"type"`             // 消息类型
    Topic           string            `json:"topic"`            // 主题
    Source          string            `json:"source"`           // 消息源
    Destination     string            `json:"destination"`      // 目标(可选)
    Timestamp       int64             `json:"timestamp"`        // 时间戳
    
    // 同步相关字段
    IsSync          bool              `json:"is_sync"`          // 是否同步消息
    RequestID       string            `json:"request_id"`       // 请求ID(同步消息)
    ResponseTo      string            `json:"response_to"`      // 响应的请求ID
    Timeout         int64             `json:"timeout"`          // 超时时间(毫秒)
    
    // 消息内容
    Headers         map[string]string `json:"headers"`          // 消息头
    ContentType     string            `json:"content_type"`     // 内容类型
    ContentEncoding string            `json:"content_encoding"` // 内容编码
    Payload         []byte            `json:"payload"`          // 消息负载
    
    // 路由相关
    Priority        Priority          `json:"priority"`         // 优先级
    QoS             QoSLevel          `json:"qos"`             // QoS级别
    TTL             int64             `json:"ttl"`             // 生存时间
    
    // 流相关(针对数据流)
    StreamID        string            `json:"stream_id"`        // 流ID
    StreamType      StreamType        `json:"stream_type"`      // 流类型
    IsStreamStart   bool              `json:"is_stream_start"`  // 是否流开始
    IsStreamEnd     bool              `json:"is_stream_end"`    // 是否流结束
    
    // 元数据
    Metadata        map[string]string `json:"metadata"`         // 扩展元数据
}

type MessageType int
const (
    MessageTypeAsync MessageType = iota  // 异步消息
    MessageTypeSync                      // 同步请求
    MessageTypeResponse                  // 同步响应
    MessageTypeStream                    // 流消息
    MessageTypeControl                   // 控制消息
    MessageTypeBroadcast                // 广播消息
)
```

### 6.2 同步消息处理

```go
// 同步消息管理器
type SyncMessageManager struct {
    pendingRequests map[string]*SyncRequest  // 待处理的同步请求
    responseWaiters map[string]chan *SGCSFMessage // 响应等待者
    timeout         time.Duration            // 默认超时时间
    cleanupInterval time.Duration            // 清理间隔
    mutex           sync.RWMutex
}

type SyncRequest struct {
    RequestID    string           `json:"request_id"`    // 请求ID
    Message      *SGCSFMessage    `json:"message"`       // 原始消息
    ResponseChan chan *SGCSFMessage `json:"-"`          // 响应通道
    CreatedAt    time.Time        `json:"created_at"`    // 创建时间
    Timeout      time.Duration    `json:"timeout"`       // 超时时间
    Timer        *time.Timer      `json:"-"`            // 超时定时器
}

// 发送同步消息
func (s *SyncMessageManager) SendSyncMessage(topic string, message *SGCSFMessage, timeout time.Duration) (*SGCSFMessage, error) {
    // 1. 生成请求ID
    requestID := generateRequestID()
    
    // 2. 设置同步消息字段
    message.IsSync = true
    message.RequestID = requestID
    message.Timeout = timeout.Milliseconds()
    
    // 3. 创建响应等待通道
    responseChan := make(chan *SGCSFMessage, 1)
    
    // 4. 注册同步请求
    syncReq := &SyncRequest{
        RequestID:    requestID,
        Message:      message,
        ResponseChan: responseChan,
        CreatedAt:    time.Now(),
        Timeout:      timeout,
        Timer:        time.NewTimer(timeout),
    }
    
    s.mutex.Lock()
    s.pendingRequests[requestID] = syncReq
    s.responseWaiters[requestID] = responseChan
    s.mutex.Unlock()
    
    // 5. 发送消息
    err := s.publishMessage(topic, message)
    if err != nil {
        s.cleanupRequest(requestID)
        return nil, err
    }
    
    // 6. 等待响应
    select {
    case response := <-responseChan:
        s.cleanupRequest(requestID)
        return response, nil
        
    case <-syncReq.Timer.C:
        s.cleanupRequest(requestID)
        return nil, fmt.Errorf("sync message timeout after %v", timeout)
    }
}

// 处理同步响应
func (s *SyncMessageManager) HandleSyncResponse(response *SGCSFMessage) error {
    if response.ResponseTo == "" {
        return fmt.Errorf("invalid sync response: missing response_to field")
    }
    
    s.mutex.RLock()
    responseChan, exists := s.responseWaiters[response.ResponseTo]
    s.mutex.RUnlock()
    
    if !exists {
        return fmt.Errorf("no waiter for sync response: %s", response.ResponseTo)
    }
    
    // 发送响应到等待通道
    select {
    case responseChan <- response:
        return nil
    default:
        return fmt.Errorf("response channel full for request: %s", response.ResponseTo)
    }
}

// 发送同步响应
func (s *SyncMessageManager) SendSyncResponse(originalMessage *SGCSFMessage, responsePayload []byte) error {
    if !originalMessage.IsSync {
        return fmt.Errorf("original message is not sync")
    }
    
    response := &SGCSFMessage{
        ID:           generateMessageID(),
        Type:         MessageTypeResponse,
        Topic:        originalMessage.Topic + "/response",
        Source:       getCurrentClientID(),
        Destination:  originalMessage.Source,
        Timestamp:    time.Now().UnixNano(),
        IsSync:       false,
        ResponseTo:   originalMessage.RequestID,
        ContentType:  "application/json",
        Payload:      responsePayload,
        Headers:      make(map[string]string),
    }
    
    return s.publishMessage(response.Topic, response)
}
```

### 6.3 异步消息处理

```go
// 异步消息管理器
type AsyncMessageManager struct {
    messageQueue    chan *SGCSFMessage      // 消息队列
    workers         []*AsyncWorker          // 工作者
    dlq             *DeadLetterQueue        // 死信队列
    retryManager    *RetryManager           // 重试管理器
    workerCount     int                     // 工作者数量
}

type AsyncWorker struct {
    id              int
    manager         *AsyncMessageManager
    workChan        chan *SGCSFMessage
    stopChan        chan struct{}
    processor       MessageProcessor
}

type MessageProcessor interface {
    Process(message *SGCSFMessage) error
    CanProcess(message *SGCSFMessage) bool
    GetProcessorType() string
}

// 处理异步消息
func (a *AsyncMessageManager) HandleAsyncMessage(message *SGCSFMessage) error {
    if message.IsSync {
        return fmt.Errorf("message is sync, not async")
    }
    
    // 添加到消息队列
    select {
    case a.messageQueue <- message:
        return nil
    default:
        // 队列满，可能需要背压处理
        return fmt.Errorf("message queue full")
    }
}

// 工作者处理逻辑
func (w *AsyncWorker) run() {
    for {
        select {
        case message := <-w.workChan:
            w.processMessage(message)
        case <-w.stopChan:
            return
        }
    }
}

func (w *AsyncWorker) processMessage(message *SGCSFMessage) {
    // 1. 检查处理器是否能处理此消息
    if !w.processor.CanProcess(message) {
        w.manager.dlq.Add(message, "no suitable processor")
        return
    }
    
    // 2. 处理消息
    err := w.processor.Process(message)
    if err != nil {
        // 处理失败，尝试重试
        if w.manager.retryManager.ShouldRetry(message) {
            w.manager.retryManager.ScheduleRetry(message)
        } else {
            // 重试次数超限，放入死信队列
            w.manager.dlq.Add(message, err.Error())
        }
    }
}
```

## 7. 实际应用场景示例

### 7.1 文件下载场景

```go
// 场景：地面站请求星5下载一个大文件

// 1. 地面站发送下载请求
func requestFileDownload() {
    client := NewSGCSFClient("ground-station-beijing")
    
    // 订阅下载进度主题
    client.Subscribe("/satellite/sat5/data/download/progress", func(msg *SGCSFMessage) {
        var progress DownloadProgress
        json.Unmarshal(msg.Payload, &progress)
        fmt.Printf("Download progress: %d%%\n", progress.Percentage)
    })
    
    // 发送下载请求消息
    request := &DownloadRequest{
        FileID:      "sat5-sensor-data-20240301.dat",
        Destination: "/data/downloads/",
        Priority:    PriorityHigh,
        Checksum:    true,
    }
    
    requestPayload, _ := json.Marshal(request)
    message := &SGCSFMessage{
        Topic:       "/satellite/sat5/data/download/request",
        Type:        MessageTypeAsync,
        Priority:    PriorityHigh,
        Payload:     requestPayload,
    }
    
    client.Publish(message.Topic, message)
}

// 2. 星5组件接收下载请求
func handleDownloadRequest() {
    client := NewSGCSFClient("satellite-sat5-dataserver")
    
    // 订阅下载请求主题
    client.Subscribe("/satellite/sat5/data/download/request", func(msg *SGCSFMessage) {
        var request DownloadRequest
        json.Unmarshal(msg.Payload, &request)
        
        // 启动文件下载任务
        go startFileDownload(request, msg.Source)
    })
}

// 3. 启动文件流传输
func startFileDownload(request DownloadRequest, destination string) {
    // 获取文件信息
    fileInfo, err := getFileInfo(request.FileID)
    if err != nil {
        sendErrorMessage(destination, err)
        return
    }
    
    // 创建数据流
    streamMgr := GetStreamManager()
    stream, err := streamMgr.CreateStream(
        "/satellite/sat5/data/download/stream",
        "satellite-sat5-dataserver",
        destination,
        StreamFile,
        fileInfo.Size,
    )
    
    if err != nil {
        sendErrorMessage(destination, err)
        return
    }
    
    // 读取文件并分块发送
    file, _ := os.Open(fileInfo.Path)
    defer file.Close()
    
    buffer := make([]byte, stream.ChunkSize)
    chunkIndex := 0
    
    for {
        n, err := file.Read(buffer)
        if err == io.EOF {
            break
        }
        
        chunk := &StreamChunk{
            StreamID:   stream.ID,
            ChunkIndex: chunkIndex,
            ChunkSize:  n,
            Data:       buffer[:n],
            IsLast:     false,
        }
        
        // 发送数据块
        stream.sendChan <- chunk
        chunkIndex++
        
        // 报告进度
        reportProgress(destination, chunkIndex, stream.TotalChunks)
    }
    
    // 发送最后一块
    finalChunk := &StreamChunk{
        StreamID:   stream.ID,
        ChunkIndex: chunkIndex,
        ChunkSize:  0,
        Data:       []byte{},
        IsLast:     true,
    }
    stream.sendChan <- finalChunk
}
```

### 7.2 监控数据收集场景

```go
// 场景：Prometheus收集星上节点的监控指标

// 1. Prometheus适配器配置
func setupPrometheusAdapter() {
    adapter := &HTTPAdapter{
        sgcsfClient: NewSGCSFClient("prometheus-server"),
        routeMapping: map[string]Topic{
            "GET /metrics": "/monitoring/metrics/collect",
        },
    }
    
    // 启动HTTP服务器
    http.HandleFunc("/", adapter.ServeHTTP)
    go http.ListenAndServe(":9090", nil)
    
    // 订阅指标响应
    adapter.sgcsfClient.Subscribe("/monitoring/metrics/response", 
        adapter.handleMetricsResponse)
}

// 2. 星上节点NodeExporter适配器
func setupNodeExporterAdapter() {
    adapter := &HTTPAdapter{
        sgcsfClient: NewSGCSFClient("node-exporter-sat5"),
    }
    
    // 订阅指标收集请求
    adapter.sgcsfClient.Subscribe("/monitoring/metrics/collect", func(msg *SGCSFMessage) {
        // 收集本地指标
        metrics := collectLocalMetrics()
        
        // 发送响应
        response := &SGCSFMessage{
            Topic:      "/monitoring/metrics/response",
            Type:       MessageTypeResponse,
            ResponseTo: msg.RequestID,
            Payload:    metrics,
        }
        
        adapter.sgcsfClient.Publish(response.Topic, response)
    })
}
```

### 7.3 Kubernetes Pod管理场景

```go
// 场景：CloudCore通过SGCSF管理星上的Pod

// 1. CloudCore适配器
func setupCloudCoreAdapter() {
    adapter := &HTTPAdapter{
        sgcsfClient: NewSGCSFClient("cloudcore-master"),
        routeMapping: map[string]Topic{
            "GET /api/v1/pods":     "/k8s/pods/list",
            "POST /api/v1/pods":    "/k8s/pods/create", 
            "DELETE /api/v1/pods/*": "/k8s/pods/delete",
        },
    }
    
    // 启动Kubernetes API服务器代理
    http.HandleFunc("/api/", adapter.ServeHTTP)
    go http.ListenAndServe(":8080", nil)
}

// 2. EdgeCore适配器  
func setupEdgeCoreAdapter() {
    adapter := &HTTPAdapter{
        sgcsfClient: NewSGCSFClient("edgecore-sat5"),
    }
    
    // 订阅Pod操作请求
    adapter.sgcsfClient.Subscribe("/k8s/pods/create", handlePodCreate)
    adapter.sgcsfClient.Subscribe("/k8s/pods/delete", handlePodDelete)
    adapter.sgcsfClient.Subscribe("/k8s/pods/list", handlePodList)
}

func handlePodCreate(msg *SGCSFMessage) {
    var podSpec v1.Pod
    json.Unmarshal(msg.Payload, &podSpec)
    
    // 调用本地kubelet创建Pod
    err := createPodOnNode(podSpec)
    
    // 发送同步响应
    response := &SGCSFMessage{
        Topic:      "/k8s/pods/create/response",
        Type:       MessageTypeResponse,
        ResponseTo: msg.RequestID,
    }
    
    if err != nil {
        response.Headers["Status"] = "500"
        response.Payload = []byte(err.Error())
    } else {
        response.Headers["Status"] = "201"
        response.Payload, _ = json.Marshal(podSpec)
    }
    
    GetSGCSFClient().Publish(response.Topic, response)
}
```

这个重新设计的方案解决了你提到的核心问题：

1. **统一接入**: 通过协议适配器让现有HTTP/WebSocket组件无缝接入SGCSF
2. **发布订阅**: 基于主题的消息机制，支持单播和广播
3. **数据流传输**: 通过流式分块传输处理大文件等数据流
4. **同步异步**: 明确区分同步消息(需要回复)和异步消息(不需要回复)

这样的设计既保持了现有组件的兼容性，又提供了强大的统一通信能力。