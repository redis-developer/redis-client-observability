# Redis Client Observability

A helper repository for exploring Redis client observability with OpenTelemetry. This repo provides working examples in **Go (go-redis)**, **Python (redis-py)**, and **TypeScript (node-redis)** along with a complete observability stack that you can spin up locally to easily try out Redis client metrics and tracing.

## Features

- **Python Example (redis-py)** - Complete working example in Python with OpenTelemetry instrumentation
- **Go Example (go-redis)** - Complete working example in Go with OpenTelemetry instrumentation
- **TypeScript Example (node-redis)** - Complete working example in TypeScript with OpenTelemetry instrumentation
- **One-Command Stack** - Start the entire observability infrastructure with `make start`
- **Pre-built Dashboards** - Ready-to-use Grafana dashboards for Redis client metrics


## 🚀 Quick Start

### Prerequisites

- Docker and Docker Compose installed
- Make utility available
- Redis server running (or use: `docker run -d -p 6379:6379 redis:latest`)
- Python 3.8+ (for Python example), Go 1.21+ (for Go example), or Node.js 18+ (for the TypeScript example)

### 1. Start the Observability Stack

```bash
# Start all services
make start

# Check status
make status
```

This starts:
- **OpenTelemetry Collector** on ports 4317 (gRPC) and 4318 (HTTP)
- **Prometheus** on port 9090
- **Grafana** on port 3000

### 2. Access Grafana

Open http://localhost:3000
- **Username:** admin
- **Password:** admin

Navigate to **Dashboards** → **Redis** folder to see the pre-built dashboard.

### 3. Run Example Applications

**Python Example (redis-py):**
```bash
cd examples/python
pip install -r requirements.txt
python main.py
```

**Go Example (go-redis):**
```bash
cd examples/go
go mod download
go run main.go
```

**TypeScript Example (node-redis):**
```bash
cd examples/ts
npm install
npm start
```

The Python, Go & TypeScript examples will:
- Connect to Redis on `localhost:6379`
- Send metrics to the OTLP collector
- Execute various Redis operations
- Export metrics every 10 seconds


## 📚 Supported Client Libraries

This observability stack works with Redis client libraries that have OpenTelemetry support:

- **[redis-py](https://github.com/redis/redis-py)** v7.2.1+ - Python Redis client
- **[go-redis](https://github.com/redis/go-redis)** v9.18.0+ - Go Redis client
- **[node-redis](https://github.com/redis/node-redis)** - Node.js Redis client

More client libraries coming soon!

## 📊 Grafana Dashboards

A Redis Client Observability Dashboard is included and automatically provisioned:

**File:** `grafana/dashboards/redis-client-observability.json`

Monitors Redis client library performance with SDK-level metrics:
- **Operation Rate** - Commands per second by operation type
- **Operation Duration** - Latency percentiles (p50, p95, p99)
- **Error Rate** - Failed operations over time
- **Connection Pool** - Active/idle connections
- **Retry Metrics** - Retry attempts and backoff

## 📝 License

MIT License - see [LICENSE](./LICENSE) for details.
