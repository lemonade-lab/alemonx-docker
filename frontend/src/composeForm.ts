import { isMap, parseDocument, type YAMLMap } from 'yaml'

export type FormFields = {
  image: string
  build: string
  command: string
  restart: string
  ports: string
  environment: string
  volumes: string
  dependsOn: string
  topNetworks: string
  topVolumes: string
}

export const blankForm: FormFields = {
  image: '',
  build: '',
  command: '',
  restart: '',
  ports: '',
  environment: '',
  volumes: '',
  dependsOn: '',
  topNetworks: '',
  topVolumes: ''
}

function lines(value: unknown): string {
  if (Array.isArray(value)) return value.map(String).join('\n')
  if (value && typeof value === 'object') {
    return Object.entries(value as Record<string, unknown>)
      .map(([key, item]) => key + '=' + String(item))
      .join('\n')
  }
  return ''
}

export function serviceNames(content: string): string[] {
  try {
    const all = parseDocument(content).toJS() as { services?: Record<string, unknown> }
    return Object.keys(all?.services || {})
  } catch {
    return []
  }
}

function topNames(content: string, key: string): string {
  try {
    const all = parseDocument(content).toJS() as Record<string, Record<string, unknown>>
    const value = all?.[key]
    return value && typeof value === 'object' ? Object.keys(value).join('\n') : ''
  } catch {
    return ''
  }
}

export function formFor(content: string, service: string): FormFields {
  try {
    const all = parseDocument(content).toJS() as {
      services?: Record<string, Record<string, unknown>>
      networks?: Record<string, unknown>
      volumes?: Record<string, unknown>
    }
    const item = all?.services?.[service] || {}
    return {
      image: String(item.image || ''),
      build: typeof item.build === 'string' ? item.build : '',
      command: typeof item.command === 'string' ? item.command : '',
      restart: String(item.restart || ''),
      ports: lines(item.ports),
      environment: lines(item.environment),
      volumes: lines(item.volumes),
      dependsOn: lines(item.depends_on),
      topNetworks: topNames(content, 'networks'),
      topVolumes: topNames(content, 'volumes')
    }
  } catch {
    return { ...blankForm }
  }
}

function setField(node: YAMLMap, key: string, value: string, list = false) {
  const clean = value.trim()
  if (!clean) {
    node.delete(key)
    return
  }
  node.set(key, list ? clean.split(/\r?\n/).map(item => item.trim()).filter(Boolean) : clean)
}

// Updates the top-level networks/volumes maps: keeps configuration of names
// that stay, adds missing names as empty maps, and removes names no longer in
// the form. An empty field deletes the whole top-level key.
function setTopLevel(doc: ReturnType<typeof parseDocument>, key: string, value: string) {
  const root = doc.contents as YAMLMap | null
  if (!root || !isMap(root)) throw new Error('Compose 根节点必须是对象')
  const names = value.split(/\r?\n/).map(name => name.trim()).filter(Boolean)
  if (!names.length) {
    root.delete(key)
    return
  }
  const existing = root.get(key)
  let map: YAMLMap
  if (isMap(existing)) {
    map = existing
  } else {
    map = doc.createNode({}) as YAMLMap
    root.set(key, map)
  }
  const kept = new Set(names)
  for (const pair of [...map.items]) {
    if (!kept.has(String(pair.key))) map.delete(pair.key)
  }
  for (const name of names) {
    if (!map.has(name)) map.set(name, {})
  }
}

// Applies only the form-managed fields to the YAML AST so comments, unknown
// keys and x-* extensions survive a form save untouched.
export function writeForm(content: string, service: string, fields: FormFields): string {
  const doc = parseDocument(content)
  if (doc.errors.length) throw new Error(doc.errors[0].message)
  const root = doc.contents as YAMLMap | null
  if (!root || !isMap(root)) throw new Error('Compose 根节点必须是对象')
  const existingServices = root.get('services')
  let services: YAMLMap
  if (isMap(existingServices)) {
    services = existingServices
  } else {
    services = doc.createNode({}) as YAMLMap
    root.set('services', services)
  }
  const existingItem = services.get(service)
  let item: YAMLMap
  if (isMap(existingItem)) {
    item = existingItem
  } else {
    item = doc.createNode({}) as YAMLMap
    services.set(service, item)
  }
  setField(item, 'image', fields.image)
  setField(item, 'build', fields.build)
  setField(item, 'command', fields.command)
  setField(item, 'restart', fields.restart)
  setField(item, 'ports', fields.ports, true)
  setField(item, 'environment', fields.environment, true)
  setField(item, 'volumes', fields.volumes, true)
  setField(item, 'depends_on', fields.dependsOn, true)
  setTopLevel(doc, 'networks', fields.topNetworks)
  setTopLevel(doc, 'volumes', fields.topVolumes)
  return doc.toString()
}
