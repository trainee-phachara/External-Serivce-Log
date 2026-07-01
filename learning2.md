# Learning Log 2: สร้าง user-service จาก 0 (CRUD ผ่าน PostgreSQL + ส่ง log แบบ fire-and-forget ผ่าน gRPC)

บันทึกนี้ต่อจาก [learning.md](learning.md) — รอบนี้สร้าง `user-service` ตัวแรกในบรรดา business service (order/user/payment) โดยมีหน้าที่สองอย่าง:
1. เป็น **CRUD API ของ User** (เก็บข้อมูลใน PostgreSQL จริง ไม่ใช่ MongoDB เหมือน `external-service-log`)
2. **ส่ง log ของทุก request/response กลับไปที่ `external-service-log` แบบ fire-and-forget ผ่าน gRPC** — โดยใช้ `ingest.proto` contract เดิมที่ทำไว้ในรอบก่อน

แนวทางคือ "ก๊อปปี้สไตล์" จาก `external-service-log` มาให้มากที่สุด (โครงสร้างไฟล์, DI ผ่าน `createApp`, threshold coverage 85%, แยก validation/storage/transport ออกจากกัน) แล้วค่อยแก้เฉพาะส่วนที่ต่างจริงๆ คือ "Postgres แทน MongoDB" และ "เป็น gRPC client แทนที่จะเป็น gRPC server"

---

## 1. เริ่มจาก 0: scaffold โครงสร้างโปรเจกต์

โครงสร้างไฟล์ที่ได้:
```
user-service/
├── src/
│   ├── types/
│   │   └── user.ts          # User, CreateUserInput, UpdateUserInput
│   ├── db/
│   │   └── pool.ts            # createPool() + ensureUsersTable()
│   ├── users/
│   │   ├── userRepository.ts  # CRUD ผ่าน pg (SQL ตรงๆ)
│   │   └── validateUser.ts    # validateCreateUser / validateUpdateUser
│   ├── routes/
│   │   └── users.ts           # Express router: 5 endpoints
│   ├── grpc/
│   │   ├── ingest.proto       # contract เดิมจาก external-service-log (ฝั่ง client)
│   │   └── logClient.ts       # createLogClient() -> { sendLog, close }
│   ├── middleware/
│   │   └── requestLogger.ts   # ดัก request/response ทุกตัว ส่ง log แบบ fire-and-forget
│   ├── app.ts                 # createApp(repo, logClient) — DI factory
│   └── index.ts               # entrypoint จริง
└── tests/
    ├── tsconfig.json
    ├── unit/
    │   ├── pool.test.ts
    │   ├── validateUser.test.ts
    │   ├── userRepository.test.ts   # ทดสอบกับ pg-mem
    │   └── requestLogger.test.ts
    └── integration/
        ├── users.test.ts        # supertest ยิงผ่าน createApp จริง
        └── grpcLogClient.test.ts # mock gRPC server ฝั่ง external-service-log
```

**`package.json`** — dependencies คือของจริงที่ต้องใช้ตอนรัน: `express`, `pg` (PostgreSQL client), `@grpc/grpc-js` + `@grpc/proto-loader` (gRPC client) ส่วน devDependencies เพิ่ม `pg-mem` เข้ามาใหม่ (ตัวจำลอง Postgres ในหน่วยความจำ สำหรับเทส) นอกนั้นเหมือนรอบก่อนทุกอย่าง (`typescript`, `jest`, `ts-jest`, `supertest`, `ts-node-dev`, `@types/*`)

**`tsconfig.json`** — หน้าตาเหมือนของ `external-service-log` เป๊ะๆ (`rootDir: "./src"`, `outDir: "./dist"`, `strict: true`, `exclude: ["node_modules", "dist", "tests"]`)

**`jest.config.js`** — เหมือนรอบก่อน ตั้ง `coverageThreshold` ที่ 85% ทั้ง 4 มิติ ต่างจุดเดียวคือ `collectCoverageFrom: ['src/**/*.ts', '!src/index.ts']` — ยกเว้นแค่ `index.ts` (entrypoint ที่ต่อ DB/gRPC จริง ไม่มีทาง unit test ได้อยู่แล้ว)

**`tests/tsconfig.json`** — เขียนให้ `exclude` override เป็น `["../node_modules", "../dist"]` ตั้งแต่ไฟล์แรกเลย (ไม่รวม `"../tests"` ที่สืบทอดมาจาก `tsconfig.json` หลัก) เพราะรอบที่แล้วเจอบั๊กนี้มาแล้ว (ดู [learning.md หัวข้อ 3.4](learning.md)) — ผลคือรอบนี้รัน `npx tsc -p tests/tsconfig.json --noEmit` แล้วผ่านสะอาดตั้งแต่ครั้งแรก ไม่ต้องมานั่งไล่หาว่าทำไมไฟล์ test ไม่ถูกมองเห็น

---

## 2. ไล่ทีละไฟล์: ทำไมถึงมีไฟล์นี้

### [user-service/src/types/user.ts](user-service/src/types/user.ts)
นิยาม `User` (shape ของ row ใน DB จริง รวม `id`, `created_at`, `updated_at`), `CreateUserInput` (`name`, `email` — ตอนสร้างต้องมีครบ), `UpdateUserInput` (ทั้งสอง field เป็น optional — เพราะ `PUT` อนุญาตให้แก้แค่บาง field) เหตุผลที่แยก 3 type นี้เหมือนที่ `LogEntry`/`IngestRequestBody` แยกกันในรอบก่อน: **สิ่งที่ "รับเข้ามา" กับ "เก็บจริง" ไม่จำเป็นต้องมีหน้าตาเหมือนกัน** — `id`/`created_at`/`updated_at` เป็นสิ่งที่ DB สร้างให้เอง ไม่มีทางอยู่ใน input

### [user-service/src/db/pool.ts](user-service/src/db/pool.ts)
คู่เทียบของ `config/mongo.ts` ในรอบก่อน แต่เรียบง่ายกว่ามาก เพราะ Postgres ไม่มีแนวคิด "Time Series collection" แบบ MongoDB:
- `createPool(connectionString)` — แค่ wrap `new Pool({ connectionString })` จาก `pg`
- `ensureUsersTable(pool)` — รัน `CREATE TABLE IF NOT EXISTS users (...)` ครั้งเดียวตอน start service เพื่อให้ schema พร้อมใช้งานเสมอ (คล้ายๆ `ensureTimeSeriesCollections()` แต่เป็นตารางธรรมดา ไม่ใช่ time series)

ตาราง `users` มี `id SERIAL PRIMARY KEY`, `email VARCHAR(255) UNIQUE` (กันอีเมลซ้ำที่ระดับ DB), `created_at`/`updated_at TIMESTAMPTZ DEFAULT now()`

### [user-service/src/users/userRepository.ts](user-service/src/users/userRepository.ts)
ชั้น data access — รับ `pg.Pool` เข้ามาทาง constructor แล้วรัน SQL ตรงๆ ทุก method:
- `create` → `INSERT ... RETURNING *`
- `findAll` → `SELECT * ... ORDER BY id ASC`
- `findById` → `SELECT * WHERE id = $1` คืน `null` ถ้าไม่เจอ
- `update` → เรียก `findById` ก่อน ถ้าไม่เจอคืน `null` ทันที (เป็นจุดเดียวที่ route ใช้เช็ค 404), ถ้าเจอก็เอาค่าที่ส่งมา (`input.name`/`input.email`) มา merge กับค่าเดิมด้วย `??` แล้วค่อย `UPDATE ... RETURNING *`
- `remove` → `DELETE ...` คืน `true`/`false` จาก `rowCount`

**ทำไม `update` ต้อง `findById` ก่อน?** เพราะ `PUT` รองรับการแก้แค่บาง field (`{ name: "ใหม่" }` โดยไม่ส่ง `email`) แต่ SQL `UPDATE` ต้องระบุค่าทุกคอลัมน์ที่จะ set — ถ้าไม่รู้ค่าปัจจุบันของ field ที่ไม่ได้ส่งมา ก็จะเขียนทับด้วย `NULL` โดยไม่ตั้งใจ การ query หาแถวเดิมก่อนแล้ว merge คือวิธีที่ตรงไปตรงมาที่สุด (แลกกับ query เพิ่ม 1 ครั้ง ซึ่งสำหรับ service ขนาดนี้ถือว่าคุ้ม)

### [user-service/src/users/validateUser.ts](user-service/src/users/validateUser.ts)
หน้าตา/โครงเหมือน `validateIngest.ts` ทุกประการ คืนค่า `{ valid: boolean, errors: string[] }` เสมอ:
- `validateCreateUser` — `name`/`email` ต้องเป็น string ไม่ว่าง, `email` ต้องผ่าน regex รูปแบบอีเมล
- `validateUpdateUser` — แต่ละ field เป็น optional แต่ถ้าใส่มาต้องไม่ว่างและถูกรูปแบบ และ **ต้องมีอย่างน้อย 1 field** (ส่ง `{}` มาเฉยๆ ถือว่า invalid เพราะไม่มีอะไรให้อัปเดต)
- `toCreateUserInput`/`toUpdateUserInput` — แปลง `unknown` (จาก `req.body`) เป็น type ที่ repository ต้องการ หลังผ่าน validate แล้ว

แยกออกมาจาก route ด้วยเหตุผลเดียวกับ `validateIngest.ts`: ทดสอบกฎ validation ตรงๆ ได้โดยไม่ต้องยิง HTTP

### [user-service/src/routes/users.ts](user-service/src/routes/users.ts)
Express router รับ `UserRepository` เข้ามาทาง factory function `createUsersRouter(repo)` มี 5 endpoint ตามสเปก CRUD:
- `POST /users` → validate → `400` ถ้าไม่ผ่าน, ไม่งั้น `repo.create()` → `201`
- `GET /users` → `repo.findAll()` → `200`
- `GET /users/:id` → เช็ค `id` เป็นจำนวนเต็มก่อน (`400` ถ้าไม่ใช่) → `repo.findById()` → `404` ถ้าไม่เจอ ไม่งั้น `200`
- `PUT /users/:id` → เช็ค `id` → validate body → `repo.update()` → `404`/`200`
- `DELETE /users/:id` → เช็ค `id` → `repo.remove()` → `404`/`204`

ทุก endpoint คืน error เป็น `{ errors: string[] }` รูปแบบเดียวกันหมด เพื่อให้ caller (frontend หรือ service อื่น) parse error ได้แบบเดียวกันไม่ว่าจะพังจาก validation หรือ "ไม่เจอ"

### [user-service/src/grpc/ingest.proto](user-service/src/grpc/ingest.proto)
**ก๊อปมาทั้งไฟล์จาก [external-service-log/src/grpc/ingest.proto](external-service-log/src/grpc/ingest.proto) แบบไม่แก้แม้แต่ตัวอักษรเดียว** เหตุผลตรงไปตรงมา: gRPC client กับ server ต้องคุยกันด้วย "สัญญา" (contract) เดียวกัน ถ้า field ไม่ตรงกันแม้แต่ชื่อเดียว การส่ง/รับจะพังหรือได้ค่าว่างเงียบๆ การก๊อปไฟล์ตรงๆ คือวิธีที่ง่ายและชัวร์ที่สุดที่จะการันตีว่าทั้งสองฝั่งเห็น message shape ตรงกัน (ในระบบจริงอาจแชร์ `.proto` ผ่าน package กลางหรือ git submodule แต่สำหรับ repo เดียวกันนี้ก๊อปไฟล์ก็เพียงพอ)

### [user-service/src/grpc/logClient.ts](user-service/src/grpc/logClient.ts)
เป็น "ภาพสะท้อน" ของ [external-service-log/src/grpc/server.ts](external-service-log/src/grpc/server.ts) แต่กลับด้าน — ฝั่งนั้นเป็น **server** ที่รับ `Ingest` request, ฝั่งนี้เป็น **client** ที่ยิง `Ingest` request ออกไป:
- โหลด `.proto` ด้วย `protoLoader.loadSync(..., { keepCase: true, longs: String, defaults: true })` — **ใช้ option ชุดเดียวกับฝั่ง server เป๊ะๆ** เพราะ `keepCase: true` คุมว่าชื่อ field จะเป็น `snake_case` (ตรงกับ `.proto`) หรือถูกแปลงเป็น `camelCase` — ถ้าสอง services ใช้ option ต่างกัน อาจจะส่ง field ชื่อหนึ่งแต่ฝั่งรับมองหาอีกชื่อหนึ่ง
- `createLogClient(address)` คืน object หน้าตาเรียบง่าย: `{ sendLog(entry), close() }`
- **`sendLog` คือหัวใจของ "fire-and-forget"**: เรียก `client.Ingest(entry, callback)` โดย callback มีหน้าที่แค่ `console.error` ถ้าเกิด error — ไม่ throw, ไม่ return promise ให้ caller ต้อง `await` เพราะถ้า logging service ล่มหรือช้า **ต้องไม่กระทบ response ที่ส่งกลับ user-service เอง**

### [user-service/src/middleware/requestLogger.ts](user-service/src/middleware/requestLogger.ts)
ชิ้นส่วนใหม่ที่ไม่มีในรอบก่อน — Express middleware ที่ mount แค่ครั้งเดียวใน `app.ts` แล้วครอบทุก route โดยอัตโนมัติ ไม่ต้องเขียนโค้ด logging ซ้ำในแต่ละ route หลักการทำงาน:

1. ตอน request เข้ามา → สร้าง `trace_id` ด้วย `crypto.randomUUID()` ทันที (ใช้ตัวเดียวกันได้ทั้ง request/response เพราะอยู่ใน closure เดียวกัน)
2. **ดัก `res.json`** — เขียนทับ `res.json` ของ instance นี้ให้เก็บ `body` ที่ route handler ส่งมาไว้ในตัวแปร `responseBody` ก่อน แล้วค่อยเรียก `res.json` ตัวเดิม (ของจริง) ต่อ — วิธีนี้ทำให้ middleware "เห็น" response body โดยที่ route handler ไม่ต้องรู้ตัวเลยว่าโดนดักอยู่
3. ลงทะเบียน `res.on('finish', ...)` — event นี้ยิงก็ต่อเมื่อ **response ถูกส่งกลับไปหา client เรียบร้อยแล้ว** ถึงตอนนี้ค่อยประกอบ object ตาม `ingest.proto` แล้วเรียก `logClient.sendLog(...)`:
   ```ts
   {
     source: { app_name: 'user-service', service_name: 'user' },
     trace_id,                                  // จากข้อ 1
     endpoint: req.path,
     http_status: String(res.statusCode),
     type: 'response',
     direction: 'inbound',
     metadata_json: JSON.stringify({ method: req.method }),
     raw_payload_json: JSON.stringify(req.body ?? {}),
     payload_json: JSON.stringify(responseBody ?? {})
   }
   ```
4. `next()` ถูกเรียกทันทีตั้งแต่ต้น (ไม่รอ logging) — route handler ทำงานตามปกติ การ logging เป็นแค่ "ผู้สังเกตการณ์" ที่แอบทำงานอยู่ข้างหลัง

**ทำไมเลือก `res.on('finish')` แทนที่จะ log ทันทีตอน request เข้า?** เพราะตอน request เข้ามาเรายังไม่รู้ผลลัพธ์ (`http_status`, response body) — ต้องรอให้ route handler ทำงานเสร็จและตอบกลับไปแล้วจริงๆ ก่อน ถึงจะมีข้อมูลครบสำหรับสร้าง 1 log entry ที่สมบูรณ์ (คำขอ + คำตอบ ในเรคคอร์ดเดียว)

ผลพลอยได้ที่ได้ "ฟรี": เพราะ field shape ตรงกับ `ingest.proto` ของ `external-service-log` เป๊ะๆ — **`user-service` ไม่ต้องรู้เรื่องว่า log จะเก็บยังไง** แค่ส่งข้อมูลดิบไปให้ถูก field ก็พอ ฝั่ง `external-service-log` จะเก็บลง collection `service_logs` แล้วแยกด้วย `type` field เวลา query

### [user-service/src/app.ts](user-service/src/app.ts) และ [user-service/src/index.ts](user-service/src/index.ts)
- `app.ts` — `createApp(repo: UserRepository, logClient: LogClient)` factory เหมือนรอบก่อนเป๊ะๆ: รับ dependency จากภายนอกทั้งคู่ ทำให้ test เขียน app instance ที่ผูกกับ `pg-mem` repo + mock `logClient` ได้ ไม่ต้องพึ่ง DB/gRPC จริง
- `index.ts` — entrypoint: อ่าน env (`PORT` default `3001` กันชนกับ `external-service-log` ที่ใช้ `3000`, `DATABASE_URL`, `LOG_SERVICE_GRPC_URL` default `localhost:50051`), เรียก `createPool` + `ensureUsersTable`, สร้าง `logClient`, สร้าง app, เปิด server, และ graceful shutdown (`server.close()` → `logClient.close()` → `pool.end()`)

---

## 3. การตัดสินใจออกแบบที่ต่างจากรอบแรก

### 3.1 ใช้ `pg-mem` แทนการ mock `pool.query` ตรงๆ
ใน [learning.md หัวข้อ 2](learning.md) เคยบันทึกไว้ว่า `config/mongo.ts` ไม่มี unit test ตรงๆ (เทสผ่านแค่ทางอ้อมจาก integration test) รอบนี้ตั้งใจไม่ให้เกิดช่องโหว่แบบเดียวกันกับ `userRepository.ts`/`db/pool.ts` เพราะสองไฟล์นี้ "เป็น SQL string ตรงๆ" — ถ้าเขียนผิด syntax หรือชื่อคอลัมน์ผิด การ mock `pool.query = jest.fn()` จะจับไม่ได้เลย (เพราะ mock ไม่สนใจว่า SQL ที่ส่งเข้ามาถูกต้องไหม)

`pg-mem` (`newDb().adapters.createPg()`) ให้ `Pool`/`Client` ที่ "หน้าตา" เหมือน `pg` ของจริงแต่รัน SQL จริงในหน่วยความจำ — `tests/unit/userRepository.test.ts` และ `tests/unit/pool.test.ts` (ทดสอบ `ensureUsersTable`) จึงได้ทดสอบ "SQL ที่เขียนถูกไหม" จริงๆ ไม่ใช่แค่ "เรียก `query()` ด้วย arguments ที่คาดไว้รึเปล่า"

### 3.2 Centralize logging ไว้ที่ middleware เดียว
ถ้าเขียนโค้ด "ส่ง log" แทรกไว้ในทุก route handler (5 endpoints × ใส่โค้ด log เอง) จะเกิดโค้ดซ้ำ 5 ที่ และเสี่ยงพลาด field บางตัวในบาง endpoint การรวมไว้ที่ `requestLogger.ts` middleware ตัวเดียว ทำให้ทุก endpoint (รวมถึง endpoint ที่จะเพิ่มในอนาคต) ได้ logging ฟรีโดยอัตโนมัติ — ตรงกับสปิริตเดียวกับที่รอบก่อนรวม validate→push ไว้ใน `processIngest.ts` ตัวเดียว (ดู [learning.md หัวข้อ 5.1](learning.md))

### 3.3 ใช้ `ingest.proto` ตัวเดิมโดยไม่แก้
อธิบายไว้แล้วในหัวข้อ 2 — ประเด็นสำคัญคือ field `metadata_json`/`raw_payload_json`/`payload_json` ที่ `external-service-log` เลือกแทน arbitrary JSON ด้วย "string ที่เข้ารหัส JSON" (แทนที่จะใช้ `google.protobuf.Struct`) ทำให้ฝั่ง client (`user-service`) แค่ `JSON.stringify(...)` ไปตรงๆ ก็พอ ไม่ต้องยุ่งกับ well-known types ของ protobuf เลย

---

## 4. อุปสรรคที่เจอ และวิธีแก้

### 4.1 TypeScript บ่นว่า parameter เป็น `'any'` ตอนสร้าง mock gRPC server ใน test (`addService`)
เขียน `tests/integration/grpcLogClient.test.ts` ที่ต้องสร้าง mock `IngestService` server (เพื่อทดสอบว่า `logClient.sendLog` ส่งข้อมูลถูกต้องจริง) ตอนแรกเขียนแบบนี้:
```ts
server.addService(proto.logging.IngestService.service, {
  Ingest: (call, callback) => ingestImpl(call, callback)
});
```
รัน `tsc -p tests/tsconfig.json --noEmit` แล้วได้ error:
```
error TS7006: Parameter 'call' implicitly has an 'any' type.
error TS7006: Parameter 'callback' implicitly has an 'any' type.
```

สาเหตุ: `addService` คาดหวัง object ที่มี type `UntypedServiceImplementation` (ซึ่งแต่ละ method มี type เป็น `(call: any, callback: any) => void`) — แต่ TypeScript ไม่ได้ "มองไปข้างหน้า" ว่า object literal นี้กำลังจะถูกใช้เป็น argument ของ `addService` ตอน type-check arrow function แต่ละตัวใน object literal (ไม่มี contextual type ให้ใช้ตอนนั้น) เลย infer parameter เป็น `any` แบบ implicit ซึ่ง `strict: true` ห้ามไว้

**วิธีแก้**: ดึง handler ออกมาประกาศเป็นตัวแปรแยก พร้อมกำกับ type ตรงๆ:
```ts
const ingestHandler: IngestHandler = (call, callback) => ingestImpl(call, callback);

server.addService(proto.logging.IngestService.service, {
  Ingest: ingestHandler
});
```
พอประกาศ type `IngestHandler` (= `grpc.handleUnaryCall<...>`) ไว้กับตัวแปรตรงๆ TypeScript จะใช้ type นั้นเป็น **contextual type** ให้กับ arrow function ทันที ทำให้ `call`/`callback` มี type ที่ถูกต้องโดยไม่ต้อง annotate เอง — พอ assign `{ Ingest: ingestHandler }` ให้ `addService` ก็ผ่าน เพราะ `handleUnaryCall<Specific, Specific>` assignable เข้ากับ `handleUnaryCall<any, any>` ได้ (ฝั่ง `any` ยอมรับทุกอย่าง)

บทเรียน: `as SomeType` ที่ต่อท้าย expression **ไม่ได้ทำให้ TypeScript ย้อนกลับไปช่วย infer type ของ sub-expression ข้างใน** (เช่น parameter ของ arrow function ใน object literal) ถ้าต้องการ contextual typing ต้อง declare type ไว้ที่ตำแหน่งที่ TypeScript "มองเห็นล่วงหน้า" จริงๆ เช่น ผ่าน type annotation ของตัวแปร หรือ parameter type ของฟังก์ชันที่ expression นั้นถูกส่งเข้าไปตรงๆ

### 4.2 Branch coverage ของ `userRepository.ts` ไม่ครบ 100% ตอนแรก (77.77%)
รัน `npm run test:coverage` รอบแรก (48 tests ผ่านหมด, coverage รวม 100/95.65/100/100) แต่ `userRepository.ts` มี branch coverage แค่ 77.77% เหลือ 2 บรรทัดที่ยังไม่ถูกแตะ:
```ts
const name = input.name ?? existing.name;     // บรรทัด 31
...
return (result.rowCount ?? 0) > 0;            // บรรทัด 43
```

**บรรทัด 31**: ตอนนั้นมีแค่เทส "อัปเดตเฉพาะ `name`" (ทำให้ `input.email` เป็น `undefined` → ฝั่งขวาของ `??` ที่บรรทัด 32 ถูกแตะ) แต่ไม่เคยมีเทส "อัปเดตเฉพาะ `email`" (ซึ่งจะทำให้ `input.name` เป็น `undefined` → ฝั่งขวาของ `??` ที่บรรทัด 31 ถูกแตะ)

**วิธีแก้**: เพิ่มเทส `'updates only the email, leaving the name unchanged'` ใน [user-service/tests/unit/userRepository.test.ts](user-service/tests/unit/userRepository.test.ts) — สร้าง user แล้ว `repo.update(id, { email: '...' })` (ไม่ส่ง `name`) แล้วเช็คว่า `name` เดิมยังอยู่ ผลคือ branch coverage ของไฟล์นี้ขยับจาก 77.77% → 88.88% ทันที (รวม 49 tests, coverage รวม 100/97.82/100/100)

**บรรทัด 43 (`rowCount ?? 0`) ยังเหลือไม่ครอบคลุม** — และตั้งใจปล่อยไว้แบบนั้น เพราะนี่คือสถานการณ์เดียวกับที่อธิบายไว้ใน [learning.md หัวข้อ 5.3](learning.md): type ของ `pg` ประกาศ `result.rowCount: number | null` ไว้แบบ defensive (เผื่อ driver บางสถานการณ์คืน `null`) แต่ในทางปฏิบัติ — ทั้งใน `pg-mem` และ `pg` จริงสำหรับ query ที่รันสำเร็จ — `rowCount` จะเป็นตัวเลขเสมอ ไม่มีทางเป็น `null` การจะไล่ปิด branch นี้ให้ครบต้อง mock `pool.query` ให้คืน `{ rowCount: null }` ตรงๆ ซึ่งขัดกับหลักการข้อ 3.1 (อยากเทสผ่าน SQL จริงผ่าน pg-mem ไม่ใช่ mock low-level) — coverage 97.82% เกิน threshold 85% ไปมากแล้ว เลยเลือกไม่ไล่ปิด branch ที่ "การันตีว่าไม่เกิดขึ้นจริง" อันนี้ ตามบทเรียนเดิม

---

## 5. สรุปผลลัพธ์

- **6 test suites, 49 tests** ผ่านหมด (`Tests: 49 passed, 49 total`)
- coverage รวม: **100% statements / 97.82% branches / 100% functions / 100% lines** — เกินเป้า 85% ในทุกมิติ

| ไฟล์ | Stmts | Branch | Funcs | Lines |
|---|---|---|---|---|
| `app.ts` | 100% | 100% | 100% | 100% |
| `db/pool.ts` | 100% | 100% | 100% | 100% |
| `grpc/logClient.ts` | 100% | 100% | 100% | 100% |
| `middleware/requestLogger.ts` | 100% | 100% | 100% | 100% |
| `routes/users.ts` | 100% | 100% | 100% | 100% |
| `users/userRepository.ts` | 100% | 88.88% | 100% | 100% |
| `users/validateUser.ts` | 100% | 100% | 100% | 100% |

- `npm run build` (`tsc` + copy `ingest.proto` ไปที่ `dist/grpc/`) ผ่านสำเร็จ ไม่มี type error
- automated test (unit + integration) **ไม่ต้องพึ่ง PostgreSQL หรือ external-service-log จริงเลย** — ใช้ `pg-mem` แทน Postgres และ mock gRPC server แบบ in-process แทน `external-service-log` ทั้งคู่
- ตอนรันจริง ต้องมี: PostgreSQL ที่ `DATABASE_URL` ชี้ถึง (default `postgres://postgres:postgres@localhost:5432/user_service`, สร้างตาราง `users` อัตโนมัติตอน start) และ `external-service-log`'s gRPC server รันอยู่ที่ `LOG_SERVICE_GRPC_URL` (default `localhost:50051`) — ถ้า logging service ไม่พร้อม `user-service` ยังทำงานได้ปกติ (fire-and-forget แค่ `console.error` แล้วไปต่อ)

ส่วนที่เหลือตามสเปกเดิม (order-service, payment-service, SvelteKit frontend, `docker-compose.yml`) จะทำในรอบถัดๆ ไป
