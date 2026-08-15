import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'

const project = {
  id: 'demo-project-12345678',
  name: 'Demo',
  source: '新建',
  createdAt: '2026-08-15T00:00:00Z',
  updatedAt: '2026-08-15T00:00:00Z',
  content: '# demo\nservices:\n  web:\n    image: nginx:latest\n'
}

const containerRow = {
  id: 'abc123',
  names: 'web-1',
  image: 'nginx:latest',
  state: 'running',
  status: 'Up 2 minutes',
  ports: '0.0.0.0:8080->80/tcp',
  labels: 'com.docker.compose.project=myapp,com.docker.compose.service=web',
  project: 'myapp',
  published: [{ hostPort: 8080, containerPort: 80, protocol: 'tcp' }]
}

function json(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(data), { status, headers: { 'Content-Type': 'application/json' } })
}

function installFetchMock(options: { withProject?: boolean; withContainer?: boolean } = {}) {
  const calls: Array<{ method: string; url: string; body?: string }> = []
  const taskResults: Record<string, unknown> = {}
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const body = typeof init?.body === 'string' ? init.body : ''
    calls.push({ method: init?.method || 'GET', url, body })
    if (url.includes('/upload')) {
      taskResults['task-2'] = undefined
      return json({ id: 'task-2' })
    }
    if (url.endsWith('/actions') && body) {
      const action = JSON.parse(body).action
      if (action === 'project-read') taskResults['task-1'] = project
      else if (action === 'project-create') taskResults['task-1'] = { ...project, id: 'new-demo-12345678' }
      else if (action === 'project-import-target') taskResults['task-1'] = { project, destination: '/tmp/alx-docker/projects/new-demo-12345678' }
      else taskResults['task-1'] = undefined
      return json({ id: 'task-1' })
    }
    if (url.includes('/robot/tasks')) {
      return json(Object.entries(taskResults).map(([id, data]) => ({ id, status: 'completed', output: '✓ ok', data })))
    }
    if (url.includes('action=docker-status')) {
      return json({
        cli: { available: true, version: '27.0.0' },
        compose: { available: true, version: 'v2.29.0' },
        daemon: { available: true, version: '27.0.0' },
        platform: 'darwin/arm64'
      })
    }
    if (url.includes('action=project-list')) {
      return json(options.withProject ? [{ id: project.id, name: project.name, source: project.source, createdAt: project.createdAt, updatedAt: project.updatedAt }] : [])
    }
    if (url.includes('action=image-list')) {
      return json([])
    }
    if (url.includes('action=container-list')) {
      return json(options.withContainer ? [containerRow] : [])
    }
    return json({})
  }))
  return calls
}

beforeEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('App workspaces', () => {
  it('renders the four workspaces and Docker environment checks', async () => {
    installFetchMock()
    render(<App />)
    for (const label of ['环境', '项目', '镜像', '容器']) {
      expect(screen.getAllByText(label).length).toBeGreaterThan(0)
    }
    await waitFor(() => expect(screen.getAllByText('27.0.0').length).toBeGreaterThanOrEqual(2))
    expect(screen.getByText('v2.29.0')).toBeInTheDocument()
  })

  it('shows the project empty state', async () => {
    installFetchMock()
    render(<App />)
    fireEvent.click(screen.getByText('项目'))
    expect(await screen.findByText('尚未创建项目。')).toBeInTheDocument()
  })

  it('switches between the form and advanced YAML editors', async () => {
    installFetchMock({ withProject: true })
    render(<App />)
    fireEvent.click(screen.getByText('项目'))
    fireEvent.click(await screen.findByText('Demo'))
    fireEvent.click(await screen.findByText('高级 YAML'))
    const editor = await screen.findByDisplayValue(/services:/)
    expect(editor).toBeInTheDocument()
    fireEvent.click(screen.getByText('表单'))
    await waitFor(() => expect(screen.getAllByText('镜像').length).toBeGreaterThan(1))
  })

  it('requires confirmation before compose down', async () => {
    const calls = installFetchMock({ withProject: true })
    const confirmMock = vi.fn(() => false)
    window.confirm = confirmMock
    render(<App />)
    fireEvent.click(screen.getByText('项目'))
    fireEvent.click(await screen.findByText('Demo'))
    fireEvent.click(await screen.findByText('关闭'))
    expect(confirmMock).toHaveBeenCalledTimes(1)
    expect(calls.filter(call => call.body?.includes('compose-down'))).toHaveLength(0)
    confirmMock.mockReturnValue(true)
    fireEvent.click(screen.getByText('关闭'))
    await waitFor(() => {
      expect(calls.filter(call => call.body?.includes('compose-down'))).toHaveLength(1)
    })
  })

  it('shows an error notice when the status refresh fails', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('oops', { status: 500 })))
    render(<App />)
    expect(await screen.findByText(/请求失败/)).toBeInTheDocument()
  })

  it('creates a project and uploads a compose file', async () => {
    const calls = installFetchMock({ withProject: true })
    render(<App />)
    fireEvent.click(screen.getByText('项目'))
    fireEvent.click(screen.getByText('新建项目'))
    fireEvent.change(screen.getByPlaceholderText('项目名称'), { target: { value: 'New Demo' } })
    const label = screen.getByText('拖入或选择 Compose 文件')
    const input = label.closest('label')!.querySelector('input')!
    const file = new File(['services:\n  app:\n    image: busybox\n'], 'compose.yml', { type: 'text/yaml' })
    fireEvent.change(input, { target: { files: [file] } })
    await waitFor(() => {
      expect(calls.some(call => call.url.includes('/upload'))).toBe(true)
    }, { timeout: 6000 })
  })

  it('separates the project list from creation and returns to the list', async () => {
    installFetchMock({ withProject: true })
    render(<App />)
    fireEvent.click(screen.getByText('项目'))
    expect(await screen.findByText('Demo')).toBeInTheDocument()
    expect(screen.queryByText('新建空项目')).not.toBeInTheDocument()
    fireEvent.click(screen.getByText('新建项目'))
    expect(screen.getByText('新建空项目')).toBeInTheDocument()
    expect(screen.queryByText('Demo')).not.toBeInTheDocument()
    fireEvent.click(screen.getByText('项目列表'))
    fireEvent.click(await screen.findByText('Demo'))
    expect(await screen.findByText('返回列表')).toBeInTheDocument()
    expect(screen.queryByText('新建空项目')).not.toBeInTheDocument()
    fireEvent.click(screen.getByText('返回列表'))
    expect(await screen.findByText('Demo')).toBeInTheDocument()
    expect(screen.queryByText('返回列表')).not.toBeInTheDocument()
  })

  it('opens an embedded webview for a container port and applies a route', async () => {
    installFetchMock({ withContainer: true })
    render(<App />)
    fireEvent.click(screen.getByText('容器'))
    fireEvent.click(await screen.findByText('打开页面'))
    const frame = await screen.findByTitle('web-1 容器页面')
    expect(frame.getAttribute('src')).toContain('/api/v1/services/dynamic/alemonx-docker/8080/')
    fireEvent.change(screen.getByPlaceholderText('指定路由，例如 /login'), { target: { value: '/login' } })
    fireEvent.click(screen.getByText('打开路由'))
    await waitFor(() => {
      expect(screen.getByTitle('web-1 容器页面').getAttribute('src')).toContain('/api/v1/services/dynamic/alemonx-docker/8080/login')
    })
  })
})
