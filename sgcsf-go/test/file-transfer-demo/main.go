package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// FileTransferDemo 文件传输演示主程序
type FileTransferDemo struct {
	groundManager       *GroundFileManager
	satelliteAgents     map[string]*SatelliteFileAgent
	constellation       *ConstellationConfig
	serverAddr          string
	
	// 演示控制
	ctx                 context.Context
	cancel              context.CancelFunc
	wg                  sync.WaitGroup
	
	// 离线模拟
	offlineSimulator    *OfflineSimulator
}

// OfflineSimulator 离线模拟器
type OfflineSimulator struct {
	constellation *ConstellationConfig
	isRunning     bool
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewOfflineSimulator 创建离线模拟器
func NewOfflineSimulator(constellation *ConstellationConfig) *OfflineSimulator {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &OfflineSimulator{
		constellation: constellation,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start 启动离线模拟
func (os *OfflineSimulator) Start() {
	os.mu.Lock()
	if os.isRunning {
		os.mu.Unlock()
		return
	}
	os.isRunning = true
	os.mu.Unlock()
	
	go os.simulateOrbitCycles()
	log.Println("Offline simulator started")
}

// Stop 停止离线模拟
func (os *OfflineSimulator) Stop() {
	os.mu.Lock()
	if !os.isRunning {
		os.mu.Unlock()
		return
	}
	os.isRunning = false
	os.mu.Unlock()
	
	os.cancel()
	log.Println("Offline simulator stopped")
}

// simulateOrbitCycles 模拟轨道周期
func (os *OfflineSimulator) simulateOrbitCycles() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-os.ctx.Done():
			return
		case <-ticker.C:
			os.updateSatelliteStatus()
		}
	}
}

// updateSatelliteStatus 更新卫星状态
func (os *OfflineSimulator) updateSatelliteStatus() {
	os.constellation.UpdateSatelliteStatus()
	
	// 添加一些随机性来模拟真实的卫星通信窗口
	now := time.Now()
	
	for _, sat := range os.constellation.Satellites {
		// 模拟80%的时间离线
		if now.Unix()%10 < 2 { // 20%的时间在线
			for _, node := range sat.Nodes {
				node.Status = "online"
			}
		} else {
			for _, node := range sat.Nodes {
				node.Status = "offline"
			}
		}
	}
}

// main 主函数
func main() {
	fmt.Println("=== SGCSF 文件传输演示系统 ===")
	fmt.Println()
	
	// 创建演示实例
	demo := &FileTransferDemo{
		serverAddr: "localhost:8080",
	}
	
	if err := demo.initialize(); err != nil {
		log.Fatalf("Failed to initialize demo: %v", err)
	}
	
	// 启动演示
	if err := demo.start(); err != nil {
		log.Fatalf("Failed to start demo: %v", err)
	}
	
	// 运行交互式命令行
	demo.runInteractiveMode()
	
	// 清理资源
	demo.stop()
	
	fmt.Println("演示结束")
}

// initialize 初始化演示系统
func (demo *FileTransferDemo) initialize() error {
	demo.ctx, demo.cancel = context.WithCancel(context.Background())
	demo.constellation = GetConstellationConfig()
	demo.satelliteAgents = make(map[string]*SatelliteFileAgent)
	
	// 创建测试数据目录
	if err := demo.createTestData(); err != nil {
		return fmt.Errorf("failed to create test data: %v", err)
	}
	
	// 创建地面文件管理器
	demo.groundManager = NewGroundFileManager("ground-control", demo.serverAddr)
	
	// 创建卫星文件代理
	for satelliteID, satellite := range demo.constellation.Satellites {
		for nodeID := range satellite.Nodes {
			agentKey := fmt.Sprintf("%s-%s", satelliteID, nodeID)
			agent := NewSatelliteFileAgent(satelliteID, nodeID, demo.serverAddr)
			demo.satelliteAgents[agentKey] = agent
		}
	}
	
	// 创建离线模拟器
	demo.offlineSimulator = NewOfflineSimulator(demo.constellation)
	
	return nil
}

// start 启动演示系统
func (demo *FileTransferDemo) start() error {
	log.Println("启动文件传输演示系统...")
	
	// 启动地面文件管理器
	if err := demo.groundManager.Start(); err != nil {
		return fmt.Errorf("failed to start ground manager: %v", err)
	}
	
	// 启动所有卫星文件代理
	for agentKey, agent := range demo.satelliteAgents {
		if err := agent.Start(); err != nil {
			return fmt.Errorf("failed to start agent %s: %v", agentKey, err)
		}
	}
	
	// 启动离线模拟器
	demo.offlineSimulator.Start()
	
	// 等待系统稳定
	time.Sleep(2 * time.Second)
	
	log.Println("文件传输演示系统启动完成")
	return nil
}

// stop 停止演示系统
func (demo *FileTransferDemo) stop() {
	log.Println("停止文件传输演示系统...")
	
	demo.cancel()
	
	// 停止离线模拟器
	demo.offlineSimulator.Stop()
	
	// 停止地面文件管理器
	demo.groundManager.Stop()
	
	// 停止所有卫星文件代理
	for _, agent := range demo.satelliteAgents {
		agent.Stop()
	}
	
	demo.wg.Wait()
	
	log.Println("文件传输演示系统已停止")
}

// createTestData 创建测试数据
func (demo *FileTransferDemo) createTestData() error {
	testDataDir := "./test-data"
	uploadsDir := "./uploads"
	
	os.MkdirAll(testDataDir, 0755)
	os.MkdirAll(uploadsDir, 0755)
	
	// 创建不同大小的测试文件
	testFiles := []struct {
		name    string
		size    int
		content string
	}{
		{"config.json", 1024, `{"satellite_id": "sat-01", "config": {"mode": "active", "interval": 30}}`},
		{"system.log", 100 * 1024, ""},
		{"sensor-data.csv", 10 * 1024 * 1024, ""},
		{"firmware-update.bin", 50 * 1024 * 1024, ""},
	}
	
	for _, tf := range testFiles {
		filePath := filepath.Join(testDataDir, tf.name)
		
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			file, err := os.Create(filePath)
			if err != nil {
				return err
			}
			
			if tf.content != "" {
				file.WriteString(tf.content)
			} else {
				// 写入随机数据
				data := make([]byte, 1024)
				for i := range data {
					data[i] = byte(i % 256)
				}
				
				for written := 0; written < tf.size; written += len(data) {
					remaining := tf.size - written
					if remaining < len(data) {
						file.Write(data[:remaining])
					} else {
						file.Write(data)
					}
				}
			}
			
			file.Close()
			log.Printf("Created test file: %s (%d bytes)", tf.name, tf.size)
		}
	}
	
	return nil
}

// runInteractiveMode 运行交互式模式
func (demo *FileTransferDemo) runInteractiveMode() {
	scanner := bufio.NewScanner(os.Stdin)
	
	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		<-sigChan
		fmt.Println("\n接收到退出信号，正在停止系统...")
		demo.cancel()
	}()
	
	demo.showHelp()
	
	for {
		select {
		case <-demo.ctx.Done():
			return
		default:
		}
		
		fmt.Print("\nSGCSF> ")
		if !scanner.Scan() {
			break
		}
		
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		
		parts := strings.Fields(input)
		if len(parts) == 0 {
			continue
		}
		
		command := parts[0]
		args := parts[1:]
		
		switch command {
		case "help", "h":
			demo.showHelp()
			
		case "status", "s":
			demo.showStatus()
			
		case "satellites", "sat":
			demo.showSatellites()
			
		case "upload", "up":
			demo.handleUploadCommand(args)
			
		case "download", "dl":
			demo.handleDownloadCommand(args)
			
		case "transfers", "t":
			demo.showTransfers()
			
		case "online":
			demo.handleOnlineCommand(args)
			
		case "offline":
			demo.handleOfflineCommand(args)
			
		case "simulate":
			demo.handleSimulateCommand(args)
			
		case "quit", "exit", "q":
			fmt.Println("正在退出...")
			demo.cancel()
			return
			
		default:
			fmt.Printf("未知命令: %s\n", command)
			fmt.Println("输入 'help' 查看可用命令")
		}
	}
}

// showHelp 显示帮助信息
func (demo *FileTransferDemo) showHelp() {
	fmt.Println("\n=== SGCSF 文件传输演示 - 命令帮助 ===")
	fmt.Println()
	fmt.Println("系统状态:")
	fmt.Println("  status, s                    - 显示系统总体状态")
	fmt.Println("  satellites, sat              - 显示卫星状态")
	fmt.Println("  transfers, t                 - 显示传输状态")
	fmt.Println()
	fmt.Println("文件传输:")
	fmt.Println("  upload <file> <sat> <node>   - 上传文件到卫星")
	fmt.Println("  download <file> <sat> <node> - 从卫星下载文件")
	fmt.Println()
	fmt.Println("卫星控制 (用于测试):")
	fmt.Println("  online <satellite_id>        - 强制卫星上线")
	fmt.Println("  offline <satellite_id>       - 强制卫星离线")
	fmt.Println()
	fmt.Println("离线场景模拟:")
	fmt.Println("  simulate start               - 开始离线场景模拟")
	fmt.Println("  simulate stop                - 停止离线场景模拟")
	fmt.Println("  simulate status              - 显示模拟器状态")
	fmt.Println()
	fmt.Println("其他:")
	fmt.Println("  help, h                      - 显示此帮助")
	fmt.Println("  quit, exit, q                - 退出程序")
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println("  upload test-data/config.json sat-01 node-1")
	fmt.Println("  download system.log sat-02 node-1")
	fmt.Println("  online sat-01")
	fmt.Println()
}

// showStatus 显示系统状态
func (demo *FileTransferDemo) showStatus() {
	fmt.Println("\n=== 系统状态 ===")
	
	// 星座状态
	status := demo.constellation.GetSatelliteStatusSummary()
	fmt.Printf("卫星总数: %v\n", status["total_satellites"])
	fmt.Printf("在线卫星: %v\n", status["online_satellites"])
	fmt.Printf("离线卫星: %v\n", status["offline_satellites"])
	fmt.Printf("节点总数: %v\n", status["total_nodes"])
	fmt.Printf("在线节点: %v\n", status["online_nodes"])
	fmt.Printf("离线节点: %v\n", status["offline_nodes"])
	fmt.Printf("星座健康度: %.1f%%\n", status["constellation_health"])
	
	// 地面管理器状态
	fmt.Printf("\n地面文件管理器: 运行中\n")
	fmt.Printf("卫星代理数量: %d\n", len(demo.satelliteAgents))
	
	// 离线模拟器状态
	demo.offlineSimulator.mu.RLock()
	simulatorStatus := "停止"
	if demo.offlineSimulator.isRunning {
		simulatorStatus = "运行中"
	}
	demo.offlineSimulator.mu.RUnlock()
	fmt.Printf("离线模拟器: %s\n", simulatorStatus)
}

// showSatellites 显示卫星详细状态
func (demo *FileTransferDemo) showSatellites() {
	fmt.Println("\n=== 卫星状态详情 ===")
	
	for satID, sat := range demo.constellation.Satellites {
		fmt.Printf("\n%s (%s)\n", satID, sat.Name)
		fmt.Printf("  轨道高度: %.1f km\n", sat.OrbitInfo.Altitude)
		fmt.Printf("  存储容量: %d MB\n", sat.StorageCapacity)
		fmt.Printf("  带宽: %.1f Mbps\n", sat.BandwidthMbps)
		
		// 通信窗口信息
		fmt.Printf("  通信窗口: 在线%v, 离线%v\n", 
			sat.CommunicationWindow.OnlineDuration,
			sat.CommunicationWindow.OfflineDuration)
		
		// 下次上线时间
		nextOnline := sat.CommunicationWindow.NextOnlineTime
		if time.Now().Before(nextOnline) {
			fmt.Printf("  下次上线: %s (还有%v)\n", 
				nextOnline.Format("15:04:05"),
				nextOnline.Sub(time.Now()).Round(time.Second))
		} else {
			fmt.Printf("  当前: 在线中\n")
		}
		
		// 节点状态
		fmt.Printf("  节点:\n")
		for nodeID, node := range sat.Nodes {
			status := "🔴 离线"
			if node.Status == "online" {
				status = "🟢 在线"
			}
			fmt.Printf("    %s (%s): %s\n", nodeID, node.Name, status)
		}
	}
}

// showTransfers 显示传输状态
func (demo *FileTransferDemo) showTransfers() {
	fmt.Println("\n=== 文件传输状态 ===")
	
	// 这里应该调用地面管理器的API获取传输状态
	// 为了简化，我们显示一些基本信息
	fmt.Println("活跃传输: 0")
	fmt.Println("队列中的传输: 0")
	fmt.Println("离线队列: 0")
	fmt.Println("\n使用 HTTP API 获取详细传输信息:")
	fmt.Println("curl http://localhost:8082/api/transfers")
}

// handleUploadCommand 处理上传命令
func (demo *FileTransferDemo) handleUploadCommand(args []string) {
	if len(args) < 3 {
		fmt.Println("用法: upload <本地文件路径> <卫星ID> <节点ID> [远程路径]")
		fmt.Println("示例: upload test-data/config.json sat-01 node-1")
		return
	}
	
	localPath := args[0]
	satelliteID := args[1]
	nodeID := args[2]
	
	var remotePath string
	if len(args) > 3 {
		remotePath = args[3]
	} else {
		remotePath = filepath.Base(localPath)
	}
	
	// 检查本地文件是否存在
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		fmt.Printf("错误: 本地文件不存在: %s\n", localPath)
		return
	}
	
	// 检查卫星和节点是否存在
	if _, exists := demo.constellation.GetNodeInfo(satelliteID, nodeID); !exists {
		fmt.Printf("错误: 卫星节点不存在: %s/%s\n", satelliteID, nodeID)
		return
	}
	
	fmt.Printf("开始上传: %s -> %s/%s:%s\n", localPath, satelliteID, nodeID, remotePath)
	
	request, err := demo.groundManager.UploadFile(localPath, remotePath, satelliteID, nodeID)
	if err != nil {
		fmt.Printf("上传失败: %v\n", err)
		return
	}
	
	fmt.Printf("上传请求已创建: %s\n", request.ID)
	fmt.Printf("状态: %s\n", request.Status)
	
	if request.Status == "offline_queued" {
		fmt.Printf("⚠️  目标卫星当前离线，请求已加入离线队列\n")
		fmt.Printf("当卫星 %s 上线后，传输将自动开始\n", satelliteID)
	}
}

// handleDownloadCommand 处理下载命令
func (demo *FileTransferDemo) handleDownloadCommand(args []string) {
	if len(args) < 3 {
		fmt.Println("用法: download <远程文件路径> <卫星ID> <节点ID> [本地路径]")
		fmt.Println("示例: download system.log sat-02 node-1")
		return
	}
	
	remotePath := args[0]
	satelliteID := args[1]
	nodeID := args[2]
	
	var localPath string
	if len(args) > 3 {
		localPath = args[3]
	} else {
		localPath = filepath.Join("./downloads", filepath.Base(remotePath))
	}
	
	// 检查卫星和节点是否存在
	if _, exists := demo.constellation.GetNodeInfo(satelliteID, nodeID); !exists {
		fmt.Printf("错误: 卫星节点不存在: %s/%s\n", satelliteID, nodeID)
		return
	}
	
	fmt.Printf("开始下载: %s/%s:%s -> %s\n", satelliteID, nodeID, remotePath, localPath)
	
	request, err := demo.groundManager.DownloadFile(remotePath, localPath, satelliteID, nodeID)
	if err != nil {
		fmt.Printf("下载失败: %v\n", err)
		return
	}
	
	fmt.Printf("下载请求已创建: %s\n", request.ID)
	fmt.Printf("状态: %s\n", request.Status)
	
	if request.Status == "offline_queued" {
		fmt.Printf("⚠️  目标卫星当前离线，请求已加入离线队列\n")
		fmt.Printf("当卫星 %s 上线后，传输将自动开始\n", satelliteID)
	}
}

// handleOnlineCommand 处理强制上线命令
func (demo *FileTransferDemo) handleOnlineCommand(args []string) {
	if len(args) < 1 {
		fmt.Println("用法: online <卫星ID>")
		fmt.Println("示例: online sat-01")
		return
	}
	
	satelliteID := args[0]
	
	sat, exists := demo.constellation.GetSatelliteInfo(satelliteID)
	if !exists {
		fmt.Printf("错误: 卫星不存在: %s\n", satelliteID)
		return
	}
	
	// 强制卫星上线
	sat.CommunicationWindow.NextOnlineTime = time.Now()
	for _, node := range sat.Nodes {
		node.Status = "online"
	}
	
	fmt.Printf("✅ 卫星 %s 已强制上线\n", satelliteID)
}

// handleOfflineCommand 处理强制离线命令
func (demo *FileTransferDemo) handleOfflineCommand(args []string) {
	if len(args) < 1 {
		fmt.Println("用法: offline <卫星ID>")
		fmt.Println("示例: offline sat-01")
		return
	}
	
	satelliteID := args[0]
	
	sat, exists := demo.constellation.GetSatelliteInfo(satelliteID)
	if !exists {
		fmt.Printf("错误: 卫星不存在: %s\n", satelliteID)
		return
	}
	
	// 强制卫星离线
	sat.CommunicationWindow.NextOnlineTime = time.Now().Add(10 * time.Minute)
	for _, node := range sat.Nodes {
		node.Status = "offline"
	}
	
	fmt.Printf("🔴 卫星 %s 已强制离线\n", satelliteID)
}

// handleSimulateCommand 处理模拟命令
func (demo *FileTransferDemo) handleSimulateCommand(args []string) {
	if len(args) < 1 {
		fmt.Println("用法: simulate <start|stop|status>")
		return
	}
	
	action := args[0]
	
	switch action {
	case "start":
		if !demo.offlineSimulator.isRunning {
			demo.offlineSimulator.Start()
			fmt.Println("✅ 离线场景模拟已启动")
			fmt.Println("💡 卫星将随机在线/离线，模拟真实的轨道通信窗口")
		} else {
			fmt.Println("ℹ️  离线场景模拟已经在运行中")
		}
		
	case "stop":
		if demo.offlineSimulator.isRunning {
			demo.offlineSimulator.Stop()
			fmt.Println("🛑 离线场景模拟已停止")
		} else {
			fmt.Println("ℹ️  离线场景模拟没有运行")
		}
		
	case "status":
		demo.offlineSimulator.mu.RLock()
		status := "停止"
		if demo.offlineSimulator.isRunning {
			status = "运行中"
		}
		demo.offlineSimulator.mu.RUnlock()
		
		fmt.Printf("离线模拟器状态: %s\n", status)
		if demo.offlineSimulator.isRunning {
			onlineSats := demo.constellation.GetOnlineSatellites()
			fmt.Printf("当前在线卫星: %v\n", onlineSats)
		}
		
	default:
		fmt.Printf("未知的模拟命令: %s\n", action)
		fmt.Println("可用命令: start, stop, status")
	}
}