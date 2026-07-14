package jshandler

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

type jsEngine struct {
	vm            *goja.Runtime
	consoleLogger func(string)
}

const maxJSScriptBytes = 8 * 1024 * 1024

var ErrFunctionNotFound = errors.New("function not found")
var errJSTimeout = errors.New("javascript execution timeout")

func newJSEngine(consoleLogger func(string)) *jsEngine {
	if consoleLogger == nil {
		consoleLogger = func(message string) {
			slog.Info("jshandler console", "message", message)
		}
	}
	engine := &jsEngine{
		vm:            goja.New(),
		consoleLogger: consoleLogger,
	}
	engine.initConsole()
	return engine
}

func (engine *jsEngine) initConsole() {
	console := engine.vm.NewObject()
	consoleLogWrapper := func(call goja.FunctionCall) goja.Value {
		args := make([]string, len(call.Arguments))
		for i, arg := range call.Arguments {
			args[i] = fmt.Sprint(arg.Export())
		}
		engine.consoleLogger(strings.Join(args, " "))
		return goja.Undefined()
	}
	_ = console.Set("log", consoleLogWrapper)
	_ = engine.vm.Set("console", console)
}

func (engine *jsEngine) runProgram(program *goja.Program, timeout time.Duration) error {
	if program == nil {
		return errors.New("program is nil")
	}
	timer, done := engine.startInterruptTimer(timeout)
	defer engine.stopInterruptTimer(timer, done)
	_, err := engine.vm.RunProgram(program)
	if err != nil {
		return fmt.Errorf("run JS program: %w", err)
	}
	return nil
}

func (engine *jsEngine) startInterruptTimer(timeout time.Duration) (*time.Timer, <-chan struct{}) {
	done := make(chan struct{})
	timer := time.AfterFunc(timeout, func() {
		defer close(done)
		engine.vm.Interrupt(errJSTimeout)
	})
	return timer, done
}

func (engine *jsEngine) stopInterruptTimer(timer *time.Timer, done <-chan struct{}) {
	if timer == nil {
		return
	}
	if timer.Stop() {
		return
	}
	<-done
	engine.vm.ClearInterrupt()
}

func (engine *jsEngine) callFunction(name string, timeout time.Duration, args ...interface{}) (goja.Value, error) {
	jsVal := engine.vm.Get(name)
	if jsVal == nil || goja.IsUndefined(jsVal) {
		return nil, fmt.Errorf("%w: %s", ErrFunctionNotFound, name)
	}
	jsFunc, ok := goja.AssertFunction(jsVal)
	if !ok {
		return nil, fmt.Errorf("function %s is invalid", name)
	}
	jsArgs := make([]goja.Value, len(args))
	for i, arg := range args {
		jsArgs[i] = engine.vm.ToValue(arg)
	}
	timer, done := engine.startInterruptTimer(timeout)
	defer engine.stopInterruptTimer(timer, done)
	return jsFunc(goja.Undefined(), jsArgs...)
}

func (engine *jsEngine) frozenStringArray(values []string) (goja.Value, error) {
	items := make([]interface{}, len(values))
	for i, value := range values {
		items[i] = value
	}
	array := engine.vm.NewArray(items...)
	objectValue := engine.vm.Get("Object")
	if objectValue == nil || goja.IsUndefined(objectValue) {
		return nil, errors.New("Object constructor is unavailable")
	}
	freezeValue := objectValue.ToObject(engine.vm).Get("freeze")
	freezeFunc, ok := goja.AssertFunction(freezeValue)
	if !ok {
		return nil, errors.New("Object.freeze is unavailable")
	}
	if _, errFreeze := freezeFunc(goja.Undefined(), array); errFreeze != nil {
		return nil, errFreeze
	}
	return array, nil
}

// runProgramAndCall shares one deadline for program init and hook invocation.
func (engine *jsEngine) runProgramAndCall(program *goja.Program, hookName string, budget time.Duration, args ...interface{}) (goja.Value, error) {
	if budget <= 0 {
		budget = time.Second
	}
	deadline := time.Now().Add(budget)
	if err := engine.runProgram(program, time.Until(deadline)); err != nil {
		return nil, err
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return nil, errJSTimeout
	}
	return engine.callFunction(hookName, remaining, args...)
}

type jsCachedProgram struct {
	program *goja.Program
	modTime time.Time
}

var (
	jsProgramsMU    sync.RWMutex
	jsProgramsCache = make(map[string]jsCachedProgram)
)

func getJSProgram(path string) (*goja.Program, error) {
	program, _, err := getJSProgramWithModTime(path)
	return program, err
}

func getJSProgramWithModTime(path string) (*goja.Program, time.Time, error) {
	cleanPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return nil, time.Time{}, err
	}
	resolvedPath, err := filepath.EvalSymlinks(cleanPath)
	if err != nil {
		return nil, time.Time{}, err
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return nil, time.Time{}, err
	}
	if info.Size() > maxJSScriptBytes {
		return nil, time.Time{}, fmt.Errorf("JS script %s too large: %d bytes", resolvedPath, info.Size())
	}
	modTime := info.ModTime()

	jsProgramsMU.RLock()
	cached, exists := jsProgramsCache[resolvedPath]
	jsProgramsMU.RUnlock()
	if exists && cached.modTime.Equal(modTime) {
		return cached.program, modTime, nil
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, time.Time{}, err
	}
	compiled, err := goja.Compile(resolvedPath, string(data), false)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("compile %s: %w", resolvedPath, err)
	}

	jsProgramsMU.Lock()
	defer jsProgramsMU.Unlock()
	if cached, exists = jsProgramsCache[resolvedPath]; exists && cached.modTime.Equal(modTime) {
		return cached.program, modTime, nil
	}
	jsProgramsCache[resolvedPath] = jsCachedProgram{program: compiled, modTime: modTime}
	return compiled, modTime, nil
}