import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readPending, connectSession, safeOrigin, validateRequest, sessionFromCookies, deliverSession } from './connect.mjs'

const now = Date.now()
const request = () => ({ version: '1', origin: 'https://gateway.example.test', nonce: 'a'.repeat(64), ownerId: 7,
  ownerLabel: 'Owner — owner@example.test', accountId: 95, expiresAt: Date.now() + 60_000 })
const cookies = () => ['hatch_sess', 'hatch_gw', 'hatch_native_auth_device'].map(name => ({ name, domain: '.muse.ai', path: '/', value: 'fixture-'+name, expirationDate: Date.now()/1000+3600 }))

function browser(receiver = request()) {
  const calls = { cookies: 0, deliveries: 0 }
  const tab = { id: 3, url: receiver.origin+'/admin/accounts' }
  const api = {
    tabs: { query: async () => [tab], get: async () => tab },
    cookies: { getAll: async () => { calls.cookies++; return cookies() } },
    scripting: { executeScript: async options => {
      assert.deepEqual(options.target, { tabId: 3, frameIds: [0] })
      if (options.args) { calls.deliveries++; return [{ result: true }] }
      return [{ result: receiver }]
    } },
  }
  return { api, calls, tab, receiver }
}

test('accepts HTTPS self-hosted paths and HTTP loopback only', () => {
  assert.equal(safeOrigin('https://gateway.example.test/admin/accounts'), 'https://gateway.example.test')
  for (const value of ['http://localhost:4318/a','http://127.0.0.1:4318','http://[::1]:4318']) assert.ok(safeOrigin(value))
  for (const value of ['http://gateway.example.test','http://localhost.evil.test','https://user:secret@gateway.example.test','file:///x','chrome://extensions','javascript:alert(1)']) assert.throws(() => safeOrigin(value))
})

test('rejects malformed, expired, mismatched and unreasonably long-lived requests', () => {
  const valid=request()
  for (const patch of [{version:1},{origin:'https://evil.test'},{nonce:'short'},{ownerId:0},{ownerId:1.5},{accountId:-1},
    {ownerLabel:''},{ownerLabel:'bad\nlabel'},{expiresAt:now-1},{expiresAt:Date.now()+121_000}]) {
    assert.throws(() => validateRequest({...valid,...patch},valid.origin))
  }
})

test('popup discovery does not read cookies or deliver anything before approval', async () => {
  const b=browser();const approved=await readPending(b.api)
  assert.equal(approved.request.ownerId,7)
  assert.deepEqual(b.calls,{cookies:0,deliveries:0})
  await connectSession(b.api,approved)
  assert.deepEqual(b.calls,{cookies:1,deliveries:1})
})

test('navigation and changed owner/account/nonce stop approval before reading cookies', async () => {
  for (const field of ['ownerId','accountId','nonce']) {
    const b=browser();const approved=await readPending(b.api)
    approved.request={...approved.request}
    b.receiver[field]=field==='nonce'?'b'.repeat(64):44
    await assert.rejects(connectSession(b.api,approved))
    assert.equal(b.calls.cookies,0)
  }
  const b=browser();const approved=await readPending(b.api);b.tab.url='https://elsewhere.example.test'
  await assert.rejects(connectSession(b.api,approved));assert.equal(b.calls.cookies,0)
})

test('restricts cookie selection to the named Muse auth cookies on the root path', () => {
  const all=[...cookies(),{name:'unrelated',domain:'muse.ai',path:'/',value:'private-other-cookie'},
    {name:'hatch_sess',domain:'other.example.test',path:'/',value:'other-domain'},
    {name:'hatch_sess',domain:'muse.ai',path:'/other',value:'other-path'}]
  const result=sessionFromCookies(all)
  assert.equal(Object.keys(result.cookies).length,3)
  assert.equal(result.cookies.hatch_sess,'fixture-hatch_sess')
  assert.throws(()=>sessionFromCookies([...cookies(),{...cookies()[0],value:'conflicting'}]))
  assert.throws(()=>sessionFromCookies(cookies().slice(1)))
  assert.throws(()=>sessionFromCookies(cookies().map(c=>({...c,expirationDate:1}))))
  assert.throws(()=>sessionFromCookies(cookies().map(c=>({...c,expirationDate:0}))))
  assert.throws(()=>sessionFromCookies(cookies().map(c=>({...c,value:'bad\nvalue'}))))
})

test('delivery is fenced again inside the destination tab after cookie collection', () => {
  const r=request(), dataset={nonce:r.nonce,origin:r.origin,ownerId:'7',ownerLabel:r.ownerLabel,accountId:'95',expiresAt:String(r.expiresAt)}
  const posted=[];const win={location:{origin:r.origin},postMessage:(...args)=>posted.push(args)};win.top=win
  const oldWindow=globalThis.window,oldDocument=globalThis.document
  globalThis.window=win;globalThis.document={querySelectorAll:()=>[{dataset}]}
  try {
    assert.equal(deliverSession(r,{cookies:{fixture:'value'}}),true)
    assert.equal(posted.length,1);assert.equal(posted[0][1],r.origin)
    dataset.nonce='b'.repeat(64);assert.equal(deliverSession(r,{}),false)
    dataset.nonce=r.nonce;win.location.origin='https://elsewhere.example.test';assert.equal(deliverSession(r,{}),false)
    assert.equal(posted.length,1)
  } finally {globalThis.window=oldWindow;globalThis.document=oldDocument}
})
