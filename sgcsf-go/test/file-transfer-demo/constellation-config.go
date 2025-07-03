package main

import (
	"fmt"
	"time"
)

// ConstellationConfig 卫星星座配置
type ConstellationConfig struct {
	Satellites map[string]*SatelliteConfig `json:"satellites"`
}

// SatelliteConfig 单颗卫星配置
type SatelliteConfig struct {
	ID                string                    `json:"id"`
	Name              string                    `json:"name"`
	Nodes             map[string]*NodeConfig    `json:"nodes"`
	OrbitInfo         *OrbitInfo               `json:"orbit_info"`
	CommunicationWindow *CommunicationWindow   `json:"communication_window"`
	StorageCapacity   int64                    `json:"storage_capacity_mb"`
	BandwidthMbps     float64                  `json:"bandwidth_mbps"`
}

// NodeConfig 节点配置
type NodeConfig struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Type          string            `json:"type"` // "primary", "backup", "storage"
	StoragePath   string            `json:"storage_path"`
	MaxFileSize   int64             `json:"max_file_size_mb"`
	Capabilities  []string          `json:"capabilities"`
	Status        string            `json:"status"` // "online", "offline", "maintenance"
}

// OrbitInfo 轨道信息
type OrbitInfo struct {
	Altitude          float64 `json:"altitude_km"`
	Inclination       float64 `json:"inclination_deg"`
	Period            int     `json:"period_minutes"`
	CurrentLatitude   float64 `json:"current_latitude"`
	CurrentLongitude  float64 `json:"current_longitude"`
}

// CommunicationWindow 通信窗口
type CommunicationWindow struct {
	OnlineDuration    time.Duration `json:"online_duration"`
	OfflineDuration   time.Duration `json:"offline_duration"`
	NextOnlineTime    time.Time     `json:"next_online_time"`
	LastOnlineTime    time.Time     `json:"last_online_time"`
	LatencyRange      LatencyRange  `json:"latency_range"`
}

// LatencyRange 延迟范围
type LatencyRange struct {
	MinMs int `json:"min_ms"`
	MaxMs int `json:"max_ms"`
}

// GetConstellationConfig 获取卫星星座配置
func GetConstellationConfig() *ConstellationConfig {
	config := &ConstellationConfig{
		Satellites: make(map[string]*SatelliteConfig),
	}

	// 卫星1: 3个节点
	config.Satellites["sat-01"] = &SatelliteConfig{
		ID:   "sat-01",
		Name: "Communications Satellite Alpha",
		Nodes: map[string]*NodeConfig{
			"node-1": {
				ID:           "node-1",
				Name:         "Primary Data Processing Node",
				Type:         "primary",
				StoragePath:  "/satellite/sat-01/storage/node-1",
				MaxFileSize:  500, // 500MB
				Capabilities: []string{"file_transfer", "data_processing", "backup"},
				Status:       "offline",
			},
			"node-2": {
				ID:           "node-2", 
				Name:         "Backup Communication Node",
				Type:         "backup",
				StoragePath:  "/satellite/sat-01/storage/node-2",
				MaxFileSize:  300, // 300MB
				Capabilities: []string{"file_transfer", "communication", "relay"},
				Status:       "offline",
			},
			"node-3": {
				ID:           "node-3",
				Name:         "Storage Archive Node",
				Type:         "storage",
				StoragePath:  "/satellite/sat-01/storage/node-3",
				MaxFileSize:  1000, // 1GB
				Capabilities: []string{"file_transfer", "long_term_storage", "archive"},
				Status:       "offline",
			},
		},
		OrbitInfo: &OrbitInfo{
			Altitude:         400.5,
			Inclination:      51.6,
			Period:          93,
			CurrentLatitude:  45.2,
			CurrentLongitude: 120.3,
		},
		CommunicationWindow: &CommunicationWindow{
			OnlineDuration:  2 * time.Minute,
			OfflineDuration: 8 * time.Minute,
			NextOnlineTime:  time.Now().Add(2 * time.Minute),
			LatencyRange:    LatencyRange{MinMs: 200, MaxMs: 500},
		},
		StorageCapacity: 2048, // 2GB total
		BandwidthMbps:   3.5,
	}

	// 卫星2: 3个节点
	config.Satellites["sat-02"] = &SatelliteConfig{
		ID:   "sat-02",
		Name: "Earth Observation Satellite Beta",
		Nodes: map[string]*NodeConfig{
			"node-1": {
				ID:           "node-1",
				Name:         "Image Processing Node",
				Type:         "primary",
				StoragePath:  "/satellite/sat-02/storage/node-1",
				MaxFileSize:  800, // 800MB
				Capabilities: []string{"file_transfer", "image_processing", "compression"},
				Status:       "offline",
			},
			"node-2": {
				ID:           "node-2",
				Name:         "Data Relay Node", 
				Type:         "backup",
				StoragePath:  "/satellite/sat-02/storage/node-2",
				MaxFileSize:  400, // 400MB
				Capabilities: []string{"file_transfer", "data_relay", "routing"},
				Status:       "offline",
			},
			"node-3": {
				ID:           "node-3",
				Name:         "Archive Storage Node",
				Type:         "storage",
				StoragePath:  "/satellite/sat-02/storage/node-3",
				MaxFileSize:  1500, // 1.5GB
				Capabilities: []string{"file_transfer", "massive_storage", "redundancy"},
				Status:       "offline",
			},
		},
		OrbitInfo: &OrbitInfo{
			Altitude:         450.2,
			Inclination:      97.8,
			Period:          94,
			CurrentLatitude:  -23.5,
			CurrentLongitude: 45.7,
		},
		CommunicationWindow: &CommunicationWindow{
			OnlineDuration:  2 * time.Minute,
			OfflineDuration: 8 * time.Minute,
			NextOnlineTime:  time.Now().Add(4 * time.Minute),
			LatencyRange:    LatencyRange{MinMs: 250, MaxMs: 600},
		},
		StorageCapacity: 3072, // 3GB total
		BandwidthMbps:   2.8,
	}

	// 卫星3-12: 每颗1个节点
	satelliteTypes := []struct {
		name      string
		purpose   string
		altitude  float64
		bandwidth float64
		storage   int64
		maxFile   int64
	}{
		{"Navigation Satellite Gamma", "navigation", 380.1, 2.2, 1024, 200},
		{"Weather Monitoring Delta", "weather", 420.8, 1.8, 1536, 300},
		{"Scientific Research Epsilon", "research", 480.3, 4.1, 2048, 500},
		{"Communication Relay Zeta", "communication", 350.7, 3.2, 1024, 250},
		{"Defense Surveillance Eta", "defense", 520.9, 2.5, 1536, 400},
		{"Commercial Broadcast Theta", "broadcast", 390.4, 3.8, 1024, 300},
		{"Experimental Tech Iota", "experimental", 460.2, 1.5, 512, 150},
		{"Disaster Response Kappa", "emergency", 410.6, 2.9, 1024, 350},
		{"Mining Survey Lambda", "mining", 440.1, 2.1, 1536, 400},
		{"Deep Space Relay Mu", "deep_space", 500.5, 1.2, 2048, 600},
	}

	for i, satType := range satelliteTypes {
		satID := fmt.Sprintf("sat-%02d", i+3)
		config.Satellites[satID] = &SatelliteConfig{
			ID:   satID,
			Name: satType.name,
			Nodes: map[string]*NodeConfig{
				"node-1": {
					ID:           "node-1",
					Name:         "Main Processing Node",
					Type:         "primary",
					StoragePath:  fmt.Sprintf("/satellite/%s/storage/node-1", satID),
					MaxFileSize:  satType.maxFile,
					Capabilities: []string{"file_transfer", "data_processing"},
					Status:       "offline",
				},
			},
			OrbitInfo: &OrbitInfo{
				Altitude:         satType.altitude,
				Inclination:      51.6 + float64(i)*2.3,
				Period:          90 + i*2,
				CurrentLatitude:  float64(i*30 - 180),
				CurrentLongitude: float64(i*36 - 180),
			},
			CommunicationWindow: &CommunicationWindow{
				OnlineDuration:  2 * time.Minute,
				OfflineDuration: 8 * time.Minute,
				NextOnlineTime:  time.Now().Add(time.Duration(i*30) * time.Second),
				LatencyRange:    LatencyRange{MinMs: 300 + i*20, MaxMs: 600 + i*30},
			},
			StorageCapacity: satType.storage,
			BandwidthMbps:   satType.bandwidth,
		}
	}

	return config
}

// GetSatelliteInfo 获取单个卫星信息
func (c *ConstellationConfig) GetSatelliteInfo(satelliteID string) (*SatelliteConfig, bool) {
	sat, exists := c.Satellites[satelliteID]
	return sat, exists
}

// GetNodeInfo 获取单个节点信息
func (c *ConstellationConfig) GetNodeInfo(satelliteID, nodeID string) (*NodeConfig, bool) {
	sat, exists := c.Satellites[satelliteID]
	if !exists {
		return nil, false
	}
	
	node, exists := sat.Nodes[nodeID]
	return node, exists
}

// GetOnlineSatellites 获取当前在线的卫星
func (c *ConstellationConfig) GetOnlineSatellites() []string {
	var online []string
	now := time.Now()
	
	for id, sat := range c.Satellites {
		// 检查通信窗口
		if now.After(sat.CommunicationWindow.NextOnlineTime) &&
		   now.Before(sat.CommunicationWindow.NextOnlineTime.Add(sat.CommunicationWindow.OnlineDuration)) {
			online = append(online, id)
		}
	}
	
	return online
}

// UpdateSatelliteStatus 更新卫星状态
func (c *ConstellationConfig) UpdateSatelliteStatus() {
	now := time.Now()
	
	for _, sat := range c.Satellites {
		// 更新通信窗口
		if now.After(sat.CommunicationWindow.NextOnlineTime.Add(sat.CommunicationWindow.OnlineDuration)) {
			// 当前在线窗口已结束，计算下次上线时间
			sat.CommunicationWindow.LastOnlineTime = sat.CommunicationWindow.NextOnlineTime
			sat.CommunicationWindow.NextOnlineTime = now.Add(sat.CommunicationWindow.OfflineDuration)
		}
		
		// 更新节点状态
		isOnline := now.After(sat.CommunicationWindow.NextOnlineTime) &&
		           now.Before(sat.CommunicationWindow.NextOnlineTime.Add(sat.CommunicationWindow.OnlineDuration))
		
		for _, node := range sat.Nodes {
			if isOnline {
				node.Status = "online"
			} else {
				node.Status = "offline"
			}
		}
	}
}

// GetSatelliteStatusSummary 获取星座状态摘要
func (c *ConstellationConfig) GetSatelliteStatusSummary() map[string]interface{} {
	c.UpdateSatelliteStatus()
	
	totalSats := len(c.Satellites)
	onlineSats := len(c.GetOnlineSatellites())
	
	var totalNodes, onlineNodes int
	for _, sat := range c.Satellites {
		totalNodes += len(sat.Nodes)
		for _, node := range sat.Nodes {
			if node.Status == "online" {
				onlineNodes++
			}
		}
	}
	
	return map[string]interface{}{
		"total_satellites":    totalSats,
		"online_satellites":   onlineSats,
		"offline_satellites":  totalSats - onlineSats,
		"total_nodes":        totalNodes,
		"online_nodes":       onlineNodes,
		"offline_nodes":      totalNodes - onlineNodes,
		"constellation_health": float64(onlineNodes) / float64(totalNodes) * 100,
		"last_updated":       time.Now().Unix(),
	}
}