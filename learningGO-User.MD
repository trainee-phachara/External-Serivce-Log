# Learning Log: user-service (Go)

บันทึกนี้อธิบาย `user-service` ตั้งแต่ต้น สำหรับคนที่ยังไม่คุ้นกับ Go หรือ backend concept เลย
อ่านตามลำดับได้เลย แต่ละหัวข้อปูพื้นก่อนแล้วค่อยลงรายละเอียด

---

## 1. user-service ทำอะไร

`user-service` คือโปรแกรมที่รับคำสั่งผ่าน HTTP แล้วจัดการข้อมูล **User** ใน PostgreSQL database — สร้าง/ดู/แก้ไข/ลบ (CRUD) และทุกครั้งที่มีคนเรียก API มา จะส่ง log ไปที่ `external-service-log` ผ่าน gRPC แบบ "ส่งแล้วไม่รอ" (fire-and-forget)

```
Client (เบราว์เซอร์/แอป)
        │  HTTP request
        ▼
  user-service  ──── gRPC log ────►  external-service-log
        │
        │  SQL query
        ▼
   PostgreSQL DB
```

**ทำไมต้องมีสองสาย?**
- สาย HTTP → PostgreSQL: ทำงานจริง (บันทึก/ดึง user)
- สาย gRPC → external-service-log: บันทึกว่ามีใครเรียก API อะไร ได้ผลลัพธ์อะไร ไว้ตรวจสอบภายหลัง

---

## 2. โครงสร้างโปรเจกต์

```
user-service/
├── go.mod, go.sum           ← รายชื่อ library ที่ใช้ + checksum
├── proto/ingest.proto        ← "สัญญา" ว่าจะส่ง log รูปแบบไหนให้ external-service-log
├── cmd/server/main.go        ← จุดเริ่มโปรแกรม — ต่อทุกอย่างเข้าด้วยกัน
└── internal/
    ├── types/user.go         ← นิยามหน้าตาของ "User" (struct)
    ├── db/pool.go            ← เปิด/จัดการการเชื่อมต่อ PostgreSQL
    ├── users/
    │   ├── validate.go       ← ตรวจว่า input ที่รับมาถูกต้องไหม
    │   └── repository.go     ← SQL queries ทั้งหมด (Create/Find/Update/Remove)
    ├── logclient/client.go   ← ส่ง log ไป external-service-log ผ่าน gRPC
    └── httpapi/
        ├── server.go         ← สร้าง router, กำหนดว่า URL ไหนไปฟังก์ชันไหน
        ├── users.go          ← logic ของแต่ละ endpoint (createUser, getUser, ...)
        └── middleware.go     ← "ด่านตรวจ" ที่นั่งอยู่หน้าทุก endpoint — บันทึก log
```

**ทำไม `internal/` ถึงพิเศษ?**
Go compiler บังคับว่าโค้ดใน `internal/` **ไม่สามารถถูก import โดยโปรเจกต์ภายนอก** ได้เลย เป็นการปิดกั้นระดับ compiler (ไม่ใช่แค่ตกลงกันเอง) เหมาะสำหรับโค้ด business logic ที่อยากให้ใช้ได้แค่ภายใน module นี้

---

## 3. `go.mod` — รายชื่อ library

[go.mod](user-service/go.mod) บอกว่าโปรเจกต์นี้ใช้ library อะไรบ้าง (เหมือน `package.json` ใน Node):

| Library | ทำหน้าที่อะไร |
|---|---|
| `github.com/jackc/pgx/v5` | คุยกับ PostgreSQL — ส่ง SQL query, รับผลลัพธ์กลับ |
| `google.golang.org/grpc` | ส่ง log ผ่าน gRPC ไปหา `external-service-log` |
| `google.golang.org/protobuf` | แปลงข้อมูลเป็น protobuf format ก่อนส่งผ่าน gRPC |
| `github.com/google/uuid` | สร้าง trace ID แบบสุ่ม (ตัวเลขยาวๆ ไม่ซ้ำกัน) สำหรับแนบกับทุก log |
| `testcontainers-go/.../postgres` | ใช้แค่ในเทส — รัน PostgreSQL จริงๆ ใน Docker สำหรับทดสอบ |

---

## 4. `internal/types/user.go` — หน้าตาของ User

[internal/types/user.go](user-service/internal/types/user.go)

ไฟล์นี้นิยาม **รูปร่างของข้อมูล** ที่ใช้ทั้งโปรเจกต์ ไม่มี logic อะไรเลย — แค่บอกว่า "User คืออะไร" และ "ข้อมูลที่รับเข้า-ส่งออกหน้าตาเป็นยังไง"

```go
type User struct {
    ID        int       `json:"id"`
    Name      string    `json:"name"`
    Email     string    `json:"email"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

**struct คืออะไร?** คือ "แม่พิมพ์" หรือ "แบบฟอร์ม" ที่บอกว่าข้อมูลชนิดนี้มีช่องอะไรบ้าง เหมือนตารางในกระดาษที่ระบุว่า "กรอกชื่อ, อีเมล, วันที่..."

**`json:"id"` คืออะไร?** เรียกว่า "struct tag" — เป็นคำอธิบายพิเศษต่อท้าย field ที่บอก Go ว่า "ตอนแปลงเป็น JSON ให้ใช้ชื่อ `id` (ตัวเล็ก) ไม่ใช่ `ID` (ตัวใหญ่)" เพราะ JSON convention นิยมตัวเล็ก

**struct สามรูป:**

```go
// ข้อมูล User เต็มๆ ที่เก็บใน DB (มี id, timestamp ครบ)
type User struct { ... }

// ข้อมูลที่รับจาก client ตอนสร้าง user ใหม่ (ไม่ต้องระบุ id — DB ให้อัตโนมัติ)
type CreateUserInput struct {
    Name  string
    Email string
}

// ข้อมูลที่รับจาก client ตอนแก้ไข user (ส่งแค่บางช่องก็ได้ — ไม่บังคับทุกช่อง)
type UpdateUserInput struct {
    Name  *string   // pointer — ดู section 6 (repository) ว่าทำไม
    Email *string
}
```

---

## 5. `internal/db/pool.go` — เชื่อมต่อ PostgreSQL

[internal/db/pool.go](user-service/internal/db/pool.go)

### Connection Pool คืออะไร?

นึกภาพว่า PostgreSQL คือ "คลังสินค้า" และโปรแกรมเราคือ "พนักงาน" ที่ต้องเข้าไปหยิบของบ่อยๆ

การเปิด-ปิดประตูคลังทุกครั้งที่ต้องการของ (เปิด connection ใหม่ทุก query) นั้นช้ามาก

**Connection Pool** คือการ **เปิดประตูค้างไว้หลายช่อง** ล่วงหน้า พนักงานคนไหนต้องการของก็หยิบช่องที่ว่างใช้ เสร็จแล้วคืนกลับ ไม่ต้องเปิดปิดประตูทุกครั้ง ทำให้เร็วขึ้นมาก

```go
func NewPool(ctx context.Context, connString string) (*pgxpool.Pool, error) {
    return pgxpool.New(ctx, connString)
}
```

`connString` คือ "ที่อยู่" ของ database เช่น `postgres://user:password@localhost:5432/mydb`

### `EnsureUsersTable`

```go
func EnsureUsersTable(ctx context.Context, pool *pgxpool.Pool) error {
    _, err := pool.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS users (
            id SERIAL PRIMARY KEY,
            ...
        )
    `)
    return err
}
```

สร้างตาราง `users` ใน DB ถ้ายังไม่มี (`IF NOT EXISTS` = ถ้ามีอยู่แล้วก็ข้ามไป ไม่ error) — เรียกทุกครั้งตอน server เริ่มทำงาน

**`SERIAL PRIMARY KEY`** = Go ไม่ต้องกำหนด id เอง — PostgreSQL นับให้อัตโนมัติ: user แรก id=1, คนที่สอง id=2, ...

### `context.Context` คืออะไร?

`ctx` ที่เห็นเป็น parameter แรกในทุกฟังก์ชัน คือ "นาฬิกาจับเวลา + ปุ่มยกเลิก" ที่ส่งต่อจากผู้เรียก

เช่น ถ้า client ส่ง HTTP request มา แล้วปิดหน้าต่างก่อนได้ผลลัพธ์ — `ctx` จะบอกให้ทุก operation ที่กำลังทำอยู่ (รวมถึง SQL query) หยุดทันที ไม่เปลืองทรัพยากรเปล่า

---

## 6. `internal/users/validate.go` — ด่านตรวจ Input

[internal/users/validate.go](user-service/internal/users/validate.go)

ก่อนจะแตะ database เลย โปรแกรมต้องตรวจก่อนว่า "ข้อมูลที่ client ส่งมา สมเหตุสมผลไหม?"

### ValidationResult

```go
type ValidationResult struct {
    Valid  bool
    Errors []string
}
```

ผลลัพธ์การตรวจมีแค่สองฟิลด์: ผ่านไหม? (`Valid`) และถ้าไม่ผ่านเพราะอะไรบ้าง? (`Errors` — list ของ error message)

### ทำไม input เป็น `interface{}`?

```go
func ValidateCreateUser(body interface{}) ValidationResult {
```

`interface{}` แปลว่า "อะไรก็ได้" — เพราะ HTTP request body ที่ client ส่งมาอาจเป็นอะไรก็ได้ (object, number, string, null) เราไม่รู้ล่วงหน้า จึงรับมาแบบกว้างก่อน แล้วค่อยตรวจว่าเป็นรูปแบบที่ถูกต้องไหม

```go
candidate, ok := body.(map[string]interface{})
if !ok {
    return ValidationResult{Valid: false, Errors: []string{"body must be a JSON object"}}
}
```

`body.(map[string]interface{})` คือ "ลองดูว่า body เป็น JSON object ไหม" — ถ้า client ส่ง `[1,2,3]` (array) หรือ `"hello"` (string) มา `ok` จะเป็น `false` แล้วตอบ error ทันที

### ตรวจ name และ email

```go
name := candidate["name"]
if !isNonEmptyString(name) {
    errors = append(errors, "name is required and must be a non-empty string")
}
```

- เช็คว่ามีช่อง `name` ไหม และเป็น string ที่ไม่ว่างเปล่า
- Email เพิ่มการเช็ค format ด้วย regular expression (`emailRegex`) — ต้องมี `@` และ `.` ในรูปแบบที่ถูกต้อง

### ValidateUpdateUser ต่างจาก Create ยังไง?

ตอน create — ทุกช่องบังคับ (ไม่ส่ง name มา → error)

ตอน update — ส่งแค่บางช่องก็ได้ แต่ถ้าส่งมาต้องไม่ว่าง:

```go
name, hasName := candidate["name"]
if hasName && !isNonEmptyString(name) {  // ถ้าส่งมา AND ว่างเปล่า → error
    errors = append(errors, "name must be a non-empty string when provided")
}
```

และต้องส่งอย่างน้อยหนึ่งช่อง (ห้ามส่ง body ว่างๆ แล้วแกล้งทำเป็น update):
```go
if !hasName && !hasEmail {
    errors = append(errors, "at least one of name or email must be provided")
}
```

---

## 7. `internal/users/repository.go` — คุยกับ Database

[internal/users/repository.go](user-service/internal/users/repository.go)

ไฟล์นี้คือ "ผู้ช่วย database" — รวม SQL query ทั้งหมดไว้ในที่เดียว ทุก handler แค่เรียก `repo.Create(...)` หรือ `repo.FindByID(...)` ไม่ต้องรู้ว่าข้างในเขียน SQL ว่ายังไง

### Repository Pattern คืออะไร?

นึกภาพว่า handler เป็น "นักขาย" และ database เป็น "คลังสินค้า" — นักขายไม่ต้องรู้ว่าคลังจัดเรียงของยังไง แค่บอก "ขอสินค้า A" แล้วรับของมา

Repository คือ "คนกลาง" ที่รู้วิธีคุยกับคลัง (SQL) และส่งของให้นักขายในรูปแบบที่เข้าใจได้ (Go struct)

### struct และ constructor

```go
type Repository struct {
    pool *pgxpool.Pool   // เก็บ connection pool ไว้ใช้ทุก query
}

func NewRepository(pool *pgxpool.Pool) *Repository {
    return &Repository{pool: pool}
}
```

`NewRepository` คือ "โรงงานสร้าง Repository" — รับ pool มา แล้วสร้าง Repository ที่พร้อมใช้งานกลับไป

### Create

```go
func (r *Repository) Create(ctx context.Context, input types.CreateUserInput) (types.User, error) {
    var u types.User
    row := r.pool.QueryRow(ctx,
        "INSERT INTO users (name, email) VALUES ($1, $2) RETURNING "+userColumns,
        input.Name, input.Email,
    )
    err := scanUser(row, &u)
    return u, err
}
```

- `$1, $2` คือ placeholder — pgx จะแทนค่าจริงให้อัตโนมัติ ป้องกัน SQL Injection (การโจมตีที่ client ส่ง SQL มาโดยตรง)
- `RETURNING` บอก PostgreSQL ว่า "หลัง INSERT เสร็จ ส่งแถวที่เพิ่งบันทึกกลับมาด้วย" เลยได้ `id` และ `created_at` ที่ DB สร้างให้กลับมาทันที ไม่ต้อง query ซ้ำ
- `scanUser` แปลงผลลัพธ์จาก DB (แถวข้อมูล) ใส่เข้า Go struct

### FindByID และการคืน `nil`

```go
func (r *Repository) FindByID(ctx context.Context, id int) (*types.User, error) {
    ...
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, nil   // ← ไม่ใช่ error แต่เป็นแค่ "ไม่มี"
    }
    return nil, err       // ← error จริงๆ (DB พัง ฯลฯ)
    ...
    return &u, nil        // ← เจอแล้ว
}
```

ทำไมคืน `*types.User` (pointer) ไม่ใช่ `types.User`?

เพราะต้องแยกสองกรณีออกจากกัน:
- เจอ user → คืน `&u` (pointer ชี้ไปที่ข้อมูล)
- ไม่เจอ user → คืน `nil` (pointer ที่ชี้ไปไหนไม่ได้)

ถ้าคืนเป็น `types.User` ธรรมดา (ไม่ใช่ pointer) Go จะต้องคืนค่าอะไรบางอย่างเสมอ ทำให้บอกไม่ได้ว่า "ไม่เจอจริงๆ"

### Update — ทำไม UpdateUserInput ใช้ pointer?

```go
type UpdateUserInput struct {
    Name  *string   // pointer
    Email *string   // pointer
}
```

ปัญหา: ถ้าเก็บเป็น `string` ธรรมดา เราบอกไม่ได้ว่า client "ไม่ส่ง name มา" หรือ "ส่ง name มาเป็น string ว่าง `""`" เพราะทั้งคู่จะเป็น `""` เหมือนกัน

pointer แก้ปัญหานี้ได้:
- client ไม่ส่ง name มา → `Name` เป็น `nil` ("ไม่ได้กำหนด")
- client ส่ง name มา → `Name` เป็น `&"ค่าที่ส่งมา"` (pointer ชี้ไปที่ค่า)

```go
name := existing.Name          // เริ่มจากค่าเดิม
if input.Name != nil {         // ถ้า client ส่ง name มา
    name = *input.Name         // ค่อยเปลี่ยนเป็นค่าใหม่
}
```

`*input.Name` คือการ "แกะ" ค่าออกจาก pointer — เปลี่ยนจาก "ที่อยู่" เป็น "ตัวค่า" เพื่อนำไปใช้

### Remove

```go
func (r *Repository) Remove(ctx context.Context, id int) (bool, error) {
    tag, err := r.pool.Exec(ctx, "DELETE FROM users WHERE id = $1", id)
    ...
    return tag.RowsAffected() > 0, nil
}
```

`RowsAffected()` บอกว่า DELETE ไปกี่แถว — ถ้าเป็น 0 แสดงว่าไม่มี user id นั้นอยู่ในระบบ (false = ไม่มีการลบ)

---

## 8. `internal/logclient/client.go` — ส่ง Log แบบไม่รอผล

[internal/logclient/client.go](user-service/internal/logclient/client.go)

### ไฟล์นี้ทำอะไร?

ทุกครั้งที่มี HTTP request เข้ามา `user-service` จะส่งบันทึกว่า "มี request อะไรมา ได้ผลลัพธ์อะไร" ไปที่ `external-service-log` ผ่าน gRPC

### Interface คืออะไร?

```go
type LogClient interface {
    SendLog(entry LogEntryInput)
    Close() error
}
```

Interface คือ "สัญญา" ว่า "ใครก็ตามที่อ้างว่าเป็น LogClient ต้องทำได้สองอย่างนี้"

**ทำไมต้องมี interface ไม่ใช่ใช้ struct ตรงๆ?**

เพราะในเทส เราไม่อยากเปิด connection gRPC จริงๆ — เราสร้าง "LogClient ปลอม" (mock) ที่ implement interface เดียวกันแต่แค่บันทึกว่า `SendLog` ถูกเรียกด้วย argument อะไร แล้วส่ง mock นั้นเข้าไปทดสอบแทน

### grpcLogClient (ตัวจริง)

```go
type grpcLogClient struct {
    conn   *grpc.ClientConn      // การเชื่อมต่อ gRPC
    client pb.IngestServiceClient // ตัว "โทรศัพท์" ที่ใช้โทรหา IngestService
}
```

`New(address string)` สร้าง connection ไปที่ `address` (เช่น `localhost:50051`) แล้วสร้าง IngestServiceClient ที่พร้อมโทรออก

### SendLog — Fire and Forget

```go
func (c *grpcLogClient) SendLog(entry LogEntryInput) {
    go func() {
        _, err := c.client.Ingest(context.Background(), &pb.IngestRequest{ ... })
        if err != nil {
            log.Printf("Failed to send log: %v", err)
        }
    }()
}
```

`go func() { ... }()` สร้าง **goroutine** ใหม่ — เหมือนเปิดเธรดใหม่ที่รันพร้อมกับโปรแกรมหลัก

**ทำไมต้องทำแบบนี้?**

การส่ง gRPC ต้องรอ response จาก `external-service-log` ซึ่งอาจใช้เวลา เราไม่อยากให้การรอนั้น **หน่วง HTTP response ที่ต้องส่งกลับ client**

เปรียบเหมือนร้านอาหาร: พนักงานรับออเดอร์เสร็จ เดินไปแปะใบออเดอร์ที่ห้องครัวแล้ว **กลับมารับออเดอร์ต่อทันที** ไม่ต้องยืนรอครัวทำอาหารเสร็จก่อน

ถ้า log service ล่มหรือช้า → handler ยังตอบ client ได้ปกติ แค่ `Printf` error ไว้ใน log ของตัวเอง

---

## 9. `proto/ingest.proto` — สัญญากับ external-service-log

[proto/ingest.proto](user-service/proto/ingest.proto)

ไฟล์นี้คือ "แบบฟอร์มมาตรฐาน" ที่กำหนดว่า `user-service` จะส่งข้อมูลหน้าตาไหนให้ `external-service-log`

```proto
message IngestRequest {
    LogSource source = 1;        // ส่งมาจากแอปไหน/service ไหน
    string trace_id = 2;         // ID ของ request นี้ (สุ่มสร้าง)
    string endpoint = 3;         // URL path เช่น /users/5
    string http_status = 4;      // HTTP status code เช่น "200", "404"
    string type = 5;             // "request" หรือ "response"
    string direction = 6;        // "inbound" หรือ "outbound"
    string metadata_json = 7;    // ข้อมูลเสริม (JSON string)
    string raw_payload_json = 8; // body ที่รับมาจาก client (JSON string)
    string payload_json = 9;     // body ที่ส่งกลับให้ client (JSON string)
}
```

**`option go_package`** บอก `protoc` ว่าโค้ดที่ generate ออกมาให้วางในแพ็กเกจ `user-service/internal/grpc/pb`

โค้ดที่ generate ออกมา (`internal/grpc/pb/`) เขียนโดย `protoc` อัตโนมัติ — ไม่ต้องแตะเอง

---

## 10. `internal/httpapi/` — HTTP Server

โฟลเดอร์นี้แบ่งหน้าที่เป็น 3 ไฟล์:

| ไฟล์ | ทำอะไร |
|---|---|
| `server.go` | "สารบัญ" — จับคู่ URL กับฟังก์ชัน, ใส่ middleware |
| `users.go` | logic จริงของแต่ละ endpoint |
| `middleware.go` | "ด่านตรวจ" ที่นั่งหน้าทุก endpoint — บันทึก log |

### 10.1 `server.go` — Router

[internal/httpapi/server.go](user-service/internal/httpapi/server.go)

```go
func NewHandler(repo *users.Repository, logClient logclient.LogClient) http.Handler {
    mux := http.NewServeMux()

    mux.HandleFunc("POST /users", createUser(repo))
    mux.HandleFunc("GET /users", listUsers(repo))
    mux.HandleFunc("GET /users/{id}", getUser(repo))
    mux.HandleFunc("PUT /users/{id}", updateUser(repo))
    mux.HandleFunc("DELETE /users/{id}", deleteUser(repo))

    return withRequestLogger(logClient, mux)
}
```

`http.NewServeMux()` คือ "สารบัญ" ของ server — บอกว่า "ถ้า client ส่ง GET /users มา ให้ไปเรียกฟังก์ชัน `listUsers`"

`"GET /users/{id}"` — `{id}` คือ **path parameter** ตัวแปรที่ฝังอยู่ใน URL เช่น `/users/42` → `id=42`

บรรทัดสุดท้าย `withRequestLogger(logClient, mux)` ห่อ mux ทั้งก้อนด้วย middleware — เหมือนใส่ wrapper รอบ router ทั้งหมด

**`writeJSON` และ `decodeJSONBody`** คือ helper เล็กๆ ที่ใช้ซ้ำทุก handler:
- `decodeJSONBody` อ่าน request body แล้วแปลงจาก JSON เป็น Go value (ถ้า body ว่างหรือ parse ไม่ได้ → คืน empty object แทน error)
- `writeJSON` ตั้ง Content-Type เป็น `application/json` + เขียน status code + แปลง Go value กลับเป็น JSON

### 10.2 `users.go` — Handlers

[internal/httpapi/users.go](user-service/internal/httpapi/users.go)

ทุก handler มี pattern เดียวกัน: ตรวจ input → ทำงาน → ตอบกลับ

**createUser:**
```
รับ body → validate → ถ้าไม่ผ่าน → 400 + errors
                     ถ้าผ่าน    → repo.Create → 201 + user ที่เพิ่งสร้าง
```

**getUser / updateUser / deleteUser:**
```
อ่าน {id} จาก URL → แปลงเป็น int (ถ้าแปลงไม่ได้ → 400)
→ เรียก repo → ถ้า user ไม่มี → 404
→ ตอบ 200/204
```

`r.PathValue("id")` คือวิธีอ่าน `{id}` ออกจาก URL (Go 1.22+) จะได้ string `"42"` → ต้องแปลงเป็น int ด้วย `strconv.Atoi` ก่อนใช้

**deleteUser คืน 204 ไม่ใช่ 200:**

```go
w.WriteHeader(http.StatusNoContent)
```

204 = "สำเร็จ แต่ไม่มีข้อมูลจะส่งกลับ" (ลบแล้วจะส่งอะไรกลับไป?) — ไม่ต้องเรียก `json.Encode` ตามหลัง HTTP มาตรฐาน

### 10.3 `middleware.go` — บันทึก Log ทุก Request

[internal/httpapi/middleware.go](user-service/internal/httpapi/middleware.go)

**Middleware คืออะไร?**

นึกภาพว่า handler แต่ละตัวคือ "พนักงานเฉพาะทาง" (createUser, getUser, ...) middleware คือ "ยาม" ที่นั่งอยู่หน้าประตูทางเดียว — ทุก request ที่จะเข้าไปหา handler ต้องผ่านยามคนนี้ก่อนเสมอ

```
Client → [middleware] → [mux] → [handler ที่ถูกต้อง]
         บันทึก request        ทำงานจริง
         รอ response
         บันทึก response
```

**ปัญหา:** โดยปกติ HTTP response จะถูกส่งออกไปหา client ทันทีที่ handler เขียนลง `w` — middleware ที่นั่งอยู่ด้านนอกจะ "ไม่เห็น" ว่า handler ตอบ status อะไรหรือ body อะไร

**วิธีแก้: `responseRecorder`**

```go
type responseRecorder struct {
    http.ResponseWriter   // ห่อตัว writer จริง
    status int            // ดักจับ status code
    body   bytes.Buffer   // ดักจับ body
}
```

middleware สร้าง `responseRecorder` ขึ้นมาเป็น "กระดาษทดแทน" ส่งให้ handler เขียนลงไปแทน handler เขียน status/body ลงบน recorder → recorder ส่งต่อให้ client จริงๆ พร้อมกับเก็บสำเนาไว้

```go
func withRequestLogger(logClient logclient.LogClient, next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 1. อ่าน request body ก่อน (body อ่านได้ครั้งเดียว)
        rawBody, _ := io.ReadAll(r.Body)
        r.Body = io.NopCloser(bytes.NewReader(rawBody))  // ใส่กลับให้ handler อ่านได้อีกครั้ง

        // 2. ส่ง request ไปให้ handler จริงผ่าน recorder
        rec := &responseRecorder{ResponseWriter: w}
        next.ServeHTTP(rec, r)

        // 3. ตอนนี้รู้แล้วว่า status=rec.status, body=rec.body
        logClient.SendLog(logclient.LogEntryInput{
            ...
            HTTPStatus:     strconv.Itoa(rec.status),
            RawPayloadJSON: string(rawBody),   // body จาก client
            PayloadJSON:    rec.body.String(), // body ที่ส่งกลับ
        })
    })
}
```

**`io.NopCloser(bytes.NewReader(rawBody))`** — body ของ HTTP request เป็น stream ที่อ่านได้ครั้งเดียว เมื่อ middleware อ่านไปแล้ว handler จะอ่านไม่ได้ → บรรทัดนี้ "รีเซต" stream ให้อ่านใหม่จากต้นได้

---

## 11. `cmd/server/main.go` — จุดเริ่มโปรแกรม

[cmd/server/main.go](user-service/cmd/server/main.go)

`main.go` คือไฟล์เดียวที่ Go จะรันก่อน — หน้าที่คือ "ต่อสาย" ทุกอย่างเข้าด้วยกัน

### ลำดับการเริ่มต้น

```
1. อ่านค่า config จาก environment variable
        ↓
2. เปิด connection pool กับ PostgreSQL
        ↓
3. สร้างตาราง users ถ้ายังไม่มี
        ↓
4. สร้าง Repository (เอา pool ใส่)
        ↓
5. เปิด connection gRPC ไปหา external-service-log
        ↓
6. สร้าง HTTP server
        ↓
7. รัน HTTP server ใน goroutine แยก (เพราะมันจะ block ถ้ารันตรงๆ)
        ↓
8. รอ signal SIGINT/SIGTERM (Ctrl+C หรือ docker stop)
        ↓
9. Graceful Shutdown
```

### Environment Variables

```go
port            := getEnvInt("PORT", 3001)
databaseURL     := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/user_service")
logServiceGRPCURL := getEnv("LOG_SERVICE_GRPC_URL", "localhost:50051")
```

Environment variable คือ "การตั้งค่าจากภายนอกโปรแกรม" — ทีม DevOps ตั้งค่าพวกนี้บน server โดยไม่ต้องแก้โค้ด ถ้าไม่ตั้งค่า (`getEnv`/`getEnvInt`) จะใช้ค่า default ที่กำหนดไว้

### ทำไม HTTP server รันใน goroutine?

```go
go func() {
    log.Printf("user-service listening on port %d", port)
    if err := httpServer.ListenAndServe(); ... { ... }
}()
```

`ListenAndServe()` เป็น blocking call — มันจะรันค้างไปเรื่อยๆ ไม่ return จนกว่า server จะปิด ถ้าเรียกโดยตรงใน `main()` บรรทัดถัดไปจะไม่มีวันถูกรัน ต้องเอาใส่ goroutine แยก แล้วให้ `main()` ไปรอ signal แทน

### Graceful Shutdown คืออะไร?

```go
stop := make(chan os.Signal, 1)
signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
<-stop  // หยุดรอตรงนี้จนกว่าจะได้ signal

// หลังได้ signal:
httpServer.Shutdown(shutdownCtx)  // รอ request ที่ค้างอยู่ให้เสร็จก่อน ไม่รับ request ใหม่
logClient.Close()                 // ปิด gRPC connection
pool.Close()                      // ปิด connection pool
```

เปรียบเหมือนร้านอาหารที่ "ปิดป้าย แต่ยังเสิร์ฟโต๊ะที่ยังกินอยู่จนเสร็จ" ไม่ใช่ไล่ลูกค้าออกทันที

---

## 12. ภาพรวมทั้งระบบ — Request ไหลยังไง

ตัวอย่าง: `POST /users` body = `{"name":"Alice","email":"alice@example.com"}`

```
Client
  │  POST /users
  ▼
[middleware] ← อ่าน rawBody ไว้ก่อน, สร้าง responseRecorder
  │
  ▼
[mux] → createUser handler
           │
           ├─ decodeJSONBody() → {"name":"Alice","email":"alice@..."}
           ├─ ValidateCreateUser() → {Valid:true}
           ├─ ToCreateUserInput() → {Name:"Alice", Email:"alice@..."}
           ├─ repo.Create() → INSERT INTO users ... RETURNING ...
           │                → PostgreSQL คืน {id:1, name:"Alice", ...}
           └─ writeJSON(201, user) → เขียนลง responseRecorder

  │  (handler กลับมา)
  ▼
[middleware] ← ดูว่า status=201, body={"id":1,"name":"Alice",...}
  │
  ├─ logClient.SendLog() ← fire goroutine ใหม่ ไม่รอ
  │       └─ goroutine: gRPC Ingest → external-service-log
  │
  └─ คืน response กลับไป client (201 + body)
```

---

## 13. เทส (`*_test.go`)

ทุกแพ็กเกจมีไฟล์ `_test.go` คู่กัน รันทั้งหมดด้วย:

```bash
go test ./... -race
```

`-race` คือ flag พิเศษที่ Go มาให้ — ตรวจจับว่ามีสองส่วนของโปรแกรมเข้าถึง memory เดียวกันพร้อมกันโดยไม่ปลอดภัยไหม (data race) มีประโยชน์มากกับโค้ดที่ใช้ goroutine

| ไฟล์เทส | ทดสอบอะไร | ต้องการ DB จริงไหม |
|---|---|---|
| `users/validate_test.go` | validate logic ทุกเคส (valid, missing fields, bad email) | ไม่ |
| `users/repository_test.go` | Create/FindAll/FindByID/Update/Remove กับ DB จริง | ใช่ (PostgreSQL ผ่าน Docker) |
| `logclient/client_test.go` | SendLog ส่ง request ถูกต้องไหม | ไม่ (in-memory gRPC server) |
| `httpapi/server_test.go` | HTTP handler ครบทุก endpoint | ไม่ (mock repo + mock logClient) |
| `httpapi/middleware_test.go` | middleware บันทึก log ถูกต้องไหม | ไม่ (mock logClient) |

**testcontainers-go** ใน repository_test.go — รัน PostgreSQL จริงใน Docker container ชั่วคราว ทดสอบเสร็จแล้วลบทิ้ง เป็นวิธีที่มั่นใจได้ว่า SQL query ทำงานถูกต้องกับ database จริง ไม่ใช่แค่ mock ที่อาจต่างจากของจริง

---

## 14. สรุปภาพรวม

```
go.mod              ← รายชื่อ library
proto/              ← "สัญญา" รูปแบบ log ที่ส่งหา external-service-log
internal/types/     ← นิยาม struct User / Input (ไม่มี logic)
internal/db/        ← เปิด connection pool + สร้างตาราง
internal/users/     ← validate input + SQL queries (CRUD)
internal/logclient/ ← ส่ง log แบบ fire-and-forget ผ่าน gRPC
internal/httpapi/   ← HTTP router + handlers + middleware
cmd/server/main.go  ← ต่อทุกอย่างเข้าด้วยกัน + graceful shutdown
```

ทุกชิ้นส่วนมีหน้าที่ชัดเจนและแยกจากกัน — เปลี่ยนวิธีส่ง log ก็แก้แค่ `logclient`, เปลี่ยน database schema ก็แก้แค่ `db` + `repository`, เพิ่ม endpoint ใหม่ก็แก้แค่ `users.go` + `server.go`
