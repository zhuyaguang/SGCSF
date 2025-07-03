#!/bin/bash

# SGCSF 文件传输离线场景测试脚本
# 用于模拟各种离线场景和测试文件传输的可靠性

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# 配置
BASE_URL="http://localhost:8082"
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_scenario() {
    echo -e "${CYAN}[SCENARIO]${NC} $1"
}

# 等待函数
wait_with_spinner() {
    local duration=$1
    local message=$2
    
    echo -n "$message"
    for ((i=0; i<duration; i++)); do
        sleep 1
        echo -n "."
    done
    echo " 完成"
}

# API调用函数
api_call() {
    local method=$1
    local endpoint=$2
    local data=$3
    
    if [ "$method" = "GET" ]; then
        curl -s "${BASE_URL}${endpoint}" 2>/dev/null || echo '{"error": "API调用失败"}'
    else
        curl -s -X "$method" "${BASE_URL}${endpoint}" -d "$data" 2>/dev/null || echo '{"error": "API调用失败"}'
    fi
}

# 获取系统状态
get_system_status() {
    api_call GET "/api/satellites/status"
}

# 获取传输状态
get_transfer_status() {
    api_call GET "/api/transfers"
}

# 控制卫星状态
control_satellite() {
    local satellite_id=$1
    local action=$2
    
    api_call POST "/api/satellites/${satellite_id}/${action}" ""
}

# 上传文件
upload_file() {
    local local_path=$1
    local remote_path=$2
    local satellite_id=$3
    local node_id=$4
    
    local data="local_path=${local_path}&remote_path=${remote_path}&satellite_id=${satellite_id}&node_id=${node_id}"
    api_call POST "/api/transfer/upload" "$data"
}

# 下载文件
download_file() {
    local remote_path=$1
    local satellite_id=$2
    local node_id=$3
    local local_path=${4:-""}
    
    local data="remote_path=${remote_path}&satellite_id=${satellite_id}&node_id=${node_id}"
    if [ -n "$local_path" ]; then
        data="${data}&local_path=${local_path}"
    fi
    
    api_call POST "/api/transfer/download" "$data"
}

# 等待传输完成
wait_for_transfer() {
    local transfer_id=$1
    local max_wait=${2:-60}
    local waited=0
    
    while [ $waited -lt $max_wait ]; do
        local status=$(get_transfer_status | jq -r ".transfers[] | select(.id == \"$transfer_id\") | .status" 2>/dev/null || echo "")
        
        case "$status" in
            "completed")
                return 0
                ;;
            "failed")
                return 1
                ;;
            "")
                log_warning "传输 $transfer_id 未找到"
                return 2
                ;;
        esac
        
        sleep 2
        waited=$((waited + 2))
    done
    
    log_warning "传输 $transfer_id 超时"
    return 3
}

# 显示系统状态
show_system_status() {
    log_info "获取系统状态..."
    
    local status=$(get_system_status)
    local total_sats=$(echo "$status" | jq -r '.total_satellites // "N/A"' 2>/dev/null || echo "N/A")
    local online_sats=$(echo "$status" | jq -r '.online_satellites // "N/A"' 2>/dev/null || echo "N/A")
    local offline_sats=$(echo "$status" | jq -r '.offline_satellites // "N/A"' 2>/dev/null || echo "N/A")
    local health=$(echo "$status" | jq -r '.constellation_health // "N/A"' 2>/dev/null || echo "N/A")
    
    echo "  总卫星数: $total_sats"
    echo "  在线卫星: $online_sats"
    echo "  离线卫星: $offline_sats"
    echo "  系统健康度: $health%"
    
    local transfers=$(get_transfer_status)
    local active_count=$(echo "$transfers" | jq -r '.transfers | length // 0' 2>/dev/null || echo "0")
    local queue_length=$(echo "$transfers" | jq -r '.queue_length // 0' 2>/dev/null || echo "0")
    
    echo "  活跃传输: $active_count"
    echo "  队列长度: $queue_length"
}

# 场景1: 基本的离线上传场景
scenario_offline_upload() {
    log_scenario "场景1: 向离线卫星上传文件"
    
    # 1. 确保目标卫星离线
    log_info "1. 强制卫星 sat-01 离线"
    control_satellite "sat-01" "offline" >/dev/null
    
    wait_with_spinner 3 "等待卫星状态更新"
    
    # 2. 尝试上传文件
    log_info "2. 向离线卫星上传配置文件"
    local upload_result=$(upload_file "test-data/config.json" "config.json" "sat-01" "node-1")
    local transfer_id=$(echo "$upload_result" | jq -r '.id // ""' 2>/dev/null || echo "")
    local status=$(echo "$upload_result" | jq -r '.status // ""' 2>/dev/null || echo "")
    
    if [ -n "$transfer_id" ]; then
        log_success "上传请求已创建: $transfer_id"
        if [ "$status" = "offline_queued" ]; then
            log_success "✅ 文件已正确加入离线队列"
        else
            log_warning "⚠️ 文件状态异常: $status"
        fi
    else
        log_error "❌ 上传请求创建失败"
        return 1
    fi
    
    # 3. 等待一段时间，验证文件在离线队列中
    wait_with_spinner 5 "验证离线队列状态"
    
    # 4. 卫星上线，观察传输开始
    log_info "3. 卫星上线，观察传输开始"
    control_satellite "sat-01" "online" >/dev/null
    
    wait_with_spinner 5 "等待传输开始"
    
    # 5. 等待传输完成
    log_info "4. 等待传输完成"
    if wait_for_transfer "$transfer_id" 30; then
        log_success "✅ 场景1成功: 离线上传文件成功完成"
    else
        log_error "❌ 场景1失败: 传输未能完成"
        return 1
    fi
    
    echo ""
}

# 场景2: 批量文件传输场景
scenario_batch_transfer() {
    log_scenario "场景2: 批量文件向多个离线卫星传输"
    
    # 1. 确保多个卫星离线
    log_info "1. 设置多个卫星离线"
    for sat_id in "sat-01" "sat-02" "sat-03"; do
        control_satellite "$sat_id" "offline" >/dev/null
    done
    
    wait_with_spinner 3 "等待卫星状态更新"
    
    # 2. 批量上传不同类型的文件
    log_info "2. 批量上传文件到不同卫星"
    
    declare -A transfers
    
    # 小文件到 sat-01
    local result1=$(upload_file "test-data/config.json" "config.json" "sat-01" "node-1")
    transfers["sat-01"]=$(echo "$result1" | jq -r '.id // ""' 2>/dev/null || echo "")
    
    # 日志文件到 sat-02
    local result2=$(upload_file "test-data/system.log" "system.log" "sat-02" "node-1")
    transfers["sat-02"]=$(echo "$result2" | jq -r '.id // ""' 2>/dev/null || echo "")
    
    # 传感器数据到 sat-03
    local result3=$(upload_file "test-data/sensor-data.csv" "sensor-data.csv" "sat-03" "node-1")
    transfers["sat-03"]=$(echo "$result3" | jq -r '.id // ""' 2>/dev/null || echo "")
    
    log_success "已创建 ${#transfers[@]} 个传输请求"
    
    # 3. 逐个让卫星上线，观察队列处理
    log_info "3. 逐个让卫星上线，观察队列处理"
    
    for sat_id in "sat-01" "sat-02" "sat-03"; do
        log_info "让 $sat_id 上线"
        control_satellite "$sat_id" "online" >/dev/null
        
        local transfer_id=${transfers[$sat_id]}
        if [ -n "$transfer_id" ]; then
            wait_with_spinner 10 "等待 $sat_id 的传输完成"
            
            if wait_for_transfer "$transfer_id" 30; then
                log_success "✅ $sat_id 传输完成"
            else
                log_warning "⚠️ $sat_id 传输超时或失败"
            fi
        fi
        
        # 让卫星重新离线
        control_satellite "$sat_id" "offline" >/dev/null
        sleep 2
    done
    
    log_success "✅ 场景2完成: 批量传输测试"
    echo ""
}

# 场景3: 网络中断恢复场景
scenario_network_interruption() {
    log_scenario "场景3: 模拟网络中断和恢复"
    
    # 1. 开始一个大文件传输
    log_info "1. 开始大文件传输 (固件更新)"
    control_satellite "sat-01" "online" >/dev/null
    
    wait_with_spinner 3 "等待卫星就绪"
    
    local upload_result=$(upload_file "test-data/firmware-update.bin" "firmware-update.bin" "sat-01" "node-1")
    local transfer_id=$(echo "$upload_result" | jq -r '.id // ""' 2>/dev/null || echo "")
    
    if [ -z "$transfer_id" ]; then
        log_error "❌ 无法开始大文件传输"
        return 1
    fi
    
    log_success "大文件传输已开始: $transfer_id"
    
    # 2. 等待传输开始
    wait_with_spinner 5 "等待传输开始"
    
    # 3. 模拟网络中断 (卫星离线)
    log_info "2. 模拟网络中断 (卫星强制离线)"
    control_satellite "sat-01" "offline" >/dev/null
    
    wait_with_spinner 10 "模拟网络中断期间"
    
    # 4. 恢复网络连接
    log_info "3. 恢复网络连接 (卫星重新上线)"
    control_satellite "sat-01" "online" >/dev/null
    
    # 5. 等待传输恢复和完成
    log_info "4. 等待传输恢复并完成"
    if wait_for_transfer "$transfer_id" 120; then  # 大文件需要更长时间
        log_success "✅ 场景3成功: 网络中断后传输成功恢复"
    else
        log_error "❌ 场景3失败: 传输未能在中断后恢复"
        return 1
    fi
    
    echo ""
}

# 场景4: 双向文件传输场景
scenario_bidirectional_transfer() {
    log_scenario "场景4: 双向文件传输场景"
    
    # 1. 确保卫星在线
    log_info "1. 确保目标卫星在线"
    control_satellite "sat-02" "online" >/dev/null
    
    wait_with_spinner 3 "等待卫星就绪"
    
    # 2. 先上传一个文件
    log_info "2. 上传文件到卫星"
    local upload_result=$(upload_file "test-data/system.log" "system.log" "sat-02" "node-1")
    local upload_id=$(echo "$upload_result" | jq -r '.id // ""' 2>/dev/null || echo "")
    
    if [ -z "$upload_id" ]; then
        log_error "❌ 上传失败"
        return 1
    fi
    
    # 3. 等待上传完成
    log_info "3. 等待上传完成"
    if ! wait_for_transfer "$upload_id" 30; then
        log_error "❌ 上传未完成"
        return 1
    fi
    
    log_success "文件上传完成"
    
    # 4. 立即下载同一个文件
    log_info "4. 下载刚上传的文件"
    local download_result=$(download_file "system.log" "sat-02" "node-1" "downloads/system-downloaded.log")
    local download_id=$(echo "$download_result" | jq -r '.id // ""' 2>/dev/null || echo "")
    
    if [ -z "$download_id" ]; then
        log_error "❌ 下载请求失败"
        return 1
    fi
    
    # 5. 等待下载完成
    log_info "5. 等待下载完成"
    if wait_for_transfer "$download_id" 30; then
        log_success "✅ 场景4成功: 双向文件传输完成"
        
        # 6. 验证文件完整性
        if [ -f "$DEMO_DIR/downloads/system-downloaded.log" ]; then
            local original_size=$(wc -c < "$DEMO_DIR/test-data/system.log" 2>/dev/null || echo "0")
            local downloaded_size=$(wc -c < "$DEMO_DIR/downloads/system-downloaded.log" 2>/dev/null || echo "0")
            
            if [ "$original_size" = "$downloaded_size" ]; then
                log_success "✅ 文件完整性验证通过 ($original_size 字节)"
            else
                log_warning "⚠️ 文件大小不匹配: 原始 $original_size, 下载 $downloaded_size"
            fi
        else
            log_warning "⚠️ 下载的文件未找到"
        fi
    else
        log_error "❌ 场景4失败: 下载未完成"
        return 1
    fi
    
    echo ""
}

# 场景5: 多节点卫星场景
scenario_multi_node() {
    log_scenario "场景5: 多节点卫星文件分发"
    
    # 1. 确保多节点卫星在线
    log_info "1. 启动多节点卫星 sat-01"
    control_satellite "sat-01" "online" >/dev/null
    
    wait_with_spinner 3 "等待卫星就绪"
    
    # 2. 向不同节点上传不同文件
    log_info "2. 向不同节点分发文件"
    
    declare -A node_transfers
    
    # 配置文件到 node-1
    local result1=$(upload_file "test-data/config.json" "config.json" "sat-01" "node-1")
    node_transfers["node-1"]=$(echo "$result1" | jq -r '.id // ""' 2>/dev/null || echo "")
    
    # 日志文件到 node-2
    local result2=$(upload_file "test-data/system.log" "system.log" "sat-01" "node-2")
    node_transfers["node-2"]=$(echo "$result2" | jq -r '.id // ""' 2>/dev/null || echo "")
    
    # 传感器数据到 node-3
    local result3=$(upload_file "test-data/sensor-data.csv" "sensor-data.csv" "sat-01" "node-3")
    node_transfers["node-3"]=$(echo "$result3" | jq -r '.id // ""' 2>/dev/null || echo "")
    
    log_success "已向 sat-01 的 3 个节点分发文件"
    
    # 3. 等待所有传输完成
    log_info "3. 等待所有节点传输完成"
    local all_success=true
    
    for node_id in "node-1" "node-2" "node-3"; do
        local transfer_id=${node_transfers[$node_id]}
        if [ -n "$transfer_id" ]; then
            log_info "等待 $node_id 传输完成..."
            if wait_for_transfer "$transfer_id" 60; then
                log_success "✅ $node_id 传输完成"
            else
                log_error "❌ $node_id 传输失败"
                all_success=false
            fi
        fi
    done
    
    if [ "$all_success" = true ]; then
        log_success "✅ 场景5成功: 多节点文件分发完成"
    else
        log_error "❌ 场景5部分失败: 某些节点传输未完成"
        return 1
    fi
    
    echo ""
}

# 压力测试场景
scenario_stress_test() {
    log_scenario "场景6: 压力测试 - 并发传输"
    
    # 1. 设置所有卫星在线
    log_info "1. 设置所有测试卫星在线"
    for sat_id in "sat-01" "sat-02" "sat-03" "sat-04"; do
        control_satellite "$sat_id" "online" >/dev/null
    done
    
    wait_with_spinner 5 "等待所有卫星就绪"
    
    # 2. 并发启动多个传输
    log_info "2. 启动并发传输 (8个并发任务)"
    
    declare -a concurrent_transfers
    
    # 创建8个并发传输任务
    for i in {1..8}; do
        local sat_num=$((((i-1) % 4) + 1))
        local sat_id=$(printf "sat-%02d" $sat_num)
        local node_id="node-1"
        
        # 交替上传不同类型的文件
        local file_type=$((i % 4))
        case $file_type in
            0) local_file="test-data/config.json"; remote_file="config-$i.json" ;;
            1) local_file="test-data/system.log"; remote_file="system-$i.log" ;;
            2) local_file="test-data/sensor-data.csv"; remote_file="sensor-data-$i.csv" ;;
            3) local_file="test-data/config.json"; remote_file="config-alt-$i.json" ;;
        esac
        
        local result=$(upload_file "$local_file" "$remote_file" "$sat_id" "$node_id")
        local transfer_id=$(echo "$result" | jq -r '.id // ""' 2>/dev/null || echo "")
        
        if [ -n "$transfer_id" ]; then
            concurrent_transfers+=("$transfer_id")
            log_info "启动传输 $i: $transfer_id -> $sat_id/$node_id"
        fi
        
        # 短暂延迟避免过载
        sleep 0.5
    done
    
    log_success "已启动 ${#concurrent_transfers[@]} 个并发传输"
    
    # 3. 等待所有传输完成
    log_info "3. 等待所有并发传输完成"
    local completed=0
    local failed=0
    
    for transfer_id in "${concurrent_transfers[@]}"; do
        if wait_for_transfer "$transfer_id" 90; then
            completed=$((completed + 1))
            echo -n "✅"
        else
            failed=$((failed + 1))
            echo -n "❌"
        fi
    done
    echo ""
    
    log_info "并发传输结果: 完成 $completed, 失败 $failed"
    
    if [ $failed -eq 0 ]; then
        log_success "✅ 场景6成功: 所有并发传输完成"
    else
        log_warning "⚠️ 场景6部分成功: $completed/$((completed + failed)) 传输完成"
    fi
    
    echo ""
}

# 运行所有测试场景
run_all_scenarios() {
    log_info "开始运行 SGCSF 文件传输离线场景测试"
    echo "=================================="
    echo ""
    
    # 显示初始状态
    show_system_status
    echo ""
    
    local start_time=$(date +%s)
    local total_scenarios=6
    local passed_scenarios=0
    
    # 运行各个场景
    if scenario_offline_upload; then
        passed_scenarios=$((passed_scenarios + 1))
    fi
    
    if scenario_batch_transfer; then
        passed_scenarios=$((passed_scenarios + 1))
    fi
    
    if scenario_network_interruption; then
        passed_scenarios=$((passed_scenarios + 1))
    fi
    
    if scenario_bidirectional_transfer; then
        passed_scenarios=$((passed_scenarios + 1))
    fi
    
    if scenario_multi_node; then
        passed_scenarios=$((passed_scenarios + 1))
    fi
    
    if scenario_stress_test; then
        passed_scenarios=$((passed_scenarios + 1))
    fi
    
    # 总结
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    echo "=================================="
    log_info "测试完成总结"
    echo "  总场景数: $total_scenarios"
    echo "  通过场景: $passed_scenarios"
    echo "  失败场景: $((total_scenarios - passed_scenarios))"
    echo "  总耗时: ${duration}秒"
    
    if [ $passed_scenarios -eq $total_scenarios ]; then
        log_success "🎉 所有测试场景通过!"
    else
        log_warning "⚠️ 有 $((total_scenarios - passed_scenarios)) 个场景未通过"
    fi
    
    # 显示最终状态
    echo ""
    log_info "最终系统状态:"
    show_system_status
}

# 运行单个场景
run_single_scenario() {
    local scenario_name=$1
    
    case $scenario_name in
        "offline_upload"|"1")
            scenario_offline_upload
            ;;
        "batch_transfer"|"2")
            scenario_batch_transfer
            ;;
        "network_interruption"|"3")
            scenario_network_interruption
            ;;
        "bidirectional"|"4")
            scenario_bidirectional_transfer
            ;;
        "multi_node"|"5")
            scenario_multi_node
            ;;
        "stress_test"|"6")
            scenario_stress_test
            ;;
        *)
            log_error "未知场景: $scenario_name"
            show_help
            exit 1
            ;;
    esac
}

# 显示帮助
show_help() {
    echo "SGCSF 文件传输离线场景测试脚本"
    echo ""
    echo "用法: $0 [选项] [场景]"
    echo ""
    echo "选项:"
    echo "  all                运行所有测试场景 (默认)"
    echo "  status             显示系统状态"
    echo "  cleanup            清理测试环境"
    echo "  help               显示此帮助"
    echo ""
    echo "单个场景:"
    echo "  offline_upload|1   离线上传场景"
    echo "  batch_transfer|2   批量传输场景"
    echo "  network_interruption|3  网络中断恢复场景"
    echo "  bidirectional|4    双向传输场景"
    echo "  multi_node|5       多节点分发场景"
    echo "  stress_test|6      压力测试场景"
    echo ""
    echo "示例:"
    echo "  $0                 # 运行所有场景"
    echo "  $0 offline_upload  # 只运行离线上传场景"
    echo "  $0 status          # 显示系统状态"
    echo ""
}

# 清理测试环境
cleanup_test_env() {
    log_info "清理测试环境..."
    
    # 强制所有卫星离线
    for i in {1..12}; do
        local sat_id=$(printf "sat-%02d" $i)
        control_satellite "$sat_id" "offline" >/dev/null 2>&1 || true
    done
    
    # 清理下载目录
    rm -rf "$DEMO_DIR/downloads"/* 2>/dev/null || true
    
    log_success "测试环境清理完成"
}

# 主函数
main() {
    local command=${1:-all}
    
    # 检查演示系统是否运行
    if ! curl -s "$BASE_URL/api/satellites/status" >/dev/null 2>&1; then
        log_error "SGCSF 演示系统未运行或不可访问"
        log_info "请先运行: ./run-demo.sh start"
        exit 1
    fi
    
    case $command in
        all)
            run_all_scenarios
            ;;
        status)
            show_system_status
            ;;
        cleanup)
            cleanup_test_env
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            run_single_scenario "$command"
            ;;
    esac
}

# 执行主函数
main "$@"