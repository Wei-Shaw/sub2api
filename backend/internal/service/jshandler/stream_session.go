package jshandler

import (
	"errors"
	"time"

	"github.com/dop251/goja"
)

// StreamSession reuses one goja VM for the lifetime of a single SSE response.
// Program compile cache remains global (modTime-aware); runtime init runs once
// per session (or when the script file mtime changes mid-stream).
type StreamSession struct {
	path    string
	timeout time.Duration
	program *goja.Program
	modTime time.Time
	engine  *jsEngine
}

// NewStreamSession prepares a stream-scoped hook runner for scriptPath.
func NewStreamSession(scriptPath string, timeout time.Duration) (*StreamSession, error) {
	if timeout <= 0 {
		timeout = time.Second
	}
	program, modTime, err := getJSProgramWithModTime(scriptPath)
	if err != nil {
		return nil, err
	}
	engine := newJSEngine(nil)
	if err := engine.runProgram(program, timeout); err != nil {
		return nil, err
	}
	return &StreamSession{
		path:    scriptPath,
		timeout: timeout,
		program: program,
		modTime: modTime,
		engine:  engine,
	}, nil
}

// ApplyChunk invokes on_after_stream_response without re-running top-level script
// unless the underlying file was updated.
func (s *StreamSession) ApplyChunk(in StreamChunkHookInput) (StreamChunkHookOutput, error) {
	out := StreamChunkHookOutput{Chunk: in.Chunk, Headers: cloneHeader(in.ResponseHeaders)}
	if s == nil {
		return out, errors.New("stream session is nil")
	}
	if err := s.ensureEngine(); err != nil {
		return out, err
	}
	deadline := time.Now().Add(s.timeout)
	reqCtx := map[string]any{
		"body":    string(in.RequestBody),
		"headers": in.RequestHeaders,
		"url":     "",
	}
	jsCtx := s.engine.vm.NewObject()
	_ = jsCtx.Set("id", in.RequestID)
	_ = jsCtx.Set("body", nil)
	_ = jsCtx.Set("req", reqCtx)
	_ = jsCtx.Set("protocol", in.Protocol)
	_ = jsCtx.Set("headers", headerToAnyMap(in.ResponseHeaders))
	_ = jsCtx.Set("chunk", in.Chunk)
	historyChunksValue, errHistory := s.engine.frozenStringArray(in.HistoryChunks)
	if errHistory != nil {
		return out, errHistory
	}
	if errDefine := jsCtx.DefineDataProperty("history_chunks", historyChunksValue, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); errDefine != nil {
		return out, errDefine
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return out, errJSTimeout
	}
	jsVal, err := s.engine.callFunction("on_after_stream_response", remaining, jsCtx)
	if err != nil {
		if errors.Is(err, ErrFunctionNotFound) {
			return out, nil
		}
		return out, err
	}
	return exportStreamChunkHookResult(jsVal, out)
}

func (s *StreamSession) ensureEngine() error {
	program, modTime, err := getJSProgramWithModTime(s.path)
	if err != nil {
		return err
	}
	if s.engine != nil && s.program == program && s.modTime.Equal(modTime) {
		return nil
	}
	engine := newJSEngine(nil)
	if err := engine.runProgram(program, s.timeout); err != nil {
		return err
	}
	s.program = program
	s.modTime = modTime
	s.engine = engine
	return nil
}
