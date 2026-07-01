export interface LogSource {
  app_name: string;
  service_name: string;
}

export interface LogEntry {
  timestamp: string;
  source: LogSource;
  trace_id: string;
  endpoint: string;
  http_status: string;
  type: string;
  direction: string;
  metadata: Record<string, unknown>;
  raw_payload: Record<string, unknown>;
  payload: Record<string, unknown>;
}

export interface User {
  id: number;
  name: string;
  email: string;
  created_at: string;
  updated_at: string;
}

export interface OrderItem {
  product_id: number;
  product_name: string;
  quantity: number;
  unit_price: number;
}

export interface Order {
  id: number;
  user_id: number;
  items: OrderItem[];
  status: string;
  total_amount: number;
  created_at: string;
  updated_at: string;
}

export interface Payment {
  id: number;
  order_id: number;
  user_id: number;
  amount: number;
  status: string;
  method: string;
  created_at: string;
  updated_at: string;
}