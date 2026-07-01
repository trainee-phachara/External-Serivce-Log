# Learning Log: SvelteKit Frontend

บันทึกนี้อธิบาย frontend ที่สร้างด้วย SvelteKit ตั้งแต่ต้น
สำหรับคนที่ยังไม่เคยทำ frontend หรือไม่เคยรู้จัก Svelte มาก่อน

---

## 1. SvelteKit คืออะไร

**Svelte** คือ UI framework สำหรับเขียน frontend เหมือน React หรือ Vue แต่ต่างตรงที่ Svelte **compile โค้ดเป็น JavaScript ธรรมดาตอน build** แทนที่จะใช้ virtual DOM runtime

**SvelteKit** คือ framework ครอบ Svelte อีกชั้น — เพิ่ม routing, server-side rendering, API routes ฯลฯ คล้ายกับ Next.js สำหรับ React

ทำไมเลือก SvelteKit สำหรับ project นี้:
- Bundle size เล็ก เหมาะกับ demo ที่ไม่ต้องการ dependency หนัก
- Routing ทำด้วย file-based ง่ายต่อการเข้าใจ
- Syntax อ่านง่าย ไม่มี boilerplate เยอะ

---

## 2. โครงสร้างโปรเจกต์

```
frontend/
├── src/
│   ├── lib/
│   │   ├── types.ts        ← TypeScript types (User, Order, Payment, LogEntry)
│   │   └── api.ts          ← ฟังก์ชันเรียก HTTP API ของทุก service
│   └── routes/
│       ├── +layout.svelte         ← Layout ที่ครอบทุกหน้า (navbar + global CSS)
│       ├── +page.svelte           ← หน้า / (Log dashboard)
│       ├── users/+page.svelte     ← หน้า /users
│       ├── orders/+page.svelte    ← หน้า /orders
│       └── payments/+page.svelte  ← หน้า /payments
├── package.json
└── svelte.config.js
```

---

## 3. File-based Routing

SvelteKit ใช้ **โครงสร้าง folder เป็น URL** โดยอัตโนมัติ — ไม่ต้องประกาศ route ที่ไหนเลย

| ไฟล์ | URL |
|---|---|
| `routes/+page.svelte` | `/` |
| `routes/users/+page.svelte` | `/users` |
| `routes/orders/+page.svelte` | `/orders` |
| `routes/payments/+page.svelte` | `/payments` |

ชื่อไฟล์ที่ขึ้นด้วย `+` คือชื่อพิเศษของ SvelteKit:
- `+page.svelte` — component หน้านั้น
- `+layout.svelte` — กรอบที่ครอบทุกหน้าในโฟลเดอร์เดียวกัน (และโฟลเดอร์ลูก)

---

## 4. `src/lib/types.ts` — TypeScript types

```typescript
export interface User {
  id: number;
  name: string;
  email: string;
  created_at: string;
  updated_at: string;
}
```

**Interface คืออะไร?**

`interface` ใน TypeScript คือการบอกว่า "object นี้ต้องมี field อะไรบ้าง และแต่ละ field มี type อะไร" — คล้ายกับ struct ใน Go แต่ไม่มีการ compile ตอน runtime เป็นแค่การตรวจสอบขณะเขียนโค้ด

```typescript
// Go
type User struct {
    ID    int    `json:"id"`
    Name  string `json:"name"`
}

// TypeScript (เทียบเคียง)
interface User {
  id: number;
  name: string;
}
```

Types ที่สร้างมาตรงกับ struct ของ Go ทุกตัว — เพื่อให้ TypeScript รู้ว่าข้อมูลที่ได้จาก API มีหน้าตาอย่างไร และจะแจ้ง error ทันทีถ้าเราเข้าถึง field ที่ไม่มีอยู่

---

## 5. `src/lib/api.ts` — HTTP Client

[src/lib/api.ts](frontend/src/lib/api.ts)

### Base URLs

```typescript
const LOG_URL  = 'http://localhost:3000';
const USER_URL = 'http://localhost:3001';
const ORDER_URL = 'http://localhost:3002';
const PAYMENT_URL = 'http://localhost:3003';
```

แต่ละ service รันคนละ port — frontend เรียกตรงไปยัง service ที่ต้องการ

### Generic fetch wrapper

```typescript
async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(url, options);
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `HTTP ${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}
```

**`async/await` คืออะไร?**

`fetch` เป็น asynchronous — มันส่ง HTTP request แล้วต้องรอ response ซึ่งใช้เวลา JavaScript จึงไม่บล็อกทุกอย่างในระหว่างรอ

`async function` คือฟังก์ชันที่ return `Promise` (สัญญาว่าจะมีค่าในอนาคต)
`await` คือการบอกให้รอ Promise นั้นก่อนดำเนินการต่อ

```typescript
// โดยไม่มี await — res คือ Promise ยังไม่ใช่ response จริงๆ
const res = fetch(url);

// มี await — รอจนได้ response แล้วค่อยไปบรรทัดถัดไป
const res = await fetch(url);
```

**`<T>` Generic คืออะไร?**

`request<T>` หมายความว่าฟังก์ชันนี้คืนค่า type อะไรก็ได้ที่ผู้เรียกบอก:

```typescript
// ฟังก์ชันที่เรียก request จะระบุ T ตอนเรียก
const users = await request<User[]>('/users');   // T = User[]
const user  = await request<User>('/users/1');   // T = User
```

เทียบได้กับ Generic ใน Go: `func Find[T any](...)T`

### ตัวอย่าง API functions

```typescript
// GET /users
export function getUsers(): Promise<User[]> {
  return request<User[]>(`${USER_URL}/users`);
}

// POST /users
export function createUser(name: string, email: string): Promise<User> {
  return request<User>(`${USER_URL}/users`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, email }),
  });
}

// DELETE /users/:id
export function deleteUser(id: number): Promise<void> {
  return request<void>(`${USER_URL}/users/${id}`, { method: 'DELETE' });
}
```

`JSON.stringify` แปลง JavaScript object เป็น JSON string ก่อนส่ง — ตรงข้ามกับ `JSON.parse` ที่แปลง JSON string กลับเป็น object

---

## 6. CORS คืออะไรและทำไมต้องเพิ่ม

**CORS** (Cross-Origin Resource Sharing) เป็นกลไกของ browser ที่บล็อก HTTP request ข้าม origin โดย default

"Origin" = protocol + hostname + port — ดังนั้น:
```
http://localhost:5173  (frontend)
http://localhost:3001  (user-service)
```
สองตัวนี้ต่าง origin — browser จะบล็อก request จาก frontend ไป user-service โดยอัตโนมัติ

**วิธีแก้** — ให้ server ส่ง HTTP header บอก browser ว่าอนุญาต:

```go
// เพิ่มใน server.go ของทุก Go service
func withCORS(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
        if r.Method == http.MethodOptions {
            w.WriteHeader(http.StatusNoContent)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

Browser ส่ง **preflight request** (method OPTIONS) ก่อนทุก POST/PUT/DELETE — server ต้องตอบกลับ OK จึงจะยอมให้ request จริงผ่าน นั่นคือเหตุผลที่ต้องดัก `r.Method == http.MethodOptions` แยกออกมา

---

## 7. `+layout.svelte` — Layout และ Global Styles

[src/routes/+layout.svelte](frontend/src/routes/+layout.svelte)

```svelte
<script lang="ts">
  import { page } from '$app/stores';
  let { children } = $props();
</script>

<nav>
  <span class="brand">Service Log Demo</span>
  <div class="links">
    <a href="/" class:active={$page.url.pathname === '/'}>Logs</a>
    <a href="/users" class:active={$page.url.pathname === '/users'}>Users</a>
  </div>
</nav>

<main>
  {@render children()}
</main>
```

### `$props()` — รับ children

`$props()` เป็น Svelte 5 **rune** (ฟังก์ชันพิเศษที่ขึ้นต้นด้วย `$`) สำหรับรับ props ที่ถูกส่งมาจากภายนอก

ใน layout, `children` คือ content ของหน้าลูก (หน้า +page.svelte) ที่จะถูก render ตรงที่ `{@render children()}` — เหมือนกับ `<slot>` ใน Svelte 4

### `$app/stores` และ `page` store

`$app/stores` เป็น module ของ SvelteKit ที่ export `page` store — object ที่มีข้อมูลหน้าปัจจุบัน เช่น URL, params, และ data

```svelte
import { page } from '$app/stores';

<!-- เช็คว่าเส้นทางปัจจุบันตรงกับ href ไหม -->
<a href="/" class:active={$page.url.pathname === '/'}>Logs</a>
```

**`class:active={condition}`** คือ shorthand syntax ของ Svelte — เพิ่ม class `active` เมื่อ condition เป็น `true` เทียบเท่ากับ:
```svelte
<a class={$page.url.pathname === '/' ? 'active' : ''}>Logs</a>
```

### `:global()` CSS

CSS ใน Svelte ถูก scoped ให้อยู่แค่ใน component นั้น — ไม่รั่วไปกระทบ component อื่น ซึ่งดีในภาวะปกติ แต่ `+layout.svelte` ต้องการ CSS ที่ใช้ได้ทั่วทั้งแอป จึงใช้ `:global()`:

```css
/* ใช้ได้แค่ใน layout.svelte เท่านั้น */
table { width: 100%; }

/* ใช้ได้ทุกหน้า */
:global(table) { width: 100%; }
```

---

## 8. Svelte 5 Runes — State Management

Svelte 5 เปลี่ยนจาก `let` ธรรมดาเป็น runes สำหรับ reactive state:

### `$state()` — Reactive variable

```svelte
<script lang="ts">
  let users: User[] = $state([]);   // ← reactive
  let loading = $state(false);
  let error = $state('');
</script>
```

เมื่อค่าใน `$state` เปลี่ยน Svelte จะ **re-render** ส่วนของ UI ที่ใช้ตัวแปรนั้นโดยอัตโนมัติ — ไม่ต้องเรียก `setState()` หรืออัปเดต store แบบ manual เหมือน React

ใน Go เราเปลี่ยนตัวแปรแล้วต้องส่งข้อมูลใหม่ไปที่ UI เอง ใน Svelte แค่แก้ตัวแปร UI อัปเดตเอง

### `$props()` — รับ props จากภายนอก

```svelte
<script lang="ts">
  let { children } = $props();
</script>
```

---

## 9. Template Syntax — HTML + Logic

Svelte ผสม HTML กับ logic ไว้ในไฟล์เดียว:

### `{#if}` — เงื่อนไข

```svelte
{#if loading}
  <div>Loading...</div>
{:else if error}
  <p class="error">{error}</p>
{:else}
  <table>...</table>
{/if}
```

เหมือน if-else ทั่วไป แต่อยู่ใน HTML โดยตรง

### `{#each}` — วนลูป

```svelte
{#each users as u}
  <tr>
    <td>{u.id}</td>
    <td>{u.name}</td>
    <td>{u.email}</td>
  </tr>
{/each}
```

เหมือน `for _, u := range users` ใน Go แต่ output เป็น HTML

### `{expression}` — แสดงค่า

```svelte
<td>{u.id}</td>
<td>{new Date(u.created_at).toLocaleDateString('th-TH')}</td>
```

วงเล็บปีกกา `{}` คือที่ใส่ JavaScript expression — ค่าที่ได้จะถูก render เป็น text ใน HTML

---

## 10. `bind:value` — Two-way Binding

```svelte
<input placeholder="Name" bind:value={name} />
```

`bind:value` ทำสองทิศทาง:
1. ค่าใน `name` → แสดงใน input
2. user พิมพ์ใน input → อัปเดต `name` อัตโนมัติ

ถ้าไม่ใช้ `bind:` ต้องเขียนแบบ manual:
```svelte
<input
  value={name}
  oninput={(e) => name = e.target.value}
/>
```

---

## 11. Event Handlers

```svelte
<button onclick={handleCreate}>Create</button>

<!-- inline handler -->
<button onclick={() => deleteUser(u.id)}>Delete</button>

<!-- รับ event object -->
<select onchange={(e) => handleStatus(o.id, e.target.value)}>
```

ใน Svelte 5 ใช้ `onclick` แทน `on:click` (Svelte 4) — syntax ตรงกับ DOM event มาตรฐาน

---

## 12. `onMount` — ทำอะไรบางอย่างตอนหน้าโหลดครั้งแรก

```svelte
<script lang="ts">
  import { onMount } from 'svelte';

  onMount(load);   // เรียก load() ทันทีที่ component ถูก render ลงหน้าจอ
</script>
```

`onMount` ทำงานหลัง component ถูก render ใน browser ครั้งแรก — ใช้สำหรับ fetch data ตอนเปิดหน้า

ทำไมถึงไม่เรียก `load()` ตรงๆ ใน `<script>`?

เพราะ SvelteKit support server-side rendering — code ใน `<script>` อาจรันบน Node.js ซึ่งยังไม่มี browser window หรือ DOM ก็ได้ `onMount` รัน **เฉพาะบน browser** เสมอ

---

## 13. หน้า Log Dashboard (`+page.svelte`)

[src/routes/+page.svelte](frontend/src/routes/+page.svelte)

```svelte
<script lang="ts">
  let typeFilter = $state('');

  async function load() {
    loading = true;
    error = '';
    try {
      logs = await getLogs(typeFilter, appFilter);
    } catch (e: unknown) {
      error = e instanceof Error ? e.message : 'Failed to load logs';
    } finally {
      loading = false;
    }
  }
</script>

<div class="tabs">
  {#each logTypes as lt}
    <button
      class:active={typeFilter === lt.id}
      onclick={() => { typeFilter = lt.id; load(); }}
    >{lt.label}</button>
  {/each}
</div>
```

tabs มี 4 ตัว: All (ไม่ส่ง filter), Request, Response, Event — กรอง log ด้วย `type` field แทนการเลือก collection

**try/catch/finally** ใน JavaScript เหมือนกับ Go:
```
try     → code ที่อาจ error
catch   → จับ error มาทำอะไร
finally → ทำเสมอไม่ว่าจะ error หรือไม่ (เช่น ปิด loading)
```

**`e instanceof Error`** — JavaScript มีหลาย type ของสิ่งที่ throw ได้ การเช็ค `instanceof Error` ให้แน่ใจว่า `e` มี `.message` property ก่อนใช้

---

## 14. หน้า Orders — Dynamic Array ใน Form

[src/routes/orders/+page.svelte](frontend/src/routes/orders/+page.svelte)

Orders มี `items` เป็น array — user เพิ่ม/ลบ item ได้ตอน fill form:

```svelte
let items: OrderItem[] = $state([
  { product_id: 0, product_name: '', quantity: 1, unit_price: 0 }
]);

function addItem() {
  items = [...items, { product_id: 0, product_name: '', quantity: 1, unit_price: 0 }];
}

function removeItem(i: number) {
  items = items.filter((_, idx) => idx !== i);
}
```

**Spread operator `...`** — `[...items, newItem]` สร้าง array ใหม่จาก items เดิมทั้งหมด + newItem ต่อท้าย (ไม่แก้ array เดิม)

**`.filter()`** — สร้าง array ใหม่ที่เอาเฉพาะ element ที่ผ่าน condition: `idx !== i` คือเอาทุก index ยกเว้น index ที่ต้องลบ

```svelte
{#each items as item, i}
  <div class="form-row">
    <input bind:value={item.product_name} />
    <input bind:value={item.quantity} type="number" />
    {#if items.length > 1}
      <button onclick={() => removeItem(i)}>×</button>
    {/if}
  </div>
{/each}
```

`{#each items as item, i}` — `i` คือ index ของแต่ละ item (0, 1, 2, ...) ใช้ส่งไป `removeItem(i)` เพื่อบอกว่าจะลบ item ไหน

---

## 15. ภาพรวม Flow ทั้งหมด

```
Browser (SvelteKit :5173)
    │
    │  1. user คลิก "Create User"
    │
    ▼
api.ts → fetch POST http://localhost:3001/users
    │
    ▼
user-service (:3001)
    │  withCORS → withRequestLogger (บันทึก log)
    │  INSERT INTO users
    ▼
PostgreSQL
    │
    │  gRPC fire-and-forget
    ▼
external-service-log (:3000 / :50051)
    │  buffer → batch flush
    ▼
MongoDB (service_logs collection)
    │
    │  2. user คลิก Refresh ที่หน้า Logs
    │
Browser → fetch GET http://localhost:3000/logs?type=request
    │
    ▼
external-service-log → MongoDB query → return []LogEntry
    │
    ▼
หน้า Logs แสดง log ที่เพิ่งเกิดขึ้น ✓
```

ทุกครั้งที่ทำ action ใน UI (create user, create order, create payment) จะมี log ถูกส่งไป external-service-log โดยอัตโนมัติ และสามารถดูได้ที่หน้า Logs ทันทีหลัง refresh — นี่คือ demo flow ที่พิสูจน์ว่า external-service-log ทำงานครบ

---

## 16. วิธีรัน Demo

**Terminal 1** — External service log
```bash
cd external-service-log
go run ./cmd/server
```

**Terminal 2** — User service
```bash
cd user-service
go run ./cmd/server
```

**Terminal 3** — Order service
```bash
cd order-service
go run ./cmd/server
```

**Terminal 4** — Payment service
```bash
cd payment-service
go run ./cmd/server
```

**Terminal 5** — Frontend
```bash
cd frontend
npm run dev
```

เปิด `http://localhost:5173` ในเบราว์เซอร์ได้เลย