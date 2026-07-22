# Log Archiver — คู่มืออธิบายระบบแบบละเอียด

## Changelog

| เวอร์ชัน | การเปลี่ยนแปลง |
|---|---|
| v1 | สร้าง archiver ครั้งแรก — TTL, archive logic, 6 tests, Dockerfile, docker-compose |
| v2 | แก้ 2 จุดหลังจาก code review: (1) เพิ่ม coverage ให้ `New()` และ (2) เพิ่ม context ใน error ของ `insertBatch` |

### รายละเอียดที่แก้ใน v2

**1. `New()` ไม่ถูก test**

`New()` คือ public function ที่ caller ข้างนอกต้องใช้สร้าง Archiver แต่ tests ทุกตัวเขียน `&Archiver{...}` โดยตรง ทำให้ `New` มี coverage 0%

แก้โดยเปลี่ยน `TestRun_NoLogs_DoesNothing` ให้เรียก `New(db)` แทน:

```go
// ก่อนแก้
a := &Archiver{db: db, now: time.Now}

// หลังแก้ — test ผ่าน New() เหมือน caller จริง
a := New(db)
```

**2. `insertBatch` error ไม่มี context**

เมื่อ `InsertMany` ล้มเหลวใน production จะเห็น error แค่ว่า MongoDB บอกอะไร แต่ไม่รู้ว่าพังที่ขั้นตอนไหนของ archiver

แก้โดยห่อ error ด้วย context:

```go
// ก่อนแก้
return err

// หลังแก้ — log จะบอกทันทีว่าพังที่ insert batch
return fmt.Errorf("insert batch: %w", err)
```

## ปัญหาที่ต้องแก้

ระบบของเราเก็บ log ทุกอย่างไว้ใน MongoDB collection ชื่อ `service_logs`

ก่อนหน้านี้เรากำหนด TTL (Time To Live) ไว้ที่ **30 วัน** หมายความว่า MongoDB จะ **ลบ log ที่เก่ากว่า 30 วันทิ้งโดยอัตโนมัติ**

```
วันที่ 1  log เข้า service_logs
          ...
วันที่ 30 log ยังอยู่
วันที่ 31 MongoDB ลบทิ้ง ❌
```

**ปัญหา:** ถ้าจะย้อนดู log เดือนที่แล้ว → หายหมดแล้ว

---

## วิธีแก้

แทนที่จะลบทิ้ง ให้ **ย้ายไปเก็บก่อน** แล้วค่อยปล่อยให้ลบ

```
วันที่ 1  log เข้า service_logs (hot)
          ...
วันที่ 30 archiver รันทุกคืน → ย้าย log อายุ > 30 วัน
          ไปเก็บใน service_logs_archive_2026_06
วันที่ 40 MongoDB ลบออกจาก hot collection (TTL)
          แต่ยังอยู่ใน archive ตลอดไป ✅
```

---

## MongoDB Collections ที่มีในระบบ

| Collection | หน้าที่ | TTL |
|---|---|---|
| `service_logs` | เก็บ log ใหม่ (hot) — query เร็ว | 40 วัน |
| `service_logs_archive_2026_07` | log เดือน ก.ค. 2026 (cold) | ไม่มี (เก็บตลอด) |
| `service_logs_archive_2026_08` | log เดือน ส.ค. 2026 (cold) | ไม่มี (เก็บตลอด) |
| `archive_state` | จดว่าวันไหนย้ายไปแล้ว | ไม่มี |

**ทำไม archive แยกตามเดือน?** — ถ้าวันหนึ่งอยากลบ log ทั้งเดือน ลบ collection ทั้ง collection ได้เลย เร็วกว่าลบทีละ document มาก

---

## ทำไม TTL ต้องเปลี่ยนจาก 30 เป็น 40 วัน

สมมติ archiver ค้างหรือ crash ไป 3 วัน:

```
TTL = 30 วัน:
  log อายุ 30 วัน → archiver ยังไม่ได้ย้าย → MongoDB ลบทิ้ง ❌ ข้อมูลหาย!

TTL = 40 วัน:
  log อายุ 30 วัน → archiver ยังไม่ได้ย้าย → MongoDB ยังไม่ลบ
  log อายุ 33 วัน → archiver กลับมารันใหม่ → ย้ายได้ทัน ✅
  log อายุ 40 วัน → MongoDB ลบ (แต่ย้ายไปแล้ว)
```

10 วันที่เพิ่มมาคือ **เวลาเผื่อฉุกเฉิน**

---

## โครงสร้างไฟล์ที่สร้างใหม่

```
services/external-service-log/
├── cmd/
│   ├── server/main.go          ← เดิม (HTTP + gRPC server)
│   └── archiver/main.go        ← ใหม่ (รันครั้งเดียวแล้วจบ)
│
└── internal/
    └── archiver/
        ├── archiver.go         ← ใหม่ (logic ทั้งหมด)
        └── archiver_test.go    ← ใหม่ (tests)
```

---

## การทำงานของ Archiver ทีละขั้นตอน

### ภาพรวม

```
archiver รัน 1 ครั้ง
    │
    ▼
[1] หาวันที่ยังไม่ได้ archive
    │
    ▼
[2] วนทำทีละวัน (เก่าสุดก่อน)
    │
    ├── [3] สร้าง archive collection ของเดือนนั้น (ถ้ายังไม่มี)
    │
    ├── [4] ลบข้อมูลเก่าใน archive ของวันนั้น (กันซ้ำ)
    │
    ├── [5] copy log จาก hot → archive เป็น batch
    │
    ├── [6] นับว่า copy ครบไหม
    │
    └── [7] จด "วันนี้เสร็จแล้ว" ใน archive_state
```

---

### ขั้นตอนที่ 1 — หาวันที่ยังไม่ได้ archive

```go
// cutoff = วันนี้ - 30 วัน
cutoff := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -30)

// หา log เก่าสุดใน hot collection
// เพื่อรู้ว่าต้องเริ่มนับตั้งแต่วันไหน
earliest := db.Collection("service_logs").FindOne(
    sort: {"timestamp": 1}
)

// วนจาก earliest ถึง cutoff
// เช็คว่าวันไหน "ยังไม่มีใน archive_state"
for d := startDay; d.Before(cutoff); d = d.AddDate(0, 0, 1) {
    if !done[d.Format("2006-01-02")] {
        pending = append(pending, d)
    }
}
```

**ตัวอย่าง:** วันนี้คือ 20 ก.ค. → cutoff คือ 20 มิ.ย.
Log ที่มีอายุ 19 มิ.ย. ลงไป = pending

---

### ขั้นตอนที่ 2 — เตือนถ้าช้าเกินไป

```go
ageDays := time.Since(pending[0]).Hours() / 24
if ageDays > 35 {
    log.Printf("WARNING: log อายุ %.0f วัน ใกล้ TTL 40 วันแล้ว!", ageDays)
}
```

ถ้า archiver ไม่ได้รันนาน 35 วัน → warning ออกมาใน log ทันที

---

### ขั้นตอนที่ 3 — สร้าง archive collection (ถ้ายังไม่มี)

```go
// ตรวจก่อนว่า collection มีอยู่แล้วไหม
names := db.ListCollectionNames({"name": "service_logs_archive_2026_06"})

// ถ้าไม่มี → สร้างใหม่แบบ time-series + zstd compression
db.RunCommand({
    create: "service_logs_archive_2026_06",
    timeseries: {timeField: "timestamp", metaField: "source"},
    storageEngine: {wiredTiger: {configString: "block_compressor=zstd"}}
})
```

**zstd คืออะไร?** — algorithm บีบอัดข้อมูล ทำให้ archive collection ใช้พื้นที่ disk น้อยลงประมาณ 50-70% เหมาะกับข้อมูลเก่าที่อ่านบ่อยๆ

---

### ขั้นตอนที่ 4 — Idempotency (ลบซ้ำก่อน insert)

```go
// ลบข้อมูลของวันนั้นออกจาก archive ก่อน
archiveColl.DeleteMany({
    timestamp: {$gte: dayStart, $lt: dayEnd}
})
```

**ทำไมต้องลบก่อน?** — ถ้า archiver crash ระหว่าง copy ครั้งที่แล้ว จะมีข้อมูลค้างอยู่ใน archive ครึ่งๆ กลางๆ การลบก่อนแล้ว copy ใหม่ทำให้ผลลัพธ์ถูกเสมอ ไม่ว่าจะรันกี่ครั้ง

คำนี้ในโลก software เรียกว่า **Idempotent** = รันซ้ำกี่ครั้งก็ได้ผลเหมือนกัน

---

### ขั้นตอนที่ 5 — Copy เป็น Batch

```go
// เปิด cursor อ่านทีละ document (ไม่โหลดทั้งวันขึ้นมา RAM พร้อมกัน)
cursor := db.Collection("service_logs").Find({
    timestamp: {$gte: dayStart, $lt: dayEnd}
})

batch := []LogEntry{}
for cursor.Next() {
    entry := cursor.Decode()
    batch = append(batch, entry)

    if len(batch) >= 5000 {
        archiveColl.InsertMany(batch)  // insert ทีละ 5000 รายการ
        batch = batch[:0]              // เคลียร์ batch
    }
}
// insert batch สุดท้ายที่อาจเหลืออยู่
archiveColl.InsertMany(batch)
```

**ทำไม batch 5000?** — ถ้า insert ทีละรายการจะช้ามาก ถ้า insert ทั้งวันพร้อมกัน RAM อาจไม่พอ batch 5000 คือจุดสมดุล

---

### ขั้นตอนที่ 6 — ตรวจนับ

```go
hotCount     := db.Collection("service_logs").CountDocuments(timeRange)
archiveCount := archiveColl.CountDocuments(timeRange)

if archiveCount != hotCount {
    // ไม่ mark done — วันนี้จะยังอยู่ใน pending และรอบถัดไปจะ retry
    return error("count mismatch: hot=%d archive=%d", hotCount, archiveCount)
}
```

ถ้า copy ไม่ครบ → ไม่ mark ว่าเสร็จ → รอบถัดไปจะ retry อัตโนมัติ

---

### ขั้นตอนที่ 7 — จด Status

```go
// บันทึกลง archive_state collection
{
    "_id":        "2026-06-19",   // วันที่
    "status":     "done",
    "count":      1542,           // จำนวน log ที่ย้าย
    "archivedAt": "2026-07-20T...",
    "durationMs": 3200            // ใช้เวลา 3.2 วินาที
}
```

ครั้งต่อไปที่ archiver รัน เห็น `"2026-06-19": done` → ข้ามวันนี้ไป

---

## วิธีที่ Docker Compose รัน Archiver

```yaml
archiver:
  build: ./services/external-service-log   # ใช้ image เดียวกับ server
  command: ["sh", "-c", "while true; do ./archiver; sleep 86400; done"]
  # รัน archiver → รอ 24 ชั่วโมง → รันอีกครั้ง → วนไป
```

`86400` คือจำนวนวินาทีใน 1 วัน (60 × 60 × 24 = 86400)

**ทำไม archiver binary รันครั้งเดียวแล้วจบ?** — แยก logic "ทำงาน" ออกจาก "กำหนดตาราง" เพื่อให้ยืดหยุ่น วันหนึ่งถ้าย้ายไปใช้ Kubernetes สามารถเปลี่ยนเป็น CronJob ได้ทันทีโดยไม่ต้องแก้โค้ด

---

## วิธีรัน

### รันพร้อมกับระบบทั้งหมด

```sh
docker compose up --build
```

archiver จะรันอัตโนมัติทุก 24 ชั่วโมง

### รัน archiver ด้วยมือ (สำหรับทดสอบหรือ backfill)

```sh
docker compose run --rm archiver
```

### รัน local (ต้องมี MongoDB รันอยู่)

```sh
cd services/external-service-log
go run ./cmd/archiver
```

---

## วิธีรัน Test

```sh
cd services/external-service-log
go test ./internal/archiver/...
```

Test แต่ละตัวทำอะไร:

| Test | ตรวจสอบว่า |
|---|---|
| `TestRun_NoLogs_DoesNothing` | ถ้าไม่มี log เลย → ไม่สร้าง archive |
| `TestRun_ArchivesOldLogs` | log อายุ 31 วัน → ถูก archive และ archive_state บันทึกว่า done |
| `TestRun_RecentLogsStayInHot` | log อายุ 15 วัน → ไม่ถูกแตะ |
| `TestRun_IsIdempotent` | รัน 2 ครั้ง → ไม่มี document ซ้ำใน archive |
| `TestRun_SkipsDoneDays` | วันที่ mark done แล้ว → ข้ามไม่ทำซ้ำ |
| `TestEnsureArchiveCollection_IsIdempotent` | สร้าง collection 2 ครั้ง → ไม่ error |

---

## สรุป Flow ทั้งหมด

```
service อื่น ──► POST /ingest ──► service_logs (hot, TTL 40 วัน)
                                        │
                                        │ archiver รันทุกคืน
                                        │ (log อายุ > 30 วัน)
                                        ▼
                              service_logs_archive_2026_07
                              service_logs_archive_2026_08
                              ...                          (ไม่มี TTL = เก็บตลอดไป)
                                        │
                              archive_state
                              { "2026-06-19": done, count: 1542 }
                              { "2026-06-20": done, count: 983  }
```

ข้อมูลไม่หาย — แค่ย้ายบ้านจาก "กล่องใหม่" ไป "ตู้เก็บของ"
