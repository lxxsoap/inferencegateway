# 推理网关（Inference Gateway）

一个面向大模型推理场景的轻量级网关，基于 Go 实现。

它提供：
- 多后端推理请求代理
- 健康感知的后端管理
- 负载感知调度
- 前缀缓存感知路由（提升 KV Cache 复用）

## 功能特性

- HTTP 接口：
  - `POST /infer`
  - `GET /health`
- 多种路由策略：
  - `prefix`：前缀亲和路由
  - `load`：按负载比的最小连接优先（WLC）
  - `hybrid`：前缀亲和 + 过载回退
- 后端容量控制：
  - 按后端 `max_concurrency` 控制并发
- 主动健康检查：
  - 周期探测后端 `GET /health`
  - 自动剔除不健康后端并在恢复后重新加入
- 前缀缓存：
  - LRU 淘汰
  - 可配置缓存大小和最小前缀长度

## 环境要求

- Go 1.26.1+

## 快速开始

### 1. 安装依赖

```bash
go mod tidy
```

### 2. 启动 Mock 后端

终端 A：

```bash
go run ./cmd/mockbackend -addr :8081 -id backend-1
```

终端 B：

```bash
go run ./cmd/mockbackend -addr :8082 -id backend-2
```

### 3. 启动网关

```bash
go run ./cmd/gateway -config configs/gateway.yaml
```

默认监听地址为 `:8080`。

### 4. 发送推理请求

```bash
curl -sS -X POST http://localhost:8080/infer \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "demo-model",
    "prompt": "请介绍一下北京",
    "session_id": "s1"
  }'
```

示例响应：

```json
{
  "request_id": "backend-1-1760000000000000000",
  "backend_id": "backend-1",
  "result": "[backend-1] response to: 请介绍一下北京",
  "latency_ms": 231,
  "cache_hit": false
}
```

### 5. 查看网关健康状态

```bash
curl -sS http://localhost:8080/health
```

示例响应：

```json
{"status":"ok","healthy_backends":2}
```

## API

### POST /infer

请求体：

```json
{
  "model": "model_name",
  "prompt": "user input",
  "session_id": "optional_session_id"
}
```

行为说明：
- `prompt` 必填
- 网关会将请求转发到选中的后端 `/infer`
- 响应中的 `cache_hit` 表示是否命中前缀缓存

错误码说明：
- `400`：请求体非法或缺少 `prompt`
- `503`：无可用健康后端，或选中后端已满载
- `502`：后端请求失败，或后端响应不可解析

### GET /health

返回网关进程健康状态以及健康后端数量。

## 配置说明

默认配置文件：`configs/gateway.yaml`

```yaml
listen_addr: ":8080"

backends:
  - id: "backend-1"
    address: "http://localhost:8081"
    max_concurrency: 10
  - id: "backend-2"
    address: "http://localhost:8082"
    max_concurrency: 10

router:
  strategy: "hybrid"                # prefix | load | hybrid
  prefix_min_length: 4
  load_threshold_percent: 0.8        # hybrid 策略使用
  prefix_cache_max_size: 10000

health_check:
  interval_seconds: 5
  timeout_seconds: 2
```

## 目录结构

```text
cmd/
  gateway/       # 网关入口
  mockbackend/   # Mock 后端服务
internal/
  backend/       # 后端模型、管理器、健康检查
  config/        # YAML 配置加载与校验
  router/        # 路由策略与前缀缓存
  server/        # HTTP 服务与处理器
configs/
  gateway.yaml   # 默认运行配置
docs/
  architecture.md
  推理网关.md
```

## 说明

- 当前代码实现仅包含 `/infer` 与 `/health`。
- 文档中提到的 `/metrics` 属于可观测性进阶方向，当前版本尚未实现。
