export const PLUGIN_ID = 'alemonx-docker'

type Task = { id: string; status: 'running' | 'completed' | 'failed'; output?: string; error?: string; data?: unknown }
export type ActionResult<T = unknown> = { output: string; error?: string; data?: T }

async function json<T>(response: Response): Promise<T> {
  if (!response.ok) {
    let message = `请求失败（${response.status}）`
    try { const body = await response.json(); message = body.error || body.message || message } catch { /* keep status */ }
    throw new Error(message)
  }
  return response.json() as Promise<T>
}

export async function fetchStatus<T>(action: string): Promise<T> {
  return json<T>(await fetch(`/api/v1/setup/plugins/${PLUGIN_ID}/status?action=${encodeURIComponent(action)}`))
}

export async function runAction(action: string, params: Record<string, string> = {}): Promise<string> {
  const body = await json<{ id: string }>(await fetch(`/api/v1/setup/plugins/${PLUGIN_ID}/actions`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ action, params })
  }))
  return body.id
}

async function pollTask<T>(id: string): Promise<ActionResult<T>> {
  for (let attempt = 0; attempt < 120; attempt += 1) {
    await new Promise(resolve => setTimeout(resolve, 700))
    const tasks = await json<Task[]>(await fetch('/api/v1/robot/tasks')).catch(() => [])
    const task = tasks.find(item => item.id === id)
    if (!task) continue
    if (task.status === 'completed') return { output: task.output || '', data: task.data as T }
    if (task.status === 'failed') return { output: task.output || '', error: task.error || '插件操作失败。', data: task.data as T }
  }
  return { output: '', error: '操作超时。' }
}

export async function runActionAndPoll<T = unknown>(action: string, params: Record<string, string> = {}): Promise<ActionResult<T>> {
  return pollTask<T>(await runAction(action, params))
}

export async function uploadCompose(destination: string, file: File): Promise<ActionResult> {
  const data = new FormData()
  data.append('action', 'upload-compose')
  data.append('destination', destination)
  data.append('files', file)
  const task = await json<{ id: string }>(await fetch(`/api/v1/setup/plugins/${PLUGIN_ID}/upload`, { method: 'POST', body: data }))
  return pollTask(task.id)
}

export async function installDocker(): Promise<string> {
  const result = await json<{ output?: string }>(await fetch('/api/v1/system/environment/install', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ checkId: 'docker', confirm: true })
  }))
  return result.output || 'Docker 安装已完成，请重新检查环境。'
}

export async function openDesktop(target: string): Promise<void> {
  const host = window.ALXHost
  if (!host) throw new Error('当前 ALemonX 版本不支持打开本地目录。')
  await host.desktop.open(PLUGIN_ID, target)
}

declare global {
  interface Window {
    ALXHost?: { desktop: { open: (pluginId: string, target: string) => Promise<unknown> } }
  }
}
