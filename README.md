# External Service Log

ระบบรับและจัดเก็บ log จาก microservices ผ่าน HTTP และ gRPC ลงใน MongoDB time-series collection

## Overview

```
user-service  ──┐
order-service ──┼──► gRPC / HTTP POST /ingest ──► Buffer ──► Batch Flusher ──► MongoDB
payment-service─┘
                                                                                    │
Browser ◄── GET /logs ──────────────────────────────────────────────────────────────┘
```

## Tech Stack

- **Go** — backend service
- **MongoDB** — time-series collections (api_logs, event_logs, error_logs)
- **gRPC + Protocol Buffers** — รับ log จาก services อื่น
- **HTTP** — expose ingest endpoint และ query endpoint

## Project Structure

```
cmd/server/          ← entrypoint
internal/
├── types/           ← log entry types และ models
├── buffer/          ← in-memory log buffer (thread-safe)
├── flusher/         ← batch flusher flush buffer ลง MongoDB
├── ingest/          ← validation และ classification pipeline
├── logstore/        ← LogStore interface
├── mongostore/      ← MongoDB implementation ของ LogStore
├── grpc/pb/         ← generated protobuf code
├── grpcserver/      ← gRPC server implementation
└── httpapi/         ← HTTP server (POST /ingest, GET /logs)
proto/               ← proto definition file
```

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `3000` | HTTP server port |
| `GRPC_PORT` | `50051` | gRPC server port |
| `MONGO_URI` | `mongodb://localhost:27017` | MongoDB connection URI |
| `MONGO_DB_NAME` | `service_logs` | MongoDB database name |
| `FLUSH_MAX_SIZE` | `100` | จำนวน log สูงสุดก่อน flush |
| `FLUSH_INTERVAL_MS` | `5000` | ระยะเวลา flush อัตโนมัติ (ms) |

## API Endpoints

### POST /ingest

รับ log entry และเก็บลง MongoDB

```bash
curl -X POST http://localhost:3000/ingest \
  -H "Content-Type: application/json" \
  -d '{
    "source": { "app_name": "user-service", "service_name": "user" },
    "trace_id": "abc-123",
    "endpoint": "/users",
    "http_status": "201",
    "type": "response",
    "direction": "inbound"
  }'
```

### GET /logs

ดึง log entries จาก MongoDB

```
GET /logs?collection=api_logs&app=user-service&limit=50
```

| Query Param | Default | Description |
|---|---|---|
| `collection` | `api_logs` | `api_logs` / `event_logs` / `error_logs` |
| `app` | (ทั้งหมด) | filter by app_name |
| `limit` | `50` | จำนวน entries สูงสุด |

## How to Run

```bash
# ต้องมี MongoDB รันอยู่ก่อน

go run ./cmd/server
```

หรือตั้ง environment variable เอง

```bash
PORT=3000 GRPC_PORT=50051 MONGO_URI=mongodb://localhost:27017 go run ./cmd/server
```

## Log Collections

| Collection | เก็บอะไร |
|---|---|
| `api_logs` | HTTP request/response ทั่วไป |
| `event_logs` | business events (order placed, payment completed) |
| `error_logs` | HTTP 4xx/5xx errors |

ทุก collection เป็น time-series และมี TTL 30 วัน — MongoDB จะลบ documents เก่าโดยอัตโนมัติ
