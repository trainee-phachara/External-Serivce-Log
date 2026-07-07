# CI/CD Pipeline

## CI/CD คืออะไร

**CI (Continuous Integration)** คือการรัน test และตรวจสอบ code อัตโนมัติทุกครั้งที่มีการเปลี่ยนแปลง code โดยไม่ต้องรอให้คนมานั่งรันเอง

**CD (Continuous Deployment)** คือการ deploy code ขึ้น server อัตโนมัติหลังจาก CI ผ่าน

เปรียบง่ายๆ: ทุกครั้งที่ push code → ระบบจะตรวจสอบให้อัตโนมัติ → ถ้าพังก็จะแจ้งทันที ไม่ให้ code เสียหายไปถึง production

---

## GitHub Actions คืออะไร

เครื่องมือของ GitHub ที่ใช้สร้าง CI/CD pipeline โดยเขียนเป็นไฟล์ `.yml` ไว้ใน `.github/workflows/` GitHub จะอ่านไฟล์นี้และรันให้อัตโนมัติบน VM ของ GitHub เอง ไม่ต้องมี server เป็นของตัวเอง

---

## Workflow ของเรา

ไฟล์: `.github/workflows/ci.yml`

### Trigger — เมื่อไหร่จะรัน

```yaml
on:
  push:
    branches: [main]       # ทุกครั้งที่ push ขึ้น main
  pull_request:
    branches: [main]       # ทุกครั้งที่เปิด PR เข้า main
```

### Jobs — งานที่รัน

มี 2 jobs รันพร้อมกัน (parallel) เพื่อประหยัดเวลา:

```
push / PR → main
      │
      ├── test-client          (รัน test สำหรับ client lib)
      └── test-service         (รัน test สำหรับ log server)
```

---

## แต่ละ Step ทำอะไร

### 1. Checkout

```yaml
- uses: actions/checkout@v4
```

clone repo ลงมาบน VM ของ GitHub เพื่อให้ step ถัดไปเข้าถึง code ได้

---

### 2. Setup Go

```yaml
- uses: actions/setup-go@v5
  with:
    go-version-file: go.work
    cache-dependency-path: client/go.sum
```

ติดตั้ง Go บน VM ตาม version ที่ระบุใน `go.work` และ cache dependencies ไว้เพื่อให้ครั้งถัดไปรันเร็วขึ้น ไม่ต้อง download ใหม่ทุกครั้ง

---

### 3. Build

```yaml
- name: Build
  run: go build ./...
```

**ทำอะไร:** สั่งให้ Go compile code ทั้งหมด

**ทำไมต้องมี:** test อาจผ่านได้แต่ binary build ไม่ได้ เช่น import package ผิด หรือ type ไม่ตรง step นี้จะจับ error พวกนั้นก่อน deploy

---

### 4. Lint

```yaml
- name: Lint
  uses: golangci/golangci-lint-action@v6
```

**ทำอะไร:** ใช้ `golangci-lint` ตรวจ code ว่ามี pattern ที่ไม่ดีไหม เช่น
- error ที่ไม่ได้ handle
- variable ที่ประกาศแต่ไม่ได้ใช้
- code ที่อาจก่อให้เกิด bug

**ทำไมต้องมี:** จับปัญหาที่ test จับไม่ได้ และทำให้ code ทุกคนในทีมมีมาตรฐานเดียวกัน

---

### 5. Test with Coverage Gate

```yaml
- name: Test with coverage gate (>= 85%)
  run: |
    go test -coverprofile=coverage.out -covermode=atomic ./...
    go tool cover -func=coverage.out
    awk '/^total:/{gsub(/%/,"",$3); if ($3+0 < 85) {print "❌ Coverage "$3"% is below 85%"; exit 1} ...}'
```

**ทำอะไร:** รัน test ทั้งหมด และตรวจว่า coverage รวมเกิน 85% ไหม ถ้าต่ำกว่า pipeline จะ fail

**ทำไมต้องมี:**
- `-coverprofile=coverage.out` → บันทึกผล coverage ลงไฟล์
- `-covermode=atomic` → นับ coverage แบบ thread-safe (ถูกต้องกว่าสำหรับ concurrent code)
- `go tool cover -func` → แสดงผล coverage ทีละ function
- `awk` → อ่านบรรทัด `total:` แล้วเช็คว่าเกิน 85% ไหม ถ้าไม่ → `exit 1` → pipeline fail

---

## ผล Pipeline บน GitHub

ดูได้ที่ GitHub repo → **Actions** tab

| สถานะ | ความหมาย |
|---|---|
| ✅ สีเขียว | ทุก step ผ่าน code ปลอดภัย |
| ❌ สีแดง | มี step ที่พัง ดู log เพื่อหาสาเหตุ |

---

## ทำไม Coverage Gate ต้องอยู่ที่ 85%

- 100% เป็นไปได้ยากในทางปฏิบัติ เพราะมี generated code และ edge case ที่ test ยากมาก
- ต่ำกว่า 80% เสี่ยงเกินไป
- 85% คือจุดสมดุลที่ทีมส่วนใหญ่ใช้กัน มั่นใจได้ว่า logic หลักถูก test ครบ
