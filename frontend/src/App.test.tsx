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

const recommendationRow = {
  id: 'paperless-ngx',
  name: 'Paperless-ngx',
  description: '文档管理系统，支持扫描件 OCR',
  tags: ['文档', 'OCR'],
  url: 'https://example.com/paperless/docker-compose.yml'
}

const exampleRow = {
  id: 'nginx-static',
  name: 'Nginx 静态站点',
  description: '轻量示例',
  tags: ['示例'],
  example: 'examples/nginx-static/docker-compose.yml'
}

function json(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(data), { status, headers: { 'Content-Type': 'application/json' } })
}

function installFetchMock(options: { withProject?: boolean; withContainer?: boolean; withRecommendation?: boolean; withTemplateExample?: boolean; withRegistry?: boolean } = {}) {
  const calls: Array<{ method: string; url: string; body?: string }> = []
  const taskResults: Record<string, unknown> = {}
  const taskOutputs: Record<string, string> = {}
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
      else if (action === 'compose-ps') taskResults['task-1'] = [{ id: 'abc', name: 'demo-web-1', service: 'web', state: 'running', status: 'Up 2 minutes', image: 'nginx:latest' }]
      else if (action === 'container-inspect') taskResults['task-1'] = { id: 'abc123', name: 'web-1', state: 'running' }
      else if (action === 'container-stats') taskResults['task-1'] = { id: 'abc123', name: 'web-1', cpuPerc: '0.05%', memUsage: '5MiB / 8GiB', memPerc: '0.10%', netIO: '1.5kB / 2kB', blockIO: '1.2kB / 0B', pids: '3' }
      else if (action === 'container-logs-since') { taskResults['task-1'] = undefined; taskOutputs['task-1'] = '2026-08-15T12:00:00Z hello\n2026-08-15T12:00:01Z world' }
      else taskResults['task-1'] = undefined
      return json({ id: 'task-1' })
    }
    if (url.includes('/robot/tasks')) {
      return json(Object.entries(taskResults).map(([id, data]) => ({ id, status: 'completed', output: taskOutputs[id] || '✓ ok', data })))
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
    if (url.includes('action=network-list')) {
      return json([])
    }
    if (url.includes('action=volume-list')) {
      return json([])
    }
    if (url.includes('action=external-projects')) {
      return json([])
    }
    if (url.includes('action=recommendations')) {
      const rows = options.withRecommendation ? [recommendationRow] : options.withTemplateExample ? [exampleRow] : []
      return json(rows)
    }
    if (url.includes('action=registry-list')) {
      return json(options.withRegistry ? [{ registry: 'registry.example.com', externalKey: true }] : [])
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
  it('renders all workspaces and Docker environment checks', async () => {
    installFetchMock()
    render(<App />)
    for (const label of ['环境', '项目', '镜像', '容器', '网络', '卷']) {
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

  it('shows the template center and imports an online template', async () => {
    const calls = installFetchMock({ withRecommendation: true })
    vi.stubGlobal('prompt', vi.fn(() => '我的网盘'))
    render(<App />)
    fireEvent.click(screen.getByText('项目'))
    fireEvent.click(screen.getByText('模板中心'))
    expect(await screen.findByText('Paperless-ngx')).toBeInTheDocument()
    fireEvent.click(screen.getByText('使用模板'))
    await waitFor(() => {
      expect(calls.some(call => call.url.endsWith('/actions') && call.body?.includes('project-download'))).toBe(true)
    })
  })

  it('imports a bundled example template into the project library', async () => {
    const calls = installFetchMock({ withTemplateExample: true })
    vi.stubGlobal('prompt', vi.fn(() => '我的站点'))
    render(<App />)
    fireEvent.click(screen.getByText('项目'))
    fireEvent.click(screen.getByText('模板中心'))
    fireEvent.click(await screen.findByText('使用模板'))
    await waitFor(() => {
      expect(calls.some(call => call.url.endsWith('/actions') && call.body?.includes('project-import-example'))).toBe(true)
    })
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

  it('provides network and volume management workspaces', async () => {
    installFetchMock()
    render(<App />)
    fireEvent.click(screen.getByText('网络'))
    expect(await screen.findByPlaceholderText('my-network')).toBeInTheDocument()
    fireEvent.click(screen.getByText('卷'))
    expect(await screen.findByPlaceholderText('my-volume')).toBeInTheDocument()
  })

  it('batches container actions from the selection toolbar', async () => {
    const calls = installFetchMock({ withContainer: true })
    render(<App />)
    fireEvent.click(screen.getByText('容器'))
    fireEvent.click(await screen.findByLabelText('选择 web-1'))
    fireEvent.click(await screen.findByText('批量停止'))
    await waitFor(() => {
      const call = calls.find(item => item.body?.includes('container-batch'))
      expect(call?.body).toContain('"verb":"stop"')
      expect(call?.body).toContain('"containerIDs":"abc123"')
    })
  })

  it('opens a container inspect modal', async () => {
    const calls = installFetchMock({ withContainer: true })
    render(<App />)
    fireEvent.click(screen.getByText('容器'))
    fireEvent.click(await screen.findByText('详情'))
    await waitFor(() => {
      expect(calls.some(call => call.body?.includes('container-inspect'))).toBe(true)
    })
    expect(await screen.findByText(/web-1 · 容器详情/)).toBeInTheDocument()
    expect(screen.getByText(/"name": "web-1"/)).toBeInTheDocument()
  })

  it('tags and prunes images with confirmation', async () => {
    const calls = installFetchMock()
    const confirmMock = vi.fn(() => true)
    window.confirm = confirmMock
    render(<App />)
    fireEvent.click(screen.getByText('镜像'))
    fireEvent.change(await screen.findByPlaceholderText('nginx:latest'), { target: { value: 'nginx:1.27' } })
    fireEvent.change(screen.getByPlaceholderText('registry.example.com/app/nginx:tag'), { target: { value: 'registry.example.com/app/nginx:1.27' } })
    fireEvent.click(screen.getByText('标记'))
    await waitFor(() => {
      expect(calls.some(call => call.body?.includes('image-tag'))).toBe(true)
    })
    await waitFor(() => {
      expect(screen.getByRole('button', { name: '清理镜像' })).not.toBeDisabled()
    })
    fireEvent.click(screen.getByRole('button', { name: '清理镜像' }))
    expect(confirmMock).toHaveBeenCalled()
    await waitFor(() => {
      expect(calls.some(call => call.body?.includes('image-prune'))).toBe(true)
    })
  })

  it('shows compose status and deletes a project after confirmation', async () => {
    const calls = installFetchMock({ withProject: true })
    const confirmMock = vi.fn(() => true)
    window.confirm = confirmMock
    render(<App />)
    fireEvent.click(screen.getByText('项目'))
    fireEvent.click(await screen.findByText('Demo'))
    fireEvent.click(await screen.findByText('状态'))
    expect(await screen.findByText('demo-web-1')).toBeInTheDocument()
    fireEvent.click(screen.getByText('删除项目'))
    expect(confirmMock).toHaveBeenCalled()
    await waitFor(() => {
      expect(calls.some(call => call.body?.includes('project-delete'))).toBe(true)
    })
  })

  it('creates a container from the form', async () => {
    const calls = installFetchMock()
    render(<App />)
    fireEvent.click(screen.getByText('容器'))
    fireEvent.change(await screen.findByPlaceholderText('镜像:标签'), { target: { value: 'nginx:latest' } })
    fireEvent.change(screen.getByPlaceholderText('my-container'), { target: { value: 'web' } })
    fireEvent.change(screen.getByPlaceholderText('8080:80'), { target: { value: '8080:80' } })
    fireEvent.click(screen.getByRole('button', { name: '创建容器' }))
    await waitFor(() => {
      const call = calls.find(item => item.body?.includes('container-create'))
      expect(call?.body).toContain('"image":"nginx:latest"')
      expect(call?.body).toContain('"name":"web"')
      expect(call?.body).toContain('"ports":"8080:80"')
    })
  })

  it('polls live stats in the container detail modal', async () => {
    const calls = installFetchMock({ withContainer: true })
    render(<App />)
    fireEvent.click(screen.getByText('容器'))
    fireEvent.click(await screen.findByText('详情'))
    fireEvent.click(await screen.findByText('实时统计'))
    await waitFor(() => {
      expect(calls.some(call => call.body?.includes('container-stats'))).toBe(true)
    })
    await waitFor(() => {
      expect(screen.getByText('0.05%')).toBeInTheDocument()
    }, { timeout: 5000 })
  })

  it('polls incremental logs while live mode is enabled', async () => {
    const calls = installFetchMock({ withContainer: true })
    render(<App />)
    fireEvent.click(screen.getByText('容器'))
    fireEvent.click(await screen.findByText('日志'))
    await waitFor(() => {
      expect(calls.some(call => call.body?.includes('container-logs-since'))).toBe(true)
    }, { timeout: 5000 })
    fireEvent.click(screen.getByText('实时（每 2 秒刷新）'))
    await waitFor(() => {
      expect(calls.filter(call => call.body?.includes('container-logs-since')).length).toBeGreaterThanOrEqual(2)
    }, { timeout: 6000 })
    expect(await screen.findByText(/hello/)).toBeInTheDocument()
  })

  it('logs into and out of a private registry without exposing the password', async () => {
    const calls = installFetchMock({ withRegistry: true })
    const confirmMock = vi.fn(() => true)
    window.confirm = confirmMock
    render(<App />)
    fireEvent.click(screen.getByText('镜像'))
    fireEvent.change(await screen.findByPlaceholderText('registry.example.com:5000'), { target: { value: 'registry.example.com:5000' } })
    fireEvent.change(screen.getByPlaceholderText('用户名'), { target: { value: 'user' } })
    fireEvent.change(screen.getByPlaceholderText('密码'), { target: { value: 'hunter2' } })
    fireEvent.click(screen.getByText('登录仓库'))
    await waitFor(() => {
      const call = calls.find(item => item.body?.includes('registry-login'))
      expect(call?.body).toContain('"password":"hunter2"')
    })
    expect(screen.queryByText('hunter2')).not.toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByText('退出登录')).not.toBeDisabled()
    })
    fireEvent.click(screen.getByText('退出登录'))
    expect(confirmMock).toHaveBeenCalled()
    await waitFor(() => {
      expect(calls.some(call => call.body?.includes('registry-logout'))).toBe(true)
    })
  })
})
