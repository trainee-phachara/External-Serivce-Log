# Phase 2: รวม Collection ให้เหลือตัวเดียว

บันทึกนี้อธิบายการเปลี่ยนจาก 3 MongoDB collections เป็น 1 collection ตั้งแต่เหตุผลว่าทำไมถึงเปลี่ยน, แนวคิดเบื้องหลัง, ไปจนถึงโค้ดที่แก้ทุกไฟล์

---

## 1. ก่อนเปลี่ยน — 3 Collections แยกกัน

เดิมระบบแบ่ง log ออกเป็น 3 collections ใน MongoDB:

| Collection | เก็บอะไร |
|---|---|
| `api_logs` | request/response ที่ HTTP status < 400 (สำเร็จ) |
| `error_logs` | request/response ที่ HTTP status >= 400 (error) |
| `event_logs` | business event เช่น "order placed", "payment completed" |

```
log เข้ามา
    │
    ▼
┌──────────────┐
│  classify.go │  ← ดู type + status code แล้วตัดสินว่าไป collection ไหน
└──────────────┘
    │
    ├── status >= 400?      → error_logs
    ├── type = request/response? → api_logs
    └── อื่นๆ (event)       → event_logs
```

มี function `ClassifyCollection()` ที่ทำหน้าที่นี้:

```go
// classify.go (ลบไปแล้ว)
func ClassifyCollection(body types.IngestRequestBody) types.CollectionName {
    if statusCode, err := strconv.Atoi(body.HTTPStatus); err == nil && statusCode >= 400 {
        return types.CollectionErrorLogs   // "error_logs"
    }
    if body.Type == types.LogTypeRequest || body.Type == types.LogTypeResponse {
        return types.CollectionAPILogs     // "api_logs"
    }
    return types.CollectionEventLogs       // "event_logs"
}
```

`BufferedLog` มี field `Collection` เพื่อบอกว่า log นี้ควรไปเก็บใน collection ไหน:

```go
// types/log.go (เก่า)
type BufferedLog struct {
    Collection CollectionName  // "api_logs", "event_logs", หรือ "error_logs"
    Entry      LogEntry
}
```

ตอน insert ลง MongoDB ก็ต้อง **group ตาม collection** แล้ว `InsertMany` แยกแต่ละกลุ่ม:

```go
// mongostore.go (เก่า)
func (s *Store) InsertLogs(ctx context.Context, logs []types.BufferedLog) error {
    grouped := make(map[types.CollectionName][]types.LogEntry)
    for _, log := range logs {
        grouped[log.Collection] = append(grouped[log.Collection], log.Entry)
    }
    for collectionName, entries := range grouped {
        s.db.Collection(string(collectionName)).InsertMany(ctx, entries)
    }
    return nil
}
```

---

## 2. ทำไมถึงเปลี่ยน

**ข้อมูลจริงๆ มันเหมือนกันหมดทั้ง 3 collections** — ทุก document มี field เหมือนกัน:

```json
{
  "timestamp": "...",
  "source": { "app_name": "order-service", "service_name": "order" },
  "trace_id": "abc-123",
  "endpoint": "/api/v1/orders",
  "http_status": "200",
  "type": "response",
  "direction": "inbound",
  "metadata": {},
  "raw_payload": { "..." },
  "payload": { "..." }
}
```

document ใน `api_logs`, `error_logs`, `event_logs` มี **schema เดียวกัน** — raw payload, metadata, source ทุกอย่างเหมือนกัน

สิ่งที่แตกต่างคือ **`type` field** ที่บอกอยู่แล้วว่า log นี้เป็น `"request"`, `"response"`, หรือ `"event"` — ไม่จำเป็นต้องแยก collection เพื่อแบ่งประเภท แค่ query ด้วย `type` ก็พอ

### ข้อดีของการรวม

- **ง่ายขึ้น** — ไม่ต้องมี classification logic, ไม่ต้อง group ก่อน insert
- **query ข้าม type ได้** — อยากดู log ทุก type ของ `order-service` ก็ query collection เดียว ไม่ต้อง query 3 ที่แล้วรวม
- **โค้ดน้อยลง** — ลบ classify.go ทิ้งได้เลย, InsertLogs ง่ายขึ้น

---

## 3. หลังเปลี่ยน — 1 Collection

```
log เข้ามา
    │
    ▼
validate → สร้าง LogEntry → push เข้า buffer → flush ลง service_logs
```

ไม่มีขั้นตอน classify อีกแล้ว — log ทุกตัวไปที่ collection เดียวชื่อ `service_logs`

เวลาอยากดู log เฉพาะ type ก็ query ด้วย field `type`:

```
GET /logs                     → ดูทุก log
GET /logs?type=request        → ดูเฉพาะ request logs
GET /logs?type=response       → ดูเฉพาะ response logs
GET /logs?type=event          → ดูเฉพาะ event logs
GET /logs?type=response&app=order-service → ดู response ของ order-service
```

---

## 4. ไฟล์ที่เปลี่ยน — ทีละไฟล์

### 4.1 `internal/types/log.go` — ลบ collection constants

**เก่า:** มี type `CollectionName` กับ 3 constants, `BufferedLog` มี field `Collection`

```go
type CollectionName string

const (
    CollectionAPILogs   CollectionName = "api_logs"
    CollectionEventLogs CollectionName = "event_logs"
    CollectionErrorLogs CollectionName = "error_logs"
)

type BufferedLog struct {
    Collection CollectionName
    Entry      LogEntry
}
```

**ใหม่:** เหลือแค่ constant ตัวเดียว, `BufferedLog` ไม่ต้องมี `Collection` แล้ว

```go
const CollectionName = "service_logs"

type BufferedLog struct {
    Entry LogEntry
}
```

**ทำไม `CollectionName` เปลี่ยนจาก type เป็น constant?**

เดิม `CollectionName` เป็น "typed string" เพราะมี 3 ค่าที่เป็นไปได้ — compiler ช่วยตรวจว่าไม่ใส่ค่าผิด

ตอนนี้เหลือค่าเดียว `"service_logs"` ไม่จำเป็นต้องมี type แยก — ใช้ `const` ธรรมดาพอ

**ทำไมลบ `Collection` ออกจาก `BufferedLog`?**

เมื่อก่อน log แต่ละตัวอาจไปคนละ collection — ต้องจำไว้ว่าตัวนี้ไป `api_logs` ตัวนั้นไป `error_logs` ตอนนี้ทุกตัวไปที่เดียวกันหมด ไม่ต้องจำอีกต่อไป

---

### 4.2 `internal/ingest/classify.go` — ลบทิ้ง

ไฟล์ `classify.go` และ `classify_test.go` ถูกลบทิ้ง เพราะไม่มีการ route log ไปคนละ collection อีกแล้ว

---

### 4.3 `internal/ingest/process.go` — ลบ classify call

**เก่า:** สร้าง `BufferedLog` พร้อมระบุ collection ปลายทาง

```go
buf.Push(types.BufferedLog{Collection: ClassifyCollection(body), Entry: entry})
```

**ใหม่:** push เข้า buffer ตรงๆ ไม่ต้อง classify

```go
buf.Push(types.BufferedLog{Entry: entry})
```

---

### 4.4 `internal/mongostore/mongostore.go` — single collection

**สร้าง collection:**

เก่าวน loop สร้าง 3 collections, ใหม่สร้างแค่ตัวเดียว `service_logs`

**InsertLogs:**

เก่า group ตาม collection แล้ว InsertMany แยกแต่ละกลุ่ม:

```go
// เก่า
grouped := make(map[types.CollectionName][]types.LogEntry)
for _, log := range logs {
    grouped[log.Collection] = append(grouped[log.Collection], log.Entry)
}
for collectionName, entries := range grouped {
    s.db.Collection(string(collectionName)).InsertMany(ctx, entries)
}
```

ใหม่ insert ทุกตัวเข้า collection เดียว:

```go
// ใหม่
entries := make([]interface{}, len(logs))
for i, log := range logs {
    entries[i] = log.Entry
}
s.db.Collection(types.CollectionName).InsertMany(ctx, entries)
```

**ทำไมต้องแปลงเป็น `[]interface{}`?**

`InsertMany` ของ MongoDB driver รับ `[]interface{}` ไม่ใช่ `[]LogEntry` — Go ไม่แปลง slice type ให้อัตโนมัติ (เช่น `[]LogEntry` ไม่ใช่ `[]interface{}` โดยอัตโนมัติ) ต้องแปลงทีละตัวเอง

**FindLogs:**

เก่าเลือก collection ตาม parameter:

```go
// เก่า — เลือก collection
coll := filter.Collection
if coll == "" {
    coll = types.CollectionAPILogs
}
cursor, err := s.db.Collection(string(coll)).Find(ctx, query, opts)
```

ใหม่ query จาก collection เดียว แล้ว filter ด้วย `type`:

```go
// ใหม่ — filter ด้วย type
query := bson.D{}
if filter.AppName != "" {
    query = append(query, bson.E{Key: "source.app_name", Value: filter.AppName})
}
if filter.Type != "" {
    query = append(query, bson.E{Key: "type", Value: string(filter.Type)})
}
cursor, err := s.db.Collection(types.CollectionName).Find(ctx, query, opts)
```

**`bson.D` vs `bson.E` คืออะไร?**

- `bson.D` คือ ordered list ของ key-value pairs — ใช้เป็น MongoDB query document
- `bson.E` คือ key-value pair ตัวเดียว (Element)
- `append(query, bson.E{...})` = เพิ่มเงื่อนไขเข้าไปใน query

เดิมใช้ `bson.D{{Key: "...", Value: "..."}}` สร้างทีเดียว ตอนนี้ใช้ `append` เพราะเงื่อนไขอาจมีหรือไม่มีก็ได้ (ถ้า type ว่างก็ไม่ใส่ filter → ได้ log ทุก type)

---

### 4.5 `internal/logstore/logstore.go` — เปลี่ยน filter

**เก่า:** filter เลือก collection

```go
type FindLogsFilter struct {
    Collection types.CollectionName
    AppName    string
    Limit      int64
}
```

**ใหม่:** filter ด้วย type แทน

```go
type FindLogsFilter struct {
    AppName string
    Type    types.LogType
    Limit   int64
}
```

ไฟล์นี้คือ **interface** — เป็นสัญญาว่า "ใครก็ตามที่เก็บ log ต้องรองรับ filter แบบนี้" ตอนนี้สัญญาบอกว่า filter ด้วย `Type` แทน `Collection`

---

### 4.6 `internal/httpapi/server.go` — เปลี่ยน query parameter

**เก่า:** `GET /logs?collection=api_logs`

```go
coll := types.CollectionName(q.Get("collection"))
```

**ใหม่:** `GET /logs?type=request`

```go
logType := types.LogType(q.Get("type"))
```

Frontend หรือ client ที่เรียก API ต้องเปลี่ยนจากส่ง `collection=api_logs` เป็น `type=request`

---

### 4.7 Frontend — เปลี่ยน tabs

**`src/lib/api.ts`:**

เก่าส่ง `collection` parameter:

```ts
// เก่า
export function getLogs(collection = 'api_logs', app = '', limit = 50) {
  const p = new URLSearchParams({ collection, limit: String(limit) });
  ...
}
```

ใหม่ส่ง `type` parameter:

```ts
// ใหม่
export function getLogs(type_ = '', app = '', limit = 50) {
  const p = new URLSearchParams({ limit: String(limit) });
  if (type_) p.set('type', type_);
  ...
}
```

**ทำไมชื่อ `type_` มี underscore?**

เพราะ `type` เป็น reserved word ใน TypeScript — ใช้เป็นชื่อ parameter ไม่ได้ ต้องใส่ `_` ต่อท้าย

**`src/routes/+page.svelte`:**

เก่ามี 3 tabs ตาม collection:

```
[ API Logs ] [ Event Logs ] [ Error Logs ]
```

ใหม่มี 4 tabs ตาม type (รวม All):

```
[ All ] [ Request ] [ Response ] [ Event ]
```

กด "All" จะไม่ส่ง type filter → ได้ log ทุกประเภท

---

## 5. Tests ที่เปลี่ยน

| ไฟล์ | สิ่งที่เปลี่ยน |
|---|---|
| `classify_test.go` | ลบทิ้ง — ไม่มี classify แล้ว |
| `process_test.go` | ลบ test `RoutesErrorStatusToErrorLogs` ที่เช็ค collection routing |
| `server_test.go` | ลบ test `RoutesErrorStatusToErrorLogs`, แก้ `ClassifiesAndStoresEntryShape` ไม่เช็ค collection |
| `logbuffer_test.go` | ลบ `Collection` field จาก test helper `makeLog()` |
| `batchflusher_test.go` | ลบ `Collection` field จาก test helper `makeLog()` |
| `mongostore_test.go` | เขียนใหม่ — ทดสอบ single collection + filter by type |

---

## 6. สรุปภาพรวม

```
ก่อน:
  log → validate → classify (เลือก collection) → buffer → flush → 3 collections

หลัง:
  log → validate → buffer → flush → 1 collection (service_logs)
                                     query ด้วย type field
```

ลดความซับซ้อน, ไม่ต้องดูแล classification logic, query ข้าม type ได้ง่าย
