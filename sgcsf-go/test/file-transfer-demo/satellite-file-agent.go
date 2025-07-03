package main

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
	"github.com/sgcsf/sgcsf-go/pkg/core"
)

// SatelliteFileAgent 卫星文件代理
type SatelliteFileAgent struct {
	clientID    string
	satelliteID string
	nodeID      string
	sgcsfClient *core.Client
	nodeConfig  *NodeConfig
	
	// 文件存储管理
	storagePath     string
	tempPath        string
	maxFileSize     int64
	availableSpace  int64
	
	// 传输管理
	activeUploads   map[string]*UploadSession
	activeDownloads map[string]*DownloadSession
	offlineQueue    []*QueuedOperation
	
	// 并发控制
	mu              sync.RWMutex
	maxConcurrentOps int
	chunkSize        int
	
	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// UploadSession 上传会话
type UploadSession struct {
	ID            string          `json:"id"`
	GroundClientID string         `json:"ground_client_id"`
	TargetPath    string          `json:"target_path"`
	Metadata      *FileMetadata   `json:"metadata"`
	TotalChunks   int             `json:"total_chunks"`
	ReceivedChunks map[int][]byte `json:"-"`
	ChunkCount    int             `json:"chunk_count"`
	Status        string          `json:"status"` // "receiving", "completed", "failed"
	StartTime     time.Time       `json:"start_time"`
	LastActivity  time.Time       `json:"last_activity"`
	TempFilePath  string          `json:"-"`
}

// DownloadSession 下载会话
type DownloadSession struct {
	ID           string        `json:"id"`
	GroundClientID string      `json:"ground_client_id"`
	FilePath     string        `json:"file_path"`
	Metadata     *FileMetadata `json:"metadata"`
	TotalChunks  int           `json:"total_chunks"`
	SentChunks   map[int]bool  `json:"sent_chunks"`
	Status       string        `json:"status"` // "sending", "completed", "failed"
	StartTime    time.Time     `json:"start_time"`
	LastActivity time.Time     `json:"last_activity"`
}

// QueuedOperation 离线队列操作
type QueuedOperation struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"` // "upload_request", "download_request"
	ClientID    string                 `json:"client_id"`
	Payload     map[string]interface{} `json:"payload"`
	CreatedAt   time.Time              `json:"created_at"`
	RetryCount  int                    `json:"retry_count"`
	MaxRetries  int                    `json:"max_retries"`
}

// FileInventory 文件清单
type FileInventory struct {
	Files       map[string]*StoredFile `json:"files"`
	TotalFiles  int                    `json:"total_files"`
	TotalSize   int64                  `json:"total_size"`
	LastUpdated time.Time              `json:"last_updated"`
}

// StoredFile 存储的文件信息
type StoredFile struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	Checksum    string    `json:"checksum"`
	MimeType    string    `json:"mime_type"`
	CreatedAt   time.Time `json:"created_at"`
	ModifiedAt  time.Time `json:"modified_at"`
	AccessedAt  time.Time `json:"accessed_at"`
	Tags        []string  `json:"tags,omitempty"`
}

// NewSatelliteFileAgent 创建卫星文件代理
func NewSatelliteFileAgent(satelliteID, nodeID, serverAddr string) *SatelliteFileAgent {
	ctx, cancel := context.WithCancel(context.Background())
	
	clientID := fmt.Sprintf("%s-%s-agent", satelliteID, nodeID)
	
	// 创建SGCSF客户端
	client := core.SatelliteClient(clientID, serverAddr)
	
	// 获取节点配置
	constellation := GetConstellationConfig()
	nodeConfig, exists := constellation.GetNodeInfo(satelliteID, nodeID)
	if !exists {
		log.Fatalf("Node configuration not found: %s/%s", satelliteID, nodeID)
	}
	
	// 创建存储目录
	storagePath := nodeConfig.StoragePath
	tempPath := filepath.Join(storagePath, "temp")
	
	os.MkdirAll(storagePath, 0755)
	os.MkdirAll(tempPath, 0755)
	
	agent := &SatelliteFileAgent{
		clientID:    clientID,
		satelliteID: satelliteID,
		nodeID:      nodeID,
		sgcsfClient: client,
		nodeConfig:  nodeConfig,
		
		storagePath: storagePath,
		tempPath:    tempPath,
		maxFileSize: nodeConfig.MaxFileSize * 1024 * 1024, // MB to bytes
		
		activeUploads:   make(map[string]*UploadSession),
		activeDownloads: make(map[string]*DownloadSession),
		offlineQueue:    make([]*QueuedOperation, 0),
		
		maxConcurrentOps: 3,
		chunkSize:        1024 * 1024, // 1MB chunks
		
		ctx:    ctx,
		cancel: cancel,
	}
	
	// 计算可用空间
	agent.updateAvailableSpace()
	
	return agent
}

// Start 启动文件代理
func (sfa *SatelliteFileAgent) Start() error {
	log.Printf("Starting Satellite File Agent: %s/%s", sfa.satelliteID, sfa.nodeID)
	
	// 连接SGCSF服务器
	if err := sfa.sgcsfClient.Connect(); err != nil {
		return fmt.Errorf("failed to connect to SGCSF server: %v", err)
	}
	
	// 订阅相关主题
	sfa.subscribeToTopics()
	
	// 启动后台服务
	sfa.wg.Add(4)
	go sfa.sessionManager()
	go sfa.offlineQueueProcessor()
	go sfa.storageMonitor()
	go sfa.statusReporter()
	
	log.Printf("Satellite File Agent started: %s/%s", sfa.satelliteID, sfa.nodeID)
	return nil
}

// Stop 停止文件代理
func (sfa *SatelliteFileAgent) Stop() {
	log.Printf("Stopping Satellite File Agent: %s/%s", sfa.satelliteID, sfa.nodeID)
	
	sfa.cancel()
	sfa.sgcsfClient.Disconnect()
	sfa.wg.Wait()
	
	log.Printf("Satellite File Agent stopped: %s/%s", sfa.satelliteID, sfa.nodeID)
}

// subscribeToTopics 订阅相关主题
func (sfa *SatelliteFileAgent) subscribeToTopics() {
	destinationPrefix := fmt.Sprintf("%s/%s", sfa.satelliteID, sfa.nodeID)
	
	// 订阅文件上传主题
	sfa.sgcsfClient.Subscribe("file/upload/start", sfa.handleUploadStart)
	sfa.sgcsfClient.Subscribe("file/upload/chunk", sfa.handleUploadChunk)
	sfa.sgcsfClient.Subscribe("file/upload/complete", sfa.handleUploadComplete)
	
	// 订阅文件下载主题
	sfa.sgcsfClient.Subscribe("file/download/request", sfa.handleDownloadRequest)
	
	// 订阅文件管理主题
	sfa.sgcsfClient.Subscribe("file/list/request", sfa.handleListRequest)
	sfa.sgcsfClient.Subscribe("file/delete/request", sfa.handleDeleteRequest)
	sfa.sgcsfClient.Subscribe("file/info/request", sfa.handleInfoRequest)
	
	// 订阅节点控制主题
	nodeControlTopic := fmt.Sprintf("node/control/%s", destinationPrefix)
	sfa.sgcsfClient.Subscribe(nodeControlTopic, sfa.handleNodeControl)
}

// handleUploadStart 处理上传开始请求
func (sfa *SatelliteFileAgent) handleUploadStart(msg *types.SGCSFMessage) {
	// 检查消息是否发给本节点
	if !sfa.isMessageForThisNode(msg) {
		return
	}
	
	log.Printf("Handling upload start from %s", msg.Source)
	
	var request map[string]interface{}
	if err := json.Unmarshal(msg.Payload, &request); err != nil {
		sfa.sendErrorResponse(msg, "Invalid request format")
		return
	}
	
	requestID, _ := request["request_id"].(string)
	targetPath, _ := request["target_path"].(string)
	totalChunks, _ := request["total_chunks"].(float64)
	
	// 解析文件元数据
	metadataInterface, exists := request["metadata"]
	if !exists {
		sfa.sendErrorResponse(msg, "Missing file metadata")
		return
	}
	
	metadataBytes, _ := json.Marshal(metadataInterface)
	var metadata FileMetadata
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		sfa.sendErrorResponse(msg, "Invalid metadata format")
		return
	}
	
	// 检查存储空间
	if metadata.Size > sfa.availableSpace {
		sfa.sendErrorResponse(msg, "Insufficient storage space")
		return
	}
	
	// 检查文件大小限制
	if metadata.Size > sfa.maxFileSize {
		sfa.sendErrorResponse(msg, fmt.Sprintf("File too large (max: %d MB)", sfa.maxFileSize/1024/1024))
		return
	}
	
	// 创建上传会话
	session := &UploadSession{
		ID:             requestID,
		GroundClientID: msg.Source,
		TargetPath:     targetPath,
		Metadata:       &metadata,
		TotalChunks:    int(totalChunks),
		ReceivedChunks: make(map[int][]byte),
		Status:         "receiving",
		StartTime:      time.Now(),
		LastActivity:   time.Now(),
		TempFilePath:   filepath.Join(sfa.tempPath, requestID+".tmp"),
	}
	
	sfa.mu.Lock()
	sfa.activeUploads[requestID] = session
	sfa.mu.Unlock()
	
	// 发送准备就绪响应
	response := core.QuickResponse(msg, map[string]interface{}{
		"status":     "ready",
		"request_id": requestID,
		"node_info": map[string]interface{}{
			"satellite_id": sfa.satelliteID,
			"node_id":      sfa.nodeID,
			"capabilities": sfa.nodeConfig.Capabilities,
		},
	})
	
	if err := sfa.sgcsfClient.Publish(response); err != nil {
		log.Printf("Failed to send upload start response: %v", err)
	}
	
	log.Printf("Upload session created: %s (%.2f MB)", requestID, float64(metadata.Size)/1024/1024)
}

// handleUploadChunk 处理上传分片
func (sfa *SatelliteFileAgent) handleUploadChunk(msg *types.SGCSFMessage) {
	if !sfa.isMessageForThisNode(msg) {
		return
	}
	
	var chunk FileChunk
	if err := json.Unmarshal(msg.Payload, &chunk); err != nil {
		log.Printf("Failed to parse chunk: %v", err)
		return
	}
	
	sfa.mu.Lock()
	session, exists := sfa.activeUploads[chunk.FileID]
	if !exists {
		sfa.mu.Unlock()
		log.Printf("Upload session not found: %s", chunk.FileID)
		return
	}
	
	// 验证分片校验和
	expectedChecksum := fmt.Sprintf("%x", md5.Sum(chunk.Data))
	if chunk.Checksum != expectedChecksum {
		sfa.mu.Unlock()
		log.Printf("Chunk checksum mismatch for %s chunk %d", chunk.FileID, chunk.ChunkIndex)
		return
	}
	
	// 保存分片数据
	session.ReceivedChunks[chunk.ChunkIndex] = chunk.Data
	session.ChunkCount = len(session.ReceivedChunks)
	session.LastActivity = time.Now()
	
	sfa.mu.Unlock()
	
	log.Printf("Received chunk %d/%d for %s", chunk.ChunkIndex+1, chunk.TotalChunks, chunk.FileID)
	
	// 发送确认消息（如果需要）
	ackMsg := core.NewMessage().
		Topic("file/upload/chunk/ack").
		Type(types.MessageTypeAsync).
		Source(sfa.clientID).
		Destination(msg.Source).
		Payload(map[string]interface{}{
			"file_id":     chunk.FileID,
			"chunk_index": chunk.ChunkIndex,
			"status":      "received",
		}).
		Build()
	
	sfa.sgcsfClient.Publish(ackMsg)
}

// handleUploadComplete 处理上传完成请求
func (sfa *SatelliteFileAgent) handleUploadComplete(msg *types.SGCSFMessage) {
	if !sfa.isMessageForThisNode(msg) {
		return
	}
	
	log.Printf("Handling upload complete from %s", msg.Source)
	
	var request map[string]interface{}
	if err := json.Unmarshal(msg.Payload, &request); err != nil {
		sfa.sendErrorResponse(msg, "Invalid request format")
		return
	}
	
	requestID, _ := request["request_id"].(string)
	expectedChecksum, _ := request["checksum"].(string)
	
	sfa.mu.Lock()
	session, exists := sfa.activeUploads[requestID]
	if !exists {
		sfa.mu.Unlock()
		sfa.sendErrorResponse(msg, "Upload session not found")
		return
	}
	sfa.mu.Unlock()
	
	// 验证所有分片是否都收到了
	if session.ChunkCount != session.TotalChunks {
		sfa.sendErrorResponse(msg, fmt.Sprintf("Missing chunks: got %d, expected %d", session.ChunkCount, session.TotalChunks))
		return
	}
	
	// 按顺序写入文件
	tempFile, err := os.Create(session.TempFilePath)
	if err != nil {
		sfa.sendErrorResponse(msg, fmt.Sprintf("Failed to create temp file: %v", err))
		return
	}
	defer tempFile.Close()
	
	for i := 0; i < session.TotalChunks; i++ {
		chunkData, exists := session.ReceivedChunks[i]
		if !exists {
			sfa.sendErrorResponse(msg, fmt.Sprintf("Missing chunk %d", i))
			return
		}
		
		if _, err := tempFile.Write(chunkData); err != nil {
			sfa.sendErrorResponse(msg, fmt.Sprintf("Failed to write chunk %d: %v", i, err))
			return
		}
	}
	
	tempFile.Close()
	
	// 验证文件完整性
	actualChecksum, err := sfa.calculateFileChecksum(session.TempFilePath)
	if err != nil {
		sfa.sendErrorResponse(msg, fmt.Sprintf("Failed to calculate checksum: %v", err))
		return
	}
	
	if actualChecksum != expectedChecksum {
		sfa.sendErrorResponse(msg, fmt.Sprintf("File checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum))
		return
	}
	
	// 移动到最终位置
	finalPath := filepath.Join(sfa.storagePath, session.TargetPath)
	
	// 确保目标目录存在
	if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
		sfa.sendErrorResponse(msg, fmt.Sprintf("Failed to create target directory: %v", err))
		return
	}
	
	if err := os.Rename(session.TempFilePath, finalPath); err != nil {
		sfa.sendErrorResponse(msg, fmt.Sprintf("Failed to move file to final location: %v", err))
		return
	}
	
	// 更新文件清单
	sfa.addToInventory(&StoredFile{
		Name:       session.Metadata.Name,
		Path:       session.TargetPath,
		Size:       session.Metadata.Size,
		Checksum:   actualChecksum,
		MimeType:   session.Metadata.MimeType,
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
		AccessedAt: time.Now(),
	})
	
	// 更新存储空间
	sfa.updateAvailableSpace()
	
	// 清理上传会话
	sfa.mu.Lock()
	session.Status = "completed"
	delete(sfa.activeUploads, requestID)
	sfa.mu.Unlock()
	
	// 发送成功响应
	response := core.QuickResponse(msg, map[string]interface{}{
		"status":     "success",
		"request_id": requestID,
		"file_info": map[string]interface{}{
			"path":     session.TargetPath,
			"size":     session.Metadata.Size,
			"checksum": actualChecksum,
		},
	})
	
	if err := sfa.sgcsfClient.Publish(response); err != nil {
		log.Printf("Failed to send upload complete response: %v", err)
	}
	
	log.Printf("Upload completed: %s -> %s", requestID, finalPath)
}

// handleDownloadRequest 处理下载请求
func (sfa *SatelliteFileAgent) handleDownloadRequest(msg *types.SGCSFMessage) {
	if !sfa.isMessageForThisNode(msg) {
		return
	}
	
	log.Printf("Handling download request from %s", msg.Source)
	
	var request map[string]interface{}
	if err := json.Unmarshal(msg.Payload, &request); err != nil {
		sfa.sendErrorResponse(msg, "Invalid request format")
		return
	}
	
	requestID, _ := request["request_id"].(string)
	filePath, _ := request["file_path"].(string)
	
	// 检查文件是否存在
	fullPath := filepath.Join(sfa.storagePath, filePath)
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		sfa.sendErrorResponse(msg, "File not found")
		return
	}
	
	// 计算文件校验和
	checksum, err := sfa.calculateFileChecksum(fullPath)
	if err != nil {
		sfa.sendErrorResponse(msg, fmt.Sprintf("Failed to calculate checksum: %v", err))
		return
	}
	
	// 创建文件元数据
	metadata := &FileMetadata{
		Name:       fileInfo.Name(),
		Size:       fileInfo.Size(),
		Checksum:   checksum,
		MimeType:   "application/octet-stream",
		CreatedAt:  fileInfo.ModTime(),
		ModifiedAt: fileInfo.ModTime(),
	}
	
	// 计算分片数量
	totalChunks := int((fileInfo.Size() + int64(sfa.chunkSize) - 1) / int64(sfa.chunkSize))
	
	// 创建下载会话
	session := &DownloadSession{
		ID:             requestID,
		GroundClientID: msg.Source,
		FilePath:       fullPath,
		Metadata:       metadata,
		TotalChunks:    totalChunks,
		SentChunks:     make(map[int]bool),
		Status:         "sending",
		StartTime:      time.Now(),
		LastActivity:   time.Now(),
	}
	
	sfa.mu.Lock()
	sfa.activeDownloads[requestID] = session
	sfa.mu.Unlock()
	
	// 发送下载就绪响应
	response := core.QuickResponse(msg, map[string]interface{}{
		"status":       "ready",
		"request_id":   requestID,
		"metadata":     metadata,
		"total_chunks": totalChunks,
	})
	
	if err := sfa.sgcsfClient.Publish(response); err != nil {
		log.Printf("Failed to send download ready response: %v", err)
		return
	}
	
	// 开始发送文件分片
	go sfa.sendFileChunks(session)
	
	log.Printf("Download session created: %s (%.2f MB)", requestID, float64(metadata.Size)/1024/1024)
}

// sendFileChunks 发送文件分片
func (sfa *SatelliteFileAgent) sendFileChunks(session *DownloadSession) {
	file, err := os.Open(session.FilePath)
	if err != nil {
		log.Printf("Failed to open file for download: %v", err)
		return
	}
	defer file.Close()
	
	buffer := make([]byte, sfa.chunkSize)
	chunkTopic := fmt.Sprintf("file/download/chunk/%s", session.ID)
	
	for chunkIndex := 0; chunkIndex < session.TotalChunks; chunkIndex++ {
		// 读取分片数据
		n, readErr := file.Read(buffer)
		if readErr != nil && readErr != io.EOF {
			log.Printf("Failed to read file chunk: %v", readErr)
			break
		}
		
		chunkData := buffer[:n]
		chunkChecksum := fmt.Sprintf("%x", md5.Sum(chunkData))
		
		// 创建分片消息
		chunk := &FileChunk{
			FileID:      session.ID,
			ChunkIndex:  chunkIndex,
			ChunkSize:   n,
			TotalChunks: session.TotalChunks,
			Data:        chunkData,
			Checksum:    chunkChecksum,
		}
		
		chunkMsg := core.NewMessage().
			Topic(chunkTopic).
			Type(types.MessageTypeAsync).
			Source(sfa.clientID).
			Destination(session.GroundClientID).
			Payload(chunk).
			QoS(types.QoSAtLeastOnce).
			Build()
		
		if err := sfa.sgcsfClient.Publish(chunkMsg); err != nil {
			log.Printf("Failed to send chunk %d: %v", chunkIndex, err)
			break
		}
		
		// 更新会话状态
		sfa.mu.Lock()
		session.SentChunks[chunkIndex] = true
		session.LastActivity = time.Now()
		sfa.mu.Unlock()
		
		log.Printf("Sent chunk %d/%d for %s", chunkIndex+1, session.TotalChunks, session.ID)
		
		// 模拟网络延迟
		time.Sleep(10 * time.Millisecond)
		
		if readErr == io.EOF {
			break
		}
	}
	
	// 标记下载完成
	sfa.mu.Lock()
	session.Status = "completed"
	sfa.mu.Unlock()
	
	log.Printf("Download completed: %s", session.ID)
	
	// 清理下载会话（延时清理）
	time.AfterFunc(5*time.Minute, func() {
		sfa.mu.Lock()
		delete(sfa.activeDownloads, session.ID)
		sfa.mu.Unlock()
	})
}

// 其他消息处理器
func (sfa *SatelliteFileAgent) handleListRequest(msg *types.SGCSFMessage) {
	if !sfa.isMessageForThisNode(msg) {
		return
	}
	
	inventory := sfa.getFileInventory()
	
	response := core.QuickResponse(msg, map[string]interface{}{
		"status":    "success",
		"inventory": inventory,
	})
	
	sfa.sgcsfClient.Publish(response)
}

func (sfa *SatelliteFileAgent) handleDeleteRequest(msg *types.SGCSFMessage) {
	if !sfa.isMessageForThisNode(msg) {
		return
	}
	
	var request map[string]interface{}
	if err := json.Unmarshal(msg.Payload, &request); err != nil {
		sfa.sendErrorResponse(msg, "Invalid request format")
		return
	}
	
	filePath, _ := request["file_path"].(string)
	fullPath := filepath.Join(sfa.storagePath, filePath)
	
	if err := os.Remove(fullPath); err != nil {
		sfa.sendErrorResponse(msg, fmt.Sprintf("Failed to delete file: %v", err))
		return
	}
	
	// 从清单中移除
	sfa.removeFromInventory(filePath)
	sfa.updateAvailableSpace()
	
	response := core.QuickResponse(msg, map[string]interface{}{
		"status": "success",
		"message": "File deleted successfully",
	})
	
	sfa.sgcsfClient.Publish(response)
}

func (sfa *SatelliteFileAgent) handleInfoRequest(msg *types.SGCSFMessage) {
	if !sfa.isMessageForThisNode(msg) {
		return
	}
	
	response := core.QuickResponse(msg, map[string]interface{}{
		"status": "success",
		"node_info": map[string]interface{}{
			"satellite_id":    sfa.satelliteID,
			"node_id":         sfa.nodeID,
			"storage_path":    sfa.storagePath,
			"max_file_size":   sfa.maxFileSize,
			"available_space": sfa.availableSpace,
			"capabilities":    sfa.nodeConfig.Capabilities,
			"status":          sfa.nodeConfig.Status,
		},
	})
	
	sfa.sgcsfClient.Publish(response)
}

func (sfa *SatelliteFileAgent) handleNodeControl(msg *types.SGCSFMessage) {
	log.Printf("Received node control message: %s", string(msg.Payload))
	// 处理节点控制指令（如重启、配置更新等）
}

// 后台服务
func (sfa *SatelliteFileAgent) sessionManager() {
	defer sfa.wg.Done()
	
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-sfa.ctx.Done():
			return
		case <-ticker.C:
			sfa.cleanupStaleeSessions()
		}
	}
}

func (sfa *SatelliteFileAgent) offlineQueueProcessor() {
	defer sfa.wg.Done()
	
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-sfa.ctx.Done():
			return
		case <-ticker.C:
			sfa.processOfflineQueue()
		}
	}
}

func (sfa *SatelliteFileAgent) storageMonitor() {
	defer sfa.wg.Done()
	
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-sfa.ctx.Done():
			return
		case <-ticker.C:
			sfa.updateAvailableSpace()
			sfa.checkStorageHealth()
		}
	}
}

func (sfa *SatelliteFileAgent) statusReporter() {
	defer sfa.wg.Done()
	
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-sfa.ctx.Done():
			return
		case <-ticker.C:
			sfa.reportStatus()
		}
	}
}

// 辅助方法
func (sfa *SatelliteFileAgent) isMessageForThisNode(msg *types.SGCSFMessage) bool {
	expectedDest := fmt.Sprintf("%s/%s", sfa.satelliteID, sfa.nodeID)
	return msg.Destination == expectedDest || msg.Destination == ""
}

func (sfa *SatelliteFileAgent) sendErrorResponse(msg *types.SGCSFMessage, errorMsg string) {
	response := core.QuickResponse(msg, map[string]interface{}{
		"status": "error",
		"error":  errorMsg,
	})
	
	sfa.sgcsfClient.Publish(response)
}

func (sfa *SatelliteFileAgent) calculateFileChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	
	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func (sfa *SatelliteFileAgent) updateAvailableSpace() {
	// 计算存储空间使用情况
	var totalSize int64
	
	filepath.Walk(sfa.storagePath, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})
	
	// 假设最大存储空间为节点配置的存储容量
	maxStorage := int64(2048) * 1024 * 1024 // 2GB default
	if sat, exists := GetConstellationConfig().GetSatelliteInfo(sfa.satelliteID); exists {
		maxStorage = sat.StorageCapacity * 1024 * 1024
	}
	
	sfa.availableSpace = maxStorage - totalSize
	if sfa.availableSpace < 0 {
		sfa.availableSpace = 0
	}
}

func (sfa *SatelliteFileAgent) cleanupStaleeSessions() {
	sfa.mu.Lock()
	defer sfa.mu.Unlock()
	
	now := time.Now()
	staleTimeout := 10 * time.Minute
	
	// 清理过期的上传会话
	for id, session := range sfa.activeUploads {
		if now.Sub(session.LastActivity) > staleTimeout {
			log.Printf("Cleaning up stale upload session: %s", id)
			os.Remove(session.TempFilePath)
			delete(sfa.activeUploads, id)
		}
	}
	
	// 清理过期的下载会话
	for id, session := range sfa.activeDownloads {
		if now.Sub(session.LastActivity) > staleTimeout {
			log.Printf("Cleaning up stale download session: %s", id)
			delete(sfa.activeDownloads, id)
		}
	}
}

func (sfa *SatelliteFileAgent) processOfflineQueue() {
	sfa.mu.Lock()
	defer sfa.mu.Unlock()
	
	// 处理离线队列中的操作
	for i := len(sfa.offlineQueue) - 1; i >= 0; i-- {
		op := sfa.offlineQueue[i]
		
		// 检查是否可以处理（如连接是否可用）
		if sfa.sgcsfClient != nil {
			log.Printf("Processing offline operation: %s", op.ID)
			
			// 根据操作类型处理
			switch op.Type {
			case "upload_request":
				// 重新处理上传请求
			case "download_request":
				// 重新处理下载请求
			}
			
			// 从队列中移除
			sfa.offlineQueue = append(sfa.offlineQueue[:i], sfa.offlineQueue[i+1:]...)
		}
	}
}

func (sfa *SatelliteFileAgent) checkStorageHealth() {
	// 检查存储健康状况
	usagePercent := float64(sfa.availableSpace) / float64(2048*1024*1024) * 100
	
	if usagePercent < 10 {
		log.Printf("WARNING: Low storage space on %s/%s: %.1f%% available", 
			sfa.satelliteID, sfa.nodeID, usagePercent)
	}
}

func (sfa *SatelliteFileAgent) reportStatus() {
	sfa.mu.RLock()
	activeUploads := len(sfa.activeUploads)
	activeDownloads := len(sfa.activeDownloads)
	offlineQueueSize := len(sfa.offlineQueue)
	sfa.mu.RUnlock()
	
	status := map[string]interface{}{
		"node_id":            sfa.nodeID,
		"satellite_id":       sfa.satelliteID,
		"status":             sfa.nodeConfig.Status,
		"available_space":    sfa.availableSpace,
		"active_uploads":     activeUploads,
		"active_downloads":   activeDownloads,
		"offline_queue_size": offlineQueueSize,
		"timestamp":          time.Now().Unix(),
	}
	
	statusMsg := core.NewMessage().
		Topic("satellite/status/report").
		Type(types.MessageTypeAsync).
		Source(sfa.clientID).
		Payload(status).
		Build()
	
	sfa.sgcsfClient.Publish(statusMsg)
}

// 文件清单管理
func (sfa *SatelliteFileAgent) getFileInventory() *FileInventory {
	inventory := &FileInventory{
		Files:       make(map[string]*StoredFile),
		LastUpdated: time.Now(),
	}
	
	var totalSize int64
	fileCount := 0
	
	filepath.Walk(sfa.storagePath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.Contains(path, "temp") {
			return nil
		}
		
		relPath, _ := filepath.Rel(sfa.storagePath, path)
		checksum, _ := sfa.calculateFileChecksum(path)
		
		storedFile := &StoredFile{
			Name:       info.Name(),
			Path:       relPath,
			Size:       info.Size(),
			Checksum:   checksum,
			MimeType:   "application/octet-stream",
			CreatedAt:  info.ModTime(),
			ModifiedAt: info.ModTime(),
			AccessedAt: info.ModTime(),
		}
		
		inventory.Files[relPath] = storedFile
		totalSize += info.Size()
		fileCount++
		
		return nil
	})
	
	inventory.TotalFiles = fileCount
	inventory.TotalSize = totalSize
	
	return inventory
}

func (sfa *SatelliteFileAgent) addToInventory(file *StoredFile) {
	// 实际实现中可能需要持久化到数据库
	log.Printf("Added file to inventory: %s (%d bytes)", file.Path, file.Size)
}

func (sfa *SatelliteFileAgent) removeFromInventory(filePath string) {
	// 实际实现中可能需要从数据库中删除
	log.Printf("Removed file from inventory: %s", filePath)
}