package pluginsdk

import (
	"context"
	"net/http"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Shutdown asks the plugin to release resources and then signals Run to
// exit. The actual exit happens in gracefulShutdown so the gRPC server
// has time to send the response.
func (r *runner) Shutdown(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	if r.closing.CompareAndSwap(false, true) {
		close(r.shutdown)
	}
	return &emptypb.Empty{}, nil
}

// gracefulShutdown runs the plugin's Shutdown hook, drains every reverse
// subsystem (jobs / log / settings) BEFORE closing the SDK conn, then
// stops the HTTP/gRPC servers. The order matters: any subsystem still
// holding a stream against a half-closed conn will spew warning logs on
// the way out.
//
// Each step is delegated to a helper so this function reads as a
// shutdown timeline rather than 60 lines of nil-checks.
func (r *runner) gracefulShutdown(grpcSrv *grpc.Server, httpSrv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), r.opts.shutdownWait)
	defer cancel()

	r.runPluginShutdown()
	r.stopJobs()
	r.stopRemoteLogger()
	r.stopSettings()
	r.closeSubsystems()
	r.stopServers(ctx, grpcSrv, httpSrv)
}

// runPluginShutdown invokes the plugin's user-supplied Shutdown hook.
// Errors are logged but never propagate — by this point the host has
// already decided we are going down and there is no useful recovery.
func (r *runner) runPluginShutdown() {
	if err := r.plugin.Shutdown(); err != nil {
		r.logger.Error("plugin shutdown returned error", "error", err)
	}
}

// stopJobs halts the JobScheduler subscribe loop. Must run BEFORE
// closing the SDK conn so the reconnect loop does not fire one last
// Subscribe against a dead conn and bleed warning logs.
func (r *runner) stopJobs() {
	if r.jobsClient != nil {
		r.jobsClient.stop()
	}
}

// stopRemoteLogger drains the LogProxy sender best-effort BEFORE closing
// the SDK conn so the host receives whatever is still in the buffered
// channel, then cancels the sender ctx.
func (r *runner) stopRemoteLogger() {
	if r.remoteLogHandler == nil {
		return
	}
	shutdownRemoteSlogSender(r.remoteLogHandler, r.remoteLogDone)
	if r.remoteLogCancel != nil {
		r.remoteLogCancel()
	}
}

// stopSettings stops the Settings watch goroutine before tearing down
// the gRPC connection so we do not log spurious cancelled-stream errors
// on the way out.
func (r *runner) stopSettings() {
	if r.settingsImpl != nil {
		r.settingsImpl.stopWatchLoop()
	}
}

// closeSubsystems closes the SQL handle and the reverse SDK gRPC conn.
// Errors are logged but do not abort the rest of the shutdown — the
// process is exiting and there is no useful recovery for a Close error
// on a connection we are about to abandon.
func (r *runner) closeSubsystems() {
	if r.ctxImpl != nil && r.ctxImpl.db != nil {
		if err := r.ctxImpl.db.Close(); err != nil {
			r.logger.Warn("close sql.DB", "error", err)
		}
	}
	if r.coreConn != nil {
		if err := r.coreConn.Close(); err != nil {
			r.logger.Warn("close core gRPC conn", "error", err)
		}
	}
}

// stopServers shuts down the plugin's own HTTP and gRPC servers. The
// gRPC server uses GracefulStop with a fallback to Stop if the deadline
// elapses so a stuck handler cannot block the process forever.
func (r *runner) stopServers(ctx context.Context, grpcSrv *grpc.Server, httpSrv *http.Server) {
	if httpSrv != nil {
		_ = httpSrv.Shutdown(ctx)
	}
	stopped := make(chan struct{})
	go func() {
		grpcSrv.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-ctx.Done():
		grpcSrv.Stop()
	}
}
