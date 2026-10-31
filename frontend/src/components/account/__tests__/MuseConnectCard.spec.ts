import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import MuseConnectCard from '../MuseConnectCard.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const wrappers: Array<{ unmount: () => void }> = []
const session = { cookies: { hatch_sess: 'fixture-session', hatch_gw: 'fixture-gateway', hatch_native_auth_device: 'fixture-device' } }
function create(props = {}) {
  const wrapper=mount(MuseConnectCard,{props:{ownerId:7,ownerLabel:'Owner — owner@example.test',...props}})
  wrappers.push(wrapper);return wrapper
}
async function begin(wrapper: ReturnType<typeof create>) {
  await wrapper.get('[data-testid="muse-connect-start"]').trigger('click')
  const marker=wrapper.get('[data-sub2api-muse-connect]')
  return {type:'sub2api:muse:session:v1',nonce:marker.attributes('data-nonce'),ownerId:7,accountId:0,session}
}
function send(data: unknown, origin=window.location.origin, source: MessageEventSource|null=window) {
  window.dispatchEvent(new MessageEvent('message',{data,origin,source}))
}
afterEach(()=>{wrappers.splice(0).forEach(w=>w.unmount());vi.useRealTimers()})

describe('Muse direct browser handoff',()=>{
  it('requires an assigned owner and offers a file-free primary action',()=>{
    const w=create({ownerId:0});expect(w.get('[data-testid="muse-connect-start"]').attributes('disabled')).toBeDefined()
    expect(w.find('input[type="file"]').exists()).toBe(false)
  })
  it('accepts one origin/owner/nonce-bound handoff and consumes it once',async()=>{
    const w=create();const message=await begin(w)
    expect(message.nonce).toMatch(/^[a-f0-9]{64}$/)
    send(message);send(message)
    expect(w.emitted('connected')).toEqual([[JSON.stringify(session)]])
    await w.vm.$nextTick();expect(w.find('[data-sub2api-muse-connect]').exists()).toBe(false)
  })
  it('rejects wrong origins, sources, nonces, owners and account IDs without consuming the valid request',async()=>{
    const w=create();const message=await begin(w)
    send(message,'https://untrusted.example.test');send(message,window.location.origin,null)
    for(const patch of [{nonce:'b'.repeat(64)},{ownerId:8},{accountId:9}])send({...message,...patch})
    expect(w.emitted('connected')).toBeUndefined()
    send(message);expect(w.emitted('connected')).toHaveLength(1)
  })
  it('expires a request and does not accept a late credential message',async()=>{
    vi.useFakeTimers();const w=create();const message=await begin(w)
    await vi.advanceTimersByTimeAsync(120_000);send(message)
    expect(w.emitted('connected')).toBeUndefined();expect(w.text()).toContain('admin.accounts.muse.connectExpired')
  })
  it('invalidates pending handoffs on owner changes, account changes and cancellation',async()=>{
    const w=create();const message=await begin(w)
    await w.setProps({ownerId:8});send(message);expect(w.emitted('connected')).toBeUndefined()
    await w.setProps({ownerId:7});const again=await begin(w)
    await w.setProps({accountId:95});send(again);expect(w.emitted('connected')).toBeUndefined()
    await w.setProps({accountId:undefined});const last=await begin(w)
    await w.get('button.text-primary-600').trigger('click');send(last);expect(w.emitted('connected')).toBeUndefined()
  })
  it('clears received data when its owner changes and stops listening after unmount',async()=>{
    const w=create();const message=await begin(w);send(message)
    await w.setProps({ownerId:8});expect(w.emitted('invalidated')).toEqual([[]])
    const other=create();const pending=await begin(other);other.unmount();send(pending)
    expect(other.emitted('connected')).toBeUndefined()
  })
  it('rejects malformed, unrelated and expired cookie documents without echoing secrets',async()=>{
    for(const invalid of [{unexpected:'synthetic-private-value'},{cookies:{...session.cookies,other:'synthetic-private-value'}},
      {...session,expires:{hatch_sess:1}}]) {
      const w=create();const message=await begin(w);send({...message,session:invalid});await w.vm.$nextTick()
      expect(w.emitted('connected')).toBeUndefined();expect(w.text()).not.toContain('synthetic-private-value')
      expect(w.text()).toContain('admin.accounts.muse.connectFailed')
    }
  })
})
