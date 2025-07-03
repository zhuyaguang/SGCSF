package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sgcsf/sgcsf-go/pkg/core"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// TopicCLI Topic管理命令行工具
type TopicCLI struct {
	configPath     string
	configManager  *core.TopicConfigManager
	outputFormat   string
}

// Topic管理数据结构
type TopicInfo struct {
	Name         string            `json:"name" yaml:"name"`
	Namespace    string            `json:"namespace" yaml:"namespace"`
	Type         string            `json:"type" yaml:"type"`
	QoS          string            `json:"qos" yaml:"qos"`
	Persistent   bool              `json:"persistent" yaml:"persistent"`
	Subscribers  []string          `json:"subscribers" yaml:"subscribers"`
	Publishers   []string          `json:"publishers" yaml:"publishers"`
	Metadata     map[string]string `json:"metadata" yaml:"metadata"`
}

func main() {
	cli := &TopicCLI{
		configPath:   "./config/topics.yaml",
		outputFormat: "table",
	}
	
	rootCmd := &cobra.Command{
		Use:   "topic-cli",
		Short: "SGCSF Topic管理命令行工具",
		Long:  "用于管理SGCSF系统中的Topic配置，支持增删改查和权限管理",
	}
	
	// 全局标志
	rootCmd.PersistentFlags().StringVarP(&cli.configPath, "config", "c", cli.configPath, "Topic配置文件路径")
	rootCmd.PersistentFlags().StringVarP(&cli.outputFormat, "output", "o", cli.outputFormat, "输出格式 (table|json|yaml)")
	
	// 添加子命令
	rootCmd.AddCommand(
		cli.createListCommand(),
		cli.createShowCommand(),
		cli.createAddCommand(),
		cli.createRemoveCommand(),
		cli.createValidateCommand(),
		cli.createGroupCommand(),
		cli.createAccessCommand(),
		cli.createDynamicCommand(),
		cli.createStatsCommand(),
	)
	
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

// createListCommand 创建列表命令
func (cli *TopicCLI) createListCommand() *cobra.Command {
	var namespace, group, filter string
	var showDetails bool
	
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出所有Topic",
		Long:  "列出系统中配置的所有Topic，可按命名空间或组过滤",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.listTopics(namespace, group, filter, showDetails)
		},
	}
	
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "按命名空间过滤")
	cmd.Flags().StringVarP(&group, "group", "g", "", "按Topic组过滤")
	cmd.Flags().StringVarP(&filter, "filter", "f", "", "按名称模式过滤")
	cmd.Flags().BoolVarP(&showDetails, "details", "d", false, "显示详细信息")
	
	return cmd
}

// createShowCommand 创建显示命令
func (cli *TopicCLI) createShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <topic-name>",
		Short: "显示Topic详情",
		Long:  "显示指定Topic的详细配置信息",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.showTopic(args[0])
		},
	}
	
	return cmd
}

// createAddCommand 创建添加命令
func (cli *TopicCLI) createAddCommand() *cobra.Command {
	var (
		namespace    string
		topicType    string
		qos          string
		persistent   bool
		description  string
		retention    int
		maxSize      int
		compression  bool
		encryption   bool
		subscribers  []string
		publishers   []string
	)
	
	cmd := &cobra.Command{
		Use:   "add <topic-name>",
		Short: "添加新Topic",
		Long:  "向配置文件中添加新的Topic定义",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			topicDef := &core.TopicDefinition{
				Name:             args[0],
				Namespace:        namespace,
				Description:      description,
				Type:             topicType,
				QoS:              qos,
				Persistent:       persistent,
				RetentionSeconds: retention,
				MaxSizeMB:        maxSize,
				Compression:      compression,
				Encryption:       encryption,
				Subscribers:      subscribers,
				Publishers:       publishers,
				Metadata:         make(map[string]string),
			}
			
			return cli.addTopic(topicDef)
		},
	}
	
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Topic命名空间")
	cmd.Flags().StringVarP(&topicType, "type", "t", "unicast", "Topic类型 (unicast|multicast|broadcast)")
	cmd.Flags().StringVarP(&qos, "qos", "q", "at_least_once", "QoS级别 (at_most_once|at_least_once|exactly_once)")
	cmd.Flags().BoolVarP(&persistent, "persistent", "p", false, "是否持久化")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Topic描述")
	cmd.Flags().IntVar(&retention, "retention", 300, "消息保留时间(秒)")
	cmd.Flags().IntVar(&maxSize, "max-size", 10, "最大消息大小(MB)")
	cmd.Flags().BoolVar(&compression, "compression", false, "是否启用压缩")
	cmd.Flags().BoolVar(&encryption, "encryption", false, "是否启用加密")
	cmd.Flags().StringSliceVar(&subscribers, "subscribers", []string{}, "允许订阅的客户端模式")
	cmd.Flags().StringSliceVar(&publishers, "publishers", []string{}, "允许发布的客户端模式")
	
	return cmd
}

// createRemoveCommand 创建删除命令
func (cli *TopicCLI) createRemoveCommand() *cobra.Command {
	var force bool
	
	cmd := &cobra.Command{
		Use:   "remove <topic-name>",
		Short: "删除Topic",
		Long:  "从配置文件中删除指定的Topic",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.removeTopic(args[0], force)
		},
	}
	
	cmd.Flags().BoolVarP(&force, "force", "f", false, "强制删除，不确认")
	
	return cmd
}

// createValidateCommand 创建验证命令
func (cli *TopicCLI) createValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "验证配置文件",
		Long:  "验证Topic配置文件的语法和语义正确性",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.validateConfig()
		},
	}
	
	return cmd
}

// createGroupCommand 创建组管理命令
func (cli *TopicCLI) createGroupCommand() *cobra.Command {
	groupCmd := &cobra.Command{
		Use:   "group",
		Short: "Topic组管理",
		Long:  "管理Topic组，包括创建、删除和查看组信息",
	}
	
	// group list
	groupCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "列出所有Topic组",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.listGroups()
		},
	})
	
	// group show
	groupCmd.AddCommand(&cobra.Command{
		Use:   "show <group-name>",
		Short: "显示Topic组详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.showGroup(args[0])
		},
	})
	
	// group create
	var description string
	var topics []string
	createGroupCmd := &cobra.Command{
		Use:   "create <group-name>",
		Short: "创建Topic组",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.createGroup(args[0], description, topics)
		},
	}
	createGroupCmd.Flags().StringVarP(&description, "description", "d", "", "组描述")
	createGroupCmd.Flags().StringSliceVar(&topics, "topics", []string{}, "组中的Topic列表")
	groupCmd.AddCommand(createGroupCmd)
	
	return groupCmd
}

// createAccessCommand 创建访问控制命令
func (cli *TopicCLI) createAccessCommand() *cobra.Command {
	accessCmd := &cobra.Command{
		Use:   "access",
		Short: "访问控制管理",
		Long:  "管理Topic的访问控制规则",
	}
	
	// access list
	accessCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "列出访问控制规则",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.listAccessRules()
		},
	})
	
	// access check
	var clientID, operation string
	checkCmd := &cobra.Command{
		Use:   "check <topic-name>",
		Short: "检查访问权限",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.checkAccess(args[0], clientID, operation)
		},
	}
	checkCmd.Flags().StringVar(&clientID, "client", "", "客户端ID")
	checkCmd.Flags().StringVar(&operation, "operation", "subscribe", "操作类型 (publish|subscribe)")
	accessCmd.AddCommand(checkCmd)
	
	return accessCmd
}

// createDynamicCommand 创建动态Topic命令
func (cli *TopicCLI) createDynamicCommand() *cobra.Command {
	dynamicCmd := &cobra.Command{
		Use:   "dynamic",
		Short: "动态Topic管理",
		Long:  "管理动态创建的Topic",
	}
	
	// dynamic list
	dynamicCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "列出动态Topic模式",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.listDynamicPatterns()
		},
	})
	
	// dynamic create
	var variables []string
	createCmd := &cobra.Command{
		Use:   "create <topic-name>",
		Short: "创建动态Topic实例",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			varsMap := make(map[string]string)
			for _, v := range variables {
				parts := strings.SplitN(v, "=", 2)
				if len(parts) == 2 {
					varsMap[parts[0]] = parts[1]
				}
			}
			return cli.createDynamicTopic(args[0], varsMap)
		},
	}
	createCmd.Flags().StringSliceVar(&variables, "vars", []string{}, "变量定义 (key=value)")
	dynamicCmd.AddCommand(createCmd)
	
	return dynamicCmd
}

// createStatsCommand 创建统计命令
func (cli *TopicCLI) createStatsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "显示Topic统计信息",
		Long:  "显示Topic的使用统计和系统概览",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.showStats()
		},
	}
	
	return cmd
}

// 实现命令处理方法

// listTopics 列出Topic
func (cli *TopicCLI) listTopics(namespace, group, filter string, showDetails bool) error {
	config, err := cli.loadConfig()
	if err != nil {
		return err
	}
	
	var topics []*core.TopicDefinition
	
	// 过滤逻辑
	for _, topic := range config.Topics {
		// 命名空间过滤
		if namespace != "" && topic.Namespace != namespace {
			continue
		}
		
		// 名称过滤
		if filter != "" && !strings.Contains(topic.Name, filter) {
			continue
		}
		
		topics = append(topics, topic)
	}
	
	// 组过滤
	if group != "" {
		if groupDef, exists := config.TopicGroups[group]; exists {
			groupTopics := make(map[string]bool)
			for _, name := range groupDef.Topics {
				groupTopics[name] = true
			}
			
			var filteredTopics []*core.TopicDefinition
			for _, topic := range topics {
				if groupTopics[topic.Name] {
					filteredTopics = append(filteredTopics, topic)
				}
			}
			topics = filteredTopics
		}
	}
	
	return cli.displayTopics(topics, showDetails)
}

// showTopic 显示Topic详情
func (cli *TopicCLI) showTopic(name string) error {
	config, err := cli.loadConfig()
	if err != nil {
		return err
	}
	
	for _, topic := range config.Topics {
		if topic.Name == name {
			return cli.displaySingleTopic(topic)
		}
	}
	
	return fmt.Errorf("topic not found: %s", name)
}

// addTopic 添加Topic
func (cli *TopicCLI) addTopic(topicDef *core.TopicDefinition) error {
	config, err := cli.loadConfig()
	if err != nil {
		return err
	}
	
	// 检查是否已存在
	for _, existing := range config.Topics {
		if existing.Name == topicDef.Name {
			return fmt.Errorf("topic already exists: %s", topicDef.Name)
		}
	}
	
	// 验证命名空间
	if _, exists := config.Namespaces[topicDef.Namespace]; !exists {
		return fmt.Errorf("namespace not found: %s", topicDef.Namespace)
	}
	
	// 添加到配置
	config.Topics = append(config.Topics, topicDef)
	config.LastUpdated = time.Now().Format(time.RFC3339)
	
	// 保存配置
	if err := cli.saveConfig(config); err != nil {
		return err
	}
	
	fmt.Printf("Topic added successfully: %s\n", topicDef.Name)
	return nil
}

// removeTopic 删除Topic
func (cli *TopicCLI) removeTopic(name string, force bool) error {
	config, err := cli.loadConfig()
	if err != nil {
		return err
	}
	
	// 查找Topic
	index := -1
	for i, topic := range config.Topics {
		if topic.Name == name {
			index = i
			break
		}
	}
	
	if index == -1 {
		return fmt.Errorf("topic not found: %s", name)
	}
	
	// 确认删除
	if !force {
		fmt.Printf("Are you sure you want to delete topic '%s'? (y/N): ", name)
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
			fmt.Println("Operation cancelled")
			return nil
		}
	}
	
	// 删除Topic
	config.Topics = append(config.Topics[:index], config.Topics[index+1:]...)
	config.LastUpdated = time.Now().Format(time.RFC3339)
	
	// 保存配置
	if err := cli.saveConfig(config); err != nil {
		return err
	}
	
	fmt.Printf("Topic deleted successfully: %s\n", name)
	return nil
}

// validateConfig 验证配置
func (cli *TopicCLI) validateConfig() error {
	config, err := cli.loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}
	
	// 验证命名空间
	for _, topic := range config.Topics {
		if _, exists := config.Namespaces[topic.Namespace]; !exists {
			return fmt.Errorf("topic %s references undefined namespace: %s", topic.Name, topic.Namespace)
		}
	}
	
	// 验证Topic名称唯一性
	names := make(map[string]bool)
	for _, topic := range config.Topics {
		if names[topic.Name] {
			return fmt.Errorf("duplicate topic name: %s", topic.Name)
		}
		names[topic.Name] = true
	}
	
	// 验证Topic组
	for groupName, group := range config.TopicGroups {
		for _, topicName := range group.Topics {
			found := false
			for _, topic := range config.Topics {
				if topic.Name == topicName {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("group %s references undefined topic: %s", groupName, topicName)
			}
		}
	}
	
	fmt.Println("Configuration is valid")
	return nil
}

// 辅助方法

// loadConfig 加载配置
func (cli *TopicCLI) loadConfig() (*core.TopicConfiguration, error) {
	data, err := ioutil.ReadFile(cli.configPath)
	if err != nil {
		return nil, err
	}
	
	config := &core.TopicConfiguration{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, err
	}
	
	return config, nil
}

// saveConfig 保存配置
func (cli *TopicCLI) saveConfig(config *core.TopicConfiguration) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	
	return ioutil.WriteFile(cli.configPath, data, 0644)
}

// displayTopics 显示Topic列表
func (cli *TopicCLI) displayTopics(topics []*core.TopicDefinition, showDetails bool) error {
	switch cli.outputFormat {
	case "json":
		return cli.displayJSON(topics)
	case "yaml":
		return cli.displayYAML(topics)
	default:
		return cli.displayTable(topics, showDetails)
	}
}

// displayTable 表格显示
func (cli *TopicCLI) displayTable(topics []*core.TopicDefinition, showDetails bool) error {
	if len(topics) == 0 {
		fmt.Println("No topics found")
		return nil
	}
	
	// 排序
	sort.Slice(topics, func(i, j int) bool {
		return topics[i].Name < topics[j].Name
	})
	
	if showDetails {
		// 详细表格
		fmt.Printf("%-30s %-12s %-10s %-8s %-12s %-50s\n", 
			"NAME", "NAMESPACE", "TYPE", "QOS", "PERSISTENT", "DESCRIPTION")
		fmt.Println(strings.Repeat("-", 130))
		
		for _, topic := range topics {
			fmt.Printf("%-30s %-12s %-10s %-8s %-12s %-50s\n",
				topic.Name,
				topic.Namespace,
				topic.Type,
				topic.QoS,
				strconv.FormatBool(topic.Persistent),
				truncate(topic.Description, 50))
		}
	} else {
		// 简单表格
		fmt.Printf("%-40s %-12s %-10s\n", "NAME", "NAMESPACE", "TYPE")
		fmt.Println(strings.Repeat("-", 65))
		
		for _, topic := range topics {
			fmt.Printf("%-40s %-12s %-10s\n", topic.Name, topic.Namespace, topic.Type)
		}
	}
	
	fmt.Printf("\nTotal: %d topics\n", len(topics))
	return nil
}

// displayJSON JSON显示
func (cli *TopicCLI) displayJSON(data interface{}) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(jsonData))
	return nil
}

// displayYAML YAML显示
func (cli *TopicCLI) displayYAML(data interface{}) error {
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	fmt.Print(string(yamlData))
	return nil
}

// displaySingleTopic 显示单个Topic
func (cli *TopicCLI) displaySingleTopic(topic *core.TopicDefinition) error {
	switch cli.outputFormat {
	case "json":
		return cli.displayJSON(topic)
	case "yaml":
		return cli.displayYAML(topic)
	default:
		fmt.Printf("Topic: %s\n", topic.Name)
		fmt.Printf("Namespace: %s\n", topic.Namespace)
		fmt.Printf("Type: %s\n", topic.Type)
		fmt.Printf("QoS: %s\n", topic.QoS)
		fmt.Printf("Persistent: %t\n", topic.Persistent)
		fmt.Printf("Retention: %d seconds\n", topic.RetentionSeconds)
		fmt.Printf("Max Size: %d MB\n", topic.MaxSizeMB)
		fmt.Printf("Compression: %t\n", topic.Compression)
		fmt.Printf("Encryption: %t\n", topic.Encryption)
		fmt.Printf("Description: %s\n", topic.Description)
		
		if len(topic.Subscribers) > 0 {
			fmt.Printf("Subscribers: %s\n", strings.Join(topic.Subscribers, ", "))
		}
		
		if len(topic.Publishers) > 0 {
			fmt.Printf("Publishers: %s\n", strings.Join(topic.Publishers, ", "))
		}
		
		if len(topic.Metadata) > 0 {
			fmt.Println("Metadata:")
			for k, v := range topic.Metadata {
				fmt.Printf("  %s: %s\n", k, v)
			}
		}
		
		return nil
	}
}

// listGroups 列出组
func (cli *TopicCLI) listGroups() error {
	config, err := cli.loadConfig()
	if err != nil {
		return err
	}
	
	if len(config.TopicGroups) == 0 {
		fmt.Println("No topic groups found")
		return nil
	}
	
	fmt.Printf("%-20s %-15s %-50s\n", "NAME", "TOPICS", "DESCRIPTION")
	fmt.Println(strings.Repeat("-", 87))
	
	for name, group := range config.TopicGroups {
		fmt.Printf("%-20s %-15d %-50s\n", 
			name, 
			len(group.Topics), 
			truncate(group.Description, 50))
	}
	
	return nil
}

// showGroup 显示组详情
func (cli *TopicCLI) showGroup(name string) error {
	config, err := cli.loadConfig()
	if err != nil {
		return err
	}
	
	group, exists := config.TopicGroups[name]
	if !exists {
		return fmt.Errorf("group not found: %s", name)
	}
	
	fmt.Printf("Group: %s\n", name)
	fmt.Printf("Description: %s\n", group.Description)
	fmt.Printf("Topics (%d):\n", len(group.Topics))
	
	for _, topic := range group.Topics {
		fmt.Printf("  - %s\n", topic)
	}
	
	return nil
}

// createGroup 创建组
func (cli *TopicCLI) createGroup(name, description string, topics []string) error {
	config, err := cli.loadConfig()
	if err != nil {
		return err
	}
	
	if config.TopicGroups == nil {
		config.TopicGroups = make(map[string]*core.TopicGroup)
	}
	
	if _, exists := config.TopicGroups[name]; exists {
		return fmt.Errorf("group already exists: %s", name)
	}
	
	config.TopicGroups[name] = &core.TopicGroup{
		Description: description,
		Topics:      topics,
	}
	
	config.LastUpdated = time.Now().Format(time.RFC3339)
	
	if err := cli.saveConfig(config); err != nil {
		return err
	}
	
	fmt.Printf("Group created successfully: %s\n", name)
	return nil
}

// listAccessRules 列出访问规则
func (cli *TopicCLI) listAccessRules() error {
	config, err := cli.loadConfig()
	if err != nil {
		return err
	}
	
	if config.AccessControl == nil || len(config.AccessControl.Rules) == 0 {
		fmt.Println("No access control rules found")
		return nil
	}
	
	fmt.Printf("%-30s %-20s %-20s\n", "PATTERN", "PUBLISHERS", "SUBSCRIBERS")
	fmt.Println(strings.Repeat("-", 72))
	
	for _, rule := range config.AccessControl.Rules {
		publishers := strings.Join(rule.AllowPublish, ",")
		subscribers := strings.Join(rule.AllowSubscribe, ",")
		
		fmt.Printf("%-30s %-20s %-20s\n", 
			rule.Pattern,
			truncate(publishers, 20),
			truncate(subscribers, 20))
	}
	
	return nil
}

// checkAccess 检查访问权限
func (cli *TopicCLI) checkAccess(topicName, clientID, operation string) error {
	// 这里需要实际的权限检查逻辑
	fmt.Printf("Checking %s permission for client '%s' on topic '%s'\n", operation, clientID, topicName)
	fmt.Println("Permission check would be implemented here")
	return nil
}

// listDynamicPatterns 列出动态模式
func (cli *TopicCLI) listDynamicPatterns() error {
	config, err := cli.loadConfig()
	if err != nil {
		return err
	}
	
	if config.DynamicTopics == nil || len(config.DynamicTopics.Patterns) == 0 {
		fmt.Println("No dynamic topic patterns found")
		return nil
	}
	
	fmt.Printf("%-40s %-10s %-10s %-50s\n", "PATTERN", "MAX_INST", "TTL", "DESCRIPTION")
	fmt.Println(strings.Repeat("-", 112))
	
	for _, pattern := range config.DynamicTopics.Patterns {
		fmt.Printf("%-40s %-10d %-10d %-50s\n", 
			pattern.Pattern,
			pattern.MaxInstances,
			pattern.TTLSeconds,
			truncate(pattern.Description, 50))
	}
	
	return nil
}

// createDynamicTopic 创建动态Topic
func (cli *TopicCLI) createDynamicTopic(name string, variables map[string]string) error {
	fmt.Printf("Creating dynamic topic: %s\n", name)
	fmt.Printf("Variables: %+v\n", variables)
	fmt.Println("Dynamic topic creation would be implemented here")
	return nil
}

// showStats 显示统计信息
func (cli *TopicCLI) showStats() error {
	config, err := cli.loadConfig()
	if err != nil {
		return err
	}
	
	// 统计信息
	totalTopics := len(config.Topics)
	namespaceCount := len(config.Namespaces)
	groupCount := len(config.TopicGroups)
	
	// 按类型统计
	typeStats := make(map[string]int)
	qosStats := make(map[string]int)
	namespaceStats := make(map[string]int)
	
	for _, topic := range config.Topics {
		typeStats[topic.Type]++
		qosStats[topic.QoS]++
		namespaceStats[topic.Namespace]++
	}
	
	fmt.Println("=== SGCSF Topic Statistics ===")
	fmt.Printf("Total Topics: %d\n", totalTopics)
	fmt.Printf("Namespaces: %d\n", namespaceCount)
	fmt.Printf("Topic Groups: %d\n", groupCount)
	fmt.Println()
	
	fmt.Println("By Type:")
	for t, count := range typeStats {
		fmt.Printf("  %s: %d\n", t, count)
	}
	fmt.Println()
	
	fmt.Println("By QoS:")
	for q, count := range qosStats {
		fmt.Printf("  %s: %d\n", q, count)
	}
	fmt.Println()
	
	fmt.Println("By Namespace:")
	for ns, count := range namespaceStats {
		fmt.Printf("  %s: %d\n", ns, count)
	}
	
	return nil
}

// truncate 截断字符串
func truncate(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length-3] + "..."
}