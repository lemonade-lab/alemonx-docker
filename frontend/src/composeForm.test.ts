import { describe, expect, it } from 'vitest'
import { blankForm, formFor, serviceNames, writeForm } from './composeForm'

const advanced = `# keep this comment
x-meta: keep
services:
  web:
    image: nginx:latest
    restart: unless-stopped
    ports:
      - "8080:80"
    x-custom: retained
  api:
    image: busybox
networks:
  frontend:
    driver: bridge
volumes:
  db-data:
    name: my-db
`

describe('formFor', () => {
  it('loads service fields and top-level networks/volumes', () => {
    const fields = formFor(advanced, 'web')
    expect(fields.image).toBe('nginx:latest')
    expect(fields.restart).toBe('unless-stopped')
    expect(fields.ports).toBe('8080:80')
    expect(fields.topNetworks).toBe('frontend')
    expect(fields.topVolumes).toBe('db-data')
  })

  it('falls back to a blank form for unparsable content', () => {
    expect(formFor('not: [valid', 'web')).toEqual(blankForm)
  })
})

describe('writeForm', () => {
  it('preserves comments, x-* extensions and unknown fields', () => {
    const fields = formFor(advanced, 'web')
    const next = writeForm(advanced, 'web', { ...fields, image: 'nginx:1.27', ports: '9090:80' })
    expect(next).toContain('# keep this comment')
    expect(next).toContain('x-meta: keep')
    expect(next).toContain('x-custom: retained')
    expect(next).toContain('nginx:1.27')
    expect(next).toContain('9090:80')
    expect(next).not.toContain('8080:80')
  })

  it('keeps top-level network configuration while adding names', () => {
    const fields = formFor(advanced, 'web')
    fields.topNetworks = 'frontend\nbackend'
    const next = writeForm(advanced, 'web', fields)
    expect(next).toContain('frontend:')
    expect(next).toContain('driver: bridge')
    expect(next).toContain('backend')
  })

  it('removes top-level sections when the form field is cleared', () => {
    const fields = formFor(advanced, 'web')
    fields.topVolumes = ''
    const next = writeForm(advanced, 'web', fields)
    expect(next).not.toContain('volumes:')
    expect(next).toContain('networks:')
  })

  it('creates missing services and top-level structures', () => {
    const next = writeForm('services: {}\n', 'app', { ...blankForm, image: 'busybox' })
    expect(serviceNames(next)).toEqual(['app'])
    expect(next).toContain('busybox')
  })

  it('rejects non-object roots', () => {
    expect(() => writeForm('- item\n', 'web', blankForm)).toThrow('根节点')
  })
})
