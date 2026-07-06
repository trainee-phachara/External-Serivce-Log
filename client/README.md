# logclient

Go client library for sending structured logs to the external-service-log gRPC server.

## Setup

### Using go.work (local development)

Add to your workspace's `go.work`:

```
use ./path/to/libs/client
```

### Using go get (published module)

```sh
go get github.com/trainee-phachara/External-Serivce-Log/client
```

## Usage

```go
import logclient "github.com/trainee-phachara/External-Serivce-Log/client"

// Create client — call once at startup
c, err := logclient.New(logclient.Config{
    Address: "localhost:50051", // gRPC address of external-service-log
    Timeout: 3 * time.Second,  // per-call timeout (default 5s)
})
if err != nil {
    log.Fatal(err)
}
defer c.Close()

// Send a log entry — fire-and-forget, non-blocking
c.SendLog(logclient.LogEntryInput{
    Source:         logclient.LogSource{AppName: "order-service", ServiceName: "order"},
    TraceID:        "trace-abc",
    Endpoint:       "/api/v1/orders",
    HTTPStatus:     "200",
    Type:           "response",
    Direction:      "inbound",
    MetadataJSON:   `{"user_id": "u-1"}`,
    RawPayloadJSON: `{"raw": true}`,
    PayloadJSON:    `{"id": 1}`,
})
```

### Log Types

| Type | Description |
|---|---|
| `request` | Incoming request before processing |
| `response` | Outgoing response after processing |
| `event` | Domain event (e.g. order.placed) |

### Direction

| Direction | Description |
|---|---|
| `inbound` | Request coming into the service |
| `outbound` | Request going out to another service |
