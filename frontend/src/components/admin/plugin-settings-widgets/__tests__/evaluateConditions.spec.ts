import { describe, it, expect } from 'vitest'
import { evaluateConditions } from '../evaluateConditions'

describe('evaluateConditions', () => {
  it('returns empty hidden when schema has no conditional clauses', () => {
    const schema = {
      type: 'object',
      properties: {
        name: { type: 'string' },
        age: { type: 'integer' },
      },
    }
    const r = evaluateConditions(schema, { name: 'a', age: 1 })
    expect(r.hidden.size).toBe(0)
    expect(r.cyclic).toBe(false)
  })

  it('returns empty result for non-object schema', () => {
    const r = evaluateConditions(null as unknown as Record<string, unknown>, {})
    expect(r.hidden.size).toBe(0)
    expect(r.cyclic).toBe(false)
  })

  describe('top-level if/then/else', () => {
    const schema = {
      type: 'object',
      properties: {
        mode: { type: 'string', enum: ['simple', 'advanced'] },
        threads: { type: 'integer' },
        tag: { type: 'string' },
      },
      if: {
        properties: { mode: { const: 'advanced' } },
      },
      then: {
        properties: {
          threads: { type: 'integer' },
        },
      },
      else: {
        properties: {
          tag: { type: 'string' },
        },
      },
    }

    it('mode=advanced → matches if → then branch keeps `threads`, hides `tag`', () => {
      const r = evaluateConditions(schema, { mode: 'advanced' })
      expect(r.hidden.has('tag')).toBe(true)
      expect(r.hidden.has('threads')).toBe(false)
      expect(r.hidden.has('mode')).toBe(false)
    })

    it('mode=simple → does not match → else branch keeps `tag`, hides `threads`', () => {
      const r = evaluateConditions(schema, { mode: 'simple' })
      expect(r.hidden.has('threads')).toBe(true)
      expect(r.hidden.has('tag')).toBe(false)
    })

    it('non-conditional field `mode` is never hidden', () => {
      const r1 = evaluateConditions(schema, { mode: 'advanced' })
      const r2 = evaluateConditions(schema, { mode: 'simple' })
      expect(r1.hidden.has('mode')).toBe(false)
      expect(r2.hidden.has('mode')).toBe(false)
    })
  })

  describe('dependentSchemas', () => {
    const schema = {
      type: 'object',
      properties: {
        proxy_url: { type: 'string' },
        proxy_user: { type: 'string' },
        proxy_pass: { type: 'string' },
      },
      dependentSchemas: {
        proxy_url: {
          properties: {
            proxy_user: { type: 'string' },
            proxy_pass: { type: 'string' },
          },
          required: ['proxy_user'],
        },
      },
    }

    it('proxy_url not set → proxy_user / proxy_pass hidden', () => {
      const r = evaluateConditions(schema, {})
      expect(r.hidden.has('proxy_user')).toBe(true)
      expect(r.hidden.has('proxy_pass')).toBe(true)
      expect(r.hidden.has('proxy_url')).toBe(false)
    })

    it('proxy_url set → proxy_user / proxy_pass visible, requiredOverride includes proxy_user', () => {
      const r = evaluateConditions(schema, { proxy_url: 'http://p' })
      expect(r.hidden.has('proxy_user')).toBe(false)
      expect(r.hidden.has('proxy_pass')).toBe(false)
      expect(r.requiredOverride.has('proxy_user')).toBe(true)
    })

    it('proxy_url empty string → treated as inactive, deps hidden', () => {
      const r = evaluateConditions(schema, { proxy_url: '' })
      expect(r.hidden.has('proxy_user')).toBe(true)
    })
  })

  describe('allOf[*].if/then/else', () => {
    const schema = {
      type: 'object',
      properties: {
        kind: { type: 'string' },
        a_only: { type: 'string' },
        b_only: { type: 'string' },
      },
      allOf: [
        {
          if: { properties: { kind: { const: 'A' } } },
          then: { properties: { a_only: { type: 'string' } } },
          else: { properties: { b_only: { type: 'string' } } },
        },
      ],
    }

    it('kind=A → b_only hidden', () => {
      const r = evaluateConditions(schema, { kind: 'A' })
      expect(r.hidden.has('b_only')).toBe(true)
      expect(r.hidden.has('a_only')).toBe(false)
    })

    it('kind=B → a_only hidden', () => {
      const r = evaluateConditions(schema, { kind: 'B' })
      expect(r.hidden.has('a_only')).toBe(true)
      expect(r.hidden.has('b_only')).toBe(false)
    })
  })

  describe('cycle detection', () => {
    it('A controls B and B controls A → cyclic=true, hidden empty', () => {
      const schema = {
        type: 'object',
        properties: {
          a: { type: 'string' },
          b: { type: 'string' },
        },
        allOf: [
          {
            if: { properties: { a: { const: 'x' } } },
            then: { properties: { b: { type: 'string' } } },
          },
          {
            if: { properties: { b: { const: 'y' } } },
            then: { properties: { a: { type: 'string' } } },
          },
        ],
      }
      const r = evaluateConditions(schema, { a: 'x', b: 'y' })
      expect(r.cyclic).toBe(true)
      expect(r.hidden.size).toBe(0)
    })

    it('nested if-inside-then beyond MAX_DEPTH → cyclic=true', () => {
      const schema = {
        type: 'object',
        properties: { a: { type: 'string' }, b: { type: 'string' }, c: { type: 'string' } },
        if: { properties: { a: { const: 'x' } } },
        then: {
          properties: { b: { type: 'string' } },
          if: { properties: { b: { const: 'y' } } },
          then: {
            if: { properties: { c: { const: 'z' } } },
            then: { properties: { c: { type: 'string' } } },
          },
        },
      }
      const r = evaluateConditions(schema, {})
      expect(r.cyclic).toBe(true)
      expect(r.hidden.size).toBe(0)
    })
  })

  describe('leaf matchers', () => {
    it('properties.K.enum: matches when value is in enum', () => {
      const schema = {
        type: 'object',
        properties: { mode: { type: 'string' }, x: { type: 'string' }, y: { type: 'string' } },
        if: { properties: { mode: { enum: ['a', 'b'] } } },
        then: { properties: { x: { type: 'string' } } },
        else: { properties: { y: { type: 'string' } } },
      }
      expect(evaluateConditions(schema, { mode: 'a' }).hidden.has('y')).toBe(true)
      expect(evaluateConditions(schema, { mode: 'b' }).hidden.has('y')).toBe(true)
      expect(evaluateConditions(schema, { mode: 'c' }).hidden.has('x')).toBe(true)
    })

    it('required: clause is honoured', () => {
      const schema = {
        type: 'object',
        properties: { token: { type: 'string' }, advanced: { type: 'string' } },
        if: { required: ['token'] },
        then: { properties: { advanced: { type: 'string' } } },
      }
      expect(evaluateConditions(schema, { token: 'abc' }).hidden.has('advanced')).toBe(false)
      expect(evaluateConditions(schema, {}).hidden.has('advanced')).toBe(true)
    })
  })
})
