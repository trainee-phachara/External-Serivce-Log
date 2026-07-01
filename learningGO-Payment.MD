# Learning Log: payment-service (Go)

บันทึกนี้อธิบาย `payment-service` ตั้งแต่ต้น สำหรับคนที่ยังไม่คุ้นกับ Go หรือ backend concept เลย
อ่านตามลำดับได้เลย แต่ละหัวข้อปูพื้นก่อนแล้วค่อยลงรายละเอียด

---

## 1. payment-service ทำอะไร

`payment-service` คือโปรแกรมที่รับคำสั่งผ่าน HTTP แล้วจัดการข้อมูล **Payment** ใน PostgreSQL — สร้าง/ดู/แก้ไข/ลบ (CRUD) และทุกครั้งที่มีคนเรียก API มา จะส่ง log ไปที่ `external-service-log` ผ่าน gRPC แบบ fire-and-forget เหมือน user-service และ order-service

```
Client (เบราว์เซอร์/แอป)
        │  HTTP request
        ▼
  payment-service  ──── gRPC log ────►  external-service-log
        │
        │  SQL query
        ▼
   PostgreSQL DB
```

Payment เชื่อมโยงกับ Order — เมื่อ user สั่งซื้อสินค้า (`order-service` สร้าง Order) ขั้นถัดไปคือการชำระเงิน (`payment-service` สร้าง Payment ที่อ้างถึง Order นั้น)

---

## 2. โครงสร้างโปรเจกต์

```
payment-service/
├── go.mod, go.sum
├── proto/ingest.proto          ← ก๊อปมาจาก order-service (ไม่แก้)
├── cmd/server/main.go
└── internal/
    ├── types/payment.go        ← Payment, PaymentStatus, PaymentMethod, ...
    ├── db/pool.go              ← EnsurePaymentsTable
    ├── payments/
    │   ├── validate.go
    │   ├── validate_test.go
    │   ├── repository.go       ← CRUD + FindByOrderID (เพิ่มจาก services ก่อนหน้า)
    │   └── repository_test.go
    ├── logclient/client.go
    ├── logclient/client_test.go
    ├── httpapi/
    │   ├── server.go           ← 6 endpoints (เพิ่ม GET /payments/order/{order_id})
    │   ├── payments.go
    │   ├── middleware.go
    │   ├── middleware_test.go
    │   └── server_test.go
    └── grpc/pb/
```

จุดที่ต่างจาก order-service:
- `payments/repository.go` มี method **`FindByOrderID`** เพิ่มมา
- `httpapi/server.go` มี endpoint **`GET /payments/order/{order_id}`** เพิ่มมา
- ไม่มี JSONB — payment มีแค่ scalar fields ทั้งหมด

---

## 3. `internal/types/payment.go` — หน้าตาของข้อมูล

[internal/types/payment.go](payment-service/internal/types/payment.go)

### สอง Typed Constant กลุ่ม

```go
type PaymentStatus string

const (
    PaymentStatusPending   PaymentStatus = "pending"
    PaymentStatusCompleted PaymentStatus = "completed"
    PaymentStatusFailed    PaymentStatus = "failed"
    PaymentStatusRefunded  PaymentStatus = "refunded"
)

type PaymentMethod string

const (
    PaymentMethodCreditCard   PaymentMethod = "credit_card"
    PaymentMethodBankTransfer PaymentMethod = "bank_transfer"
    PaymentMethodPromptPay    PaymentMethod = "promptpay"
)
```

Payment มีสอง enum แทนที่จะเป็นหนึ่ง — `PaymentStatus` บอก "เป็นไปแล้วหรือยัง" และ `PaymentMethod` บอก "ชำระด้วยอะไร" ทั้งคู่ใช้ pattern เดิม (type ใหม่จาก string + const กลุ่ม)

### Struct หลัก

```go
type Payment struct {
    ID        int           `json:"id"`
    OrderID   int           `json:"order_id"`
    UserID    int           `json:"user_id"`
    Amount    float64       `json:"amount"`
    Status    PaymentStatus `json:"status"`
    Method    PaymentMethod `json:"method"`
    CreatedAt time.Time     `json:"created_at"`
    UpdatedAt time.Time     `json:"updated_at"`
}

type CreatePaymentInput struct {
    OrderID int
    UserID  int
    Amount  float64
    Method  PaymentMethod
}

type UpdatePaymentInput struct {
    Status *PaymentStatus    // pointer = optional
}
```

**ทำไม Payment เก็บ UserID ซ้ำจาก Order?**

Order เก็บ UserID ไว้แล้ว — แต่การ denormalize (ซ้ำข้อมูล) ใน Payment ทำให้ query ง่ายขึ้น: "payments ของ user นี้มีอะไรบ้าง?" ดึงได้จาก payments table เดียวโดยไม่ต้อง JOIN กับ orders เหมาะกับ microservice ที่แต่ละ service มี DB แยกกัน

**ทำไม `UpdatePaymentInput` มีแค่ Status?**

Amount และ Method ไม่ควรเปลี่ยนหลังจากสร้าง payment แล้ว (ความปลอดภัยทางการเงิน) ถ้าต้องการชำระจำนวนอื่นต้องสร้าง payment ใหม่

---

## 4. `internal/db/pool.go` — สร้างตาราง

[internal/db/pool.go](payment-service/internal/db/pool.go)

```go
func EnsurePaymentsTable(ctx context.Context, pool *pgxpool.Pool) error {
    _, err := pool.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS payments (
            id         SERIAL PRIMARY KEY,
            order_id   INTEGER NOT NULL,
            user_id    INTEGER NOT NULL,
            amount     NUMERIC(10,2) NOT NULL,
            status     VARCHAR(20) NOT NULL DEFAULT 'pending',
            method     VARCHAR(20) NOT NULL,
            created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
        )
    `)
    return err
}
```

**Payment ไม่มี JSONB** — ทุก column เป็น scalar (ตัวเลข/string/เวลา) ธรรมดา ต่างจาก order-service ที่ต้องเก็บ `items` เป็น array

`NUMERIC(10,2)` สำหรับ Amount — เหตุผลเดิม: เก็บทศนิยมแน่นอน ไม่มี floating point error ซึ่งสำคัญมากสำหรับข้อมูลเงิน

ทำไมไม่ใช้ `FOREIGN KEY` ชี้ไปที่ orders table?

เพราะแต่ละ service มี database แยกกัน — payment DB ไม่รู้จัก orders table ของ order DB เลย ใน microservices เราตกลงกันผ่าน `order_id` (ตัวเลข) แทน foreign key ข้ามฐานข้อมูล

---

## 5. `internal/payments/validate.go` — ตรวจสอบ request body

[internal/payments/validate.go](payment-service/internal/payments/validate.go)

Validation ของ Payment ตรวจสองฝั่ง:

**CreatePayment** — ต้องมีครบสี่ field:
```go
// order_id: บวก + จำนวนเต็ม (เหมือน user_id ใน order-service)
if id, ok := orderIDRaw.(float64); !ok || id <= 0 || id != float64(int(id)) {
    errs = append(errs, "order_id must be a positive integer")
}

// amount: ตัวเลขบวก (ไม่ใช่ศูนย์ ไม่ใช่ลบ)
if a, ok := amountRaw.(float64); !ok || a <= 0 {
    errs = append(errs, "amount must be a positive number")
}

// method: ต้องอยู่ใน enum ที่กำหนด
var validMethods = map[string]bool{
    "credit_card": true, "bank_transfer": true, "promptpay": true,
}
if m, ok := methodRaw.(string); !ok || !validMethods[m] {
    errs = append(errs, "method must be one of: credit_card, bank_transfer, promptpay")
}
```

**UpdatePayment** — เปลี่ยนได้แค่ status:
```go
var validStatuses = map[string]bool{
    "pending": true, "completed": true, "failed": true, "refunded": true,
}
if s, ok := statusRaw.(string); !ok || !validStatuses[s] {
    errs = append(errs, "status must be one of: pending, completed, failed, refunded")
}
```

สังเกตว่า `"processing"` ไม่ใช่ status ที่ valid ใน Payment (ต่างจาก Order ที่มี `"processing"`) — เพราะ payment ตามหลัก "สำเร็จ/ล้มเหลว/รอ/คืนเงิน" ไม่มีสถานะ "กำลังดำเนินการ" ที่ค้างอยู่

**Test เพิ่มที่ไม่มีใน order-service**: `TestValidateCreatePayment_CollectsAllErrors` ส่ง body ว่างมาแล้วเช็คว่า errors ครบ 4 ตัว — ยืนยันว่าตรวจสอบทุก field โดยไม่หยุดที่ error แรก

---

## 6. `internal/payments/repository.go` — SQL queries

[internal/payments/repository.go](payment-service/internal/payments/repository.go)

### CRUD ปกติ — เหมือน user-service แต่ไม่ต้องมี JSON marshal/unmarshal

```go
func (r *Repository) Create(ctx context.Context, input types.CreatePaymentInput) (types.Payment, error) {
    var p types.Payment
    row := r.pool.QueryRow(ctx,
        "INSERT INTO payments (order_id, user_id, amount, method) VALUES ($1, $2, $3, $4) RETURNING "+paymentColumns,
        input.OrderID, input.UserID, input.Amount, input.Method,
    )
    err := scanPayment(row, &p)
    return p, err
}
```

ไม่มีขั้นตอน `json.Marshal` / `json.Unmarshal` เพราะทุก field เป็น scalar — ส่งตรงเข้า SQL ได้เลย driver จัดการ type conversion ให้

### FindByOrderID — method พิเศษที่ไม่มีใน services ก่อนหน้า

```go
func (r *Repository) FindByOrderID(ctx context.Context, orderID int) ([]types.Payment, error) {
    rows, err := r.pool.Query(ctx,
        "SELECT "+paymentColumns+" FROM payments WHERE order_id = $1 ORDER BY id ASC",
        orderID,
    )
    ...
}
```

เพิ่มมาเพราะ **use case จริง**: "Order ใบนี้จ่ายไปกี่ครั้ง?" — เช่น user อาจจ่ายผิดแล้วจ่ายใหม่ หรือระบบ retry การชำระเงิน ทำให้ order_id หนึ่งมีได้หลาย payment

ต่างจาก `FindAll` ตรงที่กรองด้วย `WHERE order_id = $1` ก่อน

---

## 7. `internal/httpapi/server.go` — 6 Endpoints

[internal/httpapi/server.go](payment-service/internal/httpapi/server.go)

payment-service มี **6 endpoints** แทนที่จะเป็น 5 เหมือน user/order:

```go
mux.HandleFunc("POST /payments",                   createPayment(repo))
mux.HandleFunc("GET /payments",                    listPayments(repo))
mux.HandleFunc("GET /payments/order/{order_id}",   listPaymentsByOrder(repo))  // ← ใหม่
mux.HandleFunc("GET /payments/{id}",               getPayment(repo))
mux.HandleFunc("PUT /payments/{id}",               updatePayment(repo))
mux.HandleFunc("DELETE /payments/{id}",            deletePayment(repo))
```

**ทำไม `GET /payments/order/{order_id}` ต้องอยู่ก่อน `GET /payments/{id}`?**

Go's `http.ServeMux` (รุ่น 1.22+) ใช้กฎ "pattern ที่เฉพาะเจาะจงกว่าชนะ" — `/payments/order/{order_id}` เฉพาะกว่า `/payments/{id}` เพราะมีส่วนคงที่ (`order`) อยู่ด้วย ดังนั้น `GET /payments/order/5` จะ match กับ route แรกก่อนเสมอ ไม่ตีความว่า `{id} = "order"`

ถ้าเรียกลำดับกลับกันก็ไม่ผิดใน 1.22+ เพราะ mux เลือก pattern ที่ match เฉพาะกว่าเอง แต่การเรียงให้เฉพาะมาก่อนเป็น convention ที่อ่านง่าย

---

## 8. เทส (`*_test.go`)

รันทั้งหมดด้วย:
```bash
go test ./... -race
```

| Package | Coverage |
|---|---|
| `internal/payments` (validate + repository) | 90.9% |
| `internal/logclient` | 88.9% |
| `internal/httpapi` | 86.4% |

| ไฟล์เทส | ทดสอบอะไร | ต้องการ DB จริงไหม |
|---|---|---|
| `payments/validate_test.go` | ทุก field ทุกกรณี, ตรวจสอบว่ารวบรวม error ครบ 4 ตัว | ไม่ |
| `payments/repository_test.go` | CRUD + `FindByOrderID` (ข้าม order, ผลลัพธ์ว่าง) | ใช่ (Docker) |
| `logclient/client_test.go` | ส่ง field ถูก, log error เมื่อพัง, close ไม่ error | ไม่ (mock gRPC) |
| `httpapi/middleware_test.go` | source ถูก, TraceID unique, default payload | ไม่ |
| `httpapi/server_test.go` | ทุก endpoint + `GET /payments/order/{id}` + log ถูก fire | ใช่ (Docker) |

### Test ที่น่าสนใจ: `TestListPaymentsByOrder_ReturnsOnlyMatchingPayments`

```go
// สร้าง payment สำหรับ order 1 สองใบ
doRequest(t, handler, http.MethodPost, "/payments", validPaymentBody())
doRequest(t, handler, http.MethodPost, "/payments", validPaymentBody())
// order 2 หนึ่งใบ
body2 := validPaymentBody()
body2["order_id"] = 2
doRequest(t, handler, http.MethodPost, "/payments", body2)

rec := doRequest(t, handler, http.MethodGet, "/payments/order/1", nil)
// ต้องได้ 2 ไม่ใช่ 3
```

test นี้ยืนยันสองสิ่ง: (1) `FindByOrderID` กรองถูกต้อง และ (2) ไม่รั่ว payment ของ order อื่นออกมาด้วย

---

## 9. สิ่งที่ต่างจาก order-service — สรุปความแตกต่าง

| จุด | order-service | payment-service |
|---|---|---|
| Fields ที่ซับซ้อน | `items []OrderItem` (JSONB) | ไม่มี — ทุก field เป็น scalar |
| JSON marshal/unmarshal | ใช่ (items) | ไม่ใช้ |
| Enum | 1 ชุด (OrderStatus) | 2 ชุด (PaymentStatus + PaymentMethod) |
| Extra query method | ไม่มี | `FindByOrderID` |
| จำนวน endpoints | 5 | 6 (เพิ่ม `/payments/order/{order_id}`) |
| Port default | 3002 | 3003 |
| Table | `orders` | `payments` |

---

## 10. ภาพรวม — ทุก service ที่มีตอนนี้

```
external-service-log (port 3000 HTTP / 50051 gRPC)
  └── รับ log จาก service ต่างๆ เก็บลง MongoDB

user-service    (port 3001)  ──gRPC log──►  external-service-log
  └── CRUD User → PostgreSQL (users table)

order-service   (port 3002)  ──gRPC log──►  external-service-log
  └── CRUD Order → PostgreSQL (orders table, items เป็น JSONB)

payment-service (port 3003)  ──gRPC log──►  external-service-log
  └── CRUD Payment → PostgreSQL (payments table)
```

service ทั้งสามส่ง log ไปที่ `external-service-log` ผ่าน gRPC โดยใช้ `ingest.proto` ชุดเดียวกัน — การเปลี่ยนที่ใดก็ตาม (เช่นเพิ่ม field ใน proto) จะกระทบทุก service พร้อมกัน ซึ่งเป็นเหตุผลว่าทำไมระบบจริงถึงแชร์ proto file ผ่าน package กลางหรือ git submodule แทนการก๊อปไฟล์

ขั้นถัดไปคือ `docker-compose.yml` ที่จะต่อทุก service เข้าด้วยกันและรัน MongoDB sharded cluster
