#!/bin/bash

# SGCSF HTTP集成演示脚本
echo "🚀 Starting SGCSF HTTP Integration Demo"

# 创建日志目录
mkdir -p logs

# 函数：等待服务启动
wait_for_service() {
    local url=$1
    local service_name=$2
    local max_attempts=30
    local attempt=1
    
    echo "⏳ Waiting for $service_name to start..."
    while [ $attempt -le $max_attempts ]; do
        if curl -s "$url" > /dev/null 2>&1; then
            echo "✅ $service_name is ready"
            return 0
        fi
        echo "   Attempt $attempt/$max_attempts failed, retrying in 2s..."
        sleep 2
        ((attempt++))
    done
    
    echo "❌ $service_name failed to start after $max_attempts attempts"
    return 1
}

# 函数：清理进程
cleanup() {
    echo "🛑 Stopping all demo processes..."
    
    # 停止所有后台进程
    if [ -f .ground-gateway.pid ]; then
        kill $(cat .ground-gateway.pid) 2>/dev/null || true
        rm .ground-gateway.pid
    fi
    
    if [ -f .satellite-gateway.pid ]; then
        kill $(cat .satellite-gateway.pid) 2>/dev/null || true
        rm .satellite-gateway.pid
    fi
    
    if [ -f .satellite-server.pid ]; then
        kill $(cat .satellite-server.pid) 2>/dev/null || true
        rm .satellite-server.pid
    fi
    
    if [ -f .ground-client.pid ]; then
        kill $(cat .ground-client.pid) 2>/dev/null || true
        rm .ground-client.pid
    fi
    
    echo "✅ Cleanup completed"
}

# 设置信号处理
trap cleanup EXIT INT TERM

echo "📝 Demo components:"
echo "   🌍 Ground Gateway  (SGCSF server for ground side)"
echo "   🛰️  Satellite Gateway (SGCSF server for satellite side)"
echo "   📊 Satellite HTTP Server (node-exporter simulation)"
echo "   📈 Ground HTTP Client (Prometheus simulation)"

# 第1步: 启动地面网关
echo ""
echo "1️⃣ Starting Ground Gateway..."
cd ../../
go run cmd/ground-gateway/main.go > test/http-integration-demo/logs/ground-gateway.log 2>&1 &
echo $! > test/http-integration-demo/.ground-gateway.pid
cd test/http-integration-demo/

# 等待地面网关启动
wait_for_service "http://localhost:8080/health" "Ground Gateway"

# 第2步: 启动卫星网关
echo ""
echo "2️⃣ Starting Satellite Gateway..."
cd ../../
SATELLITE_ID=sat-demo \
GROUND_SERVER=localhost:7000 \
LOCAL_HTTP_PORT=:8080 \
go run cmd/satellite-gateway/main.go > test/http-integration-demo/logs/satellite-gateway.log 2>&1 &
echo $! > test/http-integration-demo/.satellite-gateway.pid
cd test/http-integration-demo/

# 等待卫星网关启动
sleep 3
wait_for_service "http://localhost:8080/api/status" "Satellite Gateway"

# 第3步: 启动卫星HTTP服务器
echo ""
echo "3️⃣ Starting Satellite HTTP Server (node-exporter simulation)..."
SATELLITE_ID=sat-demo \
HTTP_PORT=:9100 \
SGCSF_SERVER=localhost:8080 \
go run satellite-http-server.go > logs/satellite-server.log 2>&1 &
echo $! > .satellite-server.pid

# 等待卫星HTTP服务器启动
wait_for_service "http://localhost:9100/health" "Satellite HTTP Server"

# 第4步: 启动地面HTTP客户端
echo ""
echo "4️⃣ Starting Ground HTTP Client (Prometheus simulation)..."
SGCSF_SERVER=localhost:8080 \
go run ground-http-client.go > logs/ground-client.log 2>&1 &
echo $! > .ground-client.pid

# 等待地面客户端启动
wait_for_service "http://localhost:8081" "Ground HTTP Client Dashboard"

echo ""
echo "🎉 All services started successfully!"
echo ""
echo "📱 Service endpoints:"
echo "   🌍 Ground Gateway:        http://localhost:8080/api/status"
echo "   🛰️  Satellite Gateway:     http://localhost:8080/api/status"  
echo "   📊 Satellite HTTP Server: http://localhost:9100/metrics"
echo "   📈 Ground Client Dashboard: http://localhost:8081"
echo ""
echo "🔍 Demo features:"
echo "   • HTTP direct communication (ground ↔ satellite)"
echo "   • SGCSF protocol communication (more reliable)"
echo "   • Real-time metrics collection"
echo "   • Offline message caching"
echo "   • Alert propagation"
echo "   • Web dashboard monitoring"
echo ""
echo "📊 Try these commands:"
echo "   curl http://localhost:9100/metrics          # Direct HTTP metrics"
echo "   curl http://localhost:9100/metrics/prometheus # Prometheus format"
echo "   curl http://localhost:8081/api/metrics      # Collected metrics"
echo "   curl http://localhost:8081/api/satellites   # Satellite status"
echo ""
echo "📋 Log files:"
echo "   Ground Gateway:    logs/ground-gateway.log"
echo "   Satellite Gateway: logs/satellite-gateway.log"
echo "   Satellite Server:  logs/satellite-server.log"
echo "   Ground Client:     logs/ground-client.log"
echo ""
echo "⏹️  Press Ctrl+C to stop all services"

# 保持脚本运行直到用户中断
while true; do
    sleep 1
done