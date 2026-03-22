// API client for the todo app.
// Types match the OpenAPI spec (openapi.yaml).

const API_BASE = '/api';

export interface Todo {
  id: string;
  userId: string;
  title: string;
  done: boolean;
  createdAt: string;
}

export interface AuthStatus {
  loggedIn: boolean;
  email?: string;
}

export async function getAuthStatus(): Promise<AuthStatus> {
  const res = await fetch(`${API_BASE}/auth/status`);
  return res.json();
}

export async function getLoginURL(): Promise<string> {
  const res = await fetch(`${API_BASE}/auth/login`);
  const data = await res.json();
  return data.url;
}

export async function logout(): Promise<void> {
  await fetch(`${API_BASE}/auth/logout`, { method: 'POST' });
}

export async function listTodos(): Promise<Todo[]> {
  const res = await fetch(`${API_BASE}/todos`);
  if (!res.ok) throw new Error(`Failed to list todos: ${res.statusText}`);
  return res.json();
}

export async function createTodo(title: string): Promise<Todo> {
  const res = await fetch(`${API_BASE}/todos`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title }),
  });
  if (!res.ok) throw new Error(`Failed to create todo: ${res.statusText}`);
  return res.json();
}

export async function deleteTodo(id: string): Promise<void> {
  const res = await fetch(`${API_BASE}/todos/${id}`, { method: 'DELETE' });
  if (!res.ok) throw new Error(`Failed to delete todo: ${res.statusText}`);
}
