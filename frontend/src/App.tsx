import { useEffect, useMemo, useRef, useState, type DragEvent, type ReactNode } from 'react'
import { fetchStatus, installDocker, openDesktop, PLUGIN_ID, runActionAndPoll, runActionOnce, uploadCompose, type ActionResult } from './api'
import { blankForm, formFor, serviceNames, writeForm, type FormFields } from './composeForm'

type View = 'environment' | 'projects' | 'images' | 'containers' | 'networks' | 'volumes'
type Check = { available: boolean; version?: string; detail?: string }
type DockerStatus = { cli: Check; compose: Check; daemon: Check; platform: string }
type ProjectMeta = { id: string; name: string; source: string; createdAt: string; updatedAt: string }
type Project = ProjectMeta & { content: string }
type ImportTarget = { project: ProjectMeta; destination: string }
type DockerImage = { id: string; repository: string; tag: string; size: string; created: string }
type PublishedPort = { hostPort: number; containerPort: number; protocol: string }
type DockerContainer = { id: string; names: string; image: string; state: string; status: string; ports: string; labels: string; project?: string; published?: PublishedPort[] }
type DockerNetwork = { id?: string; name: string; driver?: string; scope?: string; createdAt?: string }
type DockerVolume = { name: string; driver?: string; scope?: string; mountpoint?: string; createdAt?: string }
type ComposeServiceStatus = { id?: string; name: string; service: string; state: string; status?: string; image?: string }
type ExternalProject = { project: string; workingDir?: string; configFiles?: string; containerCount: number; runningCount: number; managed: boolean }
type DockerStat = { id?: string; name?: string; cpuPerc?: string; memUsage?: string; memPerc?: string; netIO?: string; blockIO?: string; pids?: string }
type Recommendation = { id: string; name: string; description?: string; tags?: string[]; url?: string; example?: string }
type RegistryEntry = { registry: string; externalKey?: boolean }

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

function Icon({ name }: { name: View | 'refresh' | 'chevron' | 'back' | 'recommend' }) {
  const paths: Record<string, ReactNode> = {
    environment: <><path d="M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18Z" /><path d="M12 3v9l5.2 3" /></>,
    projects: <><path d="m12 2.8 9 5-9 5-9-5 9-5Z" /><path d="m3.5 12.3 8.5 4.8 8.5-4.8" /><path d="m3.5 16.3 8.5 4.8 8.5-4.8" /></>,
    images: <><rect x="3.5" y="4.5" width="17" height="15" rx="2" /><circle cx="9" cy="10" r="1.6" /><path d="m5.5 17 4-3.8 2.7 2.6 3-2.8 3.3 3.2" /></>,
    containers: <><path d="M12 2.5 20 7v10l-8 4.5L4 17V7l8-4.5Z" /><path d="M4 7l8 4.5L20 7M12 11.5V21.5" /></>,
    networks: <><circle cx="6" cy="6" r="2.5" /><circle cx="18" cy="6" r="2.5" /><circle cx="12" cy="18" r="2.5" /><path d="m8.2 7.5 2.6 8M15.8 7.5l-2.6 8M8.5 6h7" /></>,
    volumes: <><ellipse cx="12" cy="6" rx="7" ry="3" /><path d="M5 6v6c0 1.7 3.1 3 7 3s7-1.3 7-3V6" /><path d="M5 12v6c0 1.7 3.1 3 7 3s7-1.3 7-3v-6" /></>,
    refresh: <><path d="M20 11a8.1 8.1 0 0 0-14.8-3.4L3 10" /><path d="M3 4v6h6M4 13a8.1 8.1 0 0 0 14.8 3.4L21 14" /><path d="M21 20v-6h-6" /></>,
    chevron: <path d="m9 18 6-6-6-6" />,
    back: <path d="m15 18-6-6 6-6" />,
    recommend: <path d="M12 3.5 14.8 9l6 .9-4.4 4.2 1.1 6L12 17.4 6.5 20l1.1-6L3.2 9.9 9.2 9 12 3.5Z" />
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
  const [networks, setNetworks] = useState<DockerNetwork[]>([])
  const [volumes, setVolumes] = useState<DockerVolume[]>([])
  const [externalProjects, setExternalProjects] = useState<ExternalProject[]>([])
  const [recommendations, setRecommendations] = useState<Recommendation[]>([])
  const [registries, setRegistries] = useState<RegistryEntry[]>([])
  const [selected, setSelected] = useState<Project>()
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState<{ text: string; error?: boolean }>()

  const refresh = async () => {
    setBusy(true)
    try {
      const [nextStatus, nextProjects, nextImages, nextContainers, nextNetworks, nextVolumes, nextExternal, nextRecommendations, nextRegistries] = await Promise.all([
        fetchStatus<DockerStatus>('docker-status'),
        fetchStatus<ProjectMeta[]>('project-list'),
        fetchStatus<DockerImage[]>('image-list').catch(() => []),
        fetchStatus<DockerContainer[]>('container-list').catch(() => []),
        fetchStatus<DockerNetwork[]>('network-list').catch(() => []),
        fetchStatus<DockerVolume[]>('volume-list').catch(() => []),
        fetchStatus<ExternalProject[]>('external-projects').catch(() => []),
        fetchStatus<Recommendation[]>('recommendations').catch(() => []),
        fetchStatus<RegistryEntry[]>('registry-list').catch(() => [])
      ])
      setStatus(nextStatus); setProjects(nextProjects); setImages(nextImages); setContainers(nextContainers); setNetworks(nextNetworks); setVolumes(nextVolumes); setExternalProjects(nextExternal); setRecommendations(nextRecommendations); setRegistries(nextRegistries)
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
  const navigation: Array<[View, string]> = [['environment', '环境'], ['projects', '项目'], ['images', '镜像'], ['containers', '容器'], ['networks', '网络'], ['volumes', '卷']]

  return <main className="settings-shell">
    <aside className="settings-sidebar" aria-label="Docker 管理分类">
      <nav aria-label="Docker 管理" role="tablist">{navigation.map(([id, label]) => <button key={id} id={`docker-tab-${id}`} role="tab" aria-selected={view === id} aria-controls={`docker-panel-${id}`} onClick={() => { setView(id); if (id !== 'projects') setSelected(undefined) }} className={view === id ? 'nav-item nav-item--active' : 'nav-item'}><Icon name={id} /><span>{label}</span></button>)}</nav>
      <div className="sidebar-footer"><button className="sidebar-action" disabled={busy} onClick={() => void refresh()}><Icon name="refresh" />刷新状态</button><small><StatusDot ok={healthy} />{status?.platform ?? '正在检测 Docker'}</small></div>
    </aside>
    <section className="settings-content" id={`docker-panel-${view}`} role="tabpanel" aria-labelledby={`docker-tab-${view}`}>
      <div className="settings-panel-content">
        {message && <div role="status" className={message.error ? 'notice notice--error' : 'notice'}>{message.text}</div>}
        {view === 'environment' && <Environment status={status} busy={busy} onInstall={async () => { setBusy(true); try { setMessage({ text: await installDocker() }); await refresh() } catch (reason) { setMessage({ text: textError(reason), error: true }) } finally { setBusy(false) } }} />}
        {view === 'projects' && <Projects projects={projects} external={externalProjects} recommendations={recommendations} selected={selected} busy={busy} onOpen={openProject} onRun={run} onSelect={setSelected} />}
        {view === 'images' && <Images images={images} registries={registries} busy={busy} onRun={run} />}
        {view === 'containers' && <Containers containers={containers} busy={busy} onRun={run} />}
        {view === 'networks' && <Networks networks={networks} busy={busy} onRun={run} />}
        {view === 'volumes' && <Volumes volumes={volumes} busy={busy} onRun={run} />}
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

function Projects({ projects, external, recommendations, selected, busy, onOpen, onRun, onSelect }: { projects: ProjectMeta[]; external: ExternalProject[]; recommendations: Recommendation[]; selected?: Project; busy: boolean; onOpen: (id: string) => void; onRun: <T>(action: string, params?: Record<string, string>, success?: (result: ActionResult<T>) => void) => void; onSelect: (project?: Project) => void }) {
  const [section, setSection] = useState<'list' | 'create' | 'recommend'>('list')
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
  const downloadRecommendation = (item: Recommendation) => {
    const name = window.prompt('项目名称（默认使用推荐名称）', item.name)
    if (!name?.trim()) return
    if (item.url) {
      onRun<Project>('project-download', { name: name.trim(), url: item.url }, result => { setSection('list'); onSelect(result.data) })
    } else {
      onRun<Project>('project-import-example', { name: name.trim(), example: item.example || '' }, result => { setSection('list'); onSelect(result.data) })
    }
  }
  return <div className="settings-stack">
    <div className="tabs">
      <button className={section === 'list' ? 'active' : ''} onClick={() => setSection('list')}>项目列表</button>
      <button className={section === 'create' ? 'active' : ''} onClick={() => setSection('create')}>新建项目</button>
      <button className={section === 'recommend' ? 'active' : ''} onClick={() => setSection('recommend')}>模板中心</button>
    </div>
    {section === 'recommend' ? <Recommendations recommendations={recommendations} busy={busy} onDownload={downloadRecommendation} /> : section === 'create' ? <div className="settings-stack">
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
      <SettingGroup title="项目列表" description="选择项目查看详情，或执行启动、停止、重启、关闭、状态、日志与 .env 管理。">
        <div className="interface-list">
          {projects.length ? projects.map(project => <button key={project.id} className={selectedID === project.id ? 'interface-row project-row--active' : 'interface-row'} onClick={() => onOpen(project.id)}><div><h3>{project.name}</h3><p>{project.source} · 更新于 {new Date(project.updatedAt).toLocaleString()}</p></div><span><Icon name="chevron" /></span></button>) : <div className="unavailable-row">尚未创建项目。</div>}
        </div>
      </SettingGroup>
      <SettingGroup title="宿主机外部 Compose 项目" description="通过 com.docker.compose.project 标签发现宿主机上的 Compose 项目；受管库外的项目仅展示，可通过文件或 URL 导入受管库。">
        <div className="interface-list">
          {external.length ? external.map(item => <div key={item.project} className="interface-row"><div><h3>{item.project}{item.managed ? <span className="badge badge--managed">受管库</span> : <span className="badge">外部</span>}</h3><p>{item.containerCount} 个容器（{item.runningCount} 运行中）{item.workingDir ? ' · ' + item.workingDir : ''}</p></div><span>{item.configFiles || ''}</span></div>) : <div className="unavailable-row">没有发现带 Compose 标签的容器。</div>}
        </div>
      </SettingGroup>
    </div>}
  </div>
}

function Recommendations({ recommendations, busy, onDownload }: { recommendations: Recommendation[]; busy: boolean; onDownload: (item: Recommendation) => void }) {
  return <div className="settings-stack">
    <SettingGroup title="模板中心" description="从内置示例或在线 Compose 模板创建受管项目；创建后可在项目详情中继续编辑与运行。">
      <div className="interface-list">
        {recommendations.length ? recommendations.map(item => <div key={item.id} className="interface-row"><div><h3>{item.name}<span className={item.example ? 'badge' : 'badge badge--managed'}>{item.example ? '内置示例' : '在线地址'}</span></h3><p>{item.description || '（无描述）'}{item.tags?.length ? ' · ' + item.tags.join(' / ') : ''}</p></div><span className="button-row"><button className="secondary-button" disabled={busy} onClick={() => onDownload(item)}>使用模板</button></span></div>) : <div className="unavailable-row">暂无可用的模板。</div>}
      </div>
    </SettingGroup>
  </div>
}

function ProjectEditor({ project, busy, onRun, onSelect }: { project: Project; busy: boolean; onRun: <T>(action: string, params?: Record<string, string>, success?: (result: ActionResult<T>) => void) => void; onSelect: (project?: Project) => void }) {
  const [mode, setMode] = useState<'form' | 'yaml'>('form')
  const [content, setContent] = useState(project.content)
  const [ps, setPS] = useState<ComposeServiceStatus[]>()
  const [logs, setLogs] = useState<string>()
  const [env, setEnv] = useState<{ content: string; dirty: boolean }>()
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
  const openPS = () => onRun<ComposeServiceStatus[]>('compose-ps', { projectID: project.id }, result => setPS(result.data || []))
  const openLogs = () => onRun<unknown>('compose-logs', { projectID: project.id, lines: '200' }, result => setLogs(result.output))
  const openEnv = () => onRun<{ content: string }>('compose-env-read', { projectID: project.id }, result => setEnv({ content: result.data?.content || '', dirty: false }))
  const saveEnv = () => { if (!env) return; onRun('compose-env-write', { projectID: project.id, content: env.content }, () => setEnv({ content: env.content, dirty: false })) }
  const removeProject = () => {
    if (!window.confirm('删除项目“' + project.name + '”会移除受管目录中的 docker-compose.yml 与 .env，无法恢复；运行中的容器不受影响。继续吗？')) return
    onRun('project-delete', { projectID: project.id }, () => onSelect(undefined))
  }
  return <div className="project-editor">
    <div className="editor-head"><div><strong className="editor-title">{project.name}</strong><span className="editor-subtitle">{project.source}</span></div><div className="button-row">
      <button className="text-button" disabled={busy} onClick={openPS}>状态</button>
      <button className="text-button" disabled={busy} onClick={openLogs}>日志</button>
      <button className="text-button" disabled={busy} onClick={openEnv}>.env</button>
      <button className="text-button" disabled={busy} onClick={openFolder}>打开目录</button>
      <button className="secondary-button" disabled={busy} onClick={() => lifecycle('compose-up')}>启动</button>
      <button className="secondary-button" disabled={busy} onClick={() => lifecycle('compose-stop')}>停止</button>
      <button className="secondary-button" disabled={busy} onClick={() => lifecycle('compose-restart')}>重启</button>
      <button className="danger-button" disabled={busy} onClick={() => lifecycle('compose-down')}>关闭</button>
      <button className="danger-button" disabled={busy} onClick={removeProject}>删除项目</button>
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
    {ps !== undefined && <div className="modal-backdrop"><section className="modal-card modal-card--wide">
      <p className="eyebrow">项目状态</p>
      <h2>{project.name}</h2>
      <p className="modal-card__lead">docker compose ps 展示的服务运行状态。</p>
      <ResourceTable headers={['服务', '容器', '状态', '镜像']} empty="项目当前没有已创建的服务。">{ps.map(row => <tr key={row.id || row.name}>
        <td>{row.service}</td>
        <td><strong>{row.name}</strong>{row.id ? <small>{row.id}</small> : null}</td>
        <td><span className={row.state === 'running' ? 'badge badge--good' : 'badge'}>{row.status || row.state}</span></td>
        <td>{row.image || '—'}</td>
      </tr>)}</ResourceTable>
      <div className="modal-actions"><button className="secondary-button" onClick={() => setPS(undefined)}>关闭</button></div>
    </section></div>}
    {logs !== undefined && <div className="modal-backdrop"><section className="modal-card modal-card--wide">
      <p className="eyebrow">项目日志</p>
      <h2>{project.name}</h2>
      <p className="modal-card__lead">最近 200 行 Compose 日志，输出过长时自动截断。</p>
      <pre className="modal-card__log">{logs || '项目尚未输出日志。'}</pre>
      <div className="modal-actions"><button className="secondary-button" onClick={() => setLogs(undefined)}>关闭</button></div>
    </section></div>}
    {env !== undefined && <div className="modal-backdrop"><section className="modal-card modal-card--wide">
      <p className="eyebrow">环境变量</p>
      <h2>{project.name} · .env</h2>
      <p className="modal-card__lead">每行 KEY=value。保存后写入项目目录的 .env，内容不会回显到任务记录。</p>
      <textarea className="env-editor" spellCheck={false} value={env.content} onChange={event => setEnv({ content: event.target.value, dirty: true })} placeholder="FOO=bar" />
      <div className="modal-actions">
        <button className="secondary-button" disabled={busy || !env.dirty} onClick={saveEnv}>保存 .env</button>
        <button className="text-button" onClick={() => setEnv(undefined)}>关闭</button>
      </div>
    </section></div>}
  </div>
}

function Images({ images, registries, busy, onRun }: { images: DockerImage[]; registries: RegistryEntry[]; busy: boolean; onRun: <T>(action: string, params?: Record<string, string>, success?: (result: ActionResult<T>) => void) => void }) {
  const [image, setImage] = useState('')
  const [tagSource, setTagSource] = useState('')
  const [tagTarget, setTagTarget] = useState('')
  const [pushRef, setPushRef] = useState('')
  const [pruneAll, setPruneAll] = useState(false)
  const [registry, setRegistry] = useState('')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const login = () => { onRun('registry-login', { registry, username, password }); setPassword(''); setRegistry(''); setUsername('') }
  return <div className="settings-stack">
    <SettingGroup title="拉取镜像" description="输入镜像:标签，例如 nginx:latest。">
      <div className="action-card"><div className="action-card__form">
        <div className="form-grid"><Field label="镜像引用" value={image} onChange={setImage} placeholder="镜像:标签" hint="仅接受受校验的镜像引用" /></div>
        <div className="button-row"><button className="secondary-button" disabled={busy || !image.trim()} onClick={() => { onRun('image-pull', { image }); setImage('') }}>拉取镜像</button></div>
      </div></div>
    </SettingGroup>
    <SettingGroup title="镜像操作" description="打标签、推送到仓库，或清理不再使用的镜像。">
      <div className="action-grid">
        <div className="action-card"><div className="action-card__form"><h3>标记镜像</h3>
          <div className="form-grid">
            <Field label="源镜像" value={tagSource} onChange={setTagSource} placeholder="nginx:latest" />
            <Field label="目标标签" value={tagTarget} onChange={setTagTarget} placeholder="registry.example.com/app/nginx:tag" />
          </div>
          <div className="button-row"><button className="secondary-button" disabled={busy || !tagSource.trim() || !tagTarget.trim()} onClick={() => { onRun('image-tag', { image: tagSource, target: tagTarget }); setTagSource(''); setTagTarget('') }}>标记</button></div>
        </div></div>
        <div className="action-card"><div className="action-card__form"><h3>推送镜像</h3>
          <Field label="镜像引用" value={pushRef} onChange={setPushRef} placeholder="registry.example.com/app/nginx:v1" />
          <div className="button-row"><button className="secondary-button" disabled={busy || !pushRef.trim()} onClick={() => { onRun('image-push', { image: pushRef }); setPushRef('') }}>推送</button></div>
        </div></div>
      </div>
      <div className="action-card"><div className="action-card__form">
        <h3>清理镜像</h3>
        <label className="check-row"><input type="checkbox" checked={pruneAll} onChange={event => setPruneAll(event.target.checked)} />同时清理所有未被容器使用的镜像（默认仅清理悬空镜像）</label>
        <div className="button-row"><button className="danger-button" disabled={busy} onClick={() => {
          if (window.confirm(pruneAll ? '将删除所有未被任何容器使用的镜像，且无法恢复。继续吗？' : '将删除所有悬空镜像（不再被任何镜像引用的旧层）。继续吗？')) onRun('image-prune', pruneAll ? { all: '1' } : {})
        }}>清理镜像</button></div>
      </div></div>
    </SettingGroup>
    <SettingGroup title="镜像仓库（凭据）" description="管理 docker login 的私有仓库凭据；密码仅用于本次登录，不会回显或存储。">
      <div className="action-card"><div className="action-card__form">
        <div className="form-grid">
          <Field label="仓库地址" value={registry} onChange={setRegistry} placeholder="registry.example.com:5000" hint="仅域名[:端口]，不含路径" />
          <Field label="用户名" value={username} onChange={setUsername} placeholder="用户名" />
          <Field label="密码" value={password} onChange={setPassword} type="password" placeholder="密码" hint="通过 --password-stdin 传输，任务记录不显示" />
        </div>
        <div className="button-row"><button className="secondary-button" disabled={busy || !registry.trim() || !username.trim() || !password} onClick={login}>登录仓库</button></div>
      </div></div>
      <ResourceTable headers={['仓库', '凭据存储', '操作']} empty="尚未配置任何仓库凭据。">{registries.map(item => <tr key={item.registry}>
        <td><strong>{item.registry}</strong></td>
        <td>{item.externalKey ? '外部凭据存储（Docker 管理）' : 'Docker 配置中'}</td>
        <td><div className="button-row"><button className="danger-button" disabled={busy} onClick={() => { if (window.confirm('退出登录 ' + item.registry + '？')) onRun('registry-logout', { registry: item.registry }) }}>退出登录</button></div></td>
      </tr>)}</ResourceTable>
    </SettingGroup>
    <SettingGroup title="本地镜像" description="当前 Docker 守护进程缓存的镜像列表。">
      <ResourceTable headers={['镜像', '标签', '大小', '创建时间', '操作']} empty="没有可显示的镜像。">{images.map(item => <tr key={item.id}>
        <td>{item.repository}</td><td>{item.tag}</td><td>{item.size}</td><td>{item.created}</td>
        <td><div className="button-row">
          <button className="text-button" disabled={busy} onClick={() => { setTagSource(item.repository + ':' + item.tag); setTagTarget('') }}>标记</button>
          <button className="danger-button" disabled={busy} onClick={() => { if (window.confirm('删除镜像 ' + item.repository + ':' + item.tag + '？')) onRun('image-remove', { image: item.repository + ':' + item.tag }) }}>删除</button>
        </div></td>
      </tr>)}</ResourceTable>
    </SettingGroup>
  </div>
}

const LOG_TIMESTAMP = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z) /
const emptyCreateForm = { image: '', name: '', command: '', restart: '', network: '', ports: '', env: '', volumes: '', labels: '' }

function Containers({ containers, busy, onRun }: { containers: DockerContainer[]; busy: boolean; onRun: <T>(action: string, params?: Record<string, string>, success?: (result: ActionResult<T>) => void) => void }) {
  const [create, setCreate] = useState(emptyCreateForm)
  const [selectedIDs, setSelectedIDs] = useState<string[]>([])
  const [logs, setLogs] = useState<{ id: string; name: string; lines: string[]; live: boolean; error?: string }>()
  const [detail, setDetail] = useState<{ id: string; name: string; data: unknown }>()
  const [detailTab, setDetailTab] = useState<'inspect' | 'stats'>('inspect')
  const [stat, setStat] = useState<DockerStat>()
  const [statsLive, setStatsLive] = useState(false)
  const [statsError, setStatsError] = useState<string>()
  const [web, setWeb] = useState<{ container: DockerContainer; port: PublishedPort }>()
  const [route, setRoute] = useState('')
  const [webURL, setWebURL] = useState('')
  const logsPre = useRef<HTMLPreElement>(null)
  const statsPolling = useRef(false)
  const logsPolling = useRef(false)
  const logsTail = useRef(0)
  const toggle = (id: string) => setSelectedIDs(current => current.includes(id) ? current.filter(item => item !== id) : [...current, id])
  const batch = (verb: string) => { if (selectedIDs.length) onRun('container-batch', { verb, containerIDs: selectedIDs.join(',') }, () => setSelectedIDs([])) }
  const openWeb = (container: DockerContainer, port: PublishedPort) => { setWeb({ container, port }); setRoute(''); setWebURL(dynamicWebURL(port.hostPort, '/')) }
  const applyRoute = () => { if (!web) return; const path = cleanRoute(route); if (!path) return alert('路由无效：不能包含 .. 或反斜杠。'); setWebURL(dynamicWebURL(web.port.hostPort, path)) }
  const openDetail = (item: DockerContainer) => { setDetailTab('inspect'); setStatsLive(false); setStat(undefined); setStatsError(undefined); onRun<unknown>('container-inspect', { containerID: item.id }, result => setDetail({ id: item.id, name: item.names, data: result.data })) }
  const openLogs = (item: DockerContainer) => {
    setLogs({ id: item.id, name: item.names, lines: [], live: false })
    logsTail.current = 0
    const since = new Date(Date.now() - 3600_000).toISOString()
    void runActionOnce<unknown>('container-logs-since', { containerID: item.id, since, lines: '200' }).then(result => {
      if (result.error) { setLogs(current => current ? { ...current, error: result.error } : current); return }
      setLogs(current => current ? { ...current, lines: consumeLogLines(result.output) } : current)
    }).catch(reason => setLogs(current => current ? { ...current, error: textError(reason) } : current))
  }
  const consumeLogLines = (raw: string): string[] => {
    const incoming = raw.split(/\r?\n/).filter(line => line.trim() !== '')
    setLogs(current => {
      if (!current) return current
      const recent = new Set(current.lines.slice(-60))
      const merged = [...current.lines]
      for (const line of incoming) {
        const match = LOG_TIMESTAMP.exec(line)
        if (match) {
          const ts = Date.parse(match[1])
          if (Number.isFinite(ts)) {
            if (ts <= logsTail.current) continue
            logsTail.current = ts
          }
        } else if (recent.has(line)) {
          continue
        }
        merged.push(line)
      }
      return { ...current, lines: merged.slice(-2000) }
    })
    return []
  }
  useEffect(() => {
    if (!detail || detailTab !== 'stats' || !statsLive) return
    const poll = () => {
      if (statsPolling.current || !detail) return
      statsPolling.current = true
      void runActionOnce<DockerStat>('container-stats', { containerID: detail.id }).then(result => {
        if (result.error) { setStatsError(result.error); setStatsLive(false) } else { setStatsError(undefined); setStat(result.data) }
      }).catch(reason => { setStatsError(textError(reason)); setStatsLive(false) }).finally(() => { statsPolling.current = false })
    }
    poll()
    const timer = setInterval(poll, 2000)
    return () => clearInterval(timer)
  }, [detail, detailTab, statsLive])
  useEffect(() => {
    if (!logs?.live) return
    const poll = () => {
      if (logsPolling.current || !logs) return
      logsPolling.current = true
      const since = new Date(logsTail.current || Date.now() - 3600_000).toISOString()
      void runActionOnce<unknown>('container-logs-since', { containerID: logs.id, since, lines: '200' }).then(result => {
        if (result.error) { setLogs(current => current ? { ...current, live: false, error: result.error } : current); return }
        if (result.output) consumeLogLines(result.output)
      }).catch(reason => setLogs(current => current ? { ...current, live: false, error: textError(reason) } : current)).finally(() => { logsPolling.current = false })
    }
    poll()
    const timer = setInterval(poll, 2000)
    return () => clearInterval(timer)
  }, [logs?.live, logs?.id])
  useEffect(() => {
    if (logs?.live && logsPre.current) logsPre.current.scrollTop = logsPre.current.scrollHeight
  }, [logs?.lines.length, logs?.live])
  return <div className="settings-stack">
    <SettingGroup title="创建容器" description="从镜像创建独立容器（docker run -d）；卷仅支持命名卷，不支持宿主路径。">
      <div className="action-card"><div className="action-card__form">
        <div className="form-grid">
          <Field label="镜像" value={create.image} onChange={image => setCreate({ ...create, image })} placeholder="镜像:标签" hint="必填，仅接受受校验的镜像引用" />
          <Field label="容器名称" value={create.name} onChange={name => setCreate({ ...create, name })} placeholder="my-container" hint="可选" />
          <Field label="命令" value={create.command} onChange={command => setCreate({ ...create, command })} placeholder="sleep 3600" hint="按引号分词传给容器，不经过 shell" />
          <label className="form-field"><span>重启策略</span><select value={create.restart} onChange={event => setCreate({ ...create, restart: event.target.value })}><option value="">默认</option><option value="no">no</option><option value="always">always</option><option value="on-failure">on-failure</option><option value="unless-stopped">unless-stopped</option></select></label>
          <Field label="网络" value={create.network} onChange={network => setCreate({ ...create, network })} placeholder="bridge" hint="可选" />
        </div>
        <div className="form-grid">
          <Area label="端口（每行一项）" value={create.ports} onChange={ports => setCreate({ ...create, ports })} placeholder="8080:80" hint="[IP:]宿主端口:容器端口[/协议]" />
          <Area label="环境变量（每行 KEY=value）" value={create.env} onChange={env => setCreate({ ...create, env })} hint="内容不会回显" />
          <Area label="命名卷（每行一项）" value={create.volumes} onChange={volumes => setCreate({ ...create, volumes })} placeholder="data:/var/lib/data:ro" hint="仅支持 卷名:容器路径[:ro|rw]" />
          <Area label="标签（每行 KEY=value）" value={create.labels} onChange={labels => setCreate({ ...create, labels })} placeholder="tier=web" />
        </div>
        <div className="button-row"><button className="secondary-button" disabled={busy || !create.image.trim()} onClick={() => { onRun('container-create', create); setCreate(emptyCreateForm) }}>创建容器</button></div>
      </div></div>
    </SettingGroup>
    <SettingGroup title="容器列表" description="勾选容器可批量启动、停止或重启；运行中的容器排在最前。">
      {selectedIDs.length > 0 && <div className="batch-bar"><span>已选择 {selectedIDs.length} 个容器</span><div className="button-row">
        <button className="secondary-button" disabled={busy} onClick={() => batch('start')}>批量启动</button>
        <button className="secondary-button" disabled={busy} onClick={() => batch('stop')}>批量停止</button>
        <button className="secondary-button" disabled={busy} onClick={() => batch('restart')}>批量重启</button>
        <button className="text-button" onClick={() => setSelectedIDs([])}>取消选择</button>
      </div></div>}
      <ResourceTable headers={['', '容器', '项目', '镜像', '状态', '端口', '操作']} empty="没有可显示的容器。">{containers.map(item => <tr key={item.id}>
        <td className="check-col"><input type="checkbox" aria-label={`选择 ${item.names}`} checked={selectedIDs.includes(item.id)} onChange={() => toggle(item.id)} /></td>
        <td><strong>{item.names}</strong><small>{item.id}</small></td>
        <td>{item.project || '—'}</td>
        <td>{item.image}</td>
        <td><span className={item.state === 'running' ? 'badge badge--good' : 'badge'}>{item.status}</span></td>
        <td>{item.ports || '—'}</td>
        <td><div className="button-row">
          {item.published?.length ? <button className="text-button" disabled={busy} onClick={() => openWeb(item, item.published![0])}>打开页面</button> : null}
          <button className="text-button" disabled={busy} onClick={() => openDetail(item)}>详情</button>
          <button className="text-button" disabled={busy} onClick={() => onRun('container-start', { containerID: item.id })}>启动</button>
          <button className="text-button" disabled={busy} onClick={() => onRun('container-stop', { containerID: item.id })}>停止</button>
          <button className="text-button" disabled={busy} onClick={() => onRun('container-restart', { containerID: item.id })}>重启</button>
          <button className="text-button" disabled={busy} onClick={() => openLogs(item)}>日志</button>
        </div></td>
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
    {detail && <div className="modal-backdrop"><section className="modal-card modal-card--wide">
      <p className="eyebrow">容器详情</p>
      <h2>{detail.name} · 容器详情</h2>
      <div className="tabs"><button className={detailTab === 'inspect' ? 'active' : ''} onClick={() => { setDetailTab('inspect'); setStatsLive(false) }}>详情</button><button className={detailTab === 'stats' ? 'active' : ''} onClick={() => { setDetailTab('stats'); setStatsLive(true); setStatsError(undefined) }}>实时统计</button></div>
      {detailTab === 'inspect' ? <div>
        <p className="modal-card__lead">docker inspect 原始输出，只读展示。</p>
        <pre className="modal-card__log">{JSON.stringify(detail.data, null, 2)}</pre>
      </div> : <div className="stat-panel">
        <label className="check-row"><input type="checkbox" checked={statsLive} onChange={event => { setStatsLive(event.target.checked); if (event.target.checked) setStatsError(undefined) }} />自动刷新（每 2 秒）</label>
        {statsError && <div className="notice notice--error">{statsError}</div>}
        {stat ? <div className="stat-grid">
          <div className="stat-chip"><span>CPU</span><strong>{stat.cpuPerc || '—'}</strong></div>
          <div className="stat-chip"><span>内存</span><strong>{stat.memUsage || '—'}</strong><small>{stat.memPerc || ''}</small></div>
          <div className="stat-chip"><span>网络 I/O</span><strong>{stat.netIO || '—'}</strong></div>
          <div className="stat-chip"><span>磁盘 I/O</span><strong>{stat.blockIO || '—'}</strong></div>
          <div className="stat-chip"><span>进程数</span><strong>{stat.pids || '—'}</strong></div>
        </div> : <div className="unavailable-row">{statsLive ? '正在读取统计…' : '打开自动刷新以读取实时统计。'}</div>}
      </div>}
      <div className="modal-actions"><button className="secondary-button" onClick={() => { setDetail(undefined); setStatsLive(false) }}>关闭</button></div>
    </section></div>}
    {logs && <div className="modal-backdrop"><section className="modal-card modal-card--wide">
      <p className="eyebrow">容器日志</p>
      <h2>{logs.name}</h2>
      <p className="modal-card__lead">最近 200 行日志；开启实时后每 2 秒增量拉取并自动滚动。</p>
      <label className="check-row"><input type="checkbox" checked={logs.live} onChange={event => setLogs(current => current ? { ...current, live: event.target.checked, error: undefined } : current)} />实时（每 2 秒刷新）</label>
      {logs.error && <div className="notice notice--error">{logs.error}</div>}
      <pre ref={logsPre} className="modal-card__log">{logs.lines.length ? logs.lines.join('\n') : '容器未输出日志。'}</pre>
      <div className="modal-actions"><button className="secondary-button" onClick={() => setLogs(undefined)}>关闭</button></div>
    </section></div>}
  </div>
}

function Networks({ networks, busy, onRun }: { networks: DockerNetwork[]; busy: boolean; onRun: <T>(action: string, params?: Record<string, string>, success?: (result: ActionResult<T>) => void) => void }) {
  const [name, setName] = useState('')
  const [driver, setDriver] = useState('')
  const [subnet, setSubnet] = useState('')
  const [gateway, setGateway] = useState('')
  const [inspect, setInspect] = useState<{ name: string; data: unknown }>()
  const create = () => onRun('network-create', { name, driver, subnet, gateway }, () => { setName(''); setDriver(''); setSubnet(''); setGateway('') })
  const remove = (target: string) => { if (window.confirm('删除网络 ' + target + '？正在使用中的网络会被 Docker 拒绝删除。')) onRun('network-remove', { name: target }) }
  const openInspect = (target: string) => onRun<unknown>('network-inspect', { name: target }, result => setInspect({ name: target, data: result.data }))
  return <div className="settings-stack">
    <SettingGroup title="新建网络" description="创建自定义 bridge 网络；子网与网关必须符合 CIDR/IP 格式。">
      <div className="action-card"><div className="action-card__form">
        <div className="form-grid">
          <Field label="网络名称" value={name} onChange={setName} placeholder="my-network" hint="仅允许字母、数字、点、下划线与短横线" />
          <Field label="驱动" value={driver} onChange={setDriver} placeholder="bridge" />
          <Field label="子网" value={subnet} onChange={setSubnet} placeholder="172.20.0.0/16" />
          <Field label="网关" value={gateway} onChange={setGateway} placeholder="172.20.0.1" />
        </div>
        <div className="button-row"><button className="secondary-button" disabled={busy || !name.trim()} onClick={create}>创建网络</button></div>
      </div></div>
    </SettingGroup>
    <SettingGroup title="网络列表" description="查看宿主上的 Docker 网络及其驱动与作用域。">
      <ResourceTable headers={['名称', '驱动', '作用域', '操作']} empty="没有可显示的网络。">{networks.map(item => <tr key={item.name}>
        <td><strong>{item.name}</strong>{item.id ? <small>{item.id}</small> : null}</td>
        <td>{item.driver || '—'}</td>
        <td>{item.scope || '—'}</td>
        <td><div className="button-row">
          <button className="text-button" disabled={busy} onClick={() => openInspect(item.name)}>详情</button>
          <button className="danger-button" disabled={busy} onClick={() => remove(item.name)}>删除</button>
        </div></td>
      </tr>)}</ResourceTable>
    </SettingGroup>
    {inspect && <InspectModal title={inspect.name + ' · 网络详情'} data={inspect.data} onClose={() => setInspect(undefined)} />}
  </div>
}

function Volumes({ volumes, busy, onRun }: { volumes: DockerVolume[]; busy: boolean; onRun: <T>(action: string, params?: Record<string, string>, success?: (result: ActionResult<T>) => void) => void }) {
  const [name, setName] = useState('')
  const [driver, setDriver] = useState('')
  const [inspect, setInspect] = useState<{ name: string; data: unknown }>()
  const create = () => onRun('volume-create', { name, driver }, () => { setName(''); setDriver('') })
  const remove = (target: string) => { if (window.confirm('删除卷 ' + target + ' 会永久移除其数据，且无法恢复。继续吗？')) onRun('volume-remove', { name: target }) }
  const openInspect = (target: string) => onRun<unknown>('volume-inspect', { name: target }, result => setInspect({ name: target, data: result.data }))
  return <div className="settings-stack">
    <SettingGroup title="新建卷" description="创建命名数据卷，供 Compose 服务或容器挂载使用。">
      <div className="action-card"><div className="action-card__form">
        <div className="form-grid">
          <Field label="卷名称" value={name} onChange={setName} placeholder="my-volume" hint="仅允许字母、数字、点、下划线与短横线" />
          <Field label="驱动" value={driver} onChange={setDriver} placeholder="local" />
        </div>
        <div className="button-row"><button className="secondary-button" disabled={busy || !name.trim()} onClick={create}>创建卷</button></div>
      </div></div>
    </SettingGroup>
    <SettingGroup title="卷列表" description="命名数据卷及其挂载点；删除前请确认没有容器正在使用。">
      <ResourceTable headers={['名称', '驱动', '作用域', '挂载点', '操作']} empty="没有可显示的卷。">{volumes.map(item => <tr key={item.name}>
        <td><strong>{item.name}</strong></td>
        <td>{item.driver || '—'}</td>
        <td>{item.scope || '—'}</td>
        <td><small>{item.mountpoint || '—'}</small></td>
        <td><div className="button-row">
          <button className="text-button" disabled={busy} onClick={() => openInspect(item.name)}>详情</button>
          <button className="danger-button" disabled={busy} onClick={() => remove(item.name)}>删除</button>
        </div></td>
      </tr>)}</ResourceTable>
    </SettingGroup>
    {inspect && <InspectModal title={inspect.name + ' · 卷详情'} data={inspect.data} onClose={() => setInspect(undefined)} />}
  </div>
}

function InspectModal({ title, data, onClose }: { title: string; data: unknown; onClose: () => void }) {
  const text = useMemo(() => {
    try { return JSON.stringify(data, null, 2) } catch { return String(data) }
  }, [data])
  return <div className="modal-backdrop"><section className="modal-card modal-card--wide">
    <p className="eyebrow">详情</p>
    <h2>{title}</h2>
    <p className="modal-card__lead">docker inspect 原始输出，只读展示。</p>
    <pre className="modal-card__log">{text}</pre>
    <div className="modal-actions"><button className="secondary-button" onClick={onClose}>关闭</button></div>
  </section></div>
}

function ResourceTable({ headers, empty, children }: { headers: string[]; empty: string; children: ReactNode }) {
  const hasRows = Array.isArray(children) ? children.length > 0 : Boolean(children)
  return <div className="table-wrap"><table className="resource-table"><thead><tr>{headers.map(header => <th key={header}>{header}</th>)}</tr></thead><tbody>{hasRows ? children : <tr><td className="empty" colSpan={headers.length}>{empty}</td></tr>}</tbody></table></div>
}
