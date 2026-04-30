// broker_test.go — unit coverage for the in-process pricing broker.
//
// The broker is the only piece of P4 infrastructure that owns its own
// goroutine-safe state, so we exercise its core invariants directly:
//
//   - Subscribe / Publish round-trip (single subscriber sees event)
//   - Multi-subscriber fan-out (every active subscriber gets every event)
//   - Slow-consumer drop (publish never blocks; chan closes when full)
//   - Unsubscribe is idempotent and final (channel closed exactly once)
//   - Close tears every subscriber down

package pricing

import (
	"sync"
	"testing"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

func TestBroker_PublishFanout(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	chA, unsubA := b.Subscribe()
	defer unsubA()
	chB, unsubB := b.Subscribe()
	defer unsubB()

	evt := &pb.PricingOverrideEvent{Op: pb.PricingOverrideEvent_UPSERT, Version: "v1"}
	b.Publish(evt)

	for name, ch := range map[string]<-chan *pb.PricingOverrideEvent{"A": chA, "B": chB} {
		select {
		case got := <-ch:
			if got != evt {
				t.Errorf("subscriber %s: got %p, want %p", name, got, evt)
			}
		case <-time.After(time.Second):
			t.Errorf("subscriber %s: timed out waiting for event", name)
		}
	}
}

func TestBroker_NilEventNoop(t *testing.T) {
	b := NewBroker()
	defer b.Close()
	ch, unsub := b.Subscribe()
	defer unsub()

	b.Publish(nil)

	select {
	case got, ok := <-ch:
		t.Fatalf("expected no event; got %v ok=%v", got, ok)
	case <-time.After(50 * time.Millisecond):
		// expected — no fanout for nil
	}
}

func TestBroker_SlowConsumerDropped(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	ch, unsub := b.Subscribe()
	defer unsub()

	// Fill the buffer + 1 to exceed brokerSubBuffer.
	evt := &pb.PricingOverrideEvent{Op: pb.PricingOverrideEvent_UPSERT}
	for i := 0; i < brokerSubBuffer+1; i++ {
		b.Publish(evt)
	}

	// Drain the buffered events; the (brokerSubBuffer+1)-th publish must
	// have triggered a drop and a close.
	deadline := time.After(time.Second)
	closed := false
	for !closed {
		select {
		case _, ok := <-ch:
			if !ok {
				closed = true
			}
		case <-deadline:
			t.Fatalf("subscriber chan never closed after overflow")
		}
	}
	if got := b.SubscriberCount(); got != 0 {
		t.Errorf("expected 0 subscribers after drop; got %d", got)
	}
}

func TestBroker_UnsubscribeIdempotent(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	ch, unsub := b.Subscribe()
	unsub()
	unsub() // second call must not panic on double-close

	// Channel must be closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("expected closed chan; got open value")
		}
	case <-time.After(time.Second):
		t.Fatalf("chan was not closed by unsubscribe")
	}
	if got := b.SubscriberCount(); got != 0 {
		t.Errorf("expected 0 subscribers; got %d", got)
	}
}

func TestBroker_CloseTearsDownSubscribers(t *testing.T) {
	b := NewBroker()
	ch, unsub := b.Subscribe()
	defer unsub()

	b.Close()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("expected closed chan after Close()")
		}
	case <-time.After(time.Second):
		t.Fatalf("Close() did not close subscriber chan")
	}

	// Subscribe after close returns an immediately-closed channel and a
	// no-op unsubscribe.
	ch2, unsub2 := b.Subscribe()
	unsub2()
	select {
	case _, ok := <-ch2:
		if ok {
			t.Fatalf("post-close Subscribe gave an open chan")
		}
	case <-time.After(time.Second):
		t.Fatalf("post-close Subscribe chan not closed")
	}

	// Publish on closed broker is a no-op (no panic, no fanout).
	b.Publish(&pb.PricingOverrideEvent{Op: pb.PricingOverrideEvent_UPSERT})
}

func TestBroker_ConcurrentPublishSubscribe(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	const subs = 8
	const publishes = 100

	var wg sync.WaitGroup
	received := make([]int, subs)
	for i := 0; i < subs; i++ {
		ch, unsub := b.Subscribe()
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			defer unsub()
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						return
					}
					received[idx]++
				case <-time.After(2 * time.Second):
					return
				}
			}
		}(i)
	}

	evt := &pb.PricingOverrideEvent{Op: pb.PricingOverrideEvent_UPSERT}
	for i := 0; i < publishes; i++ {
		b.Publish(evt)
	}

	// Allow goroutines to drain then close to release them.
	time.Sleep(200 * time.Millisecond)
	b.Close()
	wg.Wait()

	for i, n := range received {
		if n == 0 {
			t.Errorf("subscriber %d received 0 events; expected > 0", i)
		}
	}
}
