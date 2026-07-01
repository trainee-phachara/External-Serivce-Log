# Learning Log: สร้าง external-service-log จาก 0

บันทึกนี้อธิบายว่าตอนเริ่มสร้าง `external-service-log` (logging microservice) จากโฟลเดอร์ว่างเปล่า เราทำอะไรไปตามลำดับ แต่ละไฟล์มีไว้ทำอะไร และระหว่างทางเจออุปสรรคอะไรบ้าง แก้ยังไง

---

## 1. เริ่มจาก 0: scaffold โครงสร้างโปรเจกต์

ก่อนเขียนโค้ดสักบรรทัด ต้องมี "โครง" ให้ TypeScript +   Jest ทำงานได้ก่อน ขั้นตอนคือ:

1. **สร้างโฟลเดอร์ตามหน้าที่ของโค้ด** (ไม่ใช่กองทุกอย่างไว้ที่เดียว)
   ```
   external-service-log/
   ├── src/
   │   ├── config/    # การเชื่อมต่อ infra (MongoDB)
   │   ├── buffer/    # in-memory queue
   │   ├── flusher/   # ตัวจับเวลา + insertMany
   │   ├── routes/    # HTTP layer (Express routes, validation)
   │   └── types/     # TypeScript type definitions ที่ใช้ร่วมกัน
   └── tests/
       ├── unit/         # ทดสอบ logic ทีละหน่วย ไม่แตะ network/DB จริง
       └── integration/  # ทดสอบ flow ผ่าน HTTP layer
   ```
   เหตุผลที่แยกแบบนี้: แต่ละโฟลเดอร์ตอบคำถาม "หน้าที่นี้อยู่ตรงไหน" ได้ทันที และทำให้เขียน unit test แยกตามโมดูลได้ง่าย

2. **`package.json`** — บอกว่าโปรเจกต์นี้ต้องการ dependency อะไรบ้าง (express, mongodb) และ dev dependency สำหรับ build/test (typescript, jest, ts-jest, supertest, ts-node-dev) พร้อม scripts (`dev`, `build`, `test`, `test:coverage`)

3. **`tsconfig.json`** — บอก TypeScript compiler ว่า:
   - `rootDir: "./src"`, `outDir: "./dist"` → คอมไพล์เฉพาะโค้ดใน `src` ออกไปที่ `dist` (ไม่เอา test ไปปนใน build จริง)
   - `strict: true` → บังคับเช็ค type เข้มงวด ช่วยจับบั๊กตั้งแต่ตอนเขียน

4. **`jest.config.js`** — ตั้งค่า Jest ให้ใช้ `ts-jest` (รัน TypeScript ตรงๆ โดยไม่ต้อง build ก่อน), บอกว่า test อยู่ที่ `tests/`, และตั้ง `coverageThreshold` ที่ 85% ตามเป้าหมายของโปรเจกต์ — การตั้ง threshold ไว้ใน config ทำให้ `npm run test:coverage` จะ "fail" ทันทีถ้า coverage ต่ำกว่ามาตรฐาน แทนที่จะต้องมานั่งเช็ค % เองทุกครั้ง

---

## 2. ไล่ทีละไฟล์: ทำไมถึงมีไฟล์นี้

### `src/types/log.ts`
นิยาม shape ของข้อมูลที่ใช้ทั้งระบบ:
- `LogEntry` = โครงสร้าง document ที่จะถูกเก็บลง MongoDB (ตรงกับ schema ที่กำหนดไว้)
- `IngestRequestBody` = โครงสร้างของ body ที่ service อื่น (order/user/payment) จะส่งเข้ามาทาง `POST /ingest`
- `BufferedLog` = สิ่งที่อยู่ใน buffer จริงๆ (ห่อ `entry` ไว้รอ flush)

**ทำไมต้องแยก `LogEntry` กับ `IngestRequestBody`?** เพราะ body ที่รับเข้ามาไม่จำเป็นต้องมีครบทุก field (เช่น `metadata`, `raw_payload`, `payload` เป็น optional) แต่ document ที่เก็บจริงต้องมีครบ — ฝั่ง ingest route จะเป็นคนเติม default value (`{}`) และ `timestamp` ให้

### `src/ingest/classify.ts` *(ถูกลบแล้วใน Phase 2)*
> ตอนเขียน section นี้ครั้งแรก ไฟล์อยู่ที่ `src/routes/classify.ts` — ย้ายมา `src/ingest/` ในรอบสอง (ดู 5.1)
> **หมายเหตุ:** ใน Phase 2 ได้รวม 3 collections เป็น collection เดียว (`service_logs`) แล้ว — classify ถูกลบออก เวลาอยากดู log เฉพาะ type ก็ query ด้วย field `type` แทน ดู [phase2-collection.MD](phase2-collection.MD) สำหรับรายละเอียด

เดิมไฟล์นี้ตอบคำถาม "log นี้ควรไปลง collection ไหนใน 3 ตัว (`api_logs`/`event_logs`/`error_logs`)?"
กฎที่ตกลงกันไว้:
- `http_status >= 400` → `error_logs` (ไม่ว่าจะเป็น request หรือ response)
- `type` เป็น `request`/`response` และ status ปกติ → `api_logs`
- นอกนั้น → `event_logs`

แยกเป็นไฟล์ของตัวเองเพราะ "กฎการแยกประเภท" เป็น business logic ที่อาจเปลี่ยนได้บ่อย การแยกออกมาทำให้ทดสอบ และแก้ไขกฎได้โดยไม่กระทบโค้ดส่วนอื่น

### `src/buffer/logBuffer.ts`
in-memory array queue ง่ายๆ ที่มี 4 หน้าที่: `push` (ใส่ log), `size`/`isEmpty` (เช็คสถานะ), `drain` (ดึงทั้งหมดออกมาแล้วเคลียร์ array) — `drain` ออกแบบให้ atomic (สลับ array ใหม่ทันที) งนี้คือ

### `src/config/mongo.ts`
จัดการทุกอย่างที่เกี่ยวกับ MongoDB:
- `connectMongo()` → เชื่อมต่อ + เรียก `ensureTimeSeriesCollections()`
- `ensureTimeSeriesCollections()` → เช็คว่ามี collection `service_logs` หรือยัง ถ้ายังไม่มีก็สร้างแบบ Time Series (`timeField: timestamp`, `metaField: source`, TTL 30 วัน = 2,592,000 วินาที) *(เดิมสร้าง 3 collections แยกกัน เปลี่ยนเป็น collection เดียวใน Phase 2)*
- `insertLogs()` → รับ log ที่ดึงจาก buffer มา แล้วเรียก `insertMany` เข้า `service_logs` collection เดียว

### `src/flusher/batchFlusher.ts`
หัวใจของระบบ batching มี trigger สองทาง ตามที่สเปกกำหนด:
1. **Ticker** — `setInterval` ทุก `intervalMs` (default 5 วิ) เรียก `flush()`
2. **Size trigger** — ทุกครั้งที่มีการ push log ใหม่ ingest route จะเรียก `onLogPushed()` ซึ่งเช็คว่า `buffer.size() >= maxSize` (default 100) ถ้าใช่ก็ flush ทันที โดยไม่ต้องรอ ticker

มี mechanism กันปัญหา **race condition**: ถ้า flush ถูกเรียกซ้อนกัน (เช่น ticker ทำงานพร้อมกับ size trigger) `insertMany` สองชุดอาจรันพร้อมกันและดึงข้อมูลปนกัน จึงใช้ promise chain (`this.flushing = this.flushing.then(() => this.doFlush())`) บังคับให้ flush แต่ละรอบรอคิวต่อกันเสมอ ไม่มีทางรันซ้อน

### `src/ingest/validateIngest.ts`
เช็คว่า body ที่ส่งเข้ามาทาง `POST /ingest` ครบถ้วนถูกต้องไหมก่อนเอาเข้า buffer — เช็คว่า field บังคับ (`source.app_name`, `source.service_name`, `trace_id`, `endpoint`, `http_status`, `type`, `direction`) มีและเป็น string ที่ไม่ว่าง, `type`/`direction` ต้องอยู่ใน enum ที่กำหนด, ส่วน `metadata`/`raw_payload`/`payload` เป็น optional แต่ถ้าใส่มาต้องเป็น object

แยกออกมาจาก route handler เพราะ validation logic มักจะซับซ้อนขึ้นเรื่อยๆ การแยกทำให้ทดสอบกฎ validation ได้ตรงๆ โดยไม่ต้องยิง HTTP request จริง

### `src/routes/ingest.ts`
ตัว HTTP handler ของ `POST /ingest` — ผูกทุกอย่างเข้าด้วยกัน: เรียก validate → ถ้าไม่ผ่านตอบ `400` พร้อม error list → ถ้าผ่านก็สร้าง `LogEntry` (เติม `timestamp` และ default ให้ field ที่ optional) → push เข้า buffer → เรียก `flusher.onLogPushed()` → ตอบ `202 Accepted`

### `src/app.ts` และ `src/index.ts`
- `app.ts` = factory function สร้าง Express app โดยรับ `buffer`/`flusher` จากภายนอก (dependency injection) — ทำให้ test เขียน app instance ใหม่ที่ผูกกับ mock ของตัวเองได้ ไม่ต้องพึ่ง global state
- `index.ts` = entrypoint จริงตอนรันโปรดักชัน: อ่าน config จาก env, ต่อ MongoDB, สร้าง buffer/flusher/app, เปิด server, และ handle graceful shutdown (`SIGINT`/`SIGTERM` → flush ของที่ค้างอยู่ก่อนปิด)

### ไฟล์ test (`tests/unit/*.test.ts`, `tests/integration/ingest.test.ts`)
- **Unit tests** ทดสอบแต่ละโมดูลแบบแยกขาด ไม่พึ่ง MongoDB/HTTP จริง: `logBuffer` (push/drain ทำงานถูกไหม), `batchFlusher` (size trigger, ticker trigger, ไม่ flush ซ้อนกัน), `validateIngest` (เคส valid/invalid ต่างๆ)
- **Integration test** (`ingest.test.ts`) ยิง HTTP request ผ่าน `supertest` เข้า Express app จริง (แต่ผูกกับ mock buffer/flusher) เพื่อเช็ค flow ทั้งสาย: validate → 202/400 → log เข้า buffer → trigger flush ตอน buffer เต็มไหม

---

## 3. อุปสรรคที่เจอ และวิธีแก้

### 3.1 เดา schema เกินกว่าที่มีให้
ตอนแรกผมเขียน `IngestRequestBody` โดยเติม field `collection: CollectionName` เข้าไปเอง (ให้ caller ระบุปลายทางมาตรงๆ) — เป็นการเดาที่ไม่มีอะไรในสเปกรองรับ คุณทักว่า **"คุณรู้ได้ไงว่า log schema ต้องมีไรบ้าง"**

**วิธีแก้**: หยุดเขียนโค้ดต่อ แล้วถามกลับให้ชัดว่า "ใครเป็นคนตัดสินใจว่า log จะไปลง collection ไหน" แทนที่จะเดาเอง สุดท้ายตกลงกันว่าให้ service เป็นคนวิเคราะห์เอง (`classify.ts`) จาก `http_status`/`type` — บทเรียนคือ **เมื่อสเปกไม่ครอบคลุมจุดตัดสินใจสำคัญ ให้ถามก่อนเขียน ไม่ใช่เดาแล้วเขียนทับ**

> **หมายเหตุ Phase 2:** ท้ายสุดแล้วการ classify ถูกลบออกทั้งหมด — เปลี่ยนเป็นเก็บ log ทุกประเภทใน collection เดียว (`service_logs`) แล้ว query ด้วย `type` field แทน ดู [phase2-collection.MD](phase2-collection.MD)

### 3.2 `command not found: npm`
พอจะรัน `npm install` ครั้งแรก shell ใหม่ที่เปิดมาไม่มี `npm`/`node` ในเชื่อมต่อ — เพราะ `nvm` (ตัวจัดการเวอร์ชัน Node) ยังไม่ถูก source เข้า shell session นี้

**วิธีแก้**: รัน
```bash
export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"
nvm use v24.16.0
```
ก่อนทุกคำสั่งที่ต้องใช้ `npm`/`npx`/`node` ในเชลล์นั้น (เพราะ working directory คงอยู่ระหว่างคำสั่ง แต่ environment ของแต่ละ shell session ไม่ได้สืบทอดอัตโนมัติ)

### 3.3 Unit test ของ `BatchFlusher` ตกเพราะเข้าใจ async/microtask ผิด
รัน `jest --coverage` ครั้งแรกพบว่า test 2 ตัวตก:

**(ก) "does not schedule a second ticker when started twice"**
เช็ค `expect(insert).toHaveBeenCalledTimes(1)` ทันทีหลัง `jest.advanceTimersByTime(5000)` — แต่ `insert` ยังไม่ถูกเรียก (`Received: 0`)

สาเหตุ: โค้ดใน `flush()` คือ `this.flushing = this.flushing.then(() => this.doFlush())` — `.then()` callback ถูก "เลื่อนออกไปเป็น microtask" ไม่ได้รันทันทีแบบ synchronous การ `advanceTimersByTime` แค่ทำให้ `setInterval` callback ทำงาน (ซึ่งเรียก `flush()` แบบ fire-and-forget) แต่ตัว `doFlush`/`insert` จริงๆ ยังรอคิว microtask อยู่

**วิธีแก้**: เติม `await flusher.flush()` ก่อนเช็ค assertion — การ `await` บังคับให้ event loop เคลียร์ microtask queue จนกว่า promise chain จะ resolve จริง ค่อยเช็คผลลัพธ์

**(ข) "serializes concurrent flush calls so inserts do not overlap"**
คาดหวังว่าถ้าเรียก `flush()` สองครั้งติดกัน (คั่นด้วย `buffer.push`) จะได้ `insert` ถูกเรียก 2 ครั้งแยกกัน — แต่จริงๆ ถูกเรียกแค่ 1 ครั้ง (พร้อมข้อมูลรวมกันทั้ง 2 log)

สาเหตุ: เพราะ `doFlush` ถูกเลื่อนเป็น microtask (ดูข้อ ก) โค้ด synchronous ทั้งหมด (push ครั้งที่ 1 → flush → push ครั้งที่ 2 → flush) รันจบก่อนที่ `doFlush` รอบแรกจะเริ่มทำงานจริง พอมันเริ่ม `drain()` จึงเจอ log ทั้งสองตัวรออยู่แล้ว เลย insert พร้อมกันเป็นชุดเดียว — **นี่ไม่ใช่บั๊ก** เป็นพฤติกรรมที่ถูกต้อง (และดีกว่าด้วยซ้ำ เพราะลดจำนวนครั้งที่ insert ลง MongoDB)

**วิธีแก้**: ปรับ test ให้ตรงกับสิ่งที่อยากพิสูจน์จริงๆ คือ "การ flush จะไม่มีวันรันซ้อนกัน (overlap)" — เปลี่ยนมาเช็คว่า `activeInserts` ไม่เกิน 1 ตลอดเวลา และเช็คว่าสุดท้ายแล้ว log ทั้ง 3 ตัวถูก insert ครบ (ไม่สนว่าจะถูกแบ่งเป็นกี่ batch) แทนที่จะ assert จำนวนครั้งที่เรียก `insert` ตรงๆ ซึ่งขึ้นกับ timing ที่ควบคุมได้ยาก

บทเรียน: เวลาทดสอบโค้ด async/promise-chaining อย่า assert บนสมมติฐานเรื่อง "ลำดับ/จังหวะเวลา" ที่เปราะบาง — ให้ assert บน **สิ่งที่ guarantee จริงๆ** (เช่น "ไม่มีการรันซ้อน", "ข้อมูลครบ") แทน

### 3.4 IDE ฟ้อง "Cannot find name 'describe'/'jest'/'beforeEach'" ในไฟล์ test
หลังแก้ test เสร็จ IDE ขึ้น diagnostic error ว่าหาชื่อ `describe`, `jest`, `beforeEach` ไม่เจอในไฟล์ `tests/unit/batchFlusher.test.ts` — ทั้งที่ `npx jest` รันผ่านปกติ

สาเหตุ: `tsconfig.json` หลักตั้ง `"include": ["src/**/*"]` และ `"exclude": ["tests"]` (ตั้งใจให้ build จริงไม่รวม test code) ผลคือไฟล์ใน `tests/` ไม่ได้อยู่ใน "โปรแกรม" ที่ TypeScript language server ของ IDE มองเห็น เลยไม่รู้จัก type ของ Jest globals (`@types/jest` ที่ลงไว้แล้วใน devDependencies) — เป็นปัญหาแค่ฝั่ง editor/IDE เท่านั้น ไม่กระทบการรัน test จริง (เพราะ `ts-jest` คอมไพล์ไฟล์ทีละไฟล์โดยไม่สนเรื่อง `include`/`exclude` ของ build)

**วิธีแก้ (รอบแรก ยังไม่สมบูรณ์)**: สร้าง `tests/tsconfig.json` แยกต่างหาก ที่:
- `extends` จาก tsconfig หลัก (ใช้ค่าพื้นฐานร่วมกัน)
- เพิ่ม `"types": ["jest", "node"]` ให้รู้จัก global ของ Jest/Node
- ปรับ `"rootDir": ".."` และ `"include"` ให้ครอบคลุมทั้ง `tests/` และ `../src/`
- ตั้ง `"noEmit": true` (ไฟล์นี้มีไว้ให้ IDE เช็ค type เท่านั้น ไม่ได้ใช้ build จริง)

TypeScript/IDE จะหา `tsconfig.json` ที่ใกล้ที่สุดจากตำแหน่งไฟล์ที่เปิดอยู่ก่อนเสมอ — เลยควรจะเจอ `tests/tsconfig.json` ก่อน `tsconfig.json` หลัก

**แต่ไฟล์ `batchFlusher.test.ts` ก็ยังขึ้นแดงอยู่ดี** ทั้งที่สร้าง config แยกแล้ว — ไปเช็คด้วย `tsc -p tests/tsconfig.json --listFilesOnly` พบว่าไฟล์ test **ไม่ได้อยู่ในโปรแกรมเลยสักไฟล์เดียว**! ใช้ `--showConfig` ขุดดูค่า `exclude` ที่ resolve จริงแล้วเจอว่ากลายเป็น:
```json
"exclude": ["../node_modules", "../dist", "../tests"]
```

ตัวการคือค่า `"exclude": ["node_modules", "dist", "tests"]` ที่สืบทอดมาจาก tsconfig หลัก (ผ่าน `extends`) — TypeScript เอาพาธพวกนี้ไป **resolve ใหม่โดยอิงตำแหน่งของไฟล์ลูกที่ extends** ไม่ใช่ตำแหน่งไฟล์แม่ที่เป็นเจ้าของค่าเดิม ผลคือ `"tests"` (ที่แม่ตั้งใจหมายถึง `external-service-log/tests`) กลายเป็น `"../tests"` เมื่อมองจาก `tests/tsconfig.json` ซึ่ง resolve ออกมาเป็น `external-service-log/tests` พอดิบพอดี — เท่ากับ **exclude โฟลเดอร์ tests ทั้งโฟลเดอร์ ทับ config ที่เราเพิ่งสร้างมาเพื่อรวมมันเข้าไป!** เป็นกับดักที่มองด้วยตาเปล่าจาก `tests/tsconfig.json` แล้วไม่เห็นเลยว่ามีปัญหา เพราะค่า `exclude` ไม่ได้ถูกเขียนซ้ำในไฟล์นั้นตรงๆ (มันแอบมาจากแม่)

**วิธีแก้ที่ถูกต้อง**: override `"exclude"` ในไฟล์ลูกตรงๆ ไม่ให้รวม `tests`:
```json
"exclude": ["../node_modules", "../dist"]
```
หลังแก้ `tsc -p tests/tsconfig.json --listFilesOnly` แสดงไฟล์ test ครบทุกไฟล์ และ `--noEmit` ไม่ฟ้อง error อีกต่อไป

บทเรียน: เวลาใช้ `extends` ใน tsconfig **ค่า path-based อย่าง `include`/`exclude`/`rootDir`/`outDir` จะถูก resolve ใหม่จากตำแหน่งของไฟล์ที่ extend ไม่ใช่ไฟล์ต้นทาง** ถ้าไม่อยากให้ค่าที่สืบทอดมาทำงานผิดเพี้ยน ต้อง override มันตรงๆ ในไฟล์ลูก อย่าคิดว่า "ไม่ได้เขียนอะไรเพิ่ม = ปลอดภัย" — และเวลา debug ปัญหาแบบนี้ `tsc -p <config> --showConfig` คือเครื่องมือที่ช่วยให้เห็น "ค่าจริงที่ TypeScript ใช้" หลังรวม `extends` ทั้งหมดแล้ว ซึ่งช่วยขุดเจอจุดผิดได้ตรงจุดกว่าการเดา

---

## 4. สรุปผลลัพธ์ (รอบแรก: HTTP ingest pipeline)

- เทส 32 เคส ผ่านหมด (`Tests: 32 passed, 32 total`)
- coverage รวม **98.88%** (เกินเป้า 85% ที่ตั้งไว้)
- โครงสร้างไฟล์ทุกตัวมีหน้าที่ชัดเจน แยกตามความรับผิดชอบ (separation of concerns) ทำให้ทดสอบและแก้ไขทีหลังได้ง่าย

---

## 5. รอบสอง: เพิ่ม gRPC เข้าไปใน Ingest API (ตามสถาปัตยกรรมเป้าหมาย)

หลังจากดูรูป diagram สถาปัตยกรรมเป้าหมายที่คุณส่งมา พบว่า Ingest API ควรรับได้ทั้ง **gRPC และ HTTP** (ของเดิมมีแค่ HTTP) งานรอบนี้คือเติมส่วนที่ขาดให้ตรงกับรูป โดยไม่กระทบของเดิม

### 5.1 ทำไมต้อง refactor ก่อนเพิ่ม gRPC

ถ้าเขียน gRPC handler แยกไปอีกชุดนึงตรงๆ จะเกิดปัญหา **โค้ดซ้ำ**: logic การ validate → จัดประเภท collection → push เข้า buffer → trigger flush ต้องเหมือนกันทุกตัวอักษรไม่ว่าจะเข้ามาทาง HTTP หรือ gRPC (ไม่งั้น behavior สองทางจะไม่ตรงกัน) จึงต้อง **แยกแกนกลางที่ไม่ผูกกับ transport ใดๆ ออกมาก่อน**:

- ย้าย `validateIngest.ts` จาก `routes/` ไป `ingest/` — เพราะจริงๆ แล้วไฟล์นี้ไม่เคยแตะ Express เลยตั้งแต่แรก (รับ-คืนค่าเป็น plain object ล้วนๆ) การอยู่ใน `routes/` เป็นแค่ที่อยู่ที่ "สะดวกตอนแรก" ไม่ใช่ที่ที่ "ถูกต้องตามหน้าที่"
- สร้าง [src/ingest/processIngest.ts](../external-service-log/src/ingest/processIngest.ts) — ฟังก์ชันเดียวที่ห่อ flow ทั้งหมด (`validate → build entry → push → trigger flush`) คืนค่า `{ accepted: boolean, errors: string[] }` แบบเดียวกันไม่ว่าใครจะเรียก
- ผลคือ [src/routes/ingest.ts](../external-service-log/src/routes/ingest.ts) เหลือแค่ "ตัวแปลภาษา" บางๆ — รับ `req.body` มา เรียก `processIngest` แล้วแปลผลลัพธ์เป็น HTTP `202`/`400` เท่านั้น ไม่มี business logic อยู่ในนั้นแล้ว

**บทพิสูจน์ว่า refactor ไม่พังของเดิม**: รัน `tests/integration/ingest.test.ts` (เทส HTTP เดิมที่เขียนไว้ตั้งแต่รอบแรก) **ผ่านหมดโดยไม่ต้องแก้แม้แต่บรรทัดเดียว** — เพราะภายนอกมองเห็น behavior เหมือนเดิมทุกประการ นี่คือเหตุผลที่ควรมี integration test คลุม "พฤติกรรมที่สังเกตได้จากภายนอก" ไว้ก่อน แล้วค่อย refactor ภายใน — ถ้าเทสยังผ่านเหมือนเดิมแปลว่ารื้อโครงสร้างได้โดยไม่ทำพฤติกรรมเพี้ยน

### 5.2 ไฟล์ใหม่ที่เพิ่มเข้ามา และทำไมถึงออกแบบแบบนี้

**[src/ingest/processIngest.ts](../external-service-log/src/ingest/processIngest.ts)**
แกนกลางที่ใช้ร่วมกันทั้ง HTTP และ gRPC ตามที่อธิบายข้างบน — รับ `rawBody: unknown` (ไม่สนว่ามาจาก transport ไหน) คืนผลลัพธ์เป็น plain object ล้วนๆ ไม่มีอะไรที่ผูกกับ Express หรือ gRPC เจือปน

**[src/grpc/ingest.proto](../external-service-log/src/grpc/ingest.proto)**
"สัญญา" (contract) ของ gRPC service — นิยามว่า `IngestService.Ingest` รับอะไรเข้ามา (`IngestRequest`) และคืนอะไรออกไป (`IngestResponse`) จุดที่ต้องตัดสินใจคือ: `metadata`/`raw_payload`/`payload` เป็น object ที่หน้าตาไม่แน่นอน (arbitrary JSON) จะแทนด้วยอะไรใน protobuf?
- ทางเลือกที่ "ถูกต้องตามตำรา" คือ `google.protobuf.Struct` แต่ต้องพึ่งการ resolve well-known types ผ่าน `@grpc/proto-loader` ซึ่งมีจุดที่อาจพังได้มากกว่า
- เลยเลือกทางที่ **ง่ายและพังยาก**: ส่งเป็น **string ที่เข้ารหัส JSON ไว้แล้ว** (`metadata_json`, `raw_payload_json`, `payload_json`) แล้วฝั่ง server ค่อย `JSON.parse` — ไม่ต้องพึ่ง dependency เพิ่ม และเทสง่ายกว่ามาก (ส่ง string ธรรมดาในเทสได้เลย)

**[src/grpc/server.ts](../external-service-log/src/grpc/server.ts)**
ตัว gRPC server จริง หน้าที่หลักคือ "แปลภาษา" จาก proto message ให้เป็น `IngestRequestBody` ที่ `processIngest` เข้าใจ:
1. `JSON.parse` ทั้ง 3 ฟิลด์ที่เป็น JSON string (ดักจับ error ถ้า parse ไม่ผ่าน หรือ parse ได้แต่ไม่ใช่ object)
2. ประกอบร่างเป็น `IngestRequestBody` แล้วส่งต่อให้ `processIngest`
3. ส่งผลลัพธ์กลับเป็น `{ accepted, errors }` เสมอ — **ไม่ใช้ gRPC error status สำหรับ validation ที่ไม่ผ่าน** เพราะมันคือ "request เข้ามาถึงสำเร็จ แต่ข้อมูลไม่ผ่านเงื่อนไข" ไม่ใช่ "transport พัง" — สอดคล้องกับฝั่ง HTTP ที่ตอบ `400` พร้อม body ของ error แทนที่จะ throw exception

**ปรับ [src/index.ts](../external-service-log/src/index.ts)**
เพิ่มการ start gRPC server (`createGrpcServer(...).bindAsync(...)`) คู่ขนานไปกับ Express, เพิ่ม env var `GRPC_PORT` (default `50051`), และรวม `grpcServer.tryShutdown(...)` เข้าไปใน graceful shutdown sequence เดิม

### 5.3 อุปสรรคที่เจอในรอบนี้

**เขียน defensive code ป้องกันสถานการณ์ที่ไม่มีทางเกิดขึ้นจริง**
ตอนเขียน `toIngestRequestBody` ในตอนแรก ผมใส่ fallback `?? ''` ไว้ทุก field string (เช่น `request.trace_id ?? ''`) เผื่อกรณีที่ field เป็น `undefined` พอรัน coverage report พบว่า branch พวกนี้ **ไม่เคยถูกเทสแตะเลย** (`server.ts` เหลือ branch coverage แค่ 76% จาก 100% ที่อื่น)

เหตุผลที่เทสไปไม่ถึง branch นั้นเพราะ: ตอน config `protoLoader.loadSync(..., { defaults: true })` — flag `defaults: true` รับประกันว่า proto-loader จะเติมค่า default (`''` สำหรับ string) ให้ทุก field ที่ไม่ได้ถูกส่งมาเสมอ พูดอีกแบบคือ `request.trace_id` **ไม่มีทางเป็น `undefined`** ได้เลยตราบใดที่ใช้ loader config นี้ — โค้ด `?? ''` จึงเป็นการป้องกันสิ่งที่ระบบรับประกันไว้แล้วว่าไม่เกิดขึ้น

**วิธีแก้**: ลบ `?? ''` ออกจาก field ที่ proto-loader รับประกันว่าเป็น string เสมอ (`trace_id`, `endpoint`, `http_status`, `type`, `direction`) แล้วปรับ type ของ `GrpcIngestRequest` ให้สะท้อนความจริงนี้ (เปลี่ยนจาก `trace_id?: string` เป็น `trace_id: string`) — **ส่วน `source` ยังคงเก็บ `?.` ไว้** เพราะ nested message field ใน proto3 มีโอกาสเป็น `null`/ไม่ถูกตั้งค่าจริงๆ (เป็นคนละสถานการณ์กับ string field ธรรมดา)

ผลคือ branch coverage ของ `server.ts` ขยับจาก 76% → **100%** ทันที โดยไม่ต้องเพิ่ม test เลยสักเคส — เพราะโค้ดที่ลบไปคือโค้ดที่ "ไม่มีทางถูกใช้งานจริง" ตั้งแต่แรก

บทเรียน (ย้ำจากรอบแรกแต่เจอในมุมใหม่): **อย่าเขียน fallback/defensive code สำหรับสถานการณ์ที่ฝั่ง dependency รับประกันไว้แล้วว่าไม่เกิด** — นอกจากจะเป็นโค้ดส่วนเกินแล้ว มันยังโผล่เป็น "branch ที่ทดสอบไม่ได้" ใน coverage report ซึ่งเป็นสัญญาณเตือนที่ดีว่ามีโค้ดที่ไม่ควรมีอยู่ Coverage report ไม่ได้มีไว้แค่ "เช็คว่าครบ %" แต่ใช้เป็นเครื่องมือ **หาโค้ดส่วนเกินที่ไม่จำเป็น** ได้ด้วย

---

## 6. สรุปผลลัพธ์ล่าสุด (หลังเพิ่ม gRPC)

- เทสรวม **42 เคส** ผ่านหมด (เพิ่มจาก 32 → 42 ด้วย `processIngest.test.ts` + `grpcIngest.test.ts`)
- coverage **100%** ทุกมิติ (statements / branches / functions / lines) — ขยับจาก 98.88% เป็น 100% เพราะตัดโค้ดส่วนเกินที่อธิบายไว้ในข้อ 5.3 ออก
- `tests/integration/ingest.test.ts` (เทส HTTP เดิม) ผ่านโดยไม่แก้แม้แต่บรรทัดเดียว ยืนยันว่า refactor ไม่ทำพฤติกรรมเดิมพัง
- ตอนนี้ Ingest API รองรับทั้ง **HTTP** (`POST /ingest` พอร์ต 3000) และ **gRPC** (`IngestService.Ingest` พอร์ต 50051) ตรงตามสถาปัตยกรรมเป้าหมายในรูป ส่วนที่เหลือ (MongoDB sharded cluster ผ่าน mongos router) จะไปทำตอนเขียน `docker-compose.yml` เพราะเป็นเรื่อง infrastructure ล้วนๆ ไม่กระทบโค้ดแอป
