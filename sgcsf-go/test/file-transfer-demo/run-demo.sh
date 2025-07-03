#!/bin/bash

set -e

# SGCSF 文件传输演示启动脚本
# 用于模拟离线场景下的文件传输

DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SGCSF_ROOT="$(cd "$DEMO_DIR/../.." && pwd)"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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

# 检查依赖
check_dependencies() {
    log_info "检查依赖项..."
    
    # 检查Go环境
    if ! command -v go &> /dev/null; then
        log_error "Go 未安装或不在 PATH 中"
        exit 1
    fi
    
    # 检查Docker（如果需要）
    if ! command -v docker &> /dev/null; then
        log_warning "Docker 未安装，无法运行容器化演示"
    fi
    
    # 检查curl
    if ! command -v curl &> /dev/null; then
        log_error "curl 未安装，无法测试HTTP API"
        exit 1
    fi
    
    log_success "依赖检查完成"
}

# 创建目录结构
setup_directories() {
    log_info "创建目录结构..."
    
    cd "$DEMO_DIR"
    
    # 创建必要的目录
    mkdir -p {downloads,uploads,temp,logs,web}
    mkdir -p test-data
    mkdir -p satellite-storage/{sat-01,sat-02}/node-{1,2,3}
    mkdir -p satellite-storage/sat-{03..12}/node-1
    
    log_success "目录结构创建完成"
}

# 生成测试数据
generate_test_data() {
    log_info "生成测试数据..."
    
    cd "$DEMO_DIR/test-data"
    
    # 小配置文件 (1KB)
    cat > config.json << EOF
{
  "satellite_id": "sat-01",
  "mission": "earth_observation",
  "config": {
    "mode": "active",
    "interval": 30,
    "resolution": "high",
    "bands": ["visible", "infrared", "thermal"]
  },
  "updated_at": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
}
EOF

    # 中等日志文件 (100KB)
    {
        echo "=== Satellite System Log ==="
        echo "Generated at: $(date)"
        echo "=========================="
        echo ""
        
        for i in {1..1000}; do
            timestamp=$(date -u +"%Y-%m-%d %H:%M:%S")
            level=$((RANDOM % 4))
            case $level in
                0) level_str="INFO" ;;
                1) level_str="WARNING" ;;
                2) level_str="ERROR" ;;
                3) level_str="DEBUG" ;;
            esac
            
            component=("sensor" "communication" "power" "navigation" "payload")
            comp=${component[$((RANDOM % 5))]}
            
            message=("System initialized" "Data collected" "Transmission complete" "Battery check" "Position updated")
            msg=${message[$((RANDOM % 5))]}
            
            echo "[$timestamp] [$level_str] [$comp] $msg - Entry $i"
        done
    } > system.log

    # 传感器数据文件 (10MB)
    {
        echo "timestamp,latitude,longitude,altitude,temperature,humidity,pressure,radiation"
        
        for i in {1..200000}; do
            timestamp=$(date -u +"%Y-%m-%d %H:%M:%S" -d "$i seconds ago")
            lat=$(echo "scale=6; ($RANDOM - 16384) / 327.68" | bc -l 2>/dev/null || echo "0.0")
            lon=$(echo "scale=6; ($RANDOM - 16384) / 163.84" | bc -l 2>/dev/null || echo "0.0")
            alt=$(echo "scale=2; 400 + ($RANDOM % 100)" | bc -l 2>/dev/null || echo "400.0")
            temp=$(echo "scale=2; -50 + ($RANDOM % 100)" | bc -l 2>/dev/null || echo "20.0")
            hum=$(echo "scale=2; ($RANDOM % 100)" | bc -l 2>/dev/null || echo "50.0")
            press=$(echo "scale=2; 1000 + ($RANDOM % 100)" | bc -l 2>/dev/null || echo "1013.0")
            rad=$(echo "scale=4; ($RANDOM % 1000) / 100" | bc -l 2>/dev/null || echo "5.0")
            
            echo "$timestamp,$lat,$lon,$alt,$temp,$hum,$press,$rad"
        done
    } > sensor-data.csv

    # 固件更新文件 (50MB)
    log_info "生成固件文件 (50MB)..."
    dd if=/dev/urandom of=firmware-update.bin bs=1M count=50 2>/dev/null || {
        # 如果urandom不可用，使用其他方法
        head -c 52428800 /dev/zero > firmware-update.bin
    }
    
    log_success "测试数据生成完成"
    ls -lh
}

# 创建简单的Web界面
create_web_interface() {
    log_info "创建Web管理界面..."
    
    cd "$DEMO_DIR"
    
    cat > web/index.html << 'EOF'
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>SGCSF 文件传输演示</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; padding: 20px; }
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 30px; border-radius: 10px; margin-bottom: 30px; text-align: center; }
        .header h1 { font-size: 2.5em; margin-bottom: 10px; }
        .header p { font-size: 1.2em; opacity: 0.9; }
        .card { background: white; border-radius: 10px; padding: 25px; margin-bottom: 20px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); }
        .card h2 { color: #333; margin-bottom: 20px; border-bottom: 2px solid #667eea; padding-bottom: 10px; }
        .status-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 15px; margin-bottom: 20px; }
        .status-item { background: linear-gradient(135deg, #f093fb 0%, #f5576c 100%); color: white; padding: 20px; border-radius: 8px; text-align: center; }
        .status-item h3 { font-size: 2em; margin-bottom: 5px; }
        .status-item p { opacity: 0.9; }
        .btn { background: #667eea; color: white; border: none; padding: 12px 25px; border-radius: 5px; cursor: pointer; font-size: 16px; margin: 5px; transition: background 0.3s; }
        .btn:hover { background: #5a67d8; }
        .btn.danger { background: #e53e3e; }
        .btn.danger:hover { background: #c53030; }
        .btn.success { background: #38a169; }
        .btn.success:hover { background: #2f855a; }
        .form-group { margin-bottom: 15px; }
        .form-group label { display: block; margin-bottom: 5px; font-weight: bold; color: #333; }
        .form-group input, .form-group select { width: 100%; padding: 10px; border: 1px solid #ddd; border-radius: 5px; font-size: 16px; }
        .satellite-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 20px; }
        .satellite-card { border: 1px solid #e2e8f0; border-radius: 8px; padding: 20px; }
        .satellite-online { border-left: 4px solid #38a169; }
        .satellite-offline { border-left: 4px solid #e53e3e; }
        .node-list { margin-top: 15px; }
        .node-item { display: flex; justify-content: space-between; align-items: center; padding: 8px; background: #f8f9fa; margin: 5px 0; border-radius: 4px; }
        .online-indicator { width: 10px; height: 10px; border-radius: 50%; margin-right: 8px; }
        .online { background: #38a169; }
        .offline { background: #e53e3e; }
        .log-area { background: #1a1a1a; color: #00ff00; font-family: monospace; padding: 20px; border-radius: 5px; height: 300px; overflow-y: auto; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🛰️ SGCSF 文件传输演示系统</h1>
            <p>星地一体化可靠文件传输 - 支持离线场景模拟</p>
        </div>

        <div class="card">
            <h2>📊 系统状态</h2>
            <div class="status-grid" id="systemStatus">
                <div class="status-item">
                    <h3 id="totalSats">12</h3>
                    <p>总卫星数</p>
                </div>
                <div class="status-item">
                    <h3 id="onlineSats">0</h3>
                    <p>在线卫星</p>
                </div>
                <div class="status-item">
                    <h3 id="activeTransfers">0</h3>
                    <p>活跃传输</p>
                </div>
                <div class="status-item">
                    <h3 id="queuedTransfers">0</h3>
                    <p>队列传输</p>
                </div>
            </div>
        </div>

        <div class="card">
            <h2>🚀 文件传输操作</h2>
            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 30px;">
                <div>
                    <h3>📤 文件上传</h3>
                    <form id="uploadForm">
                        <div class="form-group">
                            <label>本地文件路径:</label>
                            <select id="uploadFile">
                                <option value="test-data/config.json">config.json (1KB)</option>
                                <option value="test-data/system.log">system.log (100KB)</option>
                                <option value="test-data/sensor-data.csv">sensor-data.csv (10MB)</option>
                                <option value="test-data/firmware-update.bin">firmware-update.bin (50MB)</option>
                            </select>
                        </div>
                        <div class="form-group">
                            <label>目标卫星:</label>
                            <select id="uploadSatellite">
                                <option value="sat-01">sat-01 (3 nodes)</option>
                                <option value="sat-02">sat-02 (3 nodes)</option>
                                <option value="sat-03">sat-03 (1 node)</option>
                                <option value="sat-04">sat-04 (1 node)</option>
                            </select>
                        </div>
                        <div class="form-group">
                            <label>目标节点:</label>
                            <select id="uploadNode">
                                <option value="node-1">node-1</option>
                                <option value="node-2">node-2</option>
                                <option value="node-3">node-3</option>
                            </select>
                        </div>
                        <button type="button" class="btn" onclick="startUpload()">开始上传</button>
                    </form>
                </div>
                <div>
                    <h3>📥 文件下载</h3>
                    <form id="downloadForm">
                        <div class="form-group">
                            <label>远程文件路径:</label>
                            <input type="text" id="downloadFile" placeholder="例如: config.json" value="config.json">
                        </div>
                        <div class="form-group">
                            <label>源卫星:</label>
                            <select id="downloadSatellite">
                                <option value="sat-01">sat-01 (3 nodes)</option>
                                <option value="sat-02">sat-02 (3 nodes)</option>
                                <option value="sat-03">sat-03 (1 node)</option>
                                <option value="sat-04">sat-04 (1 node)</option>
                            </select>
                        </div>
                        <div class="form-group">
                            <label>源节点:</label>
                            <select id="downloadNode">
                                <option value="node-1">node-1</option>
                                <option value="node-2">node-2</option>
                                <option value="node-3">node-3</option>
                            </select>
                        </div>
                        <button type="button" class="btn" onclick="startDownload()">开始下载</button>
                    </form>
                </div>
            </div>
        </div>

        <div class="card">
            <h2>🛰️ 卫星状态管理</h2>
            <div class="satellite-grid" id="satelliteGrid">
                <!-- 动态生成卫星状态卡片 -->
            </div>
        </div>

        <div class="card">
            <h2>🎮 离线场景模拟</h2>
            <p style="margin-bottom: 15px;">模拟真实的卫星轨道通信窗口，卫星将随机上线/离线</p>
            <button class="btn success" onclick="startSimulation()">开始模拟</button>
            <button class="btn danger" onclick="stopSimulation()">停止模拟</button>
            <button class="btn" onclick="updateStatus()">刷新状态</button>
        </div>

        <div class="card">
            <h2>📋 操作日志</h2>
            <div class="log-area" id="logArea">
                <div>系统就绪，等待操作...</div>
            </div>
        </div>
    </div>

    <script>
        // 添加日志
        function addLog(message) {
            const logArea = document.getElementById('logArea');
            const timestamp = new Date().toLocaleTimeString();
            const logEntry = document.createElement('div');
            logEntry.textContent = `[${timestamp}] ${message}`;
            logArea.appendChild(logEntry);
            logArea.scrollTop = logArea.scrollHeight;
        }

        // 更新系统状态
        async function updateStatus() {
            try {
                const response = await fetch('/api/satellites/status');
                const status = await response.json();
                
                document.getElementById('totalSats').textContent = status.total_satellites || 12;
                document.getElementById('onlineSats').textContent = status.online_satellites || 0;
                
                // 更新传输状态
                const transferResponse = await fetch('/api/transfers');
                const transfers = await transferResponse.json();
                
                document.getElementById('activeTransfers').textContent = transfers.active_count || 0;
                document.getElementById('queuedTransfers').textContent = transfers.queue_length || 0;
                
                addLog('状态更新完成');
            } catch (error) {
                addLog(`状态更新失败: ${error.message}`);
            }
        }

        // 开始上传
        async function startUpload() {
            const file = document.getElementById('uploadFile').value;
            const satellite = document.getElementById('uploadSatellite').value;
            const node = document.getElementById('uploadNode').value;
            
            try {
                const response = await fetch('/api/transfer/upload', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/x-www-form-urlencoded',
                    },
                    body: `local_path=${encodeURIComponent(file)}&remote_path=${encodeURIComponent(file.split('/').pop())}&satellite_id=${satellite}&node_id=${node}`
                });
                
                const result = await response.json();
                addLog(`上传请求已提交: ${result.id} -> ${satellite}/${node}`);
                
                if (result.status === 'offline_queued') {
                    addLog(`⚠️ 目标卫星离线，请求已加入离线队列`);
                }
            } catch (error) {
                addLog(`上传失败: ${error.message}`);
            }
        }

        // 开始下载
        async function startDownload() {
            const file = document.getElementById('downloadFile').value;
            const satellite = document.getElementById('downloadSatellite').value;
            const node = document.getElementById('downloadNode').value;
            
            try {
                const response = await fetch('/api/transfer/download', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/x-www-form-urlencoded',
                    },
                    body: `remote_path=${encodeURIComponent(file)}&satellite_id=${satellite}&node_id=${node}`
                });
                
                const result = await response.json();
                addLog(`下载请求已提交: ${satellite}/${node}:${file} -> ${result.id}`);
                
                if (result.status === 'offline_queued') {
                    addLog(`⚠️ 源卫星离线，请求已加入离线队列`);
                }
            } catch (error) {
                addLog(`下载失败: ${error.message}`);
            }
        }

        // 卫星控制
        async function controlSatellite(satelliteId, action) {
            try {
                const response = await fetch(`/api/satellites/${satelliteId}/${action}`, {
                    method: 'POST'
                });
                
                const result = await response.json();
                addLog(`卫星 ${satelliteId} ${action === 'online' ? '强制上线' : '强制离线'}`);
                updateStatus();
            } catch (error) {
                addLog(`卫星控制失败: ${error.message}`);
            }
        }

        // 开始模拟
        function startSimulation() {
            addLog('🎬 开始离线场景模拟');
            addLog('💡 卫星将随机在线/离线，模拟真实轨道通信窗口');
        }

        // 停止模拟
        function stopSimulation() {
            addLog('🛑 停止离线场景模拟');
        }

        // 初始化页面
        document.addEventListener('DOMContentLoaded', function() {
            addLog('🚀 SGCSF 文件传输演示系统已启动');
            updateStatus();
            
            // 定期更新状态
            setInterval(updateStatus, 5000);
            
            // 生成卫星网格
            generateSatelliteGrid();
        });

        // 生成卫星状态网格
        function generateSatelliteGrid() {
            const grid = document.getElementById('satelliteGrid');
            
            for (let i = 1; i <= 12; i++) {
                const satId = `sat-${i.toString().padStart(2, '0')}`;
                const nodeCount = (i <= 2) ? 3 : 1;
                
                const card = document.createElement('div');
                card.className = 'satellite-card satellite-offline';
                card.id = `sat-card-${satId}`;
                
                let nodesList = '';
                for (let j = 1; j <= nodeCount; j++) {
                    nodesList += `
                        <div class="node-item">
                            <span><span class="online-indicator offline"></span>node-${j}</span>
                            <span>离线</span>
                        </div>
                    `;
                }
                
                card.innerHTML = `
                    <h3>${satId}</h3>
                    <p>节点数: ${nodeCount}</p>
                    <div class="node-list">${nodesList}</div>
                    <div style="margin-top: 15px;">
                        <button class="btn success" onclick="controlSatellite('${satId}', 'online')">强制上线</button>
                        <button class="btn danger" onclick="controlSatellite('${satId}', 'offline')">强制离线</button>
                    </div>
                `;
                
                grid.appendChild(card);
            }
        }
    </script>
</body>
</html>
EOF

    log_success "Web界面创建完成"
}

# 编译Go程序
build_demo() {
    log_info "编译演示程序..."
    
    cd "$DEMO_DIR"
    
    # 检查go.mod是否存在
    if [ ! -f "go.mod" ]; then
        log_info "初始化Go模块..."
        go mod init sgcsf-file-transfer-demo
        go mod tidy
    fi
    
    # 编译主程序
    go build -o file-transfer-demo .
    
    log_success "编译完成"
}

# 启动演示
start_demo() {
    log_info "启动文件传输演示..."
    
    cd "$DEMO_DIR"
    
    # 检查端口是否被占用
    if lsof -Pi :8082 -sTCP:LISTEN -t >/dev/null 2>&1; then
        log_warning "端口 8082 已被占用，正在尝试停止..."
        pkill -f "file-transfer-demo" || true
        sleep 2
    fi
    
    # 启动演示程序
    log_info "启动演示程序..."
    ./file-transfer-demo &
    DEMO_PID=$!
    
    # 等待启动
    sleep 3
    
    # 检查是否启动成功
    if kill -0 $DEMO_PID 2>/dev/null; then
        log_success "演示程序启动成功 (PID: $DEMO_PID)"
        echo $DEMO_PID > .demo.pid
        
        log_info "Web界面地址: http://localhost:8082"
        log_info "使用 Ctrl+C 或运行 ./run-demo.sh stop 来停止演示"
        
        # 等待用户中断
        trap 'stop_demo; exit 0' INT TERM
        wait $DEMO_PID
    else
        log_error "演示程序启动失败"
        exit 1
    fi
}

# 停止演示
stop_demo() {
    log_info "停止文件传输演示..."
    
    if [ -f "$DEMO_DIR/.demo.pid" ]; then
        PID=$(cat "$DEMO_DIR/.demo.pid")
        if kill -0 $PID 2>/dev/null; then
            kill $PID
            rm -f "$DEMO_DIR/.demo.pid"
            log_success "演示程序已停止"
        else
            log_warning "演示程序进程不存在"
        fi
    else
        # 尝试杀死所有相关进程
        pkill -f "file-transfer-demo" || true
        log_success "已尝试停止所有相关进程"
    fi
}

# 清理环境
clean_demo() {
    log_info "清理演示环境..."
    
    stop_demo
    
    cd "$DEMO_DIR"
    
    # 清理生成的文件
    rm -rf downloads/* uploads/* temp/* logs/*
    rm -rf satellite-storage/*/node-*/
    rm -f file-transfer-demo
    rm -f .demo.pid
    
    log_success "环境清理完成"
}

# 运行测试场景
run_test_scenarios() {
    log_info "运行测试场景..."
    
    # 等待系统稳定
    sleep 3
    
    log_info "场景1: 向离线卫星上传小文件"
    curl -s -X POST "http://localhost:8082/api/transfer/upload" \
         -d "local_path=test-data/config.json&remote_path=config.json&satellite_id=sat-01&node_id=node-1" \
         | jq . 2>/dev/null || echo "上传请求已发送"
    
    sleep 2
    
    log_info "场景2: 强制卫星上线并观察传输开始"
    curl -s -X POST "http://localhost:8082/api/satellites/sat-01/online" >/dev/null
    
    sleep 5
    
    log_info "场景3: 从在线卫星下载文件"
    curl -s -X POST "http://localhost:8082/api/transfer/download" \
         -d "remote_path=config.json&satellite_id=sat-01&node_id=node-1" \
         | jq . 2>/dev/null || echo "下载请求已发送"
    
    sleep 3
    
    log_info "场景4: 上传大文件测试分片传输"
    curl -s -X POST "http://localhost:8082/api/transfer/upload" \
         -d "local_path=test-data/sensor-data.csv&remote_path=sensor-data.csv&satellite_id=sat-02&node_id=node-1" \
         | jq . 2>/dev/null || echo "大文件上传请求已发送"
    
    log_success "测试场景执行完成"
    log_info "查看传输状态: curl http://localhost:8082/api/transfers"
    log_info "查看卫星状态: curl http://localhost:8082/api/satellites/status"
}

# 显示帮助信息
show_help() {
    echo "SGCSF 文件传输演示启动脚本"
    echo ""
    echo "用法: $0 [命令]"
    echo ""
    echo "命令:"
    echo "  start       启动演示 (默认)"
    echo "  stop        停止演示"
    echo "  clean       清理环境"
    echo "  test        运行测试场景"
    echo "  build       仅编译程序"
    echo "  setup       仅设置环境"
    echo "  help        显示此帮助"
    echo ""
    echo "示例:"
    echo "  $0              # 启动完整演示"
    echo "  $0 start        # 启动演示"
    echo "  $0 test         # 运行测试场景"
    echo "  $0 stop         # 停止演示"
    echo "  $0 clean        # 清理环境"
    echo ""
}

# 主函数
main() {
    local command=${1:-start}
    
    case $command in
        start)
            check_dependencies
            setup_directories
            generate_test_data
            create_web_interface
            build_demo
            start_demo
            ;;
        stop)
            stop_demo
            ;;
        clean)
            clean_demo
            ;;
        test)
            run_test_scenarios
            ;;
        build)
            build_demo
            ;;
        setup)
            check_dependencies
            setup_directories
            generate_test_data
            create_web_interface
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            log_error "未知命令: $command"
            show_help
            exit 1
            ;;
    esac
}

# 执行主函数
main "$@"