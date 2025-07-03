package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
	"github.com/sgcsf/sgcsf-go/pkg/core"
)

// MetricsRequest 指标收集请求
type MetricsRequest struct {
	RequestID    string   `json:"request_id"`
	Timestamp    int64    `json:"timestamp"`
	MetricTypes  []string `json:"metric_types"`
	SatelliteIDs []string `json:"satellite_ids"`
	Interval     int      `json:"interval_seconds"`
}

// GroundHTTPClient 地面HTTP客户端，模拟Prometheus
type GroundHTTPClient struct {
	sgcsfClient   core.SGCSFClient
	httpClient    *http.Client
	satellites    map[string]string // satellite_id -> http_endpoint
	metricsStore  *MetricsStore
	alertsChannel chan Alert
	running       bool
	mutex         sync.RWMutex
}

// MetricsStore 指标存储
type MetricsStore struct {
	metrics map[string][]MetricData
	mutex   sync.RWMutex
}

// MetricData 指标数据
type MetricData struct {
	Timestamp   int64                  `json:"timestamp"`
	SatelliteID string                 `json:"satellite_id"`
	Source      string                 `json:"source"` // "http" or "sgcsf"
	Data        map[string]interface{} `json:"data"`
}

// Alert 告警数据
type Alert struct {
	ID          string                 `json:"id"`
	Timestamp   int64                  `json:"timestamp"`
	SatelliteID string                 `json:"satellite_id"`
	Type        string                 `json:"type"`
	Severity    string                 `json:"severity"`
	Message     string                 `json:"message"`
	Data        map[string]interface{} `json:"data"`
}

func main() {
	fmt.Println("🌍 Starting Ground HTTP Client (Prometheus simulation)")

	sgcsfServer := getEnv("SGCSF_SERVER", "localhost:8080") // 连接到地面网关
	
	client := &GroundHTTPClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		satellites: map[string]string{
			"sat-demo": "http://localhost:9100", // 卫星HTTP服务器地址
		},
		metricsStore: &MetricsStore{
			metrics: make(map[string][]MetricData),
		},
		alertsChannel: make(chan Alert, 100),
		running:       true,
	}

	// 创建SGCSF客户端连接到地面网关
	client.sgcsfClient = core.GroundClient(
		"ground-prometheus-client",
		sgcsfServer,
	)

	// 连接到SGCSF
	ctx := context.Background()
	err := client.sgcsfClient.Connect(ctx)
	if err != nil {
		log.Printf("⚠️ Failed to connect to SGCSF: %v (running in HTTP-only mode)", err)
	} else {
		fmt.Println("✅ Connected to SGCSF ground gateway")
		// 设置SGCSF订阅
		client.setupSGCSFSubscriptions()
	}

	// 启动各种功能
	go client.startHTTPMetricsCollection()
	go client.startSGCSFMetricsCollection()
	go client.startAlertHandler()
	go client.startMetricsAnalysis()
	go client.startWebDashboard()

	fmt.Printf("✅ Ground HTTP Client started\n")
	fmt.Printf("📊 Collecting metrics from satellites: %v\n", getSatelliteIDs(client.satellites))
	fmt.Printf("🌐 Web dashboard: http://localhost:8081\n")
	fmt.Printf("📈 SGCSF integration: %v\n", client.sgcsfClient != nil)

	// 等待关闭信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\n🛑 Shutting down ground client...")
	client.shutdown()
}

func (c *GroundHTTPClient) setupSGCSFSubscriptions() {
	// 订阅卫星的指标响应
	_, err := c.sgcsfClient.Subscribe(
		"/ground/metrics/response/*",
		types.MessageHandlerFunc(func(message *types.SGCSFMessage) error {
			log.Printf("📨 Received metrics response via SGCSF from %s", message.Source)
			
			// 解析指标数据
			var metricsData map[string]interface{}
			if err := json.Unmarshal(message.Payload, &metricsData); err != nil {
				log.Printf("❌ Failed to parse metrics data: %v", err)
				return err
			}

			// 存储指标
			satelliteID, _ := message.GetHeader("satellite_id")
			c.storeMetrics(satelliteID, "sgcsf", metricsData)
			
			return nil
		}),
	)
	if err != nil {
		log.Printf("❌ Failed to subscribe to metrics responses: %v", err)
	}

	// 订阅卫星告警
	_, err = c.sgcsfClient.Subscribe(
		"/ground/alerts/*",
		types.MessageHandlerFunc(func(message *types.SGCSFMessage) error {
			log.Printf("🚨 Received alert via SGCSF from %s", message.Source)
			
			// 解析告警数据
			var alertData map[string]interface{}
			if err := json.Unmarshal(message.Payload, &alertData); err != nil {
				log.Printf("❌ Failed to parse alert data: %v", err)
				return err
			}

			// 处理告警
			c.processAlert(alertData)
			
			return nil
		}),
	)
	if err != nil {
		log.Printf("❌ Failed to subscribe to alerts: %v", err)
	}

	fmt.Println("🎯 SGCSF subscriptions set up successfully")
}

func (c *GroundHTTPClient) startHTTPMetricsCollection() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	fmt.Println("📊 Starting HTTP metrics collection...")

	for {
		select {
		case <-ticker.C:
			if !c.running {
				return
			}
			c.collectHTTPMetrics()
		}
	}
}

func (c *GroundHTTPClient) collectHTTPMetrics() {
	c.mutex.RLock()
	satellites := make(map[string]string)
	for k, v := range c.satellites {
		satellites[k] = v
	}
	c.mutex.RUnlock()

	for satelliteID, endpoint := range satellites {
		go func(satID, ep string) {
			// 尝试通过HTTP直接收集指标
			metrics, err := c.fetchHTTPMetrics(ep + "/metrics")
			if err != nil {
				log.Printf("❌ HTTP metrics collection failed for %s: %v", satID, err)
				return
			}

			log.Printf("✅ Collected metrics via HTTP from %s", satID)
			c.storeMetrics(satID, "http", metrics)
		}(satelliteID, endpoint)
	}
}

func (c *GroundHTTPClient) fetchHTTPMetrics(url string) (map[string]interface{}, error) {
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var metrics map[string]interface{}
	if err := json.Unmarshal(body, &metrics); err != nil {
		return nil, err
	}

	return metrics, nil
}

func (c *GroundHTTPClient) startSGCSFMetricsCollection() {
	if c.sgcsfClient == nil {
		return
	}

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	fmt.Println("📡 Starting SGCSF metrics collection...")

	for {
		select {
		case <-ticker.C:
			if !c.running {
				return
			}
			c.collectSGCSFMetrics()
		}
	}
}

func (c *GroundHTTPClient) collectSGCSFMetrics() {
	if c.sgcsfClient == nil {
		return
	}

	c.mutex.RLock()
	satelliteIDs := getSatelliteIDs(c.satellites)
	c.mutex.RUnlock()

	for _, satelliteID := range satelliteIDs {
		go func(satID string) {
			// 通过SGCSF发送指标收集请求
			request := MetricsRequest{
				RequestID:    fmt.Sprintf("req_%d_%s", time.Now().Unix(), satID),
				Timestamp:    time.Now().Unix(),
				MetricTypes:  []string{"system", "custom", "alerts"},
				SatelliteIDs: []string{satID},
				Interval:     60,
			}

			message := core.NewMessage().
				Topic(fmt.Sprintf("/satellite/%s/metrics/request", satID)).
				Type(types.MessageTypeSync).
				Priority(types.PriorityNormal).
				QoS(types.QoSAtLeastOnce).
				ContentType("application/json").
				Source("ground-prometheus-client").
				Metadata("request_type", "metrics_collection").
				Timeout(30 * time.Second).
				Payload(request).
				Build()

			// 发送同步请求
			response, err := c.sgcsfClient.PublishSync(message.Topic, message, 30*time.Second)
			if err != nil {
				log.Printf("❌ SGCSF metrics request failed for %s: %v", satID, err)
				return
			}

			log.Printf("✅ Received metrics response via SGCSF from %s", satID)
			
			// 解析响应
			var metricsData map[string]interface{}
			if err := json.Unmarshal(response.Payload, &metricsData); err != nil {
				log.Printf("❌ Failed to parse SGCSF metrics response: %v", err)
				return
			}

			c.storeMetrics(satID, "sgcsf", metricsData)
		}(satelliteID)
	}
}

func (c *GroundHTTPClient) storeMetrics(satelliteID, source string, data map[string]interface{}) {
	c.metricsStore.mutex.Lock()
	defer c.metricsStore.mutex.Unlock()

	if c.metricsStore.metrics[satelliteID] == nil {
		c.metricsStore.metrics[satelliteID] = make([]MetricData, 0)
	}

	metric := MetricData{
		Timestamp:   time.Now().Unix(),
		SatelliteID: satelliteID,
		Source:      source,
		Data:        data,
	}

	c.metricsStore.metrics[satelliteID] = append(c.metricsStore.metrics[satelliteID], metric)

	// 保持最近100条记录
	if len(c.metricsStore.metrics[satelliteID]) > 100 {
		c.metricsStore.metrics[satelliteID] = c.metricsStore.metrics[satelliteID][1:]
	}

	log.Printf("📊 Stored metrics for %s (source: %s)", satelliteID, source)
}

func (c *GroundHTTPClient) processAlert(alertData map[string]interface{}) {
	satelliteID, _ := alertData["satellite_id"].(string)
	timestamp, _ := alertData["timestamp"].(float64)
	
	alerts, ok := alertData["alerts"].([]interface{})
	if !ok {
		return
	}

	for _, alertInterface := range alerts {
		alertMap, ok := alertInterface.(map[string]interface{})
		if !ok {
			continue
		}

		alert := Alert{
			ID:          fmt.Sprintf("alert_%d_%s", int64(timestamp), satelliteID),
			Timestamp:   int64(timestamp),
			SatelliteID: satelliteID,
			Type:        alertMap["type"].(string),
			Severity:    alertMap["severity"].(string),
			Message:     fmt.Sprintf("Alert: %s", alertMap["type"]),
			Data:        alertMap,
		}

		select {
		case c.alertsChannel <- alert:
			log.Printf("🚨 Queued alert: %s from %s", alert.Type, alert.SatelliteID)
		default:
			log.Printf("⚠️ Alert queue full, dropping alert")
		}
	}
}

func (c *GroundHTTPClient) startAlertHandler() {
	fmt.Println("🚨 Starting alert handler...")

	for alert := range c.alertsChannel {
		if !c.running {
			return
		}

		// 处理告警逻辑
		c.handleAlert(alert)
	}
}

func (c *GroundHTTPClient) handleAlert(alert Alert) {
	log.Printf("🚨 Processing alert: %s from %s (severity: %s)", 
		alert.Type, alert.SatelliteID, alert.Severity)

	// 这里可以实现告警处理逻辑：
	// 1. 发送邮件通知
	// 2. 推送到监控系统
	// 3. 触发自动修复流程
	// 4. 记录到告警数据库

	switch alert.Severity {
	case "critical":
		log.Printf("🔴 CRITICAL ALERT: %s", alert.Message)
		// 立即处理关键告警
	case "warning":
		log.Printf("🟡 WARNING ALERT: %s", alert.Message)
		// 记录警告告警
	default:
		log.Printf("ℹ️ INFO ALERT: %s", alert.Message)
	}
}

func (c *GroundHTTPClient) startMetricsAnalysis() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	fmt.Println("📈 Starting metrics analysis...")

	for {
		select {
		case <-ticker.C:
			if !c.running {
				return
			}
			c.analyzeMetrics()
		}
	}
}

func (c *GroundHTTPClient) analyzeMetrics() {
	c.metricsStore.mutex.RLock()
	defer c.metricsStore.mutex.RUnlock()

	for satelliteID, metrics := range c.metricsStore.metrics {
		if len(metrics) == 0 {
			continue
		}

		// 分析最近的指标
		recent := metrics[len(metrics)-1]
		
		// 提取关键指标进行分析
		if data, ok := recent.Data["cpu_metrics"].(map[string]interface{}); ok {
			if usage, ok := data["usage_percent"].(float64); ok {
				if usage > 80 {
					log.Printf("⚠️ High CPU usage detected on %s: %.2f%%", satelliteID, usage)
				}
			}
		}

		if data, ok := recent.Data["memory_metrics"].(map[string]interface{}); ok {
			if usage, ok := data["usage_percent"].(float64); ok {
				if usage > 85 {
					log.Printf("⚠️ High memory usage detected on %s: %.2f%%", satelliteID, usage)
				}
			}
		}

		// 分析数据源可用性
		httpCount := 0
		sgcsfCount := 0
		for _, metric := range metrics {
			if metric.Source == "http" {
				httpCount++
			} else if metric.Source == "sgcsf" {
				sgcsfCount++
			}
		}

		log.Printf("📊 %s metrics sources - HTTP: %d, SGCSF: %d", 
			satelliteID, httpCount, sgcsfCount)
	}
}

func (c *GroundHTTPClient) startWebDashboard() {
	mux := http.NewServeMux()

	// 仪表板主页
	mux.HandleFunc("/", c.handleDashboard)
	
	// 指标API
	mux.HandleFunc("/api/metrics", c.handleMetricsAPI)
	
	// 卫星状态API
	mux.HandleFunc("/api/satellites", c.handleSatellitesAPI)
	
	// 实时数据API
	mux.HandleFunc("/api/realtime", c.handleRealtimeAPI)

	server := &http.Server{
		Addr:    ":8081",
		Handler: mux,
	}

	go func() {
		log.Printf("🌐 Web dashboard started on http://localhost:8081")
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("Dashboard server error: %v", err)
		}
	}()
}

func (c *GroundHTTPClient) handleDashboard(w http.ResponseWriter, r *http.Request) {
	html := `
<!DOCTYPE html>
<html>
<head>
    <title>SGCSF Ground Control Dashboard</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        .header { background: #2c3e50; color: white; padding: 20px; margin-bottom: 20px; }
        .metrics { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 20px; }
        .card { border: 1px solid #ddd; padding: 15px; border-radius: 5px; }
        .metric-value { font-size: 24px; font-weight: bold; color: #3498db; }
        .status-online { color: #27ae60; }
        .status-offline { color: #e74c3c; }
    </style>
</head>
<body>
    <div class="header">
        <h1>🌍 SGCSF Ground Control Dashboard</h1>
        <p>Monitoring satellite systems via HTTP and SGCSF protocols</p>
    </div>
    <div class="metrics">
        <div class="card">
            <h3>📡 Connection Status</h3>
            <p>HTTP: <span class="status-online">Online</span></p>
            <p>SGCSF: <span class="status-online">Connected</span></p>
        </div>
        <div class="card">
            <h3>🛰️ Satellites</h3>
            <p>Active: <span class="metric-value">1</span></p>
            <p>Total: <span class="metric-value">1</span></p>
        </div>
        <div class="card">
            <h3>📊 Data Sources</h3>
            <a href="/api/metrics">Metrics API</a><br>
            <a href="/api/satellites">Satellites API</a><br>
            <a href="/api/realtime">Realtime API</a>
        </div>
    </div>
    <script>
        // 这里可以添加实时数据更新的JavaScript
        setInterval(() => {
            console.log('Fetching latest metrics...');
        }, 5000);
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func (c *GroundHTTPClient) handleMetricsAPI(w http.ResponseWriter, r *http.Request) {
	c.metricsStore.mutex.RLock()
	defer c.metricsStore.mutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(c.metricsStore.metrics)
}

func (c *GroundHTTPClient) handleSatellitesAPI(w http.ResponseWriter, r *http.Request) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	satellites := make([]map[string]interface{}, 0)
	for satID, endpoint := range c.satellites {
		satellites = append(satellites, map[string]interface{}{
			"id":       satID,
			"endpoint": endpoint,
			"status":   "online",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"satellites": satellites,
		"total":      len(satellites),
	})
}

func (c *GroundHTTPClient) handleRealtimeAPI(w http.ResponseWriter, r *http.Request) {
	// 实时数据接口，可以用WebSocket实现
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"status":    "realtime data endpoint",
	})
}

func (c *GroundHTTPClient) shutdown() {
	c.mutex.Lock()
	c.running = false
	c.mutex.Unlock()

	close(c.alertsChannel)
	
	if c.sgcsfClient != nil {
		c.sgcsfClient.Disconnect()
	}
	
	fmt.Println("✅ Ground HTTP Client shut down gracefully")
}

func getSatelliteIDs(satellites map[string]string) []string {
	ids := make([]string, 0, len(satellites))
	for id := range satellites {
		ids = append(ids, id)
	}
	return ids
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}