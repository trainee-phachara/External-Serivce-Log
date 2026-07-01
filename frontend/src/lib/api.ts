import type { LogEntry, User, Order, Payment, OrderItem } from './types';

const LOG_URL = 'http://localhost:3000';
const USER_URL = 'http://localhost:3001';
const ORDER_URL = 'http://localhost:3002';
const PAYMENT_URL = 'http://localhost:3003';

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(url, options);
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `HTTP ${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

// ── Logs ──────────────────────────────────────────────────────────────────────

export function getLogs(type_ = '', app = '', limit = 50): Promise<LogEntry[]> {
  const p = new URLSearchParams({ limit: String(limit) });
  if (type_) p.set('type', type_);
  if (app) p.set('app', app);
  return request<LogEntry[]>(`${LOG_URL}/logs?${p}`);
}

// ── Users ─────────────────────────────────────────────────────────────────────

export function getUsers(): Promise<User[]> {
  return request<User[]>(`${USER_URL}/users`);
}

export function createUser(name: string, email: string): Promise<User> {
  return request<User>(`${USER_URL}/users`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, email }),
  });
}

export function updateUser(id: number, name: string, email: string): Promise<User> {
  return request<User>(`${USER_URL}/users/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, email }),
  });
}

export function deleteUser(id: number): Promise<void> {
  return request<void>(`${USER_URL}/users/${id}`, { method: 'DELETE' });
}

// ── Orders ────────────────────────────────────────────────────────────────────

export function getOrders(): Promise<Order[]> {
  return request<Order[]>(`${ORDER_URL}/orders`);
}

export function createOrder(userId: number, items: OrderItem[]): Promise<Order> {
  return request<Order>(`${ORDER_URL}/orders`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user_id: userId, items }),
  });
}

export function updateOrderStatus(id: number, status: string): Promise<Order> {
  return request<Order>(`${ORDER_URL}/orders/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ status }),
  });
}

export function deleteOrder(id: number): Promise<void> {
  return request<void>(`${ORDER_URL}/orders/${id}`, { method: 'DELETE' });
}

// ── Payments ──────────────────────────────────────────────────────────────────

export function getPayments(): Promise<Payment[]> {
  return request<Payment[]>(`${PAYMENT_URL}/payments`);
}

export function createPayment(
  orderId: number,
  userId: number,
  amount: number,
  method: string
): Promise<Payment> {
  return request<Payment>(`${PAYMENT_URL}/payments`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ order_id: orderId, user_id: userId, amount, method }),
  });
}

export function updatePaymentStatus(id: number, status: string): Promise<Payment> {
  return request<Payment>(`${PAYMENT_URL}/payments/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ status }),
  });
}

export function deletePayment(id: number): Promise<void> {
  return request<void>(`${PAYMENT_URL}/payments/${id}`, { method: 'DELETE' });
}