# Learning Log: order-service (Go)

บันทึกนี้อธิบาย `order-service` ตั้งแต่ต้น สำหรับคนที่ยังไม่คุ้นกับ Go หรือ backend concept เลย
อ่านตามลำดับได้เลย แต่ละหัวข้อปูพื้นก่อนแล้วค่อยลงรายละเอียด

---

## 1. order-service ทำอะไร

`order-service` คือโปรแกรมที่รับคำสั่งผ่าน HTTP แล้วจัดการข้อมูล **Order** ใน PostgreSQL database — สร้าง/ดู/แก้ไข/ลบ (CRUD) และทุกครั้งที่มีคนเรียก API มา จะส่ง log ไปที่ `external-service-log` ผ่าน gRPC แบบ "ส่งแล้วไม่รอ" (fire-and-forget) เหมือนกับ `user-service` ทุกประการ

```
Client (เบราว์เซอร์/แอป)
        │  HTTP request
        ▼
  order-service  ──── gRPC log ────►  external-service-log
        │
        │  SQL query
        ▼
   PostgreSQL DB
```

**Order ต่างจาก User ยังไง?**

User เป็นข้อมูลง่ายๆ (name, email) แต่ Order มี **ความซับซ้อนสองชั้น**:
- Order หนึ่งใบมีหลาย **items** (รายการสินค้า) — เป็น "หนึ่งต่อหลาย" (one-to-many)
- แต่ละ item มีทั้ง product_id, product_name, quantity, unit_price
- Order มี **status** ที่เปลี่ยนได้ตามลำดับ (pending → processing → shipped → delivered หรือ cancelled)
- `total_amount` คำนวณจาก items อัตโนมัติ ไม่ต้องให้ client ส่งมา

---

## 2. โครงสร้างโปรเจกต์

```
order-service/
├── go.mod, go.sum              ← รายชื่อ library + checksum (เหมือน user-service)
├── proto/ingest.proto          ← "สัญญา" ส่ง log ให้ external-service-log (ก๊อปมาทั้งไฟล์)
├── cmd/server/main.go          ← จุดเริ่มโปรแกรม — ต่อทุกอย่างเข้าด้วยกัน
└── internal/
    ├── types/order.go          ← นิยาม struct ทั้งหมด (Order, OrderItem, OrderStatus, ...)
    ├── db/pool.go              ← เปิด/จัดการการเชื่อมต่อ PostgreSQL
    ├── orders/
    │   ├── validate.go         ← ตรวจว่า request body ถูกต้องไหม
    │   ├── validate_test.go
    │   ├── repository.go       ← SQL queries ทั้งหมด (Create/Find/Update/Remove)
    │   └── repository_test.go
    ├── logclient/client.go     ← ส่ง log ไป external-service-log ผ่าน gRPC
    ├── logclient/client_test.go
    ├── httpapi/
    │   ├── server.go           ← สร้าง router, กำหนดว่า URL ไหนไปฟังก์ชันไหน
    │   ├── orders.go           ← logic ของแต่ละ endpoint (createOrder, getOrder, ...)
    │   ├── middleware.go       ← "ด่านตรวจ" ที่นั่งอยู่หน้าทุก endpoint — บันทึก log
    │   ├── middleware_test.go
    │   └── server_test.go
    └── grpc/pb/                ← โค้ดที่ protoc generate จาก ingest.proto
```

โครงสร้างแทบเหมือน `user-service` เป๊ะๆ — เปลี่ยนแค่ชื่อ package (`users` → `orders`) และ type ของข้อมูล

---

## 3. `internal/types/order.go` — หน้าตาของข้อมูลทั้งหมด

[internal/types/order.go](order-service/internal/types/order.go)

### Typed Constant สำหรับ OrderStatus

```go
type OrderStatus string

const (
    OrderStatusPending    OrderStatus = "pending"
    OrderStatusProcessing OrderStatus = "processing"
    OrderStatusShipped    OrderStatus = "shipped"
    OrderStatusDelivered  OrderStatus = "delivered"
    OrderStatusCancelled  OrderStatus = "cancelled"
)
```

เหมือนกับที่ `external-service-log` ใช้กับ `LogType` — Go ไม่มี `enum` แต่เราสร้าง "enum แบบ Go" ได้ด้วย `type` + `const` กลุ่ม ผลคือ compiler จะ error ถ้าส่ง string ธรรมดาเข้าไปในฟังก์ชันที่รับ `OrderStatus` — ป้องกัน typo โดยไม่ต้องเช็คเอง

### Struct หลักสี่ตัว

```go
type OrderItem struct {
    ProductID   int     `json:"product_id"`
    ProductName string  `json:"product_name"`
    Quantity    int     `json:"quantity"`
    UnitPrice   float64 `json:"unit_price"`
}

type Order struct {
    ID          int         `json:"id"`
    UserID      int         `json:"user_id"`
    Items       []OrderItem `json:"items"`
    Status      OrderStatus `json:"status"`
    TotalAmount float64     `json:"total_amount"`
    CreatedAt   time.Time   `json:"created_at"`
    UpdatedAt   time.Time   `json:"updated_at"`
}

type CreateOrderInput struct {
    UserID int
    Items  []OrderItem
}

type UpdateOrderInput struct {
    Status *OrderStatus    // pointer = "optional" — nil หมายถึง "ไม่ได้ส่งมา"
}
```

**ทำไม `Items []OrderItem` แทนที่จะเป็น `[]int` (แค่ product_id)?**

เพราะ order ต้องเก็บ snapshot ของราคาและชื่อสินค้า ณ เวลาที่สั่ง — ถ้าเก็บแค่ product_id ต่อมาสินค้าเปลี่ยนราคา order เก่าก็จะเห็นราคาใหม่ซึ่งผิดประวัติศาสตร์ การ embed ข้อมูลสินค้าไว้เลยเป็น pattern ที่ระบบ order ทั่วไปใช้

**ทำไม `UpdateOrderInput` ไม่มี Items?**

การเปลี่ยน items ใน order ที่สร้างแล้วเป็น operation ที่ซับซ้อนและมักจะไม่อนุญาตในระบบจริง (ต้อง cancel แล้วสร้าง order ใหม่แทน) เราจึงออกแบบให้อัปเดตได้แค่ `status` เท่านั้น — เพิ่มทีหลังได้ถ้าต้องการ

---

## 4. `internal/db/pool.go` — เชื่อมต่อ PostgreSQL

[internal/db/pool.go](order-service/internal/db/pool.go)

```go
func EnsureOrdersTable(ctx context.Context, pool *pgxpool.Pool) error {
    _, err := pool.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS orders (
            id           SERIAL PRIMARY KEY,
            user_id      INTEGER NOT NULL,
            items        JSONB NOT NULL DEFAULT '[]',
            status       VARCHAR(20) NOT NULL DEFAULT 'pending',
            total_amount NUMERIC(10,2) NOT NULL DEFAULT 0,
            created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
            updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
        )
    `)
    return err
}
```

### JSONB — เก็บ array ใน column เดียว

`items JSONB` คือ column ที่เก็บ JSON array ของ `OrderItem` ไว้ในคอลัมน์เดียว

**ทางเลือกที่สอง** คือสร้างตาราง `order_items` แยก แล้วใช้ foreign key เชื่อม:
```sql
CREATE TABLE order_items (
    id       SERIAL PRIMARY KEY,
    order_id INTEGER REFERENCES orders(id),
    ...
)
```

| | JSONB | แยกตาราง |
|---|---|---|
| query ง่าย? | ✓ ดึง order พร้อม items ในครั้งเดียว | ✗ ต้อง JOIN หรือ query สองรอบ |
| เปลี่ยน items แต่ละชิ้นได้? | ✗ ต้อง replace ทั้ง array | ✓ UPDATE แถวเดี่ยวได้ |
| เหมาะกับ order? | ✓ เพราะ items ไม่ค่อยถูกแก้ทีละชิ้น | เกินจำเป็นสำหรับ use case นี้ |

เราเลือก JSONB เพราะ pattern ของ order คือ "อ่านทั้งก้อน, ไม่ค่อยแก้ทีละชิ้น" — และลดจำนวน table ทำให้ service เรียบง่ายกว่า

### NUMERIC(10,2) vs float64

`NUMERIC(10,2)` คือชนิดข้อมูลใน PostgreSQL ที่เก็บทศนิยม **แน่นอน** (ไม่มี floating point error) เหมาะกับเงิน — ถ้าใช้ `FLOAT` อาจได้ `9.9999999` แทน `10.00`

---

## 5. `internal/orders/validate.go` — ตรวจสอบ request body

[internal/orders/validate.go](order-service/internal/orders/validate.go)

ส่วนที่ต่างจาก `user-service` อย่างเห็นได้ชัดคือ `ValidateCreateOrder` ต้องตรวจสอบ **array ซ้อนใน object**:

```go
itemsRaw, hasItems := candidate["items"]
if !hasItems {
    errs = append(errs, "items is required")
} else {
    itemSlice, ok := itemsRaw.([]interface{})   // type assertion: เป็น array ไหม?
    if !ok {
        errs = append(errs, "items must be an array")
    } else if len(itemSlice) == 0 {
        errs = append(errs, "items must not be empty")
    } else {
        for i, raw := range itemSlice {         // วนตรวจแต่ละ item
            item, ok := raw.(map[string]interface{})
            if !ok {
                errs = append(errs, fmt.Sprintf("items[%d] must be an object", i))
                continue
            }
            errs = append(errs, validateItem(item, i)...)  // ต่อ slice errors เข้ากัน
        }
    }
}
```

**`append(errs, validateItem(item, i)...)`** — `...` ที่ต่อท้าย slice แปลว่า "spread" — แตก slice ออกเป็น arguments แยกกันแต่ละตัว เหมือน `...args` ใน JavaScript เพื่อให้ `append` รับ error หลายตัวพร้อมกันได้

### เหตุผลที่ JSON numbers เป็น `float64` ใน Go

เวลา `json.Decoder` parse JSON ตัวเลขเช่น `{"user_id": 1}` เข้า `interface{}` — Go ไม่รู้ว่า `1` ควรเป็น int หรือ float เลยเลือก `float64` เสมอ (เพราะ `float64` รองรับทั้งสองได้)

ดังนั้นการตรวจสอบ `user_id` ต้องทำสามขั้น:
```go
userIDFloat, ok := userIDRaw.(float64)           // 1. เป็น float64 ไหม?
if !ok || userIDFloat <= 0 ||                    // 2. บวกไหม?
   userIDFloat != float64(int(userIDFloat)) {    // 3. เป็นจำนวนเต็มไหม? (ไม่ใช่ 1.5)
    errs = append(errs, "user_id must be a positive integer")
}
```

`userIDFloat != float64(int(userIDFloat))` ทำงานยังไง? แปลง `float64` เป็น `int` (ตัดทศนิยมทิ้ง) แล้วแปลงกลับเป็น `float64` — ถ้าค่าเดิม = ค่าที่ได้กลับมา แสดงว่าไม่มีทศนิยม (คือเป็นจำนวนเต็ม) ถ้าต่างกัน = มีทศนิยม → error

---

## 6. `internal/orders/repository.go` — SQL queries

[internal/orders/repository.go](order-service/internal/orders/repository.go)

### JSONB และ encoding/json

เพราะ `items` ใน DB เป็น `JSONB` การ insert/select ต้องแปลง `[]OrderItem` ↔ JSON bytes:

```go
// ตอน Insert: แปลง []OrderItem → JSON bytes ก่อนส่งให้ DB
itemsJSON, err := json.Marshal(input.Items)

// ตอน Scan: รับ JSON bytes กลับมาจาก DB แล้วแปลงเป็น []OrderItem
func scanOrder(row pgx.Row, o *types.Order) error {
    var itemsJSON []byte
    if err := row.Scan(&o.ID, &o.UserID, &itemsJSON, &o.Status, &o.TotalAmount, &o.CreatedAt, &o.UpdatedAt); err != nil {
        return err
    }
    return json.Unmarshal(itemsJSON, &o.Items)   // แปลงกลับเป็น struct
}
```

`json.Marshal` = Go struct → JSON bytes (`[]byte`)
`json.Unmarshal` = JSON bytes → Go struct

### total_amount คำนวณฝั่ง Go ไม่ใช่ฝั่ง DB

```go
func computeTotal(items []types.OrderItem) float64 {
    total := 0.0
    for _, item := range items {
        total += float64(item.Quantity) * item.UnitPrice
    }
    return total
}
```

เหตุผลที่ไม่ให้ DB คำนวณ (เช่น `SET total_amount = (SELECT SUM(...) FROM ...)`): DB ไม่รู้ว่า items อยู่ที่ไหน เพราะเราเก็บเป็น JSONB ไม่ใช่แยกตาราง — เลย compute ใน Go แล้วส่งค่าสำเร็จรูปไปให้ DB

### Update — เปลี่ยนได้แค่ status

```go
func (r *Repository) Update(ctx context.Context, id int, input types.UpdateOrderInput) (*types.Order, error) {
    existing, err := r.FindByID(ctx, id)
    if err != nil || existing == nil {
        return nil, err
    }

    status := existing.Status
    if input.Status != nil {
        status = *input.Status   // dereference pointer
    }

    // UPDATE แค่ status กับ updated_at ไม่แตะ items หรือ total_amount
    row := r.pool.QueryRow(ctx,
        "UPDATE orders SET status = $1, updated_at = now() WHERE id = $2 RETURNING "+orderColumns,
        status, id,
    )
    ...
}
```

**ทำไม `FindByID` ก่อน?** เพราะต้องรู้ว่า order มีอยู่จริงก่อน ถ้าส่ง UPDATE โดยตรงโดยไม่เช็ค DB จะ return "0 rows affected" เงียบๆ — การ `FindByID` ก่อนทำให้คืน `nil` กลับไปชัดเจนว่า "ไม่เจอ" แล้ว handler จะตอบ 404

**`*input.Status`** คือ dereference pointer — ดึงค่าออกจาก pointer เหมือนกับที่ `user-service` ใช้กับ `*input.Name`

---

## 7. `internal/logclient/client.go` — ส่ง log แบบ fire-and-forget

[internal/logclient/client.go](order-service/internal/logclient/client.go)

ไฟล์นี้เหมือน `user-service` ทุกบรรทัด ยกเว้นชื่อ module (`order-service`) ที่ import path เพราะใช้ `ingest.proto` ชุดเดียวกัน — contract ระหว่าง service กับ `external-service-log` ไม่เปลี่ยน

```go
func (c *grpcLogClient) SendLog(entry LogEntryInput) {
    go func() {
        _, err := c.client.Ingest(context.Background(), &pb.IngestRequest{...})
        if err != nil {
            log.Printf("Failed to send log to external-service-log: %v", err)
        }
    }()
}
```

ทำไม `go func()`? เพราะ gRPC call มีโอกาสรอ network — ถ้ารันใน goroutine เดียวกับที่รับ request จะทำให้ response ช้าโดยไม่จำเป็น `go func()` แยก goroutine ใหม่แล้วเรียก HTTP handler ต่อทันที logging ทำงานอยู่ข้างหลัง

---

## 8. `internal/httpapi/` — รับ HTTP request

### `middleware.go` — บันทึก log ทุก request อัตโนมัติ

[internal/httpapi/middleware.go](order-service/internal/httpapi/middleware.go)

middleware ทำงานคล้ายกับ "ห่อ" handler ทุกตัว — request เข้ามาต้องผ่าน middleware ก่อนเสมอ แล้ว response ออกไปก็ผ่านซ้ำ ทำให้ code logging อยู่ที่เดียวไม่กระจาย

เทคนิคที่ middleware ใช้จับ response body:

```go
type responseRecorder struct {
    http.ResponseWriter
    status int
    body   bytes.Buffer
}

func (r *responseRecorder) Write(b []byte) (int, error) {
    r.body.Write(b)                    // เก็บ copy ไว้ก่อน
    return r.ResponseWriter.Write(b)   // แล้วส่งออกไปจริงๆ
}
```

`responseRecorder` embed `http.ResponseWriter` ตัวจริง (Go embedding ทำให้ทุก method ของตัวในถูก "ยืม" มา) แล้ว override เฉพาะ `Write` และ `WriteHeader` ให้แอบเก็บค่าไว้ด้วย — handler ที่ข้างในเรียก `w.Write(...)` โดยไม่รู้เลยว่ากำลังถูก intercept อยู่

**`io.NopCloser`** ทำหน้าที่อะไร?

```go
rawBody, _ := io.ReadAll(r.Body)
r.Body = io.NopCloser(bytes.NewReader(rawBody))
```

`r.Body` เป็น stream ที่อ่านได้แค่ครั้งเดียว — พอ middleware อ่านด้วย `io.ReadAll` แล้ว handler ข้างในจะอ่านซ้ำไม่ได้ เลยต้องสร้าง reader ใหม่จาก bytes ที่อ่านมาแล้ว แล้วใส่กลับเข้าไปใน `r.Body` — `NopCloser` wrap `bytes.Reader` ให้มี method `Close()` ที่ `http.Request.Body` ต้องการ (แต่ไม่ทำอะไรเลย)

### `orders.go` — handler แต่ละ endpoint

[internal/httpapi/orders.go](order-service/internal/httpapi/orders.go)

pattern เหมือน `user-service` ทุกตัว: decode → validate → call repository → writeJSON

จุดที่ต่างเล็กน้อยคือ `createOrder` ตอบ `201 Created` ไม่ใช่ `202 Accepted` (ต่างจาก `external-service-log`) เพราะ order ถูกบันทึกลง DB ทันที ไม่ได้เข้า buffer รอเหมือน log

---

## 9. `cmd/server/main.go` — จุดเริ่มโปรแกรม

[cmd/server/main.go](order-service/cmd/server/main.go)

```
1. อ่าน config จาก environment variable
        ↓
2. Connect PostgreSQL + สร้าง table ถ้ายังไม่มี
        ↓
3. สร้าง Repository
        ↓
4. Connect LogClient (gRPC ไป external-service-log)
        ↓
5. เปิด HTTP server ใน goroutine แยก
        ↓
6. รอ signal SIGINT/SIGTERM (Ctrl+C หรือ docker stop)
        ↓
7. Graceful Shutdown
```

Environment variables:

| Variable | Default | ความหมาย |
|---|---|---|
| `PORT` | `3002` | พอร์ต HTTP (3001 ใช้โดย user-service แล้ว) |
| `DATABASE_URL` | `postgres://...@localhost/order_service` | PostgreSQL connection string |
| `LOG_SERVICE_GRPC_URL` | `localhost:50051` | ที่อยู่ external-service-log |

Graceful Shutdown:
```go
_ = httpServer.Shutdown(shutdownCtx)  // หยุดรับ request ใหม่ รอของเก่าเสร็จ
_ = logClient.Close()                 // ปิด gRPC connection
pool.Close()                          // ปิด DB connection pool
```

ลำดับสำคัญ: ปิด HTTP ก่อน (ไม่รับ request ใหม่) แล้วค่อยปิด gRPC และ DB — ถ้าปิด DB ก่อน request ที่ค้างอยู่จะ error

---

## 10. เทส (`*_test.go`)

รันทั้งหมดด้วย:
```bash
go test ./... -race
```

| ไฟล์เทส | ทดสอบอะไร | ต้องการ DB จริงไหม |
|---|---|---|
| `orders/validate_test.go` | validate ทุก field ทุกกรณี รวม nested items | ไม่ |
| `orders/repository_test.go` | CRUD ครบ + items JSON round-trip | ใช่ (PostgreSQL ผ่าน Docker) |
| `logclient/client_test.go` | ส่ง field ถูก, log error เมื่อ server พัง, close ไม่ error | ไม่ (mock gRPC server) |
| `httpapi/middleware_test.go` | บันทึก log ครบ field, default payload `{}`, TraceID ต่างกันทุก request | ไม่ |
| `httpapi/server_test.go` | ทุก endpoint ทุกเคส (valid/invalid/not found) + log ถูก fire | ใช่ (PostgreSQL ผ่าน Docker) |

---

## 11. สิ่งที่ต่างจาก user-service — สรุปความแตกต่าง

| จุด | user-service | order-service |
|---|---|---|
| โครงสร้างข้อมูล | flat (name, email) | nested (Order มี []OrderItem) |
| การเก็บใน DB | coluumns ธรรมดา | `items` เป็น JSONB |
| การคำนวณ | ไม่มี | `total_amount` คำนวณจาก items ใน Go |
| อะไรอัปเดตได้ | name, email | status อย่างเดียว |
| Port default | 3001 | 3002 |
| Table | `users` | `orders` |

---

## 12. ภาพรวม — Request ไหลยังไง

ตัวอย่าง: `POST /orders` สร้าง order ใหม่

```
Client
  │  POST /orders body={user_id:1, items:[{product_id:10, ...}]}
  ▼
[middleware]
  ├─ io.ReadAll(r.Body) → rawBody
  ├─ r.Body = io.NopCloser(...rawBody...)  ← ทำให้ handler อ่านได้อีกรอบ
  ├─ rec = responseRecorder{...}
  └─ handler.ServeHTTP(rec, r)
        ↓
[createOrder handler]
  ├─ decodeJSONBody → body map
  ├─ ValidateCreateOrder → valid ✓
  ├─ ToCreateOrderInput → typed struct
  ├─ computeTotal → 10.00
  ├─ json.Marshal(items) → []byte
  ├─ pool.QueryRow("INSERT ... RETURNING ...") → Order
  └─ writeJSON(w, 201, order)
        ↓
[middleware — หลัง handler เสร็จ]
  └─ logClient.SendLog(...)  ← fire-and-forget ใน goroutine แยก

Client ← 201 Created + order JSON
```
