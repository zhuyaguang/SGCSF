# SGCSF框架下HTTP客户端-服务器通信流程详解

## 场景描述

**原始场景**: 地面HTTP客户端直接访问星上HTTP服务器
```
地面HTTP客户端 ──HTTP GET──> 星上HTTP服务器
```

**SGCSF框架场景**: 通过SGCSF统一通信框架进行透明代理
```
地面HTTP客户端 ──HTTP──> 地面SGCSF适配器 ──QUIC/SGCSF──> 星上SGCSF适配器 ──HTTP──> 星上HTTP服务器
```

## 详细处理流程

### 1. 整体架构图

```
┌─────────────────────────────────────────────────────────────┐
│                    完整通信流程                              │
├─────────────────────────────────────────────────────────────┤
│  地面站                                卫星节点               │
│  ┌─────────────┐                      ┌─────────────────┐   │
│  │HTTP Client  │                      │  HTTP Server    │   │
│  │             │                      │  (星上应用)      │   │
│  └──────┬──────┘                      └────────▲────────┘   │
│         │ ①HTTP GET                            │ ⑥HTTP GET  │
│         │ /api/data                            │ /api/data  │
│         ▼                                      │            │
│  ┌─────────────┐                      ┌───────┴─────────┐   │
│  │SGCSF HTTP   │                      │  SGCSF HTTP     │   │
│  │Adapter      │                      │  Adapter        │   │
│  │(Ground)     │                      │  (Satellite)    │   │
│  └──────┬──────┘                      └────────▲────────┘   │
│         │ ②SGCSF Message                       │ ⑤SGCSF Msg │
│         │ Topic:/sat5/http/request             │ Response   │
│         ▼                                      │            │
│  ┌─────────────┐     ③QUIC Transport   ┌─────────────────┐   │
│  │SGCSF Core   │◄────────────────────►│   SGCSF Core    │   │
│  │(Ground)     │                      │   (Satellite)   │   │
│  └─────────────┘                      └─────────────────┘   │
│         │ ⑨HTTP Response                      │ ④Subscribe  │
│         │ 200 OK                               │ & Process   │
│         ▼                                      │            │
│  ┌─────────────┐                      ┌─────────────────┐   │
│  │HTTP Client  │                      │                 │   │
│  │(Response)   │                      │                 │   │
│  └─────────────┘                      └─────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### 2. 步骤详解

#### Step 1: HTTP客户端发起请求

```bash
# 地面HTTP客户端发起GET请求
curl -X GET "http://ground-sgcsf-proxy:8080/api/satellite/sat5/data?type=sensor"
```

地面HTTP客户端认为自己在访问一个普通的HTTP服务，实际上访问的是SGCSF地面适配器。

#### Step 2: 地面SGCSF HTTP适配器处理

```go
// 地面SGCSF HTTP适配器处理逻辑
func (h *GroundHTTPAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // 1. 解析请求路径，确定目标卫星
    targetSat, apiPath := h.parseTargetPath(r.URL.Path)
    // /api/satellite/sat5/data -> targetSat=sat5, apiPath=/api/data
    
    // 2. 构建SGCSF消息
    sgcsfMsg := &SGCSFMessage{
        ID:          generateMessageID(),
        Topic:       fmt.Sprintf("/satellite/%s/http/request", targetSat),
        Type:        MessageTypeSync,  // HTTP需要响应，使用同步消息
        Source:      "ground-http-adapter",
        Destination: fmt.Sprintf("satellite-%s-http-adapter", targetSat),
        Timestamp:   time.Now().UnixNano(),
        IsSync:      true,
        RequestID:   generateRequestID(),
        Timeout:     30000, // 30秒超时
        
        Headers: map[string]string{
            "HTTP-Method":      r.Method,
            "HTTP-Path":        apiPath,
            "HTTP-Query":       r.URL.RawQuery,
            "Content-Type":     r.Header.Get("Content-Type"),
            "User-Agent":       r.Header.Get("User-Agent"),
            "Authorization":    r.Header.Get("Authorization"),
        },
        ContentType: "application/http-proxy",
    }
    
    // 3. 如果有请求体，读取并包含
    if r.ContentLength > 0 {
        body, err := ioutil.ReadAll(r.Body)
        if err == nil {
            sgcsfMsg.Payload = body
        }
    }
    
    // 4. 发送同步消息到星上
    response, err := h.sgcsfClient.PublishSync(
        sgcsfMsg.Topic, 
        sgcsfMsg, 
        30*time.Second,
    )
    
    if err != nil {
        http.Error(w, fmt.Sprintf("SGCSF error: %v", err), http.StatusBadGateway)
        return
    }
    
    // 5. 将SGCSF响应转换为HTTP响应
    h.convertSGCSFResponseToHTTP(response, w)
}

// 解析目标路径
func (h *GroundHTTPAdapter) parseTargetPath(path string) (satellite, apiPath string) {
    // /api/satellite/sat5/data -> sat5, /api/data
    parts := strings.Split(strings.Trim(path, "/"), "/")
    if len(parts) >= 3 && parts[0] == "api" && parts[1] == "satellite" {
        satellite = parts[2]
        apiPath = "/" + strings.Join(parts[3:], "/")
        if apiPath == "/" {
            apiPath = ""
        }
        // 重新构建完整的API路径
        apiPath = "/api" + apiPath
    }
    return
}

// 转换SGCSF响应为HTTP响应
func (h *GroundHTTPAdapter) convertSGCSFResponseToHTTP(sgcsfResponse *SGCSFMessage, w http.ResponseWriter) {
    // 1. 设置HTTP状态码
    statusCode := 200
    if code, exists := sgcsfResponse.Headers["HTTP-Status-Code"]; exists {
        if parsed, err := strconv.Atoi(code); err == nil {
            statusCode = parsed
        }
    }
    
    // 2. 设置响应头
    for key, value := range sgcsfResponse.Headers {
        if strings.HasPrefix(key, "HTTP-Header-") {
            headerName := strings.TrimPrefix(key, "HTTP-Header-")
            w.Header().Set(headerName, value)
        }
    }
    
    // 3. 设置Content-Type
    if contentType, exists := sgcsfResponse.Headers["HTTP-Content-Type"]; exists {
        w.Header().Set("Content-Type", contentType)
    }
    
    // 4. 写入状态码和响应体
    w.WriteHeader(statusCode)
    if len(sgcsfResponse.Payload) > 0 {
        w.Write(sgcsfResponse.Payload)
    }
}
```

#### Step 3: QUIC传输到星上

SGCSF核心层通过QUIC协议将消息传输到目标卫星：

```go
// SGCSF核心传输逻辑
func (s *SGCSFCore) PublishSync(topic string, message *SGCSFMessage, timeout time.Duration) (*SGCSFMessage, error) {
    // 1. 根据主题路由确定目标卫星
    target := s.topicRouter.GetTarget(topic)
    // /satellite/sat5/http/request -> target = "satellite-sat5"
    
    // 2. 获取到目标卫星的QUIC连接
    conn, err := s.connectionManager.GetConnection(target)
    if err != nil {
        return nil, fmt.Errorf("no connection to %s: %v", target, err)
    }
    
    // 3. 分配流ID并发送
    streamID := s.streamAllocator.AllocateStream("http-sync")
    
    // 4. 序列化消息
    data, err := json.Marshal(message)
    if err != nil {
        return nil, err
    }
    
    // 5. 通过QUIC流发送
    stream, err := conn.OpenStream()
    if err != nil {
        return nil, err
    }
    defer stream.Close()
    
    // 发送消息
    _, err = stream.Write(data)
    if err != nil {
        return nil, err
    }
    
    // 6. 等待响应
    responseData := make([]byte, MaxMessageSize)
    n, err := stream.Read(responseData)
    if err != nil {
        return nil, err
    }
    
    // 7. 反序列化响应
    var response SGCSFMessage
    err = json.Unmarshal(responseData[:n], &response)
    if err != nil {
        return nil, err
    }
    
    return &response, nil
}
```

#### Step 4: 星上SGCSF适配器接收并处理

```go
// 星上SGCSF HTTP适配器
type SatelliteHTTPAdapter struct {
    sgcsfClient    *SGCSFClient
    httpClient     *http.Client
    targetServer   string  // 目标HTTP服务器地址
}

func (s *SatelliteHTTPAdapter) Start() {
    // 订阅来自地面的HTTP请求
    s.sgcsfClient.Subscribe("/satellite/sat5/http/request", s.handleHTTPRequest)
}

func (s *SatelliteHTTPAdapter) handleHTTPRequest(sgcsfMsg *SGCSFMessage) {
    // 1. 解析SGCSF消息中的HTTP信息
    httpReq := s.convertSGCSFToHTTPRequest(sgcsfMsg)
    
    // 2. 转发给本地HTTP服务器
    httpResp, err := s.httpClient.Do(httpReq)
    if err != nil {
        s.sendErrorResponse(sgcsfMsg, err)
        return
    }
    defer httpResp.Body.Close()
    
    // 3. 读取HTTP响应
    respBody, err := ioutil.ReadAll(httpResp.Body)
    if err != nil {
        s.sendErrorResponse(sgcsfMsg, err)
        return
    }
    
    // 4. 构建SGCSF响应消息
    sgcsfResp := &SGCSFMessage{
        ID:           generateMessageID(),
        Topic:        "/satellite/sat5/http/response",
        Type:         MessageTypeResponse,
        Source:       "satellite-sat5-http-adapter",
        Destination:  sgcsfMsg.Source,
        Timestamp:    time.Now().UnixNano(),
        ResponseTo:   sgcsfMsg.RequestID,
        
        Headers: map[string]string{
            "HTTP-Status-Code":   strconv.Itoa(httpResp.StatusCode),
            "HTTP-Content-Type":  httpResp.Header.Get("Content-Type"),
        },
        Payload: respBody,
    }
    
    // 5. 复制HTTP响应头
    for name, values := range httpResp.Header {
        if len(values) > 0 {
            sgcsfResp.Headers["HTTP-Header-"+name] = values[0]
        }
    }
    
    // 6. 发送响应
    s.sgcsfClient.PublishResponse(sgcsfResp)
}

// 转换SGCSF消息为HTTP请求
func (s *SatelliteHTTPAdapter) convertSGCSFToHTTPRequest(sgcsfMsg *SGCSFMessage) *http.Request {
    // 1. 构建目标URL
    method := sgcsfMsg.Headers["HTTP-Method"]
    path := sgcsfMsg.Headers["HTTP-Path"]
    query := sgcsfMsg.Headers["HTTP-Query"]
    
    url := fmt.Sprintf("http://%s%s", s.targetServer, path)
    if query != "" {
        url += "?" + query
    }
    
    // 2. 创建HTTP请求
    var body io.Reader
    if len(sgcsfMsg.Payload) > 0 {
        body = bytes.NewReader(sgcsfMsg.Payload)
    }
    
    req, _ := http.NewRequest(method, url, body)
    
    // 3. 设置请求头
    if contentType := sgcsfMsg.Headers["Content-Type"]; contentType != "" {
        req.Header.Set("Content-Type", contentType)
    }
    if userAgent := sgcsfMsg.Headers["User-Agent"]; userAgent != "" {
        req.Header.Set("User-Agent", userAgent)
    }
    if auth := sgcsfMsg.Headers["Authorization"]; auth != "" {
        req.Header.Set("Authorization", auth)
    }
    
    return req
}
```

#### Step 5: 星上HTTP服务器处理

```go
// 星上HTTP服务器（原有应用，无需修改）
func starHttpServer() {
    http.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
        // 正常的业务逻辑，完全不知道请求来自SGCSF
        dataType := r.URL.Query().Get("type")
        
        var response interface{}
        switch dataType {
        case "sensor":
            response = map[string]interface{}{
                "temperature": 25.6,
                "humidity":    65.2,
                "pressure":    1013.25,
                "timestamp":   time.Now().Unix(),
            }
        default:
            response = map[string]string{"error": "unknown data type"}
        }
        
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(response)
    })
    
    log.Println("Star HTTP Server listening on :8080")
    http.ListenAndServe(":8080", nil)
}
```

#### Step 6-9: 响应返回流程

响应按相反的路径返回：
1. 星上HTTP服务器返回JSON响应
2. 星上SGCSF适配器将HTTP响应转换为SGCSF消息
3. 通过QUIC传输回地面
4. 地面SGCSF适配器将SGCSF消息转换为HTTP响应
5. 返回给原始HTTP客户端

### 3. 完整的配置示例

#### 地面SGCSF适配器配置

```yaml
# ground-sgcsf-config.yaml
sgcsf:
  client_id: "ground-http-adapter"
  server_endpoint: "127.0.0.1:7000"
  
http_adapter:
  listen_port: 8080
  target_mapping:
    - path_prefix: "/api/satellite/sat5/"
      target_topic: "/satellite/sat5/http/request"
      timeout: 30s
    - path_prefix: "/api/satellite/sat3/"
      target_topic: "/satellite/sat3/http/request" 
      timeout: 30s
      
  default_headers:
    "X-Proxied-By": "SGCSF-Ground-Adapter"
```

#### 星上SGCSF适配器配置

```yaml
# satellite-sgcsf-config.yaml
sgcsf:
  client_id: "satellite-sat5-http-adapter"
  server_endpoint: "sgcsf-core:7000"
  
http_adapter:
  target_server: "localhost:8080"  # 本地HTTP服务器
  subscribe_topics:
    - "/satellite/sat5/http/request"
  
  response_topic: "/satellite/sat5/http/response"
  max_request_size: 10MB
  request_timeout: 25s
```

### 4. 关键优势

1. **透明性**: HTTP客户端和服务器完全无感知，无需修改代码
2. **统一管理**: 所有HTTP通信都通过SGCSF框架，便于监控和管理
3. **可靠传输**: 基于QUIC协议，支持自动重传和拥塞控制
4. **扩展性**: 可以轻松支持多个卫星和多个HTTP服务
5. **监控能力**: 可以监控所有HTTP请求的性能和状态

### 5. 性能优化

1. **连接复用**: QUIC连接可以复用，减少建立连接的开销
2. **并发处理**: 支持多个HTTP请求并发传输
3. **压缩传输**: 可以对HTTP payload进行压缩
4. **缓存机制**: 可以缓存频繁访问的响应

这样，原本简单的HTTP请求通过SGCSF框架变成了一个完整的分布式通信流程，但对于应用层来说完全透明！