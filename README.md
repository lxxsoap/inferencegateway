# Inference Gateway

[English](README.md) | [简体中文](README.zh-CN.md)

A lightweight inference gateway for large-model serving scenarios, implemented in Go.

It provides:
- Multi-backend proxying for inference requests
- Health-aware backend management
- Load-aware scheduling
- Prefix-cache-aware routing to improve KV cache reuse

## Features

- HTTP endpoints:
  - `POST /infer`
  - `GET /health`
- Multiple routing strategies:
  - `prefix`: prefix affinity routing
  - `load`: weighted least-connections (by load ratio)
  - `hybrid`: prefix affinity + overload fallback
- Backend capacity control:
  - Per-backend `max_concurrency`
- Active health checks:
  - Periodic `GET /health` probing
  - Automatic remove/recover of backends
- Prefix cache:
  - LRU eviction
  - Configurable cache size and minimum prefix length

## Requirements

- Go 1.26.1+

## Quick Start

### 1. Install dependencies

```bash
go mod tidy
```

### 2. Start mock backends

Terminal A:

```bash
go run ./cmd/mockbackend -addr :8081 -id backend-1
```

Terminal B:

```bash
go run ./cmd/mockbackend -addr :8082 -id backend-2
```

### 3. Start the gateway

```bash
go run ./cmd/gateway -config configs/gateway.yaml
```

The gateway listens on `:8080` by default.

### 4. Send an inference request

```bash
curl -sS -X POST http://localhost:8080/infer \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "demo-model",
    "prompt": "Please introduce Beijing",
    "session_id": "s1"
  }'
```

Example response:

```json
{
  "request_id": "backend-1-1760000000000000000",
  "backend_id": "backend-1",
  "result": "[backend-1] response to: Please introduce Beijing",
  "latency_ms": 231,
  "cache_hit": false
}
```

### 5. Check gateway health

```bash
curl -sS http://localhost:8080/health
```

Example response:

```json
{"status":"ok","healthy_backends":2}
```

## API

### POST /infer

Request body:

```json
{
  "model": "model_name",
  "prompt": "user input",
  "session_id": "optional_session_id"
}
```

Behavior notes:
- `prompt` is required
- Gateway forwards request to selected backend `/infer`
- `cache_hit` in response indicates whether prefix cache was used

Error responses:
- `400`: invalid body or missing prompt
- `503`: no healthy backends / selected backend at capacity
- `502`: backend request failed or invalid backend response

### GET /health

Returns gateway process health and healthy backend count.

## Configuration

Default config file: `configs/gateway.yaml`

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
  load_threshold_percent: 0.8        # used by hybrid strategy
  prefix_cache_max_size: 10000

health_check:
  interval_seconds: 5
  timeout_seconds: 2
```

## Project Structure

```text
cmd/
  gateway/       # gateway entrypoint
  mockbackend/   # mock backend service
internal/
  backend/       # backend model, manager, health checker
  config/        # yaml config loader and validation
  router/        # routing strategies and prefix cache
  server/        # http server and request handlers
configs/
  gateway.yaml   # default runtime config
docs/
  architecture.md
  推理网关.md
```

## Notes

- This repository currently exposes `/infer` and `/health` only.
- A `/metrics` endpoint is described in docs as an optional/advanced direction, but not implemented in current code.
