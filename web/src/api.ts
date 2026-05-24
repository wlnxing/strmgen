export interface OpenListSettings {
  base_url: string
  username: string
  password_set: boolean
}

export interface Task {
  id: number
  name: string
  enabled: boolean
  cron: string
  output_root: string
  scan_dirs: string[]
  sync_mode: 'loose' | 'strict'
  strm_extensions: string[]
  download_extensions: string[]
  blacklist: string[]
  encode_url: boolean
  download_concurrency: number
  download_timeout_seconds: number
  created_at?: string
  updated_at?: string
}

export interface RunStats {
  dirs: number
  files: number
  strm_written: number
  downloads: number
  deleted: number
  skipped: number
  errors: number
}

export interface Run {
  id: number
  task_id: number
  task_name?: string
  trigger: string
  status: string
  stats: RunStats
  error?: string
  started_at: string
  ended_at?: string
}

export interface RunEvent {
  id: number
  run_id: number
  level: string
  message: string
  created_at: string
}

export interface ActiveRun {
  run_id: number
  task_id: number
  task_name: string
  trigger: string
  started_at: string
}

export interface ApiError extends Error {
  status: number
}

export function defaultTask(): Task {
  return {
    id: 0,
    name: '默认任务',
    enabled: true,
    cron: '0 3 * * *',
    output_root: '/media',
    scan_dirs: ['/'],
    sync_mode: 'loose',
    strm_extensions: ['.avi', '.flv', '.iso', '.m2ts', '.m4v', '.mkv', '.mov', '.mp4', '.ts', '.webm', '.wmv'],
    download_extensions: ['.ass', '.idx', '.jpeg', '.jpg', '.nfo', '.png', '.srt', '.ssa', '.sub'],
    blacklist: [],
    encode_url: true,
    download_concurrency: 2,
    download_timeout_seconds: 120
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  const res = await fetch(path, { ...init, headers, credentials: 'same-origin' })
  const payload = await res.json().catch(() => ({}))
  if (!res.ok) {
    const err = new Error(payload.error || `HTTP ${res.status}`) as ApiError
    err.status = res.status
    throw err
  }
  return payload.data as T
}

export const api = {
  login: (username: string, password: string) => request<{ username: string }>('/api/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) }),
  logout: () => request<{ ok: boolean }>('/api/auth/logout', { method: 'POST' }),
  session: () => request<{ authenticated: boolean; username?: string }>('/api/auth/session'),
  me: () => request<{ username: string }>('/api/auth/me'),
  status: () => request<{ active_runs: ActiveRun[] }>('/api/status'),
  getSettings: () => request<OpenListSettings>('/api/settings/openlist'),
  saveSettings: (payload: { base_url: string; username: string; password?: string }) => request<OpenListSettings>('/api/settings/openlist', { method: 'PUT', body: JSON.stringify(payload) }),
  testSettings: () => request<{ ok: boolean }>('/api/settings/openlist/test', { method: 'POST' }),
  listTasks: async () => (await request<Task[] | null>('/api/tasks')) ?? [],
  createTask: (task: Task) => request<Task>('/api/tasks', { method: 'POST', body: JSON.stringify(task) }),
  updateTask: (task: Task) => request<Task>(`/api/tasks/${task.id}`, { method: 'PUT', body: JSON.stringify(task) }),
  deleteTask: (id: number) => request<{ ok: boolean }>(`/api/tasks/${id}`, { method: 'DELETE' }),
  runTask: (id: number) => request<Run>(`/api/tasks/${id}/run`, { method: 'POST' }),
  stopTask: (id: number) => request<{ stopped: boolean }>(`/api/tasks/${id}/stop`, { method: 'POST' }),
  listRuns: async () => (await request<Run[] | null>('/api/runs?limit=80')) ?? [],
  getRun: (id: number) => request<{ run: Run; events: RunEvent[] }>(`/api/runs/${id}`)
}
