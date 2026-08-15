import { useEffect, useMemo, useState, type DragEvent, type ReactNode } from 'react'
import { fetchStatus, installDocker, openDesktop, PLUGIN_ID, runActionAndPoll, uploadCompose, type ActionResult } from './api'
import { blankForm, formFor, serviceNames, writeForm, type FormFields } from './composeForm'

type View = 'environment' | 'projects' | 'images' | 'containers'
type Check = { available: boolean; version?: string; detail?: string }
type DockerStatus = { cli: Check; compose: Check; daemon: Check; platform: string }
type ProjectMeta = { id: string; name: string; source: string; createdAt: string; updatedAt: string }
type Project = ProjectMeta & { content: string }
type ImportTarget = { project: ProjectMeta; destination: string }
type DockerImage = { id: string; repository: string; tag: string; size: string; created: string }
type PublishedPort = { hostPort: number; containerPort: number; protocol: string }
type DockerContainer = { id: string; names: string; image: string; state: string; status: string; ports: string; labels: string; project?: string; published?: PublishedPort[] }

function textError(reason: unknown) { return reason instanceof Error ? reason.message : String(reason) }

// cleanRoute normalises a user-supplied route into a safe single path segment
// list. It strips query/hash and rejects any .. traversal; the host validates
// the same rules again before proxying.
function cleanRoute(route: string): string {
  const clean = route.trim().replace(/\\/g, '/').split(/[?#]/)[0]
  if (clean === '' || clean === '/') return '/'
  const value = clean.startsWith('/') ? clean : '/' + clean
  if (value.split('/').includes('..')) return ''
  return value
}

// dynamicWebURL builds the same-origin proxy mount for a Docker-published
// loopback port (host: /api/v1/services/dynamic/<plugin>/<port>/<route>).
function dynamicWebURL(hostPort: number, route: string): string {
  const path = cleanRoute(route) || '/'
  return `/api/v1/services/dynamic/${PLUGIN_ID}/${hostPort}${path}`
}

function Icon({ name }: { name: View | 'refresh' | 'chevron' | 'back' }) {
  const paths: Record<string, ReactNode> = {
    environment: <><path d="M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18Z" /><path d="M12 3v9l5.2 3" /></>,
    projects: <><path d="m12 2.8 9 5-9 5-9-5 9-5Z" /><path d="m3.5 12.3 8.5 4.8 8.5-4.8" /><path d="m3.5 16.3 8.5 4.8 8.5-4.8" /></>,
    images: <><rect x="3.5" y="4.5" width="17" height="15" rx="2" /><circle cx="9" cy="10" r="1.6" /><path d="m5.5 17 4-3.8 2.7 2.6 3-2.8 3.3 3.2" /></>,
    containers: <><path d="M12 2.5 20 7v10l-8 4.5L4 17V7l8-4.5Z" /><path d="M4 7l8 4.5L20 7M12 11.5V21.5" /></>,
    refresh: <><path d="M20 11a8.1 8.1 0 0 0-14.8-3.4L3 10" /><path d="M3 4v6h6M4 13a8.1 8.1 0 0 0 14.8 3.4L21 14" /><path d="M21 20v-6h-6" /></>,
    chevron: <path d="m9 18 6-6-6-6" />,
    back: <path d="m15 18-6-6 6-6" />
  }
  return <svg aria-hidden="true" className="ui-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">{paths[name]}</svg>
}

function StatusDot({ ok }: { ok: boolean }) { return <span className={ok ? 'status-dot status-dot--good' : 'status-dot status-dot--quiet'} /> }

function SettingGroup({ title, description, children }: { title: string; description?: string; children: ReactNode }) {
  return <section className="setting-group"><div className="setting-group__head"><h2>{title}</h2>{description && <p>{description}</p>}</div>{children}</section>
}

function Field({ label, value, onChange, placeholder, type = 'text', hint }: { label: string; value: string; onChange: (value: string) => void; placeholder?: string; type?: string; hint?: string }) {
  return <label className="form-field"><span>{label}</span><input type={type} value={value} placeholder={placeholder} onChange={event => onChange(event.target.value)} />{hint && <small>{hint}</small>}</label>
}

function Area({ label, value, onChange, placeholder, hint }: { label: string; value: string; onChange: (value: string) => void; placeholder?: string; hint?: string }) {
  return <label className="form-field"><span>{label}</span><textarea value={value} placeholder={placeholder} onChange={event => onChange(event.target.value)} />{hint && <small>{hint}</small>}</label>
}

function CheckRow({ label, check }: { label: string; check: Check }) {
  return <div className="interface-row"><div><h3><StatusDot ok={check.available} />{label}</h3><p>{check.available ? check.version || '可用' : check.detail || '不可用'}</p></div><span>{check.available ? '可用' : '异常'}</span></div>
}

export default function App() {
  const [view, setView] = useState<View>('environment')
  const [status, setStatus] = useState<DockerStatus>()
  const [projects, setProjects] = useState<ProjectMeta[]>([])
  const [images, setImages] = useState<DockerImage[]>([])
  const [containers, setContainers] = useState<DockerContainer[]>([])
  const [selected, setSelected] = useState<Project>()
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState<{ text: string; error?: boolean }>()

  const refresh = async () => {
    setBusy(true)
    try {
      const [nextStatus, nextProjects, nextImages, nextContainers] = await Promise.all([
        fetchStatus<DockerStatus>('docker-status'),
        fetchStatus<ProjectMeta[]>('project-list'),
        fetchStatus<DockerImage[]>('image-list').catch(() => []),
        fetchStatus<DockerContainer[]>('container-list').catch(() => [])
      ])
      setStatus(nextStatus); setProjects(nextProjects); setImages(nextImages); setContainers(nextContainers)
    } catch (reason) { setMessage({ text: textError(reason), error: true }) } finally { setBusy(false) }
  }
  useEffect(() => { void refresh() }, [])
  const run = async <T,>(action: string, params: Record<string, string> = {}, success?: (result: ActionResult<T>) => void) => {
    setBusy(true)
    try {
      const result = await runActionAndPoll<T>(action, params)
      if (result.error) throw new Error(result.error)
      setMessage({ text: result.output || '操作已完成。' })
      success?.(result)
      await refresh()
    } catch (reason) { setMessage({ text: textError(reason), error: true }) } finally { setBusy(false) }
  }
  const openProject = (id: string) => void run<Project>('project-read', { projectID: id }, result => setSelected(result.data))
  const healthy = Boolean(status && status.cli.available && status.compose.available && status.daemon.available)
  const navigation: Array<[View, string]> = [['environment', '环境'], ['projects', '项目'], ['images', '镜像'], ['containers', '容器']]

  return <main className="settings-shell">
    <aside className="settings-sidebar" aria-label="Docker 管理分类">
      <nav aria-label="Docker 管理" role="tablist">{navigation.map(([id, label]) => <button key={id} id={`docker-tab-${id}`} role="tab" aria-selected={view === id} aria-controls={`docker-panel-${id}`} onClick={() => { setView(id); if (id !== 'projects') setSelected(undefined) }} className={view === id ? 'nav-item nav-item--active' : 'nav-item'}><Icon name={id} /><span>{label}</span></button>)}</nav>
      <div className="sidebar-footer"><button className="sidebar-action" disabled={busy} onClick={() => void refresh()}><Icon name="refresh" />刷新状态</button><small><StatusDot ok={healthy} />{status?.platform ?? '正在检测 Docker'}</small></div>
    </aside>
    <section className="settings-content" id={`docker-panel-${view}`} role="tabpanel" aria-labelledby={`docker-tab-${view}`}>
      <div className="settings-panel-content">
        {message && <div role="status" className={message.error ? 'notice notice--error' : 'notice'}>{message.text}</div>}
        {view === 'environment' && <Environment status={status} busy={busy} onInstall={async () => { setBusy(true); try { setMessage({ text: await installDocker() }); await refresh() } catch (reason) { setMessage({ text: textError(reason), error: true }) } finally { setBusy(false) } }} />}
        {view === 'projects' && <Projects projects={projects} selected={selected} busy={busy} onOpen={openProject} onRun={run} onSelect={setSelected} />}
        {view === 'images' && <Images images={images} busy={busy} onRun={run} />}
        {view === 'containers' && <Containers containers={containers} busy={busy} onRun={run} />}
      </div>
    </section>
  </main>
}

function Environment({ status, busy, onInstall }: { status?: DockerStatus; busy: boolean; onInstall: () => Promise<void> }) {
  const needsInstall = Boolean(status && (!status.cli.available || !status.compose.available))
  return <div className="settings-stack">
    <SettingGroup title="运行环境" description="Docker CLI、Compose 插件与守护进程分别检测，任一缺失都会影响容器操作。">
      <div className="interface-list">
        <CheckRow label="Docker CLI" check={status?.cli || { available: false, detail: '正在检测' }} />
        <CheckRow label="Docker Compose" check={status?.compose || { available: false, detail: '正在检测' }} />
        <CheckRow label="Docker 守护进程" check={status?.daemon || { available: false, detail: '正在检测' }} />
      </div>
    </SettingGroup>
    {needsInstall && <SettingGroup title="安装 Docker" description="首版不会替你启动 Docker Desktop、Colima 或系统守护进程。">
      <div className="unavailable-row install-row"><span>将调用 ALemonX 的固定系统安装流程；安装完成后请启动本机 Docker 运行时，再刷新此页。</span><button className="secondary-button" disabled={busy} onClick={() => void onInstall()}>{busy ? '正在安装…' : '安装 Docker'}</button></div>
    </SettingGroup>}
    {status && status.cli.available && !status.daemon.available && <div className="notice notice--warning">Docker 已安装，但守护进程不可用。请启动本机 Docker 运行时，或检查当前账户是否有 Docker socket 权限。</div>}
  </div>
}

function Projects({ projects, selected, busy, onOpen, onRun, onSelect }: { projects: ProjectMeta[]; selected?: Project; busy: boolean; onOpen: (id: string) => void; onRun: <T>(action: string, params?: Record<string, string>, success?: (result: ActionResult<T>) => void) => void; onSelect: (project?: Project) => void }) {
  const [section, setSection] = useState<'list' | 'create'>('list')
  const [name, setName] = useState('')
  const [url, setURL] = useState('')
  const selectedID = selected?.id
  const create = () => { if (name.trim()) onRun<Project>('project-create', { name }, result => { setName(''); onSelect(result.data); setSection('list') }) }
  const download = () => { if (name.trim() && url.trim()) onRun<Project>('project-download', { name, url }, result => { setName(''); setURL(''); onSelect(result.data); setSection('list') }) }
  const importFile = async (file: File) => {
    if (!name.trim()) return alert('请先填写项目名称。')
    try {
      const created = await runActionAndPoll<Project>('project-create', { name })
      if (created.error || !created.data) throw new Error(created.error || '无法创建项目')
      const target = await runActionAndPoll<ImportTarget>('project-import-target', { projectID: created.data.id })
      if (target.error || !target.data) throw new Error(target.error || '无法准备导入目录')
      const result = await uploadCompose(target.data.destination, file)
      if (result.error) throw new Error(result.error)
      setName(''); onOpen(created.data.id); setSection('list')
    } catch (reason) { alert(textError(reason)) }
  }
  return <div className="settings-stack">
    <div className="tabs">
      <button className={section === 'list' ? 'active' : ''} onClick={() => setSection('list')}>项目列表</button>
      <button className={section === 'create' ? 'active' : ''} onClick={() => setSection('create')}>新建项目</button>
    </div>
    {section === 'create' ? <div className="settings-stack">
      <div className="action-card"><div className="action-card__form">
        <div className="form-grid"><Field label="项目名称" value={name} onChange={setName} placeholder="项目名称" hint="项目 ID 仅由安全字符生成" /></div>
        <div className="button-row">
          <button className="secondary-button" disabled={busy || !name.trim()} onClick={create}>新建空项目</button>
          <label className="drop-zone" onDragOver={event => event.preventDefault()} onDrop={(event: DragEvent) => { event.preventDefault(); const file = event.dataTransfer.files[0]; if (file) void importFile(file) }}>
            <input type="file" accept=".yml,.yaml" onChange={event => { const file = event.target.files?.[0]; if (file) void importFile(file) }} />拖入或选择 Compose 文件
          </label>
        </div>
        <div className="url-import"><input value={url} onChange={event => setURL(event.target.value)} placeholder="https://…/docker-compose.yml" /><button className="secondary-button" disabled={busy || !name.trim() || !url.trim()} onClick={download}>从 URL 下载</button></div>
      </div></div>
    </div> : selected ? <div className="settings-stack">
      <div className="back-row"><button className="text-button" onClick={() => onSelect(undefined)}><Icon name="back" />返回列表</button></div>
      <SettingGroup title="项目编辑" description="表单只更新受管字段，保留注释与未知扩展；高级 YAML 可编辑完整文件。"><ProjectEditor project={selected} busy={busy} onRun={onRun} onSelect={onSelect} /></SettingGroup>
    </div> : <div className="settings-stack">
      <SettingGroup title="项目列表" description="选择项目进行编辑，或执行启动、停止、重启与关闭。">
        <div className="interface-list">
          {projects.length ? projects.map(project => <button key={project.id} className={selectedID === project.id ? 'interface-row project-row--active' : 'interface-row'} onClick={() => onOpen(project.id)}><div><h3>{project.name}</h3><p>{project.source} · 更新于 {new Date(project.updatedAt).toLocaleString()}</p></div><span><Icon name="chevron" /></span></button>) : <div className="unavailable-row">尚未创建项目。</div>}
        </div>
      </SettingGroup>
    </div>}
  </div>
}

function ProjectEditor({ project, busy, onRun, onSelect }: { project: Project; busy: boolean; onRun: <T>(action: string, params?: Record<string, string>, success?: (result: ActionResult<T>) => void) => void; onSelect: (project: Project) => void }) {
  const [mode, setMode] = useState<'form' | 'yaml'>('form')
  const [content, setContent] = useState(project.content)
  const services = useMemo(() => serviceNames(content), [content])
  const [service, setService] = useState(services[0] || '')
  const [fields, setFields] = useState<FormFields>(() => formFor(content, services[0] || ''))
  useEffect(() => { setContent(project.content); const names = serviceNames(project.content); setService(names[0] || ''); setFields(formFor(project.content, names[0] || '')) }, [project.id, project.content])
  useEffect(() => { if (service) setFields(formFor(content, service)) }, [service])
  const saveYAML = () => onRun<Project>('project-save', { projectID: project.id, content }, result => { if (result.data) onSelect(result.data) })
  const saveForm = () => {
    try {
      const next = writeForm(content, service, fields)
      setContent(next)
      onRun<Project>('project-save', { projectID: project.id, content: next }, result => { if (result.data) onSelect(result.data) })
    } catch (reason) { alert(textError(reason)) }
  }
  const addService = () => {
    const next = window.prompt('服务名称')
    if (!next?.trim()) return
    if (services.includes(next.trim())) return alert('服务已存在。')
    try {
      const nextContent = writeForm(content, next.trim(), blankForm)
      setContent(nextContent); setService(next.trim()); setFields(blankForm)
    } catch (reason) { alert(textError(reason)) }
  }
  const lifecycle = (action: 'compose-up' | 'compose-stop' | 'compose-restart' | 'compose-down') => {
    if (action === 'compose-down' && !window.confirm('关闭项目会停止并移除其容器和网络，但不会删除卷或镜像。继续吗？')) return
    onRun(action, { projectID: project.id })
  }
  const openFolder = () => onRun<ImportTarget>('project-import-target', { projectID: project.id }, result => { if (result.data) void openDesktop(result.data.destination).catch(error => alert(textError(error))) })
  return <div className="project-editor">
    <div className="editor-head"><div><strong className="editor-title">{project.name}</strong><span className="editor-subtitle">{project.source}</span></div><div className="button-row">
      <button className="text-button" disabled={busy} onClick={openFolder}>打开目录</button>
      <button className="secondary-button" disabled={busy} onClick={() => lifecycle('compose-up')}>启动</button>
      <button className="secondary-button" disabled={busy} onClick={() => lifecycle('compose-stop')}>停止</button>
      <button className="secondary-button" disabled={busy} onClick={() => lifecycle('compose-restart')}>重启</button>
      <button className="danger-button" disabled={busy} onClick={() => lifecycle('compose-down')}>关闭</button>
    </div></div>
    <div className="tabs"><button className={mode === 'form' ? 'active' : ''} onClick={() => setMode('form')}>表单</button><button className={mode === 'yaml' ? 'active' : ''} onClick={() => setMode('yaml')}>高级 YAML</button></div>
    {mode === 'yaml' ? <div className="editor">
      <textarea value={content} spellCheck={false} onChange={event => setContent(event.target.value)} />
      <div><button className="secondary-button" disabled={busy} onClick={saveYAML}>验证并保存 YAML</button></div>
    </div> : <div className="form-editor">
      <div className="service-tabs">{services.map(name => <button key={name} className={service === name ? 'active' : ''} onClick={() => setService(name)}>{name}</button>)}<button onClick={addService}>+ 服务</button></div>
      {service ? <>
        <div className="form-grid">
          <Field label="镜像" value={fields.image} onChange={image => setFields({ ...fields, image })} placeholder="nginx:latest" />
          <Field label="构建目录" value={fields.build} onChange={build => setFields({ ...fields, build })} placeholder="./app" />
          <Field label="命令" value={fields.command} onChange={command => setFields({ ...fields, command })} />
          <Field label="重启策略" value={fields.restart} onChange={restart => setFields({ ...fields, restart })} placeholder="unless-stopped" />
          <Area label="端口（每行一项）" value={fields.ports} onChange={ports => setFields({ ...fields, ports })} placeholder="8080:80" />
          <Area label="环境变量（每行 KEY=value）" value={fields.environment} onChange={environment => setFields({ ...fields, environment })} />
          <Area label="卷（每行一项）" value={fields.volumes} onChange={volumes => setFields({ ...fields, volumes })} />
          <Area label="依赖服务（每行一项）" value={fields.dependsOn} onChange={dependsOn => setFields({ ...fields, dependsOn })} />
        </div>
        <div className="form-grid">
          <Area label="顶层网络（每行一项）" value={fields.topNetworks} onChange={topNetworks => setFields({ ...fields, topNetworks })} placeholder="frontend" />
          <Area label="顶层卷（每行一项）" value={fields.topVolumes} onChange={topVolumes => setFields({ ...fields, topVolumes })} placeholder="db-data" />
        </div>
        <div><button className="secondary-button" disabled={busy} onClick={saveForm}>保存表单更改</button></div>
      </> : <div className="unavailable-row">此文件还没有服务。点击“+ 服务”开始编辑。</div>}
    </div>}
  </div>
}

function Images({ images, busy, onRun }: { images: DockerImage[]; busy: boolean; onRun: (action: string, params?: Record<string, string>) => void }) {
  const [image, setImage] = useState('')
  return <div className="settings-stack">
    <SettingGroup title="拉取镜像" description="输入镜像:标签，例如 nginx:latest。">
      <div className="action-card"><div className="action-card__form">
        <div className="form-grid"><Field label="镜像引用" value={image} onChange={setImage} placeholder="镜像:标签" hint="仅接受受校验的镜像引用" /></div>
        <div className="button-row"><button className="secondary-button" disabled={busy || !image.trim()} onClick={() => { onRun('image-pull', { image }); setImage('') }}>拉取镜像</button></div>
      </div></div>
    </SettingGroup>
    <SettingGroup title="本地镜像" description="当前 Docker 守护进程缓存的镜像列表。">
      <ResourceTable headers={['镜像', '标签', '大小', '创建时间', '']} empty="没有可显示的镜像。">{images.map(item => <tr key={item.id}>
        <td>{item.repository}</td><td>{item.tag}</td><td>{item.size}</td><td>{item.created}</td>
        <td><button className="danger-button" disabled={busy} onClick={() => { if (window.confirm('删除镜像 ' + item.repository + ':' + item.tag + '？')) onRun('image-remove', { image: item.repository + ':' + item.tag }) }}>删除</button></td>
      </tr>)}</ResourceTable>
    </SettingGroup>
  </div>
}

function Containers({ containers, busy, onRun }: { containers: DockerContainer[]; busy: boolean; onRun: <T>(action: string, params?: Record<string, string>, success?: (result: ActionResult<T>) => void) => void }) {
  const [logs, setLogs] = useState<{ name: string; text: string }>()
  const [web, setWeb] = useState<{ container: DockerContainer; port: PublishedPort }>()
  const [route, setRoute] = useState('')
  const [webURL, setWebURL] = useState('')
  const openWeb = (container: DockerContainer, port: PublishedPort) => { setWeb({ container, port }); setRoute(''); setWebURL(dynamicWebURL(port.hostPort, '/')) }
  const applyRoute = () => { if (!web) return; const path = cleanRoute(route); if (!path) return alert('路由无效：不能包含 .. 或反斜杠。'); setWebURL(dynamicWebURL(web.port.hostPort, path)) }
  return <div className="settings-stack">
    <SettingGroup title="容器列表" description="查看状态、端口与所属 Compose 项目，运行中的容器排在最前。">
      <ResourceTable headers={['容器', '项目', '镜像', '状态', '端口', '操作']} empty="没有可显示的容器。">{containers.map(item => <tr key={item.id}>
        <td><strong>{item.names}</strong><small>{item.id}</small></td>
        <td>{item.project || '—'}</td>
        <td>{item.image}</td>
        <td><span className={item.state === 'running' ? 'badge badge--good' : 'badge'}>{item.status}</span></td>
        <td>{item.ports || '—'}</td>
        <td className="button-row">
          {item.published?.length ? <button className="text-button" disabled={busy} onClick={() => openWeb(item, item.published![0])}>打开页面</button> : null}
          <button className="text-button" disabled={busy} onClick={() => onRun('container-start', { containerID: item.id })}>启动</button>
          <button className="text-button" disabled={busy} onClick={() => onRun('container-stop', { containerID: item.id })}>停止</button>
          <button className="text-button" disabled={busy} onClick={() => onRun('container-restart', { containerID: item.id })}>重启</button>
          <button className="text-button" disabled={busy} onClick={() => onRun<unknown>('container-logs', { containerID: item.id }, result => setLogs({ name: item.names, text: result.output }))}>日志</button>
        </td>
      </tr>)}</ResourceTable>
    </SettingGroup>
    {web && <SettingGroup title="容器页面" description="通过宿主同源代理内嵌容器 Web UI，可指定访问路由；部分应用可能拒绝被内嵌，可改用浏览器打开。">
      <div className="webview-card">
        <div className="webview-head">
          <div className="webview-title"><strong className="editor-title">{web.container.names}</strong><span className="editor-subtitle">宿主端口 {web.port.hostPort} → 容器 {web.port.containerPort}/{web.port.protocol}</span></div>
          <div className="button-row">
            {web.container.published && web.container.published.length > 1 ? <select className="route-select" value={web.port.hostPort} onChange={event => { const next = web.container.published!.find(port => String(port.hostPort) === event.target.value); if (next) { setWeb({ ...web, port: next }); setWebURL(dynamicWebURL(next.hostPort, route)) } }}>
              {web.container.published.map(port => <option key={`${port.hostPort}-${port.protocol}`} value={port.hostPort}>{port.hostPort}（容器 {port.containerPort}/{port.protocol}）</option>)}
            </select> : null}
            <input className="route-input" value={route} placeholder="指定路由，例如 /login" onChange={event => setRoute(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') applyRoute() }} />
            <button className="secondary-button" onClick={applyRoute}>打开路由</button>
            <button className="text-button" onClick={() => void openDesktop('http://127.0.0.1:' + web.port.hostPort + (cleanRoute(route) || '/')).catch(error => alert(textError(error)))}>浏览器打开</button>
            <button className="text-button" onClick={() => setWeb(undefined)}>关闭</button>
          </div>
        </div>
        <iframe className="webview-frame" src={webURL} title={`${web.container.names} 容器页面`} />
      </div>
    </SettingGroup>}
    {logs && <div className="modal-backdrop"><section className="modal-card">
      <p className="eyebrow">容器日志</p>
      <h2>{logs.name}</h2>
      <p className="modal-card__lead">最近 200 行日志，输出过长时自动截断。</p>
      <pre className="modal-card__log">{logs.text || '容器未输出日志。'}</pre>
      <div className="modal-actions"><button className="secondary-button" onClick={() => setLogs(undefined)}>关闭</button></div>
    </section></div>}
  </div>
}

function ResourceTable({ headers, empty, children }: { headers: string[]; empty: string; children: ReactNode }) {
  const hasRows = Array.isArray(children) ? children.length > 0 : Boolean(children)
  return <div className="table-wrap"><table className="resource-table"><thead><tr>{headers.map(header => <th key={header}>{header}</th>)}</tr></thead><tbody>{hasRows ? children : <tr><td className="empty" colSpan={headers.length}>{empty}</td></tr>}</tbody></table></div>
}
