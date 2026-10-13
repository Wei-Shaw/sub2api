//go:build windows

package service

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func launchAutoWorker(node, script, origin, ticket string, sink io.Writer) (func() error, string) {
	var session uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &session); err != nil {
		return nil, "session_lookup_failed"
	}
	if session != 0 {
		return launchSameSessionWorker(node, script, origin, ticket, sink)
	}
	return launchInteractiveWorker(node, script, origin, ticket, sink)
}

// The always-on server runs as SYSTEM in session 0. Browsers started in that
// session cannot appear on a signed-in user's desktop. Start only the browser
// helper in the configured user's interactive session; pass its short-lived
// capability through an inherited stdin pipe, never the command line or disk.
func launchInteractiveWorker(node, script, origin, ticket string, sink io.Writer) (func() error, string) {
	expectedSID := os.Getenv("ACCOUNT_VAULT_AUTO_WORKER_USER_SID")
	if expectedSID == "" {
		return nil, "interactive_user_not_configured"
	}
	session := windows.WTSGetActiveConsoleSessionId()
	if session == ^uint32(0) || session == 0 {
		return nil, "interactive_session_unavailable"
	}
	if err := enableSystemTcbPrivilege(); err != nil {
		return nil, "interactive_session_unavailable"
	}
	var token windows.Token
	if err := windows.WTSQueryUserToken(session, &token); err != nil {
		return nil, "interactive_session_unavailable"
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil || user.User.Sid.String() != expectedSID {
		return nil, "interactive_user_mismatch"
	}
	var envBlock *uint16
	if err := windows.CreateEnvironmentBlock(&envBlock, token, false); err != nil {
		return nil, "interactive_environment_failed"
	}
	defer windows.DestroyEnvironmentBlock(envBlock)
	// No server secrets are inherited. The helper receives only the signed-in
	// user's ordinary browser/profile environment; executable paths are absolute.
	inputReader, inputWriter, err := os.Pipe()
	if err != nil {
		return nil, "pipe_failed"
	}
	defer inputReader.Close()
	defer inputWriter.Close()
	outputReader, outputWriter, err := os.Pipe()
	if err != nil {
		return nil, "pipe_failed"
	}
	defer outputWriter.Close()
	// Only the child's ends are inheritable. A one-time capability is never
	// exposed in process arguments, environment, logs, or a temporary file.
	if windows.SetHandleInformation(windows.Handle(inputReader.Fd()), windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT) != nil ||
		windows.SetHandleInformation(windows.Handle(inputWriter.Fd()), windows.HANDLE_FLAG_INHERIT, 0) != nil ||
		windows.SetHandleInformation(windows.Handle(outputWriter.Fd()), windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT) != nil ||
		windows.SetHandleInformation(windows.Handle(outputReader.Fd()), windows.HANDLE_FLAG_INHERIT, 0) != nil {
		_ = outputReader.Close()
		return nil, "pipe_failed"
	}
	args := []string{node, script, "--origin", origin, "--concurrency", strconv.Itoa(VaultRotationMaxConcurrency)}
	for i := range args {
		args[i] = windows.EscapeArg(args[i])
	}
	app, _ := windows.UTF16PtrFromString(node)
	command, _ := windows.UTF16PtrFromString(strings.Join(args, " "))
	workdir, _ := windows.UTF16PtrFromString(filepath.Dir(filepath.Dir(script)))
	desktop, _ := windows.UTF16PtrFromString("winsta0\\default")
	startup := windows.StartupInfo{
		Cb: uint32(unsafe.Sizeof(windows.StartupInfo{})), Desktop: desktop,
		Flags:     windows.STARTF_USESTDHANDLES,
		StdInput:  windows.Handle(inputReader.Fd()),
		StdOutput: windows.Handle(outputWriter.Fd()),
		StdErr:    windows.Handle(outputWriter.Fd()),
	}
	var process windows.ProcessInformation
	if err := windows.CreateProcessAsUser(token, app, command, nil, nil, true,
		windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_NO_WINDOW,
		envBlock, workdir, &startup, &process); err != nil {
		_ = outputReader.Close()
		return nil, "interactive_start_failed"
	}
	_ = windows.CloseHandle(process.Thread)
	_ = inputReader.Close()
	_ = outputWriter.Close()
	drained := make(chan struct{})
	go func() {
		_, _ = io.Copy(sink, outputReader)
		_ = outputReader.Close()
		close(drained)
	}()
	if _, err := io.WriteString(inputWriter, ticket+"\n"); err != nil {
		_ = windows.TerminateProcess(process.Process, 1)
		_, _ = windows.WaitForSingleObject(process.Process, windows.INFINITE)
		_ = windows.CloseHandle(process.Process)
		<-drained
		return nil, "pipe_failed"
	}
	_ = inputWriter.Close()
	return func() error {
		_, err := windows.WaitForSingleObject(process.Process, windows.INFINITE)
		var exit uint32
		if err == nil {
			err = windows.GetExitCodeProcess(process.Process, &exit)
		}
		_ = windows.CloseHandle(process.Process)
		<-drained
		if err != nil || exit != 0 {
			return errors.New("worker exited")
		}
		return nil
	}, ""
}

func enableSystemTcbPrivilege() error {
	var processToken windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(),
		windows.TOKEN_QUERY|windows.TOKEN_ADJUST_PRIVILEGES, &processToken); err != nil {
		return err
	}
	defer processToken.Close()
	name, err := windows.UTF16PtrFromString("SeTcbPrivilege")
	if err != nil {
		return err
	}
	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, name, &luid); err != nil {
		return err
	}
	state := windows.Tokenprivileges{PrivilegeCount: 1}
	state.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
	return windows.AdjustTokenPrivileges(processToken, false, &state, 0, nil, nil)
}
