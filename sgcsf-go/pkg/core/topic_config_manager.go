package core

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
	"gopkg.in/yaml.v3"
)

// TopicConfigManager 管理基于配置文件的topic
type TopicConfigManager struct {
	configPath        string
	config           *TopicConfiguration
	topicManager     TopicManager
	dynamicTopics    map[string]*DynamicTopicInstance
	accessController *TopicAccessController
	validator        *TopicValidator
	mutex           sync.RWMutex
	
	// 配置文件监控
	lastModTime      time.Time
	autoReload       bool
	reloadInterval   time.Duration
}

// TopicConfiguration 配置文件结构
type TopicConfiguration struct {
	Version        string                        `yaml:"version"`
	LastUpdated    string                        `yaml:"last_updated"`
	Description    string                        `yaml:"description"`
	Namespaces     map[string]*NamespaceConfig   `yaml:"namespaces"`
	Topics         []*TopicDefinition            `yaml:"topics"`
	TopicGroups    map[string]*TopicGroup        `yaml:"topic_groups"`
	DynamicTopics  *DynamicTopicConfig           `yaml:"dynamic_topics"`
	AccessControl  *AccessControlConfig          `yaml:"access_control"`
	Validation     *ValidationConfig             `yaml:"validation"`
}

// NamespaceConfig 命名空间配置
type NamespaceConfig struct {
	Description string `yaml:"description"`
	Prefix      string `yaml:"prefix"`
}

// TopicDefinition topic定义
type TopicDefinition struct {
	Name             string            `yaml:"name"`
	Namespace        string            `yaml:"namespace"`
	Description      string            `yaml:"description"`
	Type             string            `yaml:"type"`
	QoS              string            `yaml:"qos"`
	Persistent       bool              `yaml:"persistent"`
	RetentionSeconds int               `yaml:"retention_seconds"`
	MaxSizeMB        int               `yaml:"max_size_mb"`
	Compression      bool              `yaml:"compression"`
	Encryption       bool              `yaml:"encryption"`
	IsTemplate       bool              `yaml:"is_template"`
	Subscribers      []string          `yaml:"subscribers"`
	Publishers       []string          `yaml:"publishers"`
	Metadata         map[string]string `yaml:"metadata"`
}

// TopicGroup topic组
type TopicGroup struct {
	Description string   `yaml:"description"`
	Topics      []string `yaml:"topics"`
}

// DynamicTopicConfig 动态topic配置
type DynamicTopicConfig struct {
	Patterns []*DynamicTopicPattern `yaml:"patterns"`
}

// DynamicTopicPattern 动态topic模式
type DynamicTopicPattern struct {
	Pattern      string `yaml:"pattern"`
	Description  string `yaml:"description"`
	MaxInstances int    `yaml:"max_instances"`
	TTLSeconds   int    `yaml:"ttl_seconds"`
	AutoCleanup  bool   `yaml:"auto_cleanup"`
}

// DynamicTopicInstance 动态topic实例
type DynamicTopicInstance struct {
	Name      string
	Pattern   string
	CreatedAt time.Time
	TTL       time.Duration
	Variables map[string]string
}

// AccessControlConfig 访问控制配置
type AccessControlConfig struct {
	Rules []*AccessRule `yaml:"rules"`
}

// AccessRule 访问规则
type AccessRule struct {
	Pattern         string   `yaml:"pattern"`
	AllowPublish    []string `yaml:"allow_publish"`
	AllowSubscribe  []string `yaml:"allow_subscribe"`
}

// ValidationConfig 验证配置
type ValidationConfig struct {
	TopicName *ValidationRule `yaml:"topic_name"`
	Namespace *ValidationRule `yaml:"namespace"`
	QoS       *ValidationRule `yaml:"qos"`
	Type      *ValidationRule `yaml:"type"`
}

// ValidationRule 验证规则
type ValidationRule struct {
	Pattern       string   `yaml:"pattern"`
	MaxLength     int      `yaml:"max_length"`
	Required      bool     `yaml:"required"`
	MustExist     bool     `yaml:"must_exist"`
	AllowedValues []string `yaml:"allowed_values"`
}

// TopicAccessController 访问控制器
type TopicAccessController struct {
	rules []*AccessRule
}

// TopicValidator topic验证器
type TopicValidator struct {
	config *ValidationConfig
}

// NewTopicConfigManager 创建topic配置管理器
func NewTopicConfigManager(configPath string, topicManager TopicManager) (*TopicConfigManager, error) {
	tcm := &TopicConfigManager{
		configPath:      configPath,
		topicManager:    topicManager,
		dynamicTopics:   make(map[string]*DynamicTopicInstance),
		autoReload:      true,
		reloadInterval:  30 * time.Second,
	}
	
	// 加载配置
	if err := tcm.LoadConfig(); err != nil {
		return nil, fmt.Errorf("failed to load topic config: %v", err)
	}
	
	// 初始化组件
	tcm.accessController = NewTopicAccessController(tcm.config.AccessControl)
	tcm.validator = NewTopicValidator(tcm.config.Validation)
	
	// 启动自动重载
	if tcm.autoReload {
		go tcm.autoReloadLoop()
	}
	
	return tcm, nil
}

// LoadConfig 加载配置文件
func (tcm *TopicConfigManager) LoadConfig() error {
	tcm.mutex.Lock()
	defer tcm.mutex.Unlock()
	
	// 读取配置文件
	data, err := ioutil.ReadFile(tcm.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %v", err)
	}
	
	// 解析YAML
	config := &TopicConfiguration{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return fmt.Errorf("failed to parse config file: %v", err)
	}
	
	// 验证配置
	if err := tcm.validateConfig(config); err != nil {
		return fmt.Errorf("config validation failed: %v", err)
	}
	
	tcm.config = config
	
	// 更新文件修改时间
	if info, err := filepath.Abs(tcm.configPath); err == nil {
		if stat, err := ioutil.ReadFile(info); err == nil {
			tcm.lastModTime = time.Now()
		}
	}
	
	return nil
}

// InitializeTopics 根据配置初始化所有topic
func (tcm *TopicConfigManager) InitializeTopics() error {
	tcm.mutex.RLock()
	defer tcm.mutex.RUnlock()
	
	for _, topicDef := range tcm.config.Topics {
		// 跳过模板topic（动态创建）
		if topicDef.IsTemplate {
			continue
		}
		
		// 转换为SGCSF TopicConfig
		sgcsfConfig := tcm.convertToSGCSFConfig(topicDef)
		
		// 创建topic
		if err := tcm.topicManager.CreateTopic(topicDef.Name, sgcsfConfig); err != nil {
			// 如果topic已存在，更新配置
			if strings.Contains(err.Error(), "already exists") {
				// 可以选择更新或跳过
				continue
			}
			return fmt.Errorf("failed to create topic %s: %v", topicDef.Name, err)
		}
	}
	
	return nil
}

// CreateDynamicTopic 创建动态topic
func (tcm *TopicConfigManager) CreateDynamicTopic(topicName string, variables map[string]string) error {
	tcm.mutex.Lock()
	defer tcm.mutex.Unlock()
	
	// 查找匹配的动态topic模式
	pattern := tcm.findMatchingPattern(topicName)
	if pattern == nil {
		return fmt.Errorf("no dynamic topic pattern matches: %s", topicName)
	}
	
	// 检查实例数量限制
	instanceCount := tcm.countPatternInstances(pattern.Pattern)
	if instanceCount >= pattern.MaxInstances {
		return fmt.Errorf("dynamic topic pattern %s exceeds max instances: %d", pattern.Pattern, pattern.MaxInstances)
	}
	
	// 创建动态topic实例
	instance := &DynamicTopicInstance{
		Name:      topicName,
		Pattern:   pattern.Pattern,
		CreatedAt: time.Now(),
		TTL:       time.Duration(pattern.TTLSeconds) * time.Second,
		Variables: variables,
	}
	
	// 创建默认配置
	config := &types.TopicConfig{
		Name:       topicName,
		Type:       types.TopicUnicast,
		Persistent: false,
		Retention:  pattern.TTLSeconds,
		QoS:        types.QoSAtLeastOnce,
		Metadata:   make(map[string]string),
	}
	
	// 复制变量到metadata
	for k, v := range variables {
		config.Metadata[k] = v
	}
	
	// 创建topic
	if err := tcm.topicManager.CreateTopic(topicName, config); err != nil {
		return err
	}
	
	// 记录实例
	tcm.dynamicTopics[topicName] = instance
	
	// 设置自动清理
	if pattern.AutoCleanup {
		go tcm.scheduleCleanup(topicName, instance.TTL)
	}
	
	return nil
}

// GetTopicDefinition 获取topic定义
func (tcm *TopicConfigManager) GetTopicDefinition(topicName string) (*TopicDefinition, error) {
	tcm.mutex.RLock()
	defer tcm.mutex.RUnlock()
	
	for _, topic := range tcm.config.Topics {
		if topic.Name == topicName {
			return topic, nil
		}
	}
	
	return nil, fmt.Errorf("topic definition not found: %s", topicName)
}

// ListTopicsByGroup 按组列出topic
func (tcm *TopicConfigManager) ListTopicsByGroup(groupName string) ([]string, error) {
	tcm.mutex.RLock()
	defer tcm.mutex.RUnlock()
	
	group, exists := tcm.config.TopicGroups[groupName]
	if !exists {
		return nil, fmt.Errorf("topic group not found: %s", groupName)
	}
	
	return group.Topics, nil
}

// GetTopicsByNamespace 按命名空间获取topic
func (tcm *TopicConfigManager) GetTopicsByNamespace(namespace string) ([]*TopicDefinition, error) {
	tcm.mutex.RLock()
	defer tcm.mutex.RUnlock()
	
	var topics []*TopicDefinition
	for _, topic := range tcm.config.Topics {
		if topic.Namespace == namespace {
			topics = append(topics, topic)
		}
	}
	
	return topics, nil
}

// CheckPublishPermission 检查发布权限
func (tcm *TopicConfigManager) CheckPublishPermission(clientID, topicName string) bool {
	return tcm.accessController.CheckPublishPermission(clientID, topicName)
}

// CheckSubscribePermission 检查订阅权限
func (tcm *TopicConfigManager) CheckSubscribePermission(clientID, topicName string) bool {
	return tcm.accessController.CheckSubscribePermission(clientID, topicName)
}

// ReloadConfig 重新加载配置
func (tcm *TopicConfigManager) ReloadConfig() error {
	oldConfig := tcm.config
	
	if err := tcm.LoadConfig(); err != nil {
		return err
	}
	
	// 比较配置变化并应用
	return tcm.applyConfigChanges(oldConfig, tcm.config)
}

// 辅助方法

// convertToSGCSFConfig 转换为SGCSF配置
func (tcm *TopicConfigManager) convertToSGCSFConfig(topicDef *TopicDefinition) *types.TopicConfig {
	config := &types.TopicConfig{
		Name:       topicDef.Name,
		Type:       tcm.parseTopicType(topicDef.Type),
		Persistent: topicDef.Persistent,
		Retention:  topicDef.RetentionSeconds,
		MaxSize:    int64(topicDef.MaxSizeMB) * 1024 * 1024,
		QoS:        tcm.parseQoSLevel(topicDef.QoS),
		Metadata:   topicDef.Metadata,
	}
	
	if config.Metadata == nil {
		config.Metadata = make(map[string]string)
	}
	
	// 添加额外的元数据
	config.Metadata["description"] = topicDef.Description
	config.Metadata["namespace"] = topicDef.Namespace
	config.Metadata["compression"] = fmt.Sprintf("%t", topicDef.Compression)
	config.Metadata["encryption"] = fmt.Sprintf("%t", topicDef.Encryption)
	
	return config
}

// parseTopicType 解析topic类型
func (tcm *TopicConfigManager) parseTopicType(typeStr string) types.TopicType {
	switch strings.ToLower(typeStr) {
	case "unicast":
		return types.TopicUnicast
	case "multicast":
		return types.TopicMulticast
	case "broadcast":
		return types.TopicBroadcast
	default:
		return types.TopicUnicast
	}
}

// parseQoSLevel 解析QoS级别
func (tcm *TopicConfigManager) parseQoSLevel(qosStr string) types.QoSLevel {
	switch strings.ToLower(qosStr) {
	case "at_most_once":
		return types.QoSAtMostOnce
	case "at_least_once":
		return types.QoSAtLeastOnce
	case "exactly_once":
		return types.QoSExactlyOnce
	default:
		return types.QoSAtLeastOnce
	}
}

// findMatchingPattern 查找匹配的动态topic模式
func (tcm *TopicConfigManager) findMatchingPattern(topicName string) *DynamicTopicPattern {
	if tcm.config.DynamicTopics == nil {
		return nil
	}
	
	for _, pattern := range tcm.config.DynamicTopics.Patterns {
		if tcm.matchDynamicPattern(pattern.Pattern, topicName) {
			return pattern
		}
	}
	
	return nil
}

// matchDynamicPattern 匹配动态模式
func (tcm *TopicConfigManager) matchDynamicPattern(pattern, topicName string) bool {
	// 将{variable}转换为正则表达式
	regexPattern := strings.ReplaceAll(pattern, "{", "(?P<")
	regexPattern = strings.ReplaceAll(regexPattern, "}", ">[^/]+)")
	regexPattern = "^" + regexPattern + "$"
	
	matched, _ := regexp.MatchString(regexPattern, topicName)
	return matched
}

// countPatternInstances 统计模式实例数
func (tcm *TopicConfigManager) countPatternInstances(pattern string) int {
	count := 0
	for _, instance := range tcm.dynamicTopics {
		if instance.Pattern == pattern {
			count++
		}
	}
	return count
}

// scheduleCleanup 调度清理任务
func (tcm *TopicConfigManager) scheduleCleanup(topicName string, ttl time.Duration) {
	time.Sleep(ttl)
	
	tcm.mutex.Lock()
	defer tcm.mutex.Unlock()
	
	// 删除动态topic
	if _, exists := tcm.dynamicTopics[topicName]; exists {
		tcm.topicManager.DeleteTopic(topicName)
		delete(tcm.dynamicTopics, topicName)
	}
}

// validateConfig 验证配置
func (tcm *TopicConfigManager) validateConfig(config *TopicConfiguration) error {
	// 验证命名空间
	for _, topic := range config.Topics {
		if _, exists := config.Namespaces[topic.Namespace]; !exists {
			return fmt.Errorf("topic %s references undefined namespace: %s", topic.Name, topic.Namespace)
		}
	}
	
	// 验证topic名称唯一性
	names := make(map[string]bool)
	for _, topic := range config.Topics {
		if names[topic.Name] {
			return fmt.Errorf("duplicate topic name: %s", topic.Name)
		}
		names[topic.Name] = true
	}
	
	return nil
}

// autoReloadLoop 自动重载循环
func (tcm *TopicConfigManager) autoReloadLoop() {
	ticker := time.NewTicker(tcm.reloadInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// 检查文件是否修改
			if tcm.isConfigModified() {
				if err := tcm.ReloadConfig(); err != nil {
					// 记录错误但继续运行
					fmt.Printf("Failed to reload config: %v\n", err)
				}
			}
		}
	}
}

// isConfigModified 检查配置文件是否修改
func (tcm *TopicConfigManager) isConfigModified() bool {
	info, err := ioutil.ReadFile(tcm.configPath)
	if err != nil {
		return false
	}
	
	// 这里应该检查文件的修改时间，简化实现
	return len(info) > 0 // 简化的检查
}

// applyConfigChanges 应用配置变化
func (tcm *TopicConfigManager) applyConfigChanges(oldConfig, newConfig *TopicConfiguration) error {
	// 这里可以实现增量更新逻辑
	// 为简化实现，重新初始化所有topic
	return tcm.InitializeTopics()
}

// NewTopicAccessController 创建访问控制器
func NewTopicAccessController(config *AccessControlConfig) *TopicAccessController {
	return &TopicAccessController{
		rules: config.Rules,
	}
}

// CheckPublishPermission 检查发布权限
func (tac *TopicAccessController) CheckPublishPermission(clientID, topicName string) bool {
	for _, rule := range tac.rules {
		if tac.matchPattern(rule.Pattern, topicName) {
			return tac.matchClientPattern(rule.AllowPublish, clientID)
		}
	}
	return false // 默认拒绝
}

// CheckSubscribePermission 检查订阅权限
func (tac *TopicAccessController) CheckSubscribePermission(clientID, topicName string) bool {
	for _, rule := range tac.rules {
		if tac.matchPattern(rule.Pattern, topicName) {
			return tac.matchClientPattern(rule.AllowSubscribe, clientID)
		}
	}
	return false // 默认拒绝
}

// matchPattern 匹配模式
func (tac *TopicAccessController) matchPattern(pattern, topic string) bool {
	// 支持通配符匹配
	if pattern == "*" {
		return true
	}
	
	// 支持前缀匹配
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(topic, prefix)
	}
	
	return pattern == topic
}

// matchClientPattern 匹配客户端模式
func (tac *TopicAccessController) matchClientPattern(patterns []string, clientID string) bool {
	for _, pattern := range patterns {
		if tac.matchPattern(pattern, clientID) {
			return true
		}
	}
	return false
}

// NewTopicValidator 创建验证器
func NewTopicValidator(config *ValidationConfig) *TopicValidator {
	return &TopicValidator{
		config: config,
	}
}

// ValidateTopicName 验证topic名称
func (tv *TopicValidator) ValidateTopicName(name string) error {
	if tv.config.TopicName == nil {
		return nil
	}
	
	rule := tv.config.TopicName
	
	// 检查长度
	if rule.MaxLength > 0 && len(name) > rule.MaxLength {
		return fmt.Errorf("topic name too long: %d > %d", len(name), rule.MaxLength)
	}
	
	// 检查模式
	if rule.Pattern != "" {
		matched, err := regexp.MatchString(rule.Pattern, name)
		if err != nil {
			return fmt.Errorf("pattern matching error: %v", err)
		}
		if !matched {
			return fmt.Errorf("topic name does not match pattern: %s", rule.Pattern)
		}
	}
	
	return nil
}