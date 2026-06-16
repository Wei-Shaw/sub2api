package openai_ws_v2

import "context"

// EntryInput 是 passthrough v2 数据面的入口参数。
type EntryInput struct {
	Ctx                context.Context
	ClientConn         FrameConn
	UpstreamConn       FrameConn
	FirstClientMessage []byte
	Options            RelayOptions
}

// RunEntry 是 openai_ws_v2 包对外的统一入口。
func RunEntry(input EntryInput) (RelayResult, *RelayExit) {
	return runCaddyStyleRelay(
		input.Ctx,
		input.ClientConn,
		input.UpstreamConn,
		input.FirstClientMessage,
		input.Options,
	)
}
