package pluginsdk

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// fakeTrigger is a tiny helper to build a wire JobTrigger without a manual
// struct literal sprinkled around tests.
type fakeTrigger struct {
	name string
	id   string
}

func (f *fakeTrigger) toPB() *pb.JobTrigger {
	return &pb.JobTrigger{JobName: f.name, TriggerId: f.id, FireTimeUnixNano: time.Now().UnixNano()}
}

// stubStream implements pb.JobScheduler_SubscribeClient so dispatch tests can
// observe the acks the SDK sends back. Recv is unused (tests that need to
// inject triggers call dispatch directly), so it blocks until ctx cancel.
type stubStream struct {
	grpc.ClientStream

	mu   sync.Mutex
	acks []*pb.JobAck
	regs []*pb.JobRegistration

	ackedCh chan struct{}
	closeCh chan struct{}
}

func newStubStream() *stubStream {
	return &stubStream{
		ackedCh: make(chan struct{}, 64),
		closeCh: make(chan struct{}),
	}
}

func (s *stubStream) Send(msg *pb.JobMessage) error {
	if msg == nil {
		return errors.New("nil msg")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch m := msg.GetMsg().(type) {
	case *pb.JobMessage_Ack:
		s.acks = append(s.acks, m.Ack)
		select {
		case s.ackedCh <- struct{}{}:
		default:
		}
	case *pb.JobMessage_Register:
		s.regs = append(s.regs, m.Register)
	}
	return nil
}

func (s *stubStream) Recv() (*pb.JobTrigger, error) {
	<-s.closeCh
	return nil, errors.New("stream closed")
}

func (s *stubStream) Header() (metadata.MD, error) { return nil, nil }
func (s *stubStream) Trailer() metadata.MD         { return nil }
func (s *stubStream) CloseSend() error             { return nil }
func (s *stubStream) Context() context.Context     { return context.Background() }
func (s *stubStream) SendMsg(any) error            { return nil }
func (s *stubStream) RecvMsg(any) error            { return nil }

func (s *stubStream) waitForAcks(t *testing.T, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		s.mu.Lock()
		got := len(s.acks)
		s.mu.Unlock()
		if got >= n {
			return
		}
		select {
		case <-s.ackedCh:
		case <-deadline:
			t.Fatalf("timed out waiting for %d acks (got %d)", n, got)
		}
	}
}
