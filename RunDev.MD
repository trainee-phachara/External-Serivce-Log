# RunDev — วิธีรัน Services ทั้งหมดในเครื่อง

รันตามลำดับจากบนลงล่าง เพราะแต่ละ service ขึ้นอยู่กับตัวก่อนหน้า

---

## 1. เริ่ม Database (Docker)

Services ทั้งหมดต้องการ database ก่อน จึงจะรันได้

### MongoDB — ใช้โดย `external-service-log`

```bash
docker run -d --name mongo-log -p 27017:27017 mongo:7
```

| ส่วน | ความหมาย |
|---|---|
| `docker run -d` | รัน container ในพื้นหลัง (detached) ไม่ block terminal |
| `--name mongo-log` | ตั้งชื่อ container ให้จำง่าย |
| `-p 27017:27017` | map port เครื่องเรา:port ใน container |
| `mongo:7` | ใช้ MongoDB image version 7 |

### PostgreSQL — ใช้โดย `user-service`, `order-service`, `payment-service`

```bash
docker run -d --name postgres-db -p 5432:5432 \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_USER=postgres \
  postgres:16
```

| ส่วน | ความหมาย |
|---|---|
| `-e POSTGRES_PASSWORD=postgres` | ตั้ง password ของ user postgres |
| `-e POSTGRES_USER=postgres` | ตั้งชื่อ user หลัก |
| `postgres:16` | ใช้ PostgreSQL image version 16 |

### สร้าง Database ใน PostgreSQL (ทำครั้งเดียว)

รอประมาณ 3 วินาที ให้ PostgreSQL พร้อมก่อน แล้วรันทีละบรรทัด:

```bash
docker exec postgres-db psql -U postgres -c "CREATE DATABASE user_service"
docker exec postgres-db psql -U postgres -c "CREATE DATABASE order_service"
docker exec postgres-db psql -U postgres -c "CREATE DATABASE payment_service"
```

`docker exec` คือการรันคำสั่งข้างใน container ที่ชื่อ `postgres-db`
`psql -U postgres -c "..."` คือรันคำสั่ง SQL ทันที

> ถ้า container หยุดแล้วเริ่มใหม่ ด้วย `docker start mongo-log postgres-db` database ที่สร้างไว้จะยังอยู่ ไม่ต้องสร้างใหม่

---

## 2. external-service-log (port 3000 + gRPC 50051)

รับ log จาก services อื่นผ่าน HTTP และ gRPC แล้วเก็บลง MongoDB

```bash
cd external-service-log
go run ./cmd/server/main.go
```

**Environment variables (ค่า default ถ้าไม่ตั้ง):**

| Variable | Default | ความหมาย |
|---|---|---|
| `PORT` | `3000` | HTTP port |
| `GRPC_PORT` | `50051` | gRPC port สำหรับรับ log จาก services |
| `MONGO_URI` | `mongodb://localhost:27017` | connection string ของ MongoDB |
| `MONGO_DB_NAME` | `service_logs` | ชื่อ database ใน MongoDB |
| `FLUSH_MAX_SIZE` | `100` | flush log ลง MongoDB เมื่อ buffer ครบ N entries |
| `FLUSH_INTERVAL_MS` | `5000` | flush ทุก N milliseconds ถึงแม้ buffer จะไม่เต็ม |

**log ที่จะเห็นเมื่อรันสำเร็จ:**
```
external-service-log HTTP ingest listening on port 3000
external-service-log gRPC ingest listening on port 50051
```

> ต้องรัน service นี้ก่อนเสมอ เพราะ user/order/payment-service ต้องเชื่อมต่อ gRPC :50051 ตอน start

---

## 3. user-service (port 3001)

CRUD User ผ่าน PostgreSQL และส่ง log ไปยัง external-service-log ผ่าน gRPC

```bash
cd user-service
go run ./cmd/server/main.go
```

**Environment variables:**

| Variable | Default | ความหมาย |
|---|---|---|
| `PORT` | `3001` | HTTP port |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/user_service` | connection string ของ PostgreSQL |
| `LOG_SERVICE_GRPC_URL` | `localhost:50051` | address ของ external-service-log gRPC |

**log ที่จะเห็นเมื่อรันสำเร็จ:**
```
user-service listening on port 3001
```

**API endpoints:**

| Method | Path | ทำอะไร |
|---|---|---|
| `GET` | `/users` | ดึง user ทั้งหมด |
| `POST` | `/users` | สร้าง user ใหม่ |
| `GET` | `/users/{id}` | ดึง user ตาม id |
| `PUT` | `/users/{id}` | แก้ไข user |
| `DELETE` | `/users/{id}` | ลบ user |

**ทดสอบด้วย curl:**

```bash
# สร้าง user
curl -X POST http://localhost:3001/users \
  -d '{"name": "Test User", "email": "test@example.com"}' \
  -H 'Content-Type: application/json'

# ดึง user ทั้งหมด
curl http://localhost:3001/users
```

---

## 4. order-service (port 3002)

CRUD Order ผ่าน PostgreSQL (items เก็บเป็น JSONB) และส่ง log ผ่าน gRPC

```bash
cd order-service
go run ./cmd/server/main.go
```

**Environment variables:**

| Variable | Default | ความหมาย |
|---|---|---|
| `PORT` | `3002` | HTTP port |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/order_service` | connection string ของ PostgreSQL |
| `LOG_SERVICE_GRPC_URL` | `localhost:50051` | address ของ external-service-log gRPC |

**log ที่จะเห็นเมื่อรันสำเร็จ:**
```
order-service listening on port 3002
```

**API endpoints:**

| Method | Path | ทำอะไร |
|---|---|---|
| `GET` | `/orders` | ดึง order ทั้งหมด |
| `POST` | `/orders` | สร้าง order ใหม่ |
| `GET` | `/orders/{id}` | ดึง order ตาม id |
| `PUT` | `/orders/{id}` | แก้ไข order |
| `DELETE` | `/orders/{id}` | ลบ order |

**ทดสอบด้วย curl:**

```bash
# สร้าง order (user_id ต้องมีอยู่ใน user-service)
curl -X POST http://localhost:3002/orders \
  -d '{
    "user_id": 1,
    "items": [
      {"product_id": 1, "product_name": "T-Shirt", "quantity": 2, "unit_price": 299}
    ]
  }' \
  -H 'Content-Type: application/json'

# ดึง order ทั้งหมด
curl http://localhost:3002/orders
```

---

## 5. payment-service (port 3003)

CRUD Payment ผ่าน PostgreSQL และส่ง log ผ่าน gRPC

```bash
cd payment-service
go run ./cmd/server/main.go
```

**Environment variables:**

| Variable | Default | ความหมาย |
|---|---|---|
| `PORT` | `3003` | HTTP port |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/payment_service` | connection string ของ PostgreSQL |
| `LOG_SERVICE_GRPC_URL` | `localhost:50051` | address ของ external-service-log gRPC |

**log ที่จะเห็นเมื่อรันสำเร็จ:**
```
payment-service listening on port 3003
```

**API endpoints:**

| Method | Path | ทำอะไร |
|---|---|---|
| `GET` | `/payments` | ดึง payment ทั้งหมด |
| `POST` | `/payments` | สร้าง payment ใหม่ |
| `GET` | `/payments/{id}` | ดึง payment ตาม id |
| `PUT` | `/payments/{id}/status` | อัปเดต status ของ payment |
| `DELETE` | `/payments/{id}` | ลบ payment |

**ทดสอบด้วย curl:**

```bash
# สร้าง payment (order_id และ user_id ต้องมีอยู่แล้ว)
curl -X POST http://localhost:3003/payments \
  -d '{
    "order_id": 1,
    "user_id": 1,
    "amount": 598,
    "method": "promptpay"
  }' \
  -H 'Content-Type: application/json'

# method ที่รองรับ: credit_card, bank_transfer, promptpay
```

---

## 6. Frontend — SvelteKit (port 5173)

```bash
cd frontend
npm run dev
```

เปิดเบราว์เซอร์ที่ `http://localhost:5173`

> ถ้า port 5173 ถูกใช้งานอยู่แล้ว Vite จะขึ้นที่ port ถัดไปอัตโนมัติ เช่น 5174

**หมายเหตุสำหรับเครื่องที่ใช้ nvm:**  
node อาจไม่อยู่ใน PATH ของ shell ใหม่ ให้โหลด nvm ก่อน:

```bash
export NVM_DIR="$HOME/.nvm" && source "$NVM_DIR/nvm.sh"
npm run dev
```

---

## สรุป Port ทั้งหมด

| Service | Port | Protocol |
|---|---|---|
| external-service-log | 3000 | HTTP |
| external-service-log | 50051 | gRPC (internal) |
| user-service | 3001 | HTTP |
| order-service | 3002 | HTTP |
| payment-service | 3003 | HTTP |
| Frontend (SvelteKit) | 5173 | HTTP |
| MongoDB | 27017 | TCP (Docker) |
| PostgreSQL | 5432 | TCP (Docker) |

---

## หยุด Services

```bash
# หยุด Go services — กด Ctrl+C ในแต่ละ terminal

# หยุด Docker containers
docker stop mongo-log postgres-db

# รัน Docker containers ใหม่ (database ข้อมูลยังอยู่)
docker start mongo-log postgres-db
```

---

## ทำความเข้าใจ Log Collection

เมื่อ services ทำงาน ทุก request จะถูกส่งไปเก็บใน MongoDB โดย `external-service-log` เก็บทุก log ใน collection เดียว `service_logs` แล้วแยกด้วย field `type`:

| Type | เก็บเมื่อ |
|---|---|
| `request` | ทุก HTTP request ที่เข้ามา |
| `response` | ทุก HTTP response ที่ตอบกลับ |
| `event` | create/update/delete สำเร็จใน user/order/payment-service |

ดู log ใน frontend ได้โดยเลือก tab All / Request / Response / Event หรือ query API ตรง เช่น `GET /logs?type=event`

**events ที่ส่งอัตโนมัติ:**

| Event | เกิดเมื่อ |
|---|---|
| `user.created` | POST /users สำเร็จ |
| `user.updated` | PUT /users/{id} สำเร็จ |
| `user.deleted` | DELETE /users/{id} สำเร็จ |
| `order.created` | POST /orders สำเร็จ |
| `order.updated` | PUT /orders/{id} สำเร็จ |
| `order.deleted` | DELETE /orders/{id} สำเร็จ |
| `payment.created` | POST /payments สำเร็จ |
| `payment.updated` | PUT /payments/{id} สำเร็จ |
| `payment.deleted` | DELETE /payments/{id} สำเร็จ |

> **หมายเหตุ:** log ไม่ได้เขียนลง MongoDB ทันที — buffer จะ flush ทุก 5 วินาที หรือเมื่อมี log สะสมครบ 100 entries รอสักครู่แล้ว refresh หน้า Logs ใน frontend
