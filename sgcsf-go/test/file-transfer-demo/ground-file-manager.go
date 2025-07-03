package main

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
	"github.com/sgcsf/sgcsf-go/pkg/core"
)

// FileTransferRequest 文件传输请求
type FileTransferRequest struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"` // "upload" or "download"
	SourcePath   string    `json:"source_path"`
	TargetPath   string    `json:"target_path"`
	SatelliteID  string    `json:"satellite_id"`
	NodeID       string    `json:"node_id"`
	Status       string    `json:"status"` // "pending", "in_progress", "completed", "failed", "offline_queued"
	Progress     float64   `json:"progress"`
	FileSize     int64     `json:"file_size"`
	Checksum     string    `json:"checksum"`
	CreatedAt    time.Time `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	RetryCount   int       `json:"retry_count"`
	MaxRetries   int       `json:"max_retries"`
}

// FileChunk 文件分片
type FileChunk struct {
	FileID     string `json:"file_id"`
	ChunkIndex int    `json:"chunk_index"`
	ChunkSize  int    `json:"chunk_size"`
	TotalChunks int   `json:"total_chunks"`
	Data       []byte `json:"data"`
	Checksum   string `json:"checksum"`
}

// FileMetadata 文件元数据
type FileMetadata struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	Checksum  string    `json:"checksum"`
	MimeType  string    `json:"mime_type"`
	CreatedAt time.Time `json:"created_at"`
	ModifiedAt time.Time `json:"modified_at"`
}

// TransferProgress 传输进度
type TransferProgress struct {
	RequestID     string    `json:"request_id"`
	BytesTransferred int64  `json:"bytes_transferred"`
	TotalBytes    int64     `json:"total_bytes"`
	ChunksCompleted int     `json:"chunks_completed"`
	TotalChunks   int       `json:"total_chunks"`
	Speed         float64   `json:"speed_mbps"`
	ETA          *time.Duration `json:"eta,omitempty"`
	LastUpdate   time.Time  `json:"last_update"`
}

// GroundFileManager 地面文件管理器
type GroundFileManager struct {
	clientID     string
	sgcsfClient  *core.Client
	constellation *ConstellationConfig
	
	// 文件传输管理
	activeTransfers  map[string]*FileTransferRequest
	transferQueue    []*FileTransferRequest
	offlineQueue     map[string][]*FileTransferRequest // satelliteID -> requests
	transferProgress map[string]*TransferProgress
	
	// 本地存储
	downloadDir string
	uploadDir   string
	tempDir     string
	
	// 并发控制
	mu                sync.RWMutex
	maxConcurrentTransfers int
	chunkSize         int
	
	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewGroundFileManager 创建地面文件管理器
func NewGroundFileManager(clientID, serverAddr string) *GroundFileManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	// 创建SGCSF客户端
	client := core.GroundClient(clientID, serverAddr)
	
	// 创建目录
	downloadDir := "./downloads"
	uploadDir := "./uploads"
	tempDir := "./temp"
	
	os.MkdirAll(downloadDir, 0755)
	os.MkdirAll(uploadDir, 0755)
	os.MkdirAll(tempDir, 0755)
	
	manager := &GroundFileManager{
		clientID:     clientID,
		sgcsfClient:  client,
		constellation: GetConstellationConfig(),
		
		activeTransfers:  make(map[string]*FileTransferRequest),
		transferQueue:    make([]*FileTransferRequest, 0),
		offlineQueue:     make(map[string][]*FileTransferRequest),
		transferProgress: make(map[string]*TransferProgress),
		
		downloadDir: downloadDir,
		uploadDir:   uploadDir,
		tempDir:     tempDir,
		
		maxConcurrentTransfers: 5,
		chunkSize:             1024 * 1024, // 1MB chunks
		
		ctx:    ctx,
		cancel: cancel,
	}
	
	return manager
}

// Start 启动文件管理器
func (gfm *GroundFileManager) Start() error {
	log.Printf("Starting Ground File Manager: %s", gfm.clientID)
	
	// 连接SGCSF服务器
	if err := gfm.sgcsfClient.Connect(); err != nil {
		return fmt.Errorf("failed to connect to SGCSF server: %v", err)
	}
	
	// 订阅文件传输相关主题
	gfm.subscribeToTopics()
	
	// 启动后台服务
	gfm.wg.Add(4)
	go gfm.transferQueueProcessor()
	go gfm.offlineQueueMonitor()
	go gfm.progressTracker()
	go gfm.httpServer()
	
	log.Println("Ground File Manager started successfully")
	return nil
}

// Stop 停止文件管理器
func (gfm *GroundFileManager) Stop() {
	log.Println("Stopping Ground File Manager...")
	
	gfm.cancel()
	gfm.sgcsfClient.Disconnect()
	gfm.wg.Wait()
	
	log.Println("Ground File Manager stopped")
}

// subscribeToTopics 订阅相关主题
func (gfm *GroundFileManager) subscribeToTopics() {
	// 订阅文件传输响应
	gfm.sgcsfClient.Subscribe("file/transfer/response", gfm.handleTransferResponse)
	
	// 订阅文件传输进度
	gfm.sgcsfClient.Subscribe("file/transfer/progress", gfm.handleTransferProgress)
	
	// 订阅卫星状态变化
	gfm.sgcsfClient.Subscribe("satellite/status", gfm.handleSatelliteStatus)
	
	// 订阅文件分片传输
	gfm.sgcsfClient.Subscribe("file/chunk/transfer", gfm.handleChunkTransfer)
}

// UploadFile 上传文件到卫星
func (gfm *GroundFileManager) UploadFile(localPath, remotePath, satelliteID, nodeID string) (*FileTransferRequest, error) {
	// 检查本地文件
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("local file not found: %v", err)
	}
	
	// 计算文件校验和
	checksum, err := gfm.calculateFileChecksum(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate checksum: %v", err)
	}
	
	// 创建传输请求
	request := &FileTransferRequest{
		ID:          fmt.Sprintf("upload-%d", time.Now().UnixNano()),
		Type:        "upload",
		SourcePath:  localPath,
		TargetPath:  remotePath,
		SatelliteID: satelliteID,
		NodeID:      nodeID,
		Status:      "pending",
		Progress:    0.0,
		FileSize:    fileInfo.Size(),
		Checksum:    checksum,
		CreatedAt:   time.Now(),
		MaxRetries:  3,
	}
	
	// 检查目标卫星是否在线
	if gfm.isSatelliteOnline(satelliteID) {
		request.Status = "pending"
		gfm.addToTransferQueue(request)
		log.Printf("Upload request queued: %s -> %s/%s%s", localPath, satelliteID, nodeID, remotePath)
	} else {
		request.Status = "offline_queued"
		gfm.addToOfflineQueue(satelliteID, request)
		log.Printf("Upload request queued offline: %s -> %s/%s%s", localPath, satelliteID, nodeID, remotePath)
	}
	
	gfm.mu.Lock()
	gfm.activeTransfers[request.ID] = request
	gfm.mu.Unlock()
	
	return request, nil
}

// DownloadFile 从卫星下载文件
func (gfm *GroundFileManager) DownloadFile(remotePath, localPath, satelliteID, nodeID string) (*FileTransferRequest, error) {
	// 创建传输请求
	request := &FileTransferRequest{
		ID:          fmt.Sprintf("download-%d", time.Now().UnixNano()),
		Type:        "download",
		SourcePath:  remotePath,
		TargetPath:  localPath,
		SatelliteID: satelliteID,
		NodeID:      nodeID,
		Status:      "pending",
		Progress:    0.0,
		CreatedAt:   time.Now(),
		MaxRetries:  3,
	}
	
	// 检查目标卫星是否在线
	if gfm.isSatelliteOnline(satelliteID) {
		request.Status = "pending"
		gfm.addToTransferQueue(request)
		log.Printf("Download request queued: %s/%s%s -> %s", satelliteID, nodeID, remotePath, localPath)
	} else {
		request.Status = "offline_queued"
		gfm.addToOfflineQueue(satelliteID, request)
		log.Printf("Download request queued offline: %s/%s%s -> %s", satelliteID, nodeID, remotePath, localPath)
	}
	
	gfm.mu.Lock()
	gfm.activeTransfers[request.ID] = request
	gfm.mu.Unlock()
	
	return request, nil
}

// transferQueueProcessor 传输队列处理器
func (gfm *GroundFileManager) transferQueueProcessor() {
	defer gfm.wg.Done()
	
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-gfm.ctx.Done():
			return
		case <-ticker.C:
			gfm.processTransferQueue()
		}
	}
}

// processTransferQueue 处理传输队列
func (gfm *GroundFileManager) processTransferQueue() {
	gfm.mu.Lock()
	defer gfm.mu.Unlock()
	
	// 统计当前进行中的传输
	activeCount := 0
	for _, req := range gfm.activeTransfers {
		if req.Status == "in_progress" {
			activeCount++
		}
	}
	
	// 如果达到最大并发数，则等待
	if activeCount >= gfm.maxConcurrentTransfers {
		return
	}
	
	// 处理队列中的请求
	for i := 0; i < len(gfm.transferQueue) && activeCount < gfm.maxConcurrentTransfers; i++ {
		request := gfm.transferQueue[i]
		
		if request.Status == "pending" && gfm.isSatelliteOnline(request.SatelliteID) {
			// 开始传输
			go gfm.executeTransfer(request)
			
			// 更新状态
			request.Status = "in_progress"
			now := time.Now()
			request.StartedAt = &now
			
			// 从队列中移除
			gfm.transferQueue = append(gfm.transferQueue[:i], gfm.transferQueue[i+1:]...)
			i--
			activeCount++
			
			log.Printf("Started transfer: %s", request.ID)
		}
	}
}

// executeTransfer 执行文件传输
func (gfm *GroundFileManager) executeTransfer(request *FileTransferRequest) {
	log.Printf("Executing transfer: %s (%s)", request.ID, request.Type)
	
	// 创建进度跟踪
	progress := &TransferProgress{
		RequestID:   request.ID,
		TotalBytes:  request.FileSize,
		LastUpdate:  time.Now(),
	}
	
	gfm.mu.Lock()
	gfm.transferProgress[request.ID] = progress
	gfm.mu.Unlock()
	
	var err error
	if request.Type == "upload" {
		err = gfm.executeUpload(request, progress)
	} else {
		err = gfm.executeDownload(request, progress)
	}
	
	// 更新请求状态
	gfm.mu.Lock()
	defer gfm.mu.Unlock()
	
	if err != nil {
		request.Status = "failed"
		request.ErrorMessage = err.Error()
		request.RetryCount++
		
		// 重试逻辑
		if request.RetryCount < request.MaxRetries {
			log.Printf("Transfer failed, retrying: %s (%d/%d)", request.ID, request.RetryCount, request.MaxRetries)
			time.AfterFunc(time.Duration(request.RetryCount)*5*time.Second, func() {
				gfm.addToTransferQueue(request)
			})
		} else {
			log.Printf("Transfer failed permanently: %s", request.ID)
		}
	} else {
		request.Status = "completed"
		request.Progress = 100.0
		now := time.Now()
		request.CompletedAt = &now
		log.Printf("Transfer completed: %s", request.ID)
	}
	
	// 清理进度跟踪
	delete(gfm.transferProgress, request.ID)
}

// executeUpload 执行文件上传
func (gfm *GroundFileManager) executeUpload(request *FileTransferRequest, progress *TransferProgress) error {
	// 打开本地文件
	file, err := os.Open(request.SourcePath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %v", err)
	}
	defer file.Close()
	
	// 计算分片数量
	totalChunks := int((request.FileSize + int64(gfm.chunkSize) - 1) / int64(gfm.chunkSize))
	progress.TotalChunks = totalChunks
	
	// 发送文件元数据
	metadata := &FileMetadata{
		Name:     filepath.Base(request.SourcePath),
		Size:     request.FileSize,
		Checksum: request.Checksum,
		MimeType: "application/octet-stream",
		CreatedAt: time.Now(),
	}
	
	// 发送上传开始消息
	startMsg := core.NewMessage().
		Topic("file/upload/start").
		Type(types.MessageTypeSync).
		Source(gfm.clientID).
		Destination(fmt.Sprintf("%s/%s", request.SatelliteID, request.NodeID)).
		Payload(map[string]interface{}{
			"request_id": request.ID,
			"target_path": request.TargetPath,
			"metadata": metadata,
			"total_chunks": totalChunks,
		}).
		Timeout(30 * time.Second).
		Build()
	
	response, err := gfm.sgcsfClient.Send(startMsg)
	if err != nil {
		return fmt.Errorf("failed to start upload: %v", err)
	}
	
	var startResponse map[string]interface{}
	if err := json.Unmarshal(response.Payload, &startResponse); err != nil {
		return fmt.Errorf("failed to parse start response: %v", err)
	}
	
	if status, ok := startResponse["status"].(string); !ok || status != "ready" {
		return fmt.Errorf("upload not accepted by satellite: %v", startResponse)
	}
	
	// 发送文件分片
	startTime := time.Now()
	buffer := make([]byte, gfm.chunkSize)
	
	for chunkIndex := 0; chunkIndex < totalChunks; chunkIndex++ {
		// 读取分片数据
		n, readErr := file.Read(buffer)
		if readErr != nil && readErr != io.EOF {
			return fmt.Errorf("failed to read file chunk: %v", readErr)
		}
		
		chunkData := buffer[:n]
		chunkChecksum := fmt.Sprintf("%x", md5.Sum(chunkData))
		
		// 创建分片消息
		chunk := &FileChunk{
			FileID:     request.ID,
			ChunkIndex: chunkIndex,
			ChunkSize:  n,
			TotalChunks: totalChunks,
			Data:       chunkData,
			Checksum:   chunkChecksum,
		}
		
		chunkMsg := core.NewMessage().
			Topic("file/upload/chunk").
			Type(types.MessageTypeAsync).
			Source(gfm.clientID).
			Destination(fmt.Sprintf("%s/%s", request.SatelliteID, request.NodeID)).
			Payload(chunk).
			QoS(types.QoSAtLeastOnce).
			Build()
		
		if err := gfm.sgcsfClient.Publish(chunkMsg); err != nil {
			return fmt.Errorf("failed to send chunk %d: %v", chunkIndex, err)
		}
		
		// 更新进度
		progress.BytesTransferred += int64(n)
		progress.ChunksCompleted = chunkIndex + 1
		
		// 计算传输速度
		elapsed := time.Since(startTime).Seconds()
		if elapsed > 0 {
			progress.Speed = float64(progress.BytesTransferred) / elapsed / 1024 / 1024
		}
		
		// 计算ETA
		if progress.Speed > 0 {
			remaining := float64(progress.TotalBytes - progress.BytesTransferred)
			eta := time.Duration(remaining / progress.Speed / 1024 / 1024) * time.Second
			progress.ETA = &eta
		}
		
		progress.LastUpdate = time.Now()
		request.Progress = float64(progress.BytesTransferred) / float64(progress.TotalBytes) * 100
		
		// 检查是否需要取消
		select {
		case <-gfm.ctx.Done():
			return fmt.Errorf("transfer cancelled")
		default:
		}
		
		// 模拟网络延迟
		time.Sleep(10 * time.Millisecond)
		
		if readErr == io.EOF {
			break
		}
	}
	
	// 发送上传完成消息
	completeMsg := core.NewMessage().
		Topic("file/upload/complete").
		Type(types.MessageTypeSync).
		Source(gfm.clientID).
		Destination(fmt.Sprintf("%s/%s", request.SatelliteID, request.NodeID)).
		Payload(map[string]interface{}{
			"request_id": request.ID,
			"checksum": request.Checksum,
		}).
		Timeout(30 * time.Second).
		Build()
	
	completeResponse, err := gfm.sgcsfClient.Send(completeMsg)
	if err != nil {
		return fmt.Errorf("failed to complete upload: %v", err)
	}
	
	var result map[string]interface{}
	if err := json.Unmarshal(completeResponse.Payload, &result); err != nil {
		return fmt.Errorf("failed to parse complete response: %v", err)
	}
	
	if status, ok := result["status"].(string); !ok || status != "success" {
		return fmt.Errorf("upload verification failed: %v", result)
	}
	
	return nil
}

// executeDownload 执行文件下载
func (gfm *GroundFileManager) executeDownload(request *FileTransferRequest, progress *TransferProgress) error {
	// 发送下载请求
	downloadMsg := core.NewMessage().
		Topic("file/download/request").
		Type(types.MessageTypeSync).
		Source(gfm.clientID).
		Destination(fmt.Sprintf("%s/%s", request.SatelliteID, request.NodeID)).
		Payload(map[string]interface{}{
			"request_id": request.ID,
			"file_path": request.SourcePath,
		}).
		Timeout(30 * time.Second).
		Build()
	
	response, err := gfm.sgcsfClient.Send(downloadMsg)
	if err != nil {
		return fmt.Errorf("failed to request download: %v", err)
	}
	
	var downloadResponse map[string]interface{}
	if err := json.Unmarshal(response.Payload, &downloadResponse); err != nil {
		return fmt.Errorf("failed to parse download response: %v", err)
	}
	
	if status, ok := downloadResponse["status"].(string); !ok || status != "ready" {
		return fmt.Errorf("download not available: %v", downloadResponse)
	}
	
	// 获取文件元数据
	metadataInterface, ok := downloadResponse["metadata"]
	if !ok {
		return fmt.Errorf("missing file metadata in response")
	}
	
	metadataBytes, _ := json.Marshal(metadataInterface)
	var metadata FileMetadata
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		return fmt.Errorf("failed to parse file metadata: %v", err)
	}
	
	request.FileSize = metadata.Size
	request.Checksum = metadata.Checksum
	progress.TotalBytes = metadata.Size
	
	totalChunks := int((metadata.Size + int64(gfm.chunkSize) - 1) / int64(gfm.chunkSize))
	progress.TotalChunks = totalChunks
	
	// 创建临时文件
	tempPath := filepath.Join(gfm.tempDir, request.ID+".tmp")
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	defer tempFile.Close()
	defer os.Remove(tempPath)
	
	// 接收文件分片
	chunks := make(map[int][]byte)
	receivedChunks := 0
	startTime := time.Now()
	
	// 订阅下载分片主题
	chunkTopic := fmt.Sprintf("file/download/chunk/%s", request.ID)
	gfm.sgcsfClient.Subscribe(chunkTopic, func(msg *types.SGCSFMessage) {
		var chunk FileChunk
		if err := json.Unmarshal(msg.Payload, &chunk); err != nil {
			log.Printf("Failed to parse chunk: %v", err)
			return
		}
		
		// 验证分片校验和
		expectedChecksum := fmt.Sprintf("%x", md5.Sum(chunk.Data))
		if chunk.Checksum != expectedChecksum {
			log.Printf("Chunk checksum mismatch: expected %s, got %s", expectedChecksum, chunk.Checksum)
			return
		}
		
		chunks[chunk.ChunkIndex] = chunk.Data
		receivedChunks++
		
		// 更新进度
		progress.BytesTransferred += int64(chunk.ChunkSize)
		progress.ChunksCompleted = receivedChunks
		
		// 计算传输速度
		elapsed := time.Since(startTime).Seconds()
		if elapsed > 0 {
			progress.Speed = float64(progress.BytesTransferred) / elapsed / 1024 / 1024
		}
		
		progress.LastUpdate = time.Now()
		request.Progress = float64(progress.BytesTransferred) / float64(progress.TotalBytes) * 100
	})
	
	// 等待所有分片接收完成
	for receivedChunks < totalChunks {
		select {
		case <-gfm.ctx.Done():
			return fmt.Errorf("download cancelled")
		case <-time.After(60 * time.Second):
			return fmt.Errorf("download timeout")
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
	
	// 按顺序写入文件
	for i := 0; i < totalChunks; i++ {
		chunkData, exists := chunks[i]
		if !exists {
			return fmt.Errorf("missing chunk %d", i)
		}
		
		if _, err := tempFile.Write(chunkData); err != nil {
			return fmt.Errorf("failed to write chunk %d: %v", i, err)
		}
	}
	
	tempFile.Close()
	
	// 验证文件完整性
	downloadedChecksum, err := gfm.calculateFileChecksum(tempPath)
	if err != nil {
		return fmt.Errorf("failed to calculate downloaded file checksum: %v", err)
	}
	
	if downloadedChecksum != metadata.Checksum {
		return fmt.Errorf("file checksum mismatch: expected %s, got %s", metadata.Checksum, downloadedChecksum)
	}
	
	// 移动到最终位置
	finalPath := request.TargetPath
	if finalPath == "" {
		finalPath = filepath.Join(gfm.downloadDir, metadata.Name)
	}
	
	// 确保目标目录存在
	if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %v", err)
	}
	
	if err := os.Rename(tempPath, finalPath); err != nil {
		return fmt.Errorf("failed to move file to final location: %v", err)
	}
	
	return nil
}

// 辅助方法实现
func (gfm *GroundFileManager) calculateFileChecksum(filePath string) (string, error) {
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

func (gfm *GroundFileManager) isSatelliteOnline(satelliteID string) bool {
	onlineSats := gfm.constellation.GetOnlineSatellites()
	for _, id := range onlineSats {
		if id == satelliteID {
			return true
		}
	}
	return false
}

func (gfm *GroundFileManager) addToTransferQueue(request *FileTransferRequest) {
	gfm.mu.Lock()
	defer gfm.mu.Unlock()
	
	request.Status = "pending"
	gfm.transferQueue = append(gfm.transferQueue, request)
}

func (gfm *GroundFileManager) addToOfflineQueue(satelliteID string, request *FileTransferRequest) {
	gfm.mu.Lock()
	defer gfm.mu.Unlock()
	
	if gfm.offlineQueue[satelliteID] == nil {
		gfm.offlineQueue[satelliteID] = make([]*FileTransferRequest, 0)
	}
	
	request.Status = "offline_queued"
	gfm.offlineQueue[satelliteID] = append(gfm.offlineQueue[satelliteID], request)
}

// 消息处理器
func (gfm *GroundFileManager) handleTransferResponse(msg *types.SGCSFMessage) {
	log.Printf("Received transfer response: %s", string(msg.Payload))
}

func (gfm *GroundFileManager) handleTransferProgress(msg *types.SGCSFMessage) {
	var progress TransferProgress
	if err := json.Unmarshal(msg.Payload, &progress); err != nil {
		log.Printf("Failed to parse transfer progress: %v", err)
		return
	}
	
	gfm.mu.Lock()
	gfm.transferProgress[progress.RequestID] = &progress
	gfm.mu.Unlock()
}

func (gfm *GroundFileManager) handleSatelliteStatus(msg *types.SGCSFMessage) {
	// 处理卫星状态变化，将离线队列中的任务移到正常队列
	gfm.processOfflineQueue()
}

func (gfm *GroundFileManager) handleChunkTransfer(msg *types.SGCSFMessage) {
	// 处理文件分片传输
	log.Printf("Received chunk transfer: %s", string(msg.Payload))
}

// 离线队列监控器
func (gfm *GroundFileManager) offlineQueueMonitor() {
	defer gfm.wg.Done()
	
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-gfm.ctx.Done():
			return
		case <-ticker.C:
			gfm.processOfflineQueue()
		}
	}
}

func (gfm *GroundFileManager) processOfflineQueue() {
	gfm.mu.Lock()
	defer gfm.mu.Unlock()
	
	for satelliteID, requests := range gfm.offlineQueue {
		if gfm.isSatelliteOnline(satelliteID) {
			// 将离线队列中的请求移到正常队列
			for _, request := range requests {
				request.Status = "pending"
				gfm.transferQueue = append(gfm.transferQueue, request)
				log.Printf("Moved offline request to queue: %s", request.ID)
			}
			
			// 清空离线队列
			delete(gfm.offlineQueue, satelliteID)
		}
	}
}

// 进度跟踪器
func (gfm *GroundFileManager) progressTracker() {
	defer gfm.wg.Done()
	
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-gfm.ctx.Done():
			return
		case <-ticker.C:
			gfm.updateProgress()
		}
	}
}

func (gfm *GroundFileManager) updateProgress() {
	gfm.mu.RLock()
	defer gfm.mu.RUnlock()
	
	// 定期更新进度信息（可以发送到监控系统）
	for requestID, progress := range gfm.transferProgress {
		if time.Since(progress.LastUpdate) > 10*time.Second {
			log.Printf("Progress for %s: %.1f%% (%d/%d chunks)", 
				requestID, 
				float64(progress.BytesTransferred)/float64(progress.TotalBytes)*100,
				progress.ChunksCompleted,
				progress.TotalChunks)
		}
	}
}

// HTTP API服务器
func (gfm *GroundFileManager) httpServer() {
	defer gfm.wg.Done()
	
	mux := http.NewServeMux()
	
	// API路由
	mux.HandleFunc("/api/transfers", gfm.apiGetTransfers)
	mux.HandleFunc("/api/transfer/upload", gfm.apiUploadFile)
	mux.HandleFunc("/api/transfer/download", gfm.apiDownloadFile)
	mux.HandleFunc("/api/satellites/status", gfm.apiGetSatelliteStatus)
	mux.HandleFunc("/api/satellites/", gfm.apiControlSatellite)
	
	// 静态文件服务
	mux.Handle("/", http.FileServer(http.Dir("./web/")))
	
	server := &http.Server{
		Addr:    ":8082",
		Handler: mux,
	}
	
	log.Println("HTTP server starting on :8082")
	
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()
	
	// 等待取消信号
	<-gfm.ctx.Done()
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}
}

// HTTP API处理器
func (gfm *GroundFileManager) apiGetTransfers(w http.ResponseWriter, r *http.Request) {
	gfm.mu.RLock()
	transfers := make([]*FileTransferRequest, 0, len(gfm.activeTransfers))
	for _, transfer := range gfm.activeTransfers {
		transfers = append(transfers, transfer)
	}
	gfm.mu.RUnlock()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"transfers": transfers,
		"queue_length": len(gfm.transferQueue),
		"offline_queue_length": func() int {
			total := 0
			for _, requests := range gfm.offlineQueue {
				total += len(requests)
			}
			return total
		}(),
	})
}

func (gfm *GroundFileManager) apiUploadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	localPath := r.FormValue("local_path")
	remotePath := r.FormValue("remote_path")
	satelliteID := r.FormValue("satellite_id")
	nodeID := r.FormValue("node_id")
	
	if localPath == "" || remotePath == "" || satelliteID == "" || nodeID == "" {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}
	
	request, err := gfm.UploadFile(localPath, remotePath, satelliteID, nodeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(request)
}

func (gfm *GroundFileManager) apiDownloadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	remotePath := r.FormValue("remote_path")
	localPath := r.FormValue("local_path")
	satelliteID := r.FormValue("satellite_id")
	nodeID := r.FormValue("node_id")
	
	if remotePath == "" || satelliteID == "" || nodeID == "" {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}
	
	request, err := gfm.DownloadFile(remotePath, localPath, satelliteID, nodeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(request)
}

func (gfm *GroundFileManager) apiGetSatelliteStatus(w http.ResponseWriter, r *http.Request) {
	status := gfm.constellation.GetSatelliteStatusSummary()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (gfm *GroundFileManager) apiControlSatellite(w http.ResponseWriter, r *http.Request) {
	// 解析路径参数
	path := r.URL.Path[len("/api/satellites/"):]
	parts := strings.Split(path, "/")
	
	if len(parts) < 2 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	
	satelliteID := parts[0]
	action := parts[1]
	
	sat, exists := gfm.constellation.GetSatelliteInfo(satelliteID)
	if !exists {
		http.Error(w, "Satellite not found", http.StatusNotFound)
		return
	}
	
	switch action {
	case "online":
		// 强制卫星上线（用于测试）
		sat.CommunicationWindow.NextOnlineTime = time.Now()
		for _, node := range sat.Nodes {
			node.Status = "online"
		}
		log.Printf("Manually brought satellite %s online", satelliteID)
		
	case "offline":
		// 强制卫星离线（用于测试）
		sat.CommunicationWindow.NextOnlineTime = time.Now().Add(10 * time.Minute)
		for _, node := range sat.Nodes {
			node.Status = "offline"
		}
		log.Printf("Manually brought satellite %s offline", satelliteID)
		
	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}