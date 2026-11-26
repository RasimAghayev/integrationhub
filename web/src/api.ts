export type Connection = {
  id: string;
  name: string;
  type: string;
  auth_type?: string;
};

export type Endpoint = { name: string; method: string; path: string };

export type Integration = {
  id: string;
  name: string;
  type: string;
  status: string;
  connection_id: string;
  endpoints: Endpoint[];
};

export type Execution = {
  id: string;
  integration_id: string;
  endpoint: string;
  status: string;
  status_code: number;
  attempts: number;
  error?: string;
  response_body?: string;
};

export type Schedule = {
  id: string;
  name: string;
  integration_id: string;
  endpoint: string;
  cron: string;
  enabled: boolean;
  next_run: string;
};

const TOKEN_KEY = "integrationhub.token";

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) ?? "";
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token);
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (init?.body) headers.set("Content-Type", "application/json");
  const token = getToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  const res = await fetch(path, { ...init, headers });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${res.status}: ${text.slice(0, 200)}`);
  }
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

export const api = {
  connections: () => request<Connection[]>("/api/v1/connections"),
  integrations: () => request<Integration[]>("/api/v1/integrations"),
  executions: (integrationId?: string) =>
    request<Execution[]>(
      `/api/v1/executions${integrationId ? `?integration_id=${integrationId}` : ""}`,
    ),
  schedules: () => request<Schedule[]>("/api/v1/schedules"),
  createSchedule: (body: Omit<Schedule, "id" | "next_run">) =>
    request<Schedule>("/api/v1/schedules", { method: "POST", body: JSON.stringify(body) }),
  deleteSchedule: (id: string) =>
    request<void>(`/api/v1/schedules/${id}`, { method: "DELETE" }),
  execute: (id: string, endpoint: string) =>
    request<Execution>(`/api/v1/integrations/${id}/execute`, {
      method: "POST",
      body: JSON.stringify({ endpoint }),
    }),
  replay: (id: string) =>
    request<void>(`/api/v1/executions/${id}/replay`, { method: "POST" }),
  token: (apiKey: string) =>
    fetch("/api/v1/auth/token", {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-API-Key": apiKey },
    }).then(async (res) => {
      if (!res.ok) throw new Error(`token request failed: ${res.status}`);
      return (await res.json()) as { token: string };
    }),
};