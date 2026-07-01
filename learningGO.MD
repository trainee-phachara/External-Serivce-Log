# Learning Log: external-service-log (Go)

บันทึกนี้อธิบาย `external-service-log` ตั้งแต่ต้น สำหรับคนที่ยังไม่คุ้นกับ Go หรือ backend concept เลย
อ่านตามลำดับได้เลย แต่ละหัวข้อปูพื้นก่อนแล้วค่อยลงรายละเอียด

---

## 1. external-service-log ทำอะไร

`external-service-log` คือโปรแกรมที่รับ "บันทึก" (log) จาก service อื่นๆ ในระบบ แล้วเก็บลง MongoDB

service อื่นๆ เช่น `user-service` จะส่ง log มาบอกว่า "มี request เข้ามา, endpoint ไหน, status code อะไร, body อะไร" — `external-service-log` รับมาแล้วจัดเรียงและบันทึกลง database

รับ log ได้ 2 ช่องทาง:
- **HTTP** `POST /ingest` — ส่ง JSON body มา
- **gRPC** `Ingest(...)` — ส่ง protobuf message มา

```
user-service ──── HTTP POST /ingest ────►┐
                                         │  external-service-log ──► MongoDB
user-service ──── gRPC Ingest() ─────────┘
```

ทำไมต้องมีสองช่องทาง? เพราะ service บางตัวอาจส่งผ่าน HTTP สะดวกกว่า บางตัวส่งผ่าน gRPC ได้เร็วกว่า — `external-service-log` รองรับทั้งคู่แล้วแปลงเป็นรูปแบบเดียวกันก่อนบันทึก

---

## 2. โครงสร้างโปรเจกต์

```
external-service-log/
├── go.mod, go.sum              ← รายชื่อ library + checksum
├── proto/ingest.proto          ← "สัญญา" รูปแบบข้อมูลของช่องทาง gRPC
├── cmd/server/main.go          ← จุดเริ่มโปรแกรม — ต่อทุกอย่างเข้าด้วยกัน
└── internal/
    ├── types/log.go            ← นิยาม struct และ constant ทั้งหมด
    ├── ingest/
    │   ├── validate.go         ← ตรวจว่า body ที่รับมาถูกต้องไหม
    │   └── process.go          ← แกนกลาง: validate → สร้าง entry → เข้า buffer
    ├── buffer/logbuffer.go     ← คิวในหน่วยความจำ รอ flush ลง MongoDB
    ├── flusher/batchflusher.go ← flush buffer ลง MongoDB เป็นช่วงๆ หรือเมื่อเต็ม
    ├── mongostore/mongostore.go← คุยกับ MongoDB
    ├── httpapi/server.go       ← HTTP handler สำหรับ POST /ingest
    ├── grpcserver/server.go    ← gRPC handler สำหรับ Ingest()
    └── grpc/pb/                ← โค้ดที่ protoc generate จาก ingest.proto
```

**ทำไม `internal/` ถึงพิเศษ?**
Go compiler บังคับว่าโค้ดใน `internal/` ไม่สามารถถูก import โดยโปรเจกต์ภายนอกได้เลย — เป็นการปิดกั้นระดับ compiler ไม่ใช่แค่ตกลงกันเอง

---

## 3. `go.mod` — รายชื่อ library

[go.mod](external-service-log/go.mod) บอกว่าโปรเจกต์นี้ใช้ library อะไรบ้าง:

| Library | ทำหน้าที่อะไร |
|---|---|
| `go.mongodb.org/mongo-driver/v2` | คุยกับ MongoDB — บันทึก/ดึงข้อมูล |
| `google.golang.org/grpc` | รับ log ผ่านช่องทาง gRPC |
| `google.golang.org/protobuf` | แปลงข้อมูล protobuf format |
| `testcontainers-go/.../mongodb` | ใช้แค่ในเทส — รัน MongoDB จริงใน Docker ชั่วคราว |

---

## 4. `internal/types/log.go` — นิยาม "หน้าตา" ของข้อมูลทั้งหมด

[internal/types/log.go](external-service-log/internal/types/log.go)

ไฟล์นี้ไม่มี logic เลย — เป็นแค่ "พจนานุกรม" ที่ทุกไฟล์ในโปรเจกต์ใช้ร่วมกัน บอกว่า "ข้อมูลแต่ละชนิดหน้าตาเป็นยังไง"

### Typed Constant — enum แบบ Go

```go
type LogType string

const (
    LogTypeRequest  LogType = "request"
    LogTypeResponse LogType = "response"
)
```

Go ไม่มี `enum` แบบภาษาอื่น วิธีที่นิยมคือประกาศ `type` ใหม่จาก `string` แล้วตามด้วย `const` กลุ่ม

ผลลัพธ์คือ compiler จะ error ถ้าส่ง string ธรรมดาเข้าไปในฟังก์ชันที่รับ `LogType` — ต้องส่ง `LogTypeRequest` หรือ `LogTypeResponse` เท่านั้น ป้องกัน typo โดยไม่ต้องเช็คเอง

เช่นเดียวกันกับ `LogDirection` (`inbound`/`outbound`)

### Struct Tags สองชุด

```go
type LogEntry struct {
    Timestamp  time.Time `bson:"timestamp" json:"timestamp"`
    Source     LogSource `bson:"source"    json:"source"`
    TraceID    string    `bson:"trace_id"  json:"trace_id"`
    ...
}
```

**struct tag** คือคำอธิบายพิเศษที่ต่อท้าย field — บอก library ต่างๆ ว่า field นี้ควรใช้ชื่ออะไร:

- `bson:"timestamp"` → MongoDB driver เวลาบันทึก/อ่านให้ใช้ชื่อ `timestamp` ใน document
- `json:"timestamp"` → `encoding/json` เวลาแปลงเป็น JSON ให้ใช้ชื่อ `timestamp`

มีสองชุดเพราะ MongoDB และ JSON ใช้ library คนละตัว ต้องบอกทั้งคู่แยกกัน

### `map[string]interface{}` คืออะไร?

```go
Metadata   map[string]interface{} `bson:"metadata" json:"metadata"`
```

`map[string]interface{}` แปลว่า "JSON object ที่ key เป็น string แต่ value เป็นอะไรก็ได้" — เหมือน `Record<string, unknown>` — ใช้เมื่อโครงสร้างข้างในไม่แน่นอน เช่น metadata ของแต่ละ log อาจต่างกันไปแล้วแต่ service ที่ส่งมา

### struct สี่รูปในไฟล์นี้

| struct | ใช้ตอนไหน |
|---|---|
| `LogSource` | ระบุว่า log นี้มาจากแอปไหน/service ไหน |
| `LogEntry` | document ที่เก็บจริงใน MongoDB (ครบทุก field รวม timestamp) |
| `IngestRequestBody` | ข้อมูลที่รับจาก client ก่อน validate (ยังไม่มี timestamp) |
| `BufferedLog` | ห่อ `LogEntry` ไว้ใน buffer รอ flush |

---

## 5. `proto/ingest.proto` และโค้ดที่ generate

[proto/ingest.proto](external-service-log/proto/ingest.proto)

### proto file คืออะไร?

ไฟล์ `.proto` คือ "สัญญา" ที่กำหนดว่า "ถ้าจะส่ง log ผ่าน gRPC ต้องส่งข้อมูลรูปแบบนี้":

```proto
message IngestRequest {
    LogSource source = 1;       // มาจากแอปไหน/service ไหน
    string trace_id = 2;        // ID ของ request นั้น
    string endpoint = 3;        // URL path เช่น /users/5
    string http_status = 4;     // เช่น "200", "404"
    string type = 5;            // "request" หรือ "response"
    string direction = 6;       // "inbound" หรือ "outbound"
    string metadata_json = 7;   // JSON object แบบ encode เป็น string
    string raw_payload_json = 8;
    string payload_json = 9;
}
```

ทำไม `metadata` ถูกส่งเป็น JSON string (`metadata_json`) ไม่ใช่ object ตรงๆ?

เพราะ protobuf เป็น strongly-typed — ทุก field ต้องรู้ชนิดล่วงหน้า แต่ metadata อาจมีหน้าตาต่างกันแล้วแต่ service ที่ส่งมา แทนที่จะใช้ `google.protobuf.Struct` (ซึ่งซับซ้อน) เราเลือก serialize เป็น JSON string ก่อนส่ง ฝั่งรับก็แค่ `json.Unmarshal` คืนมา

### โค้ดที่ generate ออกมา

`protoc` อ่าน `ingest.proto` แล้วสร้างไฟล์ 2 ไฟล์ใน [internal/grpc/pb/](external-service-log/internal/grpc/pb/):

**`ingest.pb.go`** — Go struct ของทุก message (`IngestRequest`, `IngestResponse`, `LogSource`) พร้อม getter methods และ logic แปลงข้อมูลเป็น protobuf binary

**`ingest_grpc.pb.go`** — interface `IngestServiceServer`/`IngestServiceClient` และ `UnimplementedIngestServiceServer` (struct เปล่าๆ ที่ implement ทุก method ด้วย "ยังไม่รองรับ" — ดูอธิบายเพิ่มใน section gRPC server)

ไฟล์เหล่านี้ generate อัตโนมัติ ไม่ต้องแก้เอง:
```bash
protoc --proto_path=proto \
  --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  proto/ingest.proto
```

---

## 6. `internal/ingest/` — แกนกลางของทั้งระบบ

โฟลเดอร์นี้มี 2 ไฟล์ที่ทำงานต่อกัน ทั้ง HTTP และ gRPC ผ่านมาที่นี่เหมือนกัน:

```
HTTP body ──► validate.go ──► process.go ──► buffer
gRPC req  ──►(แปลงก่อน)──► (ชุดเดิม)  ──► (ชุดเดิม)
```

### 6.1 `validate.go` — ด่านตรวจ

[internal/ingest/validate.go](external-service-log/internal/ingest/validate.go)

รับ `interface{}` (อะไรก็ได้) เพราะยังไม่รู้ว่า client ส่งอะไรมา — ตรวจทุก field ที่บังคับ แล้วรวบรวม error ทั้งหมดก่อนคืน ไม่หยุดที่ error แรก

```go
func ValidateIngestBody(body interface{}) ValidationResult {
    candidate, ok := isPlainObject(body)
    if !ok {
        return ValidationResult{Valid: false, Errors: []string{"Request body must be a JSON object"}}
    }
    ...
}
```

**`isPlainObject`** เช็คว่า value เป็น `map[string]interface{}` ไหม — ถ้า client ส่ง array หรือ string มาแทน object จะจับได้ตรงนี้

**Type assertion** คือกลไกที่ Go ใช้เช็คชนิดของ `interface{}`:
```go
s, ok := value.(string)   // ลองดูว่า value เป็น string ไหม
// ok=true  → s มีค่า, ใช้ได้
// ok=false → value ไม่ใช่ string
```

**เช็ค `type` และ `direction` ด้วย map:**
```go
var validLogTypes = map[string]bool{"request": true, "response": true}

if logType, ok := candidate["type"].(string); !ok || !validLogTypes[logType] {
    errors = append(errors, "type must be one of: request, response")
}
```

แทนที่จะ `if type == "request" || type == "response"` ใช้ map แทน — เพิ่ม/ลบค่าที่ valid ได้โดยแก้แค่บรรทัดเดียว

### 6.2 `process.go` — ต่อทุกอย่างเข้าด้วยกัน

[internal/ingest/process.go](external-service-log/internal/ingest/process.go)

```go
func ProcessIngest(rawBody interface{}, buf *buffer.LogBuffer, fl *flusher.BatchFlusher) ProcessResult {
    // 1. validate
    validation := ValidateIngestBody(rawBody)
    if !validation.Valid {
        return ProcessResult{Accepted: false, Errors: validation.Errors}
    }

    // 2. แปลงเป็น typed struct
    body := toIngestRequestBody(rawBody.(map[string]interface{}))

    // 3. สร้าง LogEntry เต็ม (เพิ่ม timestamp + default ให้ field ว่าง)
    entry := types.LogEntry{
        Timestamp: time.Now(),
        ...
        Metadata:   defaultObject(body.Metadata),   // nil → {}
        RawPayload: defaultObject(body.RawPayload),
        Payload:    defaultObject(body.Payload),
    }

    // 4. push เข้า buffer
    buf.Push(types.BufferedLog{Entry: entry})

    // 5. แจ้ง flusher ว่ามี log ใหม่เข้ามา
    fl.OnLogPushed()

    return ProcessResult{Accepted: true, Errors: []string{}}
}
```

**ทำไม `toIngestRequestBody` ใช้ type assertion แบบไม่เช็ค `ok`?**

```go
AppName: source["app_name"].(string),   // ไม่มี , ok
```

ปกติแบบนี้จะ panic ถ้า value ไม่ใช่ string — แต่ปลอดภัยที่นี่เพราะ `ValidateIngestBody` การันตีไว้แล้วว่าทุก field ผ่านการเช็คก่อนเรียกฟังก์ชันนี้ เลยไม่ต้องเช็คซ้ำอีกรอบ

---

## 7. `internal/buffer/logbuffer.go` — คิวในหน่วยความจำ

[internal/buffer/logbuffer.go](external-service-log/internal/buffer/logbuffer.go)

### ทำไมต้องมี buffer?

การบันทึกลง MongoDB ทุกครั้งที่ได้รับ log (ทีละ document) ช้าและสิ้นเปลือง — เปรียบเหมือนเดินไปไปรษณีย์ทุกครั้งที่มีจดหมายหนึ่งฉบับ

buffer คือ "ตะกร้าสะสมจดหมาย" — รับ log เข้ามาเก็บไว้ก่อน พอสะสมได้มากพอหรือถึงเวลา ค่อยส่งไปรษณีย์ทีเดียว (batch insert) → ประหยัด network round-trip มาก

### โครงสร้าง

```go
type LogBuffer struct {
    mu    sync.Mutex
    items []types.BufferedLog
}
```

- `items` คือ slice (list แบบ Go) ที่เก็บ log รอส่ง
- `mu` คือ mutex — ดูด้านล่าง

### ทำไมต้องมี Mutex?

Go รัน goroutine พร้อมกันได้จริง — HTTP handler หลาย request เข้ามาพร้อมกัน แต่ละตัวเรียก `Push` พร้อมกัน ขณะที่ flusher อีก goroutine กำลัง `Drain`

ถ้าสอง goroutine แตะ `items` พร้อมกันโดยไม่มีการป้องกัน → **data race** → ข้อมูลเสียหายหรือ crash

`sync.Mutex` คือ "กุญแจห้อง" — goroutine ที่อยากเข้าต้องล็อกก่อน goroutine อื่นที่มาทีหลังต้องรอจนกว่าจะ unlock:

```go
func (b *LogBuffer) Push(log types.BufferedLog) {
    b.mu.Lock()         // ล็อกก่อน (ถ้ามีคนล็อกอยู่ → รอ)
    defer b.mu.Unlock() // unlock อัตโนมัติเมื่อฟังก์ชัน return ไม่ว่าเหตุผลใด
    b.items = append(b.items, log)
}
```

**`defer`** คือคำสั่งให้รันบรรทัดนั้นตอนฟังก์ชัน return — ทำให้มั่นใจว่า unlock จะเกิดขึ้นเสมอ แม้เกิด panic กลางทาง

### Drain — atomic swap

```go
func (b *LogBuffer) Drain() []types.BufferedLog {
    b.mu.Lock()
    defer b.mu.Unlock()
    drained := b.items   // เก็บ slice เดิมไว้
    b.items = nil        // ตั้ง buffer เป็นว่างทันที
    return drained
}
```

การสลับ `b.items = nil` เกิดภายใต้ lock เดียวกัน ทำให้ "อะตอมิก" — ไม่มีช่วงที่ log ใหม่หลุดเข้าไปใน slice ที่กำลัง drain อยู่

---

## 8. `internal/flusher/batchflusher.go` — flush แบบชาญฉลาด

[internal/flusher/batchflusher.go](external-service-log/internal/flusher/batchflusher.go)

flusher ดึง log ออกจาก buffer แล้วส่งให้ MongoDB — trigger ได้สองทาง:

1. **ตามเวลา** — ทุกๆ N วินาที (เช่น ทุก 5 วินาที)
2. **ตามปริมาณ** — ทันทีที่ buffer เต็ม N ชิ้น (เช่น 100 logs)

### โครงสร้าง

```go
type BatchFlusher struct {
    buffer  *buffer.LogBuffer
    insert  InsertFunc      // ฟังก์ชันที่จะเรียกตอน flush (คือ mongostore.InsertLogs)
    opts    Options

    flushMu sync.Mutex      // กัน flush สองตัวรันพร้อมกัน
    startMu sync.Mutex      // กัน Start/Stop ชนกัน
    stopCh  chan struct{}    // channel สำหรับส่งสัญญาณหยุด
    doneCh  chan struct{}    // channel ยืนยันว่า goroutine ออกแล้ว
}
```

**`InsertFunc`** คือชนิดของ "ฟังก์ชัน" ที่รับ `[]types.BufferedLog` แล้วบันทึกลง DB — ทำให้ flusher ไม่ผูกติดกับ MongoDB โดยตรง ในเทสจะส่งฟังก์ชันจำลองเข้ามาแทนได้

### Start — เริ่ม goroutine ticker

```go
func (f *BatchFlusher) Start() {
    ticker := time.NewTicker(f.opts.Interval)
    f.stopCh = make(chan struct{})
    f.doneCh = make(chan struct{})
    f.runLoop(ticker.C, ticker.Stop, f.stopCh, f.doneCh)
}
```

`time.NewTicker(interval)` สร้าง ticker ที่ส่ง "tick" เข้า channel `ticker.C` ทุกๆ `interval` — เหมือนนาฬิกาปลุกที่ดัง

`runLoop` รัน goroutine แยก ที่รอรับ tick:

```go
go func() {
    defer close(done)
    defer stopTicker()
    for {
        select {
        case <-tick:      // ได้ tick → flush
            _ = f.Flush(context.Background())
        case <-stop:      // ได้สัญญาณหยุด → ออกจาก loop
            return
        }
    }
}()
```

**Channel คืออะไร?**

channel คือ "ท่อ" ส่งข้อมูลระหว่าง goroutine — `<-tick` แปลว่า "รอรับค่าจาก channel `tick`" goroutine จะหยุดรอตรงนี้จนกว่าจะมีค่าส่งมา

**`select` คืออะไร?**

เหมือน `if-else` แต่สำหรับ channel — รอว่า channel ไหนพร้อมก่อน แล้วทำ case นั้น ถ้าทั้งคู่พร้อมพร้อมกันจะสุ่มเลือก

```
รอ:  tick มาก่อน? → flush
     stop มาก่อน? → หยุด
```

### Stop — ส่งสัญญาณและรอ

```go
func (f *BatchFlusher) Stop() {
    close(f.stopCh)   // ส่งสัญญาณ "หยุด" เข้า channel
    <-f.doneCh        // รอจนกว่า goroutine จะ close(done) ยืนยันว่าออกแล้ว
    ...
}
```

`close(channel)` ปิด channel — goroutine ที่ `select` รออยู่จะได้รับ "สัญญาณ" ทันที ไม่ต้องส่งค่าใดๆ

**ทำไมต้องรอ `<-doneCh` แทนที่จะ `close` แล้วไปต่อเลย?**

เพราะถ้าไม่รอ goroutine อาจยังรัน flush ค้างอยู่ขณะที่ program กำลังจะ shutdown — รอให้ goroutine ออกก่อนถึงจะมั่นใจได้ว่า flush รอบสุดท้ายเสร็จแล้ว

### OnLogPushed — flush ทันทีเมื่อเต็ม

```go
func (f *BatchFlusher) OnLogPushed() {
    if f.buffer.Size() >= f.opts.MaxSize {
        go func() {
            _ = f.Flush(context.Background())
        }()
    }
}
```

`ProcessIngest` เรียก `OnLogPushed()` ทุกครั้งหลัง push — ถ้า buffer เต็มแล้วจะ fire goroutine ใหม่ทำ flush ทันที โดยไม่บล็อก caller

### Flush — กัน flush ซ้อนกัน

```go
func (f *BatchFlusher) Flush(ctx context.Context) error {
    f.flushMu.Lock()
    defer f.flushMu.Unlock()
    return f.doFlush(ctx)
}
```

`flushMu` กัน `doFlush` สองตัวรันพร้อมกัน — ถ้า goroutine A กำลัง flush อยู่ และ goroutine B พยายาม flush ด้วย B จะรอ A เสร็จก่อน พอได้ lock มา B จะเจอ buffer ว่างอยู่แล้ว (A drain ไปหมดแล้ว) แล้ว return ทันที

---

## 9. `internal/mongostore/mongostore.go` — เก็บลง MongoDB

[internal/mongostore/mongostore.go](external-service-log/internal/mongostore/mongostore.go)

### Time-Series Collection คืออะไร?

MongoDB มี collection ประเภทพิเศษที่ออกแบบมาเก็บข้อมูลที่มี "เวลา" เป็นแกนหลัก — เช่น log ที่ทุก entry มี timestamp

ข้อดีของ time-series collection:
- **เก็บได้กระชับกว่า** (MongoDB compress ข้อมูลที่มี pattern เวลาได้ดีกว่า)
- **ลบอัตโนมัติ** เมื่อหมดอายุ (ไม่ต้องเขียน cron job ลบเอง)
- **query ตามช่วงเวลาได้เร็ว** (index เวลาอยู่แล้ว)

### Connect — เชื่อมต่อและเตรียม collection

```go
func Connect(ctx context.Context, uri, dbName string) (*Store, error) {
    client, err := mongo.Connect(options.Client().ApplyURI(uri))
    ...
    if err := client.Ping(ctx, nil); err != nil { ... }  // ทดสอบว่าต่อติดจริง
    db := client.Database(dbName)
    ensureTimeSeriesCollections(ctx, db)                  // สร้าง collection ถ้ายังไม่มี
    return &Store{client: client, db: db}, nil
}
```

`Ping` สำคัญ — `mongo.Connect` แค่สร้าง client object แต่ยังไม่ได้ต่อจริง การ Ping บังคับให้ต่อจริงทันที ถ้า MongoDB ไม่พร้อมจะรู้ตั้งแต่ตอน start ไม่ต้องรอให้ request แรกมาแล้วค่อย error

### ensureTimeSeriesCollections

```go
func ensureTimeSeriesCollections(ctx context.Context, db *mongo.Database) error {
    existing, _ := db.ListCollectionNames(ctx, bson.D{})

    for _, name := range existing {
        if name == types.CollectionName {   // "service_logs"
            return nil                      // มีอยู่แล้ว ข้ามไป
        }
    }

    // สร้างใหม่แบบ time-series
    opts := options.CreateCollection().
        SetTimeSeriesOptions(...SetTimeField("timestamp")...).
        SetExpireAfterSeconds(ttlSeconds)   // ลบอัตโนมัติหลัง 30 วัน
    db.CreateCollection(ctx, types.CollectionName, opts)
}
```

log ทุกประเภทเก็บใน collection เดียว `service_logs` — เวลาอยากดูเฉพาะ type ก็ query ด้วย field `type` (ดู [phase2-collection.MD](phase2-collection.MD) สำหรับรายละเอียดการเปลี่ยนแปลง)

เรียกทุกครั้งตอน Connect — ถ้า collection มีอยู่แล้วก็ข้าม ไม่ error ทำให้รัน server ซ้ำได้ปลอดภัย

### InsertLogs — batch insert

```go
func (s *Store) InsertLogs(ctx context.Context, logs []types.BufferedLog) error {
    entries := make([]interface{}, len(logs))
    for i, log := range logs {
        entries[i] = log.Entry
    }
    s.db.Collection(types.CollectionName).InsertMany(ctx, entries)
    return nil
}
```

log ทุกตัวเข้า collection เดียว ไม่ต้อง group — แปลง `[]BufferedLog` เป็น `[]interface{}` แล้ว `InsertMany` ทีเดียว

### Error Wrapping ด้วย `%w`

```go
return fmt.Errorf("connect to mongo: %w", err)
```

`%w` คือ verb พิเศษที่ "ห่อ" error เดิมไว้ข้างใน — เพิ่มข้อความอธิบายบริบทให้ error แต่ยังเก็บ error ตั้งต้นไว้ด้วย caller เรียก `errors.Is(err, someSpecificError)` ได้แม้ถูก wrap แล้ว

---

## 10. `internal/httpapi/server.go` — รับ log ผ่าน HTTP

[internal/httpapi/server.go](external-service-log/internal/httpapi/server.go)

service นี้รับแค่ endpoint เดียว: `POST /ingest`

```go
func NewHandler(buf *buffer.LogBuffer, fl *flusher.BatchFlusher) http.Handler {
    mux := http.NewServeMux()

    mux.HandleFunc("POST /ingest", func(w http.ResponseWriter, r *http.Request) {
        var body interface{}
        if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
            writeJSON(w, http.StatusBadRequest, map[string]interface{}{
                "errors": []string{"Request body must be valid JSON"},
            })
            return
        }

        result := ingest.ProcessIngest(body, buf, fl)
        if !result.Accepted {
            writeJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": result.Errors})
            return
        }

        writeJSON(w, http.StatusAccepted, map[string]interface{}{"status": "accepted"})
    })

    return mux
}
```

flow ตรงไปตรงมา: decode JSON → ส่งไป ProcessIngest → ตอบ 202 (Accepted) หรือ 400 (error)

**202 Accepted ไม่ใช่ 200 OK** — 202 หมายความว่า "รับไว้แล้วจะจัดการ" ไม่ใช่ "ทำเสร็จแล้ว" — เหมาะกับ pattern นี้เพราะ log จริงๆ ยังอยู่ใน buffer ยังไม่ได้บันทึกลง MongoDB

---

## 11. `internal/grpcserver/server.go` — รับ log ผ่าน gRPC

[internal/grpcserver/server.go](external-service-log/internal/grpcserver/server.go)

### Embed UnimplementedIngestServiceServer

```go
type Server struct {
    pb.UnimplementedIngestServiceServer  // embed

    buf *buffer.LogBuffer
    fl  *flusher.BatchFlusher
}
```

`pb.UnimplementedIngestServiceServer` คือ struct ที่ protoc generate มาให้ — implement ทุก method ใน interface ด้วยการคืน error "ยังไม่รองรับ" เอาไว้ก่อน

การ embed ทำให้ `Server` ของเรา "ยืม" implementation เปล่าๆ ทุก method มาก่อน แล้วเราเขียนทับเฉพาะ `Ingest` ที่ต้องการ — ถ้าวันหนึ่ง proto เพิ่ม RPC ใหม่ โค้ดยัง compile ผ่านเพราะ method ใหม่ก็ใช้ตัวเปล่าจาก Unimplemented... ไปก่อน

### toRequestMap — แปลง proto message เป็น map

```go
func toRequestMap(req *pb.IngestRequest) (map[string]interface{}, []string) {
    body := map[string]interface{}{
        "source": map[string]interface{}{
            "app_name":     req.GetSource().GetAppName(),
            "service_name": req.GetSource().GetServiceName(),
        },
        "trace_id":    req.GetTraceId(),
        ...
    }
    ...
}
```

ทำไมใช้ `GetSource()` แทน `req.Source` ตรงๆ?

เพราะ `Source` ใน proto เป็น nested message (pointer) ที่อาจเป็น `nil` ได้ถ้า client ไม่ได้ส่งมา — getter ที่ generate มาจะเช็ค nil ให้อัตโนมัติแล้วคืนค่า default ถ้าเป็น nil ป้องกัน panic

### parseOptionalJSONObject — แกะ JSON string กลับเป็น object

```go
func parseOptionalJSONObject(value, fieldName string, errs *[]string) (map[string]interface{}, bool) {
    if value == "" {
        return nil, true   // ไม่ได้ส่งมา → ok ไม่ใช่ error
    }
    var parsed interface{}
    if err := json.Unmarshal([]byte(value), &parsed); err != nil {
        *errs = append(*errs, fieldName+" must be valid JSON")
        return nil, false
    }
    obj, ok := parsed.(map[string]interface{})
    if !ok {
        *errs = append(*errs, fieldName+" must be a JSON object")
        return nil, false
    }
    return obj, true
}
```

สามเคส:
- string ว่าง → "ไม่ได้ส่งมา" (ปกติ)
- parse ไม่ได้ → error "ต้องเป็น JSON"
- parse ได้แต่ไม่ใช่ object (เช่นเป็น array หรือ number) → error "ต้องเป็น JSON object"

### ทำไมไม่คืน gRPC error?

```go
func (s *Server) Ingest(_ context.Context, req *pb.IngestRequest) (*pb.IngestResponse, error) {
    ...
    return &pb.IngestResponse{Accepted: false, Errors: errs}, nil  // ← error เป็น nil เสมอ
}
```

กฎสำคัญ: `Ingest` คืน `(response, nil)` เสมอ ไม่เคยคืน gRPC error status

เพราะ gRPC error หมายถึง "การขนส่งพัง" (network error, server crash) แต่กรณีนี้ request มาถึงสำเร็จแล้ว แค่ข้อมูลไม่ผ่าน validate — ใช้ `Accepted: false` + `Errors` ใน response body แทน

---

## 12. `cmd/server/main.go` — จุดเริ่มโปรแกรม

[cmd/server/main.go](external-service-log/cmd/server/main.go)

### ลำดับการเริ่มต้น

```
1. อ่าน config จาก environment variable
        ↓
2. Connect MongoDB + สร้าง collection ถ้ายังไม่มี
        ↓
3. สร้าง LogBuffer (ว่างเปล่า)
        ↓
4. สร้าง BatchFlusher + Start() (เริ่ม ticker goroutine)
        ↓
5. สร้าง HTTP server ─── goroutine แยก (blocking call)
        ↓
6. สร้าง gRPC server ── goroutine แยก (blocking call)
        ↓
7. รอ signal SIGINT/SIGTERM (Ctrl+C หรือ docker stop)
        ↓
8. Graceful Shutdown (ตามลำดับ)
```

### ทำไม HTTP และ gRPC ต้องอยู่ใน goroutine แยก?

```go
go func() {
    httpServer.ListenAndServe()  // blocking — รันค้างตลอด
}()

go func() {
    grpcServer.Serve(lis)        // blocking — รันค้างตลอด
}()

<-stop  // main goroutine รอ signal ตรงนี้
```

`ListenAndServe()` และ `Serve()` เป็น **blocking call** — รันค้างอยู่ไม่ return จนกว่า server จะปิด ถ้าเรียกตรงๆ ใน main บรรทัดถัดไปจะไม่มีวันถูกรัน ต้องเอาแต่ละตัวใส่ goroutine แยก แล้วให้ main goroutine รอ signal แทน

### Environment Variables

```go
port            := getEnvInt("PORT", 3000)
grpcPort        := getEnvInt("GRPC_PORT", 50051)
mongoURI        := getEnv("MONGO_URI", "mongodb://localhost:27017")
mongoDBName     := getEnv("MONGO_DB_NAME", "service_logs")
flushMaxSize    := getEnvInt("FLUSH_MAX_SIZE", 100)
flushIntervalMS := getEnvInt("FLUSH_INTERVAL_MS", 5000)
```

ถ้าไม่ตั้งค่า env ใดๆ โปรแกรมจะใช้ค่า default และทำงานได้เลย — ตั้งค่าได้จากภายนอกโดยไม่ต้องแก้โค้ด

### Graceful Shutdown — ปิดอย่างมีระเบียบ

```go
_ = httpServer.Shutdown(shutdownCtx)  // หยุดรับ request ใหม่ รอของเก่าเสร็จ
grpcServer.GracefulStop()             // หยุดรับ RPC ใหม่ รอของเก่าเสร็จ
fl.Stop()                             // หยุด ticker
_ = fl.Flush(shutdownCtx)            // flush ของที่ค้างใน buffer ออกให้หมด
_ = store.Close(shutdownCtx)         // ปิด MongoDB connection
```

ลำดับสำคัญ — ปิด "ทางเข้า" ก่อน แล้วค่อยเคลียร์ของที่ค้างอยู่ ก่อนปิด connection กับ DB เป็นลำดับสุดท้าย ถ้าปิด DB ก่อนที่ buffer จะ flush → log หาย

---

## 13. ภาพรวม — Request ไหลยังไง

ตัวอย่าง: `POST /ingest` ส่ง log มาว่า user-service ตอบ 201

```
Client
  │  POST /ingest body={source:{...}, trace_id:"abc", http_status:"201", ...}
  ▼
[httpapi handler]
  ├─ json.Decode → body map
  ├─ ProcessIngest(body, buf, fl)
  │     ├─ ValidateIngestBody → valid ✓
  │     ├─ toIngestRequestBody → typed struct
  │     ├─ สร้าง LogEntry + Timestamp=now()
  │     ├─ buf.Push({Entry:...})
  │     └─ fl.OnLogPushed() ← buffer ยังไม่เต็ม → ไม่ flush ทันที
  └─ writeJSON(202, {"status":"accepted"})

... (ทุก 5 วินาที หรือเมื่อ buffer ถึง 100 ชิ้น)

[flusher goroutine ได้ tick]
  └─ Flush()
       └─ doFlush()
            ├─ buf.Drain() → [{entry1}, {entry2}, ...]
            └─ store.InsertLogs()
                 └─ db.Collection("service_logs").InsertMany(...)  → MongoDB ✓
```

---

## 14. เทส (`*_test.go`)

รันทั้งหมดด้วย:
```bash
go test ./... -race
```

`-race` ตรวจ data race — มีประโยชน์มากกับโค้ดที่ใช้ mutex/goroutine เยอะแบบ buffer/flusher

| ไฟล์เทส | ทดสอบอะไร | ต้องการ DB จริงไหม |
|---|---|---|
| `ingest/validate_test.go` | validate ทุก field ทุกกรณี | ไม่ |
| `ingest/process_test.go` | flow ครบ validate→push | ไม่ (mock buffer/flusher) |
| `buffer/logbuffer_test.go` | Push/Drain/Size/IsEmpty + concurrency | ไม่ |
| `flusher/batchflusher_test.go` | flush ตามเวลา, ตามขนาด, หยุดถูกต้อง | ไม่ (manual tick channel) |
| `mongostore/mongostore_test.go` | Connect, สร้าง collection, InsertLogs | ใช่ (MongoDB ผ่าน Docker) |
| `httpapi/server_test.go` | POST /ingest ทุกเคส | ไม่ (mock) |
| `grpcserver/server_test.go` | gRPC Ingest ทุกเคส | ไม่ (in-memory gRPC server) |

**flusher test ใช้ manual tick channel** — แทนที่จะรอเวลาจริง (sleep 5 วินาที) เทสส่ง tick เข้า channel เองตอนไหนก็ได้ ทำให้เทสรันเร็วและ deterministic

---

## 15. สรุปภาพรวม

```
go.mod              ← รายชื่อ library
proto/              ← "สัญญา" รูปแบบ gRPC
internal/types/     ← นิยาม struct/constant (ไม่มี logic)
internal/ingest/    ← แกนกลาง: validate, process (ใช้ร่วมกันทั้ง HTTP และ gRPC)
internal/buffer/    ← คิวรอ flush (thread-safe ด้วย mutex)
internal/flusher/   ← flush buffer ลง DB แบบ batch (ticker + size trigger)
internal/mongostore/← คุยกับ MongoDB (time-series collection)
internal/httpapi/   ← HTTP POST /ingest
internal/grpcserver/← gRPC Ingest()
cmd/server/main.go  ← ต่อทุกอย่างเข้าด้วยกัน + graceful shutdown
```

ทุกชิ้นส่วนมีหน้าที่ชัดเจน — เพิ่มช่องทางรับ log ใหม่ก็เพิ่มแค่ handler ใหม่แล้วเรียก `ProcessIngest` เดิม และเปลี่ยน database ได้ง่ายเพราะมี interface กลาง (ดู section ถัดไป)

---

## 14. Interface: เปลี่ยน Database ได้โดยไม่แตะ httpapi

หลังจาก refactor เพิ่ม `internal/logstore/` package ขึ้นมา โครงสร้างการ import เปลี่ยนไปแบบนี้:

```
internal/logstore/logstore.go  ← interface + FindLogsFilter อยู่ที่นี่
internal/mongostore/            ← implement interface นั้น
internal/httpapi/               ← รู้จักแค่ interface ไม่รู้จัก mongostore
```

### ทำไมต้องแยก

**ก่อน refactor:**
```go
// httpapi/server.go
import "external-service-log/internal/mongostore"

func NewHandler(..., store *mongostore.Store) http.Handler {
    // ผูกกับ MongoDB โดยตรง — เปลี่ยน DB = ต้องแก้ httpapi ด้วย
}
```

**หลัง refactor:**
```go
// httpapi/server.go
import "external-service-log/internal/logstore"

func NewHandler(..., store logstore.LogStore) http.Handler {
    // รู้แค่ interface — เปลี่ยน DB ไม่ต้องแตะ httpapi เลย
}
```

### Interface ใน Go คืออะไร

Interface คือการบอกว่า "ใครก็ได้ที่มี method เหล่านี้ถือว่าเป็น LogStore":

```go
// internal/logstore/logstore.go
type LogStore interface {
    InsertLogs(ctx context.Context, logs []types.BufferedLog) error
    FindLogs(ctx context.Context, filter FindLogsFilter) ([]types.LogEntry, error)
}
```

`*mongostore.Store` มีทั้งสอง method นี้อยู่แล้ว → Go ถือว่า implement interface โดยอัตโนมัติ **ไม่ต้องประกาศ** ว่า "implements LogStore" (ต่างจาก Java/C#)

### ถ้าจะเปลี่ยนเป็น PostgreSQL

สร้าง struct ใหม่ที่มี method ครบ แค่นั้นพอ — `httpapi` และ `flusher` ไม่ต้องแก้เลย:

```go
// internal/pgstore/pgstore.go (สมมติ)
type Store struct { pool *pgxpool.Pool }

func (s *Store) InsertLogs(ctx context.Context, logs []types.BufferedLog) error { ... }
func (s *Store) FindLogs(ctx context.Context, filter logstore.FindLogsFilter) ([]types.LogEntry, error) { ... }

// cmd/server/main.go — แค่เปลี่ยนบรรทัดเดียว
store, _ := pgstore.Connect(ctx, connStr)   // แทน mongostore.Connect(...)
httpapi.NewHandler(buf, fl, store)          // บรรทัดนี้ไม่ต้องแก้
```

### FindLogsFilter ต้องย้ายออกจาก mongostore

เหตุผลที่ย้าย `FindLogsFilter` ไปอยู่ใน `logstore` แทน `mongostore`:

ถ้า `FindLogsFilter` อยู่ใน `mongostore` → `httpapi` ต้อง import `mongostore` เพื่อสร้าง filter → ความตั้งใจที่จะแยก httpapi ออกจาก mongostore ก็ล้มเหลว

```
❌ httpapi → mongostore (เพื่อใช้ FindLogsFilter) → ผูกกันอีก
✅ httpapi → logstore (ใช้ FindLogsFilter + LogStore interface) → ไม่รู้จัก mongostore
```

นี่คือหลักการ **Dependency Inversion** — layer บน (httpapi) พึ่งพา abstraction (interface) ไม่ใช่ implementation (mongostore)
