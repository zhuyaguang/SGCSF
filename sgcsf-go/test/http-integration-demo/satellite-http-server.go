package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
	"github.com/sgcsf/sgcsf-go/pkg/core"
)

// SystemMetrics 模拟node-exporter的系统指标
type SystemMetrics struct {
	Timestamp    int64                  `json:"timestamp"`
	SatelliteID  string                 `json:"satellite_id"`
	NodeInfo     NodeInfo               `json:"node_info"`
	CPUMetrics   CPUMetrics             `json:"cpu_metrics"`
	MemoryMetrics MemoryMetrics         `json:"memory_metrics"`
	DiskMetrics  DiskMetrics            `json:"disk_metrics"`
	NetworkMetrics NetworkMetrics       `json:"network_metrics"`
	CustomMetrics map[string]interface{} `json:"custom_metrics"`
}

type NodeInfo struct {
	Hostname     string `json:"hostname"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	KernelVersion string `json:"kernel_version"`
	Uptime       int64  `json:"uptime_seconds"`
}

type CPUMetrics struct {
	UsagePercent float64 `json:"usage_percent"`
	UserTime     float64 `json:"user_time"`
	SystemTime   float64 `json:"system_time"`
	IdleTime     float64 `json:"idle_time"`
	LoadAverage  float64 `json:"load_average"`
	CoreCount    int     `json:"core_count"`
}

type MemoryMetrics struct {
	TotalBytes     int64   `json:"total_bytes"`
	UsedBytes      int64   `json:"used_bytes"`
	FreeBytes      int64   `json:"free_bytes"`
	UsagePercent   float64 `json:"usage_percent"`
	SwapTotal      int64   `json:"swap_total_bytes"`
	SwapUsed       int64   `json:"swap_used_bytes"`
}

type DiskMetrics struct {
	TotalBytes     int64   `json:"total_bytes"`
	UsedBytes      int64   `json:"used_bytes"`
	FreeBytes      int64   `json:"free_bytes"`
	UsagePercent   float64 `json:"usage_percent"`
	IOPSRead       float64 `json:"iops_read"`
	IOPSWrite      float64 `json:"iops_write"`
}

type NetworkMetrics struct {
	BytesReceived    int64   `json:"bytes_received"`
	BytesSent        int64   `json:"bytes_sent"`
	PacketsReceived  int64   `json:"packets_received"`
	PacketsSent      int64   `json:"packets_sent"`
	ErrorsReceived   int64   `json:"errors_received"`
	ErrorsSent       int64   `json:"errors_sent"`
	Bandwidth        float64 `json:"bandwidth_mbps"`
}

// SatelliteHTTPServer 卫星HTTP服务器，模拟node-exporter
type SatelliteHTTPServer struct {
	sgcsfClient  core.SGCSFClient
	httpServer   *http.Server
	satelliteID  string
	startTime    time.Time
	metricsCache *SystemMetrics
}

func main() {
	fmt.Println("🛰️ Starting Satellite HTTP Server (node-exporter simulation)")

	satelliteID := getEnv("SATELLITE_ID", "sat-demo")
	httpPort := getEnv("HTTP_PORT", ":9100")
	sgcsfServer := getEnv("SGCSF_SERVER", "localhost:8080") // 连接到卫星网关

	server := &SatelliteHTTPServer{
		satelliteID: satelliteID,
		startTime:   time.Now(),
	}

	// 创建SGCSF客户端连接到卫星网关
	server.sgcsfClient = core.SatelliteClient(
		fmt.Sprintf("satellite-%s-httpserver", satelliteID),
		sgcsfServer,
	)

	// 连接到SGCSF
	ctx := context.Background()
	err := server.sgcsfClient.Connect(ctx)
	if err != nil {
		log.Printf("⚠️ Failed to connect to SGCSF: %v (running in standalone mode)", err)
	} else {
		fmt.Println("✅ Connected to SGCSF satellite gateway")
		// 订阅来自地面的指标请求
		server.setupSGCSFSubscriptions()
	}

	// 启动HTTP服务器
	server.startHTTPServer(httpPort)

	// 启动指标更新
	go server.startMetricsUpdater()

	fmt.Printf("✅ Satellite HTTP Server started\n")
	fmt.Printf("📡 Satellite ID: %s\n", satelliteID)
	fmt.Printf("🌐 HTTP API: http://localhost%s\n", httpPort)
	fmt.Printf("📊 Metrics endpoint: http://localhost%s/metrics\n", httpPort)

	// 等待关闭信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\n🛑 Shutting down satellite server...")
	server.shutdown()
}

func (s *SatelliteHTTPServer) startHTTPServer(port string) {
	mux := http.NewServeMux()

	// 主要指标端点 (模拟node-exporter的/metrics)
	mux.HandleFunc("/metrics", s.handleMetrics)
	
	// Prometheus格式的指标
	mux.HandleFunc("/metrics/prometheus", s.handlePrometheusMetrics)
	
	// 系统信息端点
	mux.HandleFunc("/info", s.handleInfo)
	
	// 健康检查端点
	mux.HandleFunc("/health", s.handleHealth)
	
	// 自定义指标端点
	mux.HandleFunc("/metrics/custom", s.handleCustomMetrics)

	s.httpServer = &http.Server{
		Addr:    port,
		Handler: mux,
	}

	go func() {
		if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()
}

func (s *SatelliteHTTPServer) setupSGCSFSubscriptions() {
	// 订阅来自地面的指标收集请求
	metricsRequestTopic := fmt.Sprintf("/satellite/%s/metrics/request", s.satelliteID)
	
	_, err := s.sgcsfClient.Subscribe(metricsRequestTopic, types.MessageHandlerFunc(func(message *types.SGCSFMessage) error {
		log.Printf("📨 Received metrics request from ground via SGCSF")
		
		// 解析请求
		var request map[string]interface{}
		if err := json.Unmarshal(message.Payload, &request); err != nil {
			log.Printf("❌ Failed to parse metrics request: %v", err)
			return err
		}

		// 获取当前指标
		metrics := s.getCurrentMetrics()
		
		// 通过SGCSF发送响应
		response := core.NewMessage().
			Topic(fmt.Sprintf("/ground/metrics/response/%s", s.satelliteID)).
			Type(types.MessageTypeResponse).
			Priority(types.PriorityNormal).
			QoS(types.QoSAtLeastOnce).
			ContentType("application/json").
			Source(fmt.Sprintf("satellite-%s-httpserver", s.satelliteID)).
			Metadata("request_id", request["request_id"]).
			Metadata("satellite_id", s.satelliteID).
			Payload(metrics).
			Build()

		return s.sgcsfClient.SendResponse(message, response)
	}))

	if err != nil {
		log.Printf("❌ Failed to subscribe to metrics requests: %v", err)
	} else {
		fmt.Printf("🎯 Subscribed to metrics requests: %s\n", metricsRequestTopic)
	}

	// 订阅配置更新
	configTopic := fmt.Sprintf("/satellite/%s/config/update", s.satelliteID)
	s.sgcsfClient.Subscribe(configTopic, types.MessageHandlerFunc(func(message *types.SGCSFMessage) error {
		log.Printf("⚙️ Received configuration update via SGCSF")
		// 处理配置更新逻辑
		return nil
	}))
}

func (s *SatelliteHTTPServer) startMetricsUpdater() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.metricsCache = s.collectSystemMetrics()
		
		// 如果连接到SGCSF，主动推送重要指标
		if s.sgcsfClient != nil {
			s.publishCriticalMetrics()
		}
	}
}

func (s *SatelliteHTTPServer) collectSystemMetrics() *SystemMetrics {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	now := time.Now()
	uptime := int64(now.Sub(s.startTime).Seconds())

	return &SystemMetrics{
		Timestamp:   now.Unix(),
		SatelliteID: s.satelliteID,
		NodeInfo: NodeInfo{
			Hostname:      "sat-node-" + s.satelliteID,
			OS:            runtime.GOOS,
			Architecture:  runtime.GOARCH,
			KernelVersion: "5.4.0-satellite",
			Uptime:        uptime,
		},
		CPUMetrics: CPUMetrics{
			UsagePercent: 15.0 + rand.Float64()*20.0, // 15-35%
			UserTime:     float64(uptime) * 0.3,
			SystemTime:   float64(uptime) * 0.1,
			IdleTime:     float64(uptime) * 0.6,
			LoadAverage:  0.5 + rand.Float64()*1.0, // 0.5-1.5
			CoreCount:    runtime.NumCPU(),
		},
		MemoryMetrics: MemoryMetrics{
			TotalBytes:   int64(m.Sys),
			UsedBytes:    int64(m.Alloc),
			FreeBytes:    int64(m.Sys - m.Alloc),
			UsagePercent: float64(m.Alloc) / float64(m.Sys) * 100,
			SwapTotal:    1024 * 1024 * 1024, // 1GB
			SwapUsed:     int64(rand.Float64() * 100 * 1024 * 1024), // 0-100MB
		},
		DiskMetrics: DiskMetrics{
			TotalBytes:   100 * 1024 * 1024 * 1024, // 100GB
			UsedBytes:    int64(30 + rand.Float64()*20) * 1024 * 1024 * 1024, // 30-50GB
			FreeBytes:    int64(50 + rand.Float64()*20) * 1024 * 1024 * 1024, // 50-70GB
			UsagePercent: 30.0 + rand.Float64()*20.0, // 30-50%
			IOPSRead:     100 + rand.Float64()*100,   // 100-200 IOPS
			IOPSWrite:    50 + rand.Float64()*50,     // 50-100 IOPS
		},
		NetworkMetrics: NetworkMetrics{
			BytesReceived:   int64(uptime * 1024 * (100 + rand.Float64()*100)), // 变化的网络流量
			BytesSent:       int64(uptime * 1024 * (50 + rand.Float64()*50)),
			PacketsReceived: int64(uptime * (1000 + rand.Float64()*500)),
			PacketsSent:     int64(uptime * (800 + rand.Float64()*400)),
			ErrorsReceived:  int64(rand.Float64() * 10),
			ErrorsSent:      int64(rand.Float64() * 5),
			Bandwidth:       10.0 + rand.Float64()*5.0, // 10-15 Mbps
		},
		CustomMetrics: map[string]interface{}{
			"satellite_temperature_celsius": 20.0 + rand.Float64()*15.0,
			"solar_panel_voltage":           12.0 + rand.Float64()*2.0,
			"battery_level_percent":         80.0 + rand.Float64()*20.0,
			"signal_strength_dbm":           -80.0 + rand.Float64()*20.0,
			"orbital_altitude_km":           400.0 + rand.Float64()*50.0,
			"communication_latency_ms":      200.0 + rand.Float64()*100.0,
		},
	}
}

func (s *SatelliteHTTPServer) getCurrentMetrics() *SystemMetrics {
	if s.metricsCache == nil {
		s.metricsCache = s.collectSystemMetrics()
	}
	return s.metricsCache
}

func (s *SatelliteHTTPServer) publishCriticalMetrics() {
	metrics := s.getCurrentMetrics()
	
	// 检查是否有关键指标超出阈值
	criticalAlerts := []map[string]interface{}{}
	
	if metrics.CPUMetrics.UsagePercent > 80 {
		criticalAlerts = append(criticalAlerts, map[string]interface{}{
			"type":      "cpu_high_usage",
			"value":     metrics.CPUMetrics.UsagePercent,
			"threshold": 80.0,
			"severity":  "warning",
		})
	}
	
	if metrics.MemoryMetrics.UsagePercent > 85 {
		criticalAlerts = append(criticalAlerts, map[string]interface{}{
			"type":      "memory_high_usage",
			"value":     metrics.MemoryMetrics.UsagePercent,
			"threshold": 85.0,
			"severity":  "critical",
		})
	}
	
	if temp, ok := metrics.CustomMetrics["satellite_temperature_celsius"].(float64); ok && temp > 30 {
		criticalAlerts = append(criticalAlerts, map[string]interface{}{
			"type":      "high_temperature",
			"value":     temp,
			"threshold": 30.0,
			"severity":  "warning",
		})
	}

	// 如果有关键告警，立即推送
	if len(criticalAlerts) > 0 {
		alertMessage := core.NewMessage().
			Topic(fmt.Sprintf("/ground/alerts/%s", s.satelliteID)).
			Type(types.MessageTypeAsync).
			Priority(types.PriorityCritical).
			QoS(types.QoSExactlyOnce).
			ContentType("application/json").
			Source(fmt.Sprintf("satellite-%s-httpserver", s.satelliteID)).
			Metadata("alert_type", "critical_metrics").
			Metadata("satellite_id", s.satelliteID).
			TTL(30 * time.Minute).
			Payload(map[string]interface{}{
				"timestamp": time.Now().Unix(),
				"satellite_id": s.satelliteID,
				"alerts": criticalAlerts,
				"metrics_snapshot": metrics,
			}).
			Build()

		err := s.sgcsfClient.Publish(alertMessage.Topic, alertMessage)
		if err != nil {
			log.Printf("❌ Failed to publish critical alert: %v", err)
		} else {
			log.Printf("🚨 Published %d critical alerts to ground", len(criticalAlerts))
		}
	}
}

// HTTP处理函数

func (s *SatelliteHTTPServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metrics := s.getCurrentMetrics()
	
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Satellite-ID", s.satelliteID)
	w.Header().Set("X-Timestamp", fmt.Sprintf("%d", metrics.Timestamp))
	
	if err := json.NewEncoder(w).Encode(metrics); err != nil {
		http.Error(w, "Failed to encode metrics", http.StatusInternalServerError)
		return
	}

	log.Printf("📊 Served metrics via HTTP to %s", r.RemoteAddr)
}

func (s *SatelliteHTTPServer) handlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := s.getCurrentMetrics()
	
	w.Header().Set("Content-Type", "text/plain")
	
	// 生成Prometheus格式的指标
	fmt.Fprintf(w, "# HELP satellite_cpu_usage_percent CPU usage percentage\n")
	fmt.Fprintf(w, "# TYPE satellite_cpu_usage_percent gauge\n")
	fmt.Fprintf(w, "satellite_cpu_usage_percent{satellite_id=\"%s\"} %.2f\n", s.satelliteID, metrics.CPUMetrics.UsagePercent)
	
	fmt.Fprintf(w, "# HELP satellite_memory_usage_percent Memory usage percentage\n")
	fmt.Fprintf(w, "# TYPE satellite_memory_usage_percent gauge\n")
	fmt.Fprintf(w, "satellite_memory_usage_percent{satellite_id=\"%s\"} %.2f\n", s.satelliteID, metrics.MemoryMetrics.UsagePercent)
	
	fmt.Fprintf(w, "# HELP satellite_disk_usage_percent Disk usage percentage\n")
	fmt.Fprintf(w, "# TYPE satellite_disk_usage_percent gauge\n")
	fmt.Fprintf(w, "satellite_disk_usage_percent{satellite_id=\"%s\"} %.2f\n", s.satelliteID, metrics.DiskMetrics.UsagePercent)
	
	fmt.Fprintf(w, "# HELP satellite_temperature_celsius Satellite temperature in Celsius\n")
	fmt.Fprintf(w, "# TYPE satellite_temperature_celsius gauge\n")
	fmt.Fprintf(w, "satellite_temperature_celsius{satellite_id=\"%s\"} %.2f\n", s.satelliteID, metrics.CustomMetrics["satellite_temperature_celsius"])
	
	fmt.Fprintf(w, "# HELP satellite_uptime_seconds Satellite uptime in seconds\n")
	fmt.Fprintf(w, "# TYPE satellite_uptime_seconds counter\n")
	fmt.Fprintf(w, "satellite_uptime_seconds{satellite_id=\"%s\"} %d\n", s.satelliteID, metrics.NodeInfo.Uptime)

	log.Printf("📊 Served Prometheus metrics via HTTP to %s", r.RemoteAddr)
}

func (s *SatelliteHTTPServer) handleInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]interface{}{
		"satellite_id": s.satelliteID,
		"server_type": "node-exporter-simulation",
		"version":     "1.0.0",
		"uptime":      time.Since(s.startTime).String(),
		"sgcsf_connected": s.sgcsfClient != nil,
		"endpoints": []string{
			"/metrics",
			"/metrics/prometheus", 
			"/info",
			"/health",
			"/metrics/custom",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (s *SatelliteHTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":       "healthy",
		"timestamp":    time.Now().Unix(),
		"satellite_id": s.satelliteID,
		"uptime":       time.Since(s.startTime).String(),
		"sgcsf_status": func() string {
			if s.sgcsfClient != nil {
				return "connected"
			}
			return "disconnected"
		}(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(health)
}

func (s *SatelliteHTTPServer) handleCustomMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := s.getCurrentMetrics()
	
	response := map[string]interface{}{
		"satellite_id": s.satelliteID,
		"timestamp":    metrics.Timestamp,
		"custom_metrics": metrics.CustomMetrics,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *SatelliteHTTPServer) shutdown() {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.httpServer.Shutdown(ctx)
	}
	
	if s.sgcsfClient != nil {
		s.sgcsfClient.Disconnect()
	}
	
	fmt.Println("✅ Satellite HTTP Server shut down gracefully")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}