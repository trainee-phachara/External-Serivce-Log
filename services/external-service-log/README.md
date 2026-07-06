# external-service-log

Centralized log collection service. Accepts structured log entries from other services via HTTP or gRPC, buffers them in memory, and flushes to MongoDB.

## Overview

```
order-service ──┐
user-service  ──┼──► gRPC / HTTP POST /ingest ──► Buffer ──► Flusher ──► MongoDB (service_logs)
payment-service─┘                                                               │
                                                                                │
                        GET /logs ◄─────────────────────────────────────────────┘
```

## Project Structure

```
cmd/server/          ← entrypoint
internal/
├── types/           ← log entry types and models
├── buffer/          ← in-memory log buffer (thread-safe)
├── flusher/         ← batch flusher — flushes buffer to MongoDB
├── ingest/          ← validation pipeline
├── logstore/        ← LogStore interface
├── mongostore/      ← MongoDB implementation of LogStore
├── grpc/pb/         ← generated protobuf code
├── grpcserver/      ← gRPC server implementation
└── httpapi/         ← HTTP server (POST /ingest, GET /logs)
proto/               ← proto definition file
```

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|---|---|---|
| `PORT` | `3000` | HTTP server port |
| `GRPC_PORT` | `50051` | gRPC server port |
| `MONGO_URI` | `mongodb://localhost:27017` | MongoDB connection URI |
| `MONGO_DB_NAME` | `service_logs` | MongoDB database name |
| `FLUSH_MAX_SIZE` | `100` | Flush buffer when this many entries accumulate |
| `FLUSH_INTERVAL_MS` | `5000` | Flush buffer every N milliseconds |

## Run

### Docker Compose (recommended)

From the repo root:

```sh
docker compose up --build
```

### Local

Requires a running MongoDB instance.

```sh
go run ./cmd/server
```

With custom config:

```sh
PORT=3000 GRPC_PORT=50051 MONGO_URI=mongodb://localhost:27017 go run ./cmd/server
```

### Docker

```sh
docker build -t external-service-log .
docker run -p 3000:3000 -p 50051:50051 \
  -e MONGO_URI=mongodb://host.docker.internal:27017 \
  external-service-log
```

## API

### gRPC — IngestService

Proto: `proto/ingest.proto`

Used by services that import `libs/client` to send logs over gRPC.

### POST /ingest

Send a single log entry over HTTP.

| Field | Required | Values |
|---|---|---|
| `source.app_name` | ✓ | name of the sending service |
| `type` | ✓ | `request`, `response`, `event` |
| `direction` | ✓ | `inbound`, `outbound` |
| `http_status` | ✓ | HTTP status code as string |

```sh
curl -X POST localhost:3000/ingest \
  -H "Content-Type: application/json" \
  -d '{
    "source": {"app_name": "order-service", "service_name": "order"},
    "trace_id": "trace-abc",
    "endpoint": "/api/v1/orders",
    "http_status": "200",
    "type": "response",
    "direction": "inbound",
    "metadata": {},
    "raw_payload": {},
    "payload": {"id": 1}
  }'
```

### GET /logs

Query logs from MongoDB. All logs are stored in a single `service_logs` collection and filtered by `type`.

| Parameter | Default | Description |
|---|---|---|
| `type` | (all) | `request`, `response`, or `event` |
| `app` | (all) | filter by `source.app_name` |
| `limit` | `50` | max number of results |

```sh
curl "localhost:3000/logs?type=response&app=order-service&limit=10"
```

## Storage

All logs are stored in a single MongoDB time-series collection `service_logs` with a 30-day TTL. MongoDB automatically expires old documents.
