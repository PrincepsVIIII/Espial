package adapters

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type ProcessOptions struct {
	Clock              Clock
	StartupTimeout     time.Duration
	RequestTimeout     time.Duration
	ShutdownTimeout    time.Duration
	TerminationTimeout time.Duration
	MaxLineBytes       int
	DiagnosticBytes    int
	DiagnosticLine     int
}

func DefaultProcessOptions() ProcessOptions {
	return ProcessOptions{
		Clock: systemClock{}, StartupTimeout: 10 * time.Second, RequestTimeout: 30 * time.Second,
		ShutdownTimeout: 5 * time.Second, TerminationTimeout: 2 * time.Second,
		MaxLineBytes: MaxLineBytes, DiagnosticBytes: MaxDiagnosticBytes, DiagnosticLine: MaxDiagnosticLine,
	}
}

type pendingCall struct {
	requestID string
	operation string
	response  chan Envelope
	delivered bool
}

type Process struct {
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	codec       *Codec
	diagnostics *DiagnosticBuffer
	options     ProcessOptions
	cancel      context.CancelFunc

	mu         sync.Mutex
	pending    *pendingCall
	readySeen  bool
	stopping   bool
	stopped    bool
	version    string
	failureErr error
	exitErr    error
	ready      chan Envelope
	failed     chan struct{}
	done       chan struct{}
	failOnce   sync.Once
	stopMu     sync.Mutex
}

func StartProcess(ctx context.Context, descriptor Descriptor, options ProcessOptions) (*Process, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateDescriptor(descriptor); err != nil {
		return nil, err
	}
	options = normalizeProcessOptions(options)
	processContext, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(processContext, descriptor.Executable, descriptor.Arguments...)
	command.Dir = descriptor.WorkingDirectory
	if command.Dir == "" {
		command.Dir = filepath.Dir(descriptor.Executable)
	}
	keys := make([]string, 0, len(descriptor.Environment))
	for key := range descriptor.Environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		command.Env = append(command.Env, key+"="+descriptor.Environment[key])
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		cancel()
		return nil, runtimeError("process_start_failed")
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		cancel()
		return nil, runtimeError("process_start_failed")
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		cancel()
		return nil, runtimeError("process_start_failed")
	}
	if err := command.Start(); err != nil {
		cancel()
		return nil, runtimeError("process_start_failed")
	}
	process := &Process{
		cmd: command, stdin: stdin, codec: NewCodec(stdout, stdin, options.MaxLineBytes),
		diagnostics: NewDiagnosticBuffer(options.DiagnosticBytes, options.DiagnosticLine),
		options:     options, cancel: cancel, version: ProtocolV1,
		ready: make(chan Envelope, 1), failed: make(chan struct{}), done: make(chan struct{}),
	}
	go process.readStdout()
	go func() { _, _ = io.Copy(process.diagnostics, stderr) }()
	go process.wait()

	startupContext, startupCancel := context.WithTimeout(ctx, options.StartupTimeout)
	defer startupCancel()
	select {
	case ready := <-process.ready:
		if ready.ProtocolVersion != ProtocolV1 {
			major, _, _ := parseVersion(ready.ProtocolVersion)
			if major != 1 {
				process.fail(runtimeError("unsupported_protocol_major"))
			} else {
				process.fail(runtimeError("unsupported_protocol_minor"))
			}
			_ = process.Stop(context.Background())
			return nil, process.Failure()
		}
		return process, nil
	case <-process.failed:
		_ = process.Stop(context.Background())
		return nil, process.Failure()
	case <-process.done:
		return nil, runtimeError("process_exit")
	case <-startupContext.Done():
		process.fail(runtimeError("startup_timeout"))
		_ = process.Stop(context.Background())
		return nil, runtimeError("startup_timeout")
	}
}

func (process *Process) Call(ctx context.Context, operation string, payload any) (json.RawMessage, error) {
	return process.call(ctx, operation, payload, false)
}

func (process *Process) call(ctx context.Context, operation string, payload any, allowStopping bool) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !requestOperation(operation) {
		return nil, runtimeError("unsupported_operation")
	}
	callContext, cancel := context.WithTimeout(ctx, process.options.RequestTimeout)
	defer cancel()
	deadline, _ := callContext.Deadline()
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, runtimeError("invalid_request")
	}
	requestID, err := newRequestID()
	if err != nil {
		return nil, runtimeError("request_id_failed")
	}
	pending := &pendingCall{requestID: requestID, operation: operation, response: make(chan Envelope, 1)}
	process.mu.Lock()
	if process.failureErr != nil {
		err := process.failureErr
		process.mu.Unlock()
		return nil, err
	}
	if process.stopping && !allowStopping {
		process.mu.Unlock()
		return nil, runtimeError("process_stopping")
	}
	if process.pending != nil {
		process.mu.Unlock()
		return nil, runtimeError("request_in_flight")
	}
	process.pending = pending
	version := process.version
	process.mu.Unlock()
	defer process.clearPending(pending)

	envelope := Envelope{
		ProtocolVersion: version, Kind: KindRequest, Operation: operation,
		RequestID: requestID, SentAt: process.options.Clock.Now().UTC(), Deadline: &deadline,
		Payload: encoded,
	}
	if err := process.codec.Write(envelope); err != nil {
		process.fail(err)
		return nil, err
	}
	select {
	case response := <-pending.response:
		if callContext.Err() != nil {
			err := runtimeError("response_after_deadline")
			if !allowStopping {
				process.fail(err)
			}
			return nil, err
		}
		if response.Error != nil {
			return nil, runtimeError("adapter_error")
		}
		return append(json.RawMessage(nil), response.Payload...), nil
	case <-process.failed:
		return nil, process.Failure()
	case <-process.done:
		return nil, runtimeError("process_exit")
	case <-callContext.Done():
		code := "request_canceled"
		if errors.Is(callContext.Err(), context.DeadlineExceeded) {
			code = "request_timeout"
		}
		err := runtimeError(code)
		if !allowStopping {
			process.fail(err)
		}
		return nil, err
	}
}

func (process *Process) SetProtocolVersion(version string) {
	process.mu.Lock()
	process.version = version
	process.mu.Unlock()
}

func (process *Process) SetDiagnosticSecrets(secrets []string) {
	process.diagnostics.SetSecrets(secrets)
}

func (process *Process) Diagnostics() (string, int) { return process.diagnostics.Snapshot() }

func (process *Process) Failure() error {
	process.mu.Lock()
	defer process.mu.Unlock()
	if process.failureErr == nil {
		return nil
	}
	return process.failureErr
}

func (process *Process) Stop(ctx context.Context) error {
	process.stopMu.Lock()
	defer process.stopMu.Unlock()
	process.mu.Lock()
	if process.stopped {
		process.mu.Unlock()
		return nil
	}
	process.stopping = true
	process.mu.Unlock()

	shutdownContext, shutdownCancel := context.WithTimeout(ctx, process.options.ShutdownTimeout)
	if process.Failure() == nil {
		_, _ = process.call(shutdownContext, OperationShutdown, map[string]any{}, true)
	}
	shutdownCancel()
	_ = process.stdin.Close()
	if process.waitForExit(ctx, process.options.ShutdownTimeout) {
		process.finishStop()
		return nil
	}
	if process.cmd.Process != nil {
		_ = process.cmd.Process.Signal(os.Interrupt)
	}
	if process.waitForExit(ctx, process.options.TerminationTimeout) {
		process.finishStop()
		return nil
	}
	if process.cmd.Process != nil {
		_ = process.cmd.Process.Kill()
	}
	if !process.waitForExit(ctx, process.options.TerminationTimeout) {
		process.cancel()
		return runtimeError("process_stop_timeout")
	}
	process.finishStop()
	return nil
}

func (process *Process) readStdout() {
	for {
		envelope, err := process.codec.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			process.fail(err)
			return
		}
		process.dispatch(envelope)
	}
}

func (process *Process) dispatch(envelope Envelope) {
	process.mu.Lock()
	if process.failureErr != nil {
		process.mu.Unlock()
		return
	}
	if envelope.Kind == KindNotification {
		switch envelope.Operation {
		case OperationReady:
			if process.readySeen || process.pending != nil {
				process.mu.Unlock()
				process.fail(runtimeError("duplicate_ready"))
				return
			}
			process.readySeen = true
			process.mu.Unlock()
			select {
			case process.ready <- envelope:
			default:
				process.fail(runtimeError("duplicate_ready"))
			}
		case OperationLog:
			process.mu.Unlock()
			var logLine struct {
				Message string `json:"message"`
			}
			decoder := json.NewDecoder(jsonBytes(envelope.Payload))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&logLine) != nil || logLine.Message == "" {
				process.fail(runtimeError("invalid_log_notification"))
				return
			}
			process.diagnostics.AddLine(logLine.Message)
		default:
			process.mu.Unlock()
			process.fail(runtimeError("unsupported_notification"))
		}
		return
	}
	if envelope.Kind != KindResponse {
		process.mu.Unlock()
		process.fail(runtimeError("unexpected_request"))
		return
	}
	if process.pending == nil {
		process.mu.Unlock()
		process.fail(runtimeError("unsolicited_response"))
		return
	}
	if envelope.ProtocolVersion != process.version || envelope.RequestID != process.pending.requestID ||
		envelope.Operation != process.pending.operation {
		process.mu.Unlock()
		process.fail(runtimeError("response_mismatch"))
		return
	}
	if process.pending.delivered {
		process.mu.Unlock()
		process.fail(runtimeError("duplicate_response"))
		return
	}
	process.pending.delivered = true
	response := process.pending.response
	process.mu.Unlock()
	select {
	case response <- envelope:
	default:
		process.fail(runtimeError("duplicate_response"))
	}
}

func (process *Process) wait() {
	err := process.cmd.Wait()
	process.mu.Lock()
	process.exitErr = err
	stopping := process.stopping
	process.mu.Unlock()
	if !stopping {
		process.fail(runtimeError("process_exit"))
	}
	close(process.done)
}

func (process *Process) fail(err error) {
	process.failOnce.Do(func() {
		process.mu.Lock()
		process.failureErr = safeRuntimeError(err)
		process.mu.Unlock()
		close(process.failed)
		if process.cmd.Process != nil {
			_ = process.cmd.Process.Kill()
		}
	})
}

func (process *Process) clearPending(pending *pendingCall) {
	process.mu.Lock()
	if process.pending == pending {
		process.pending = nil
	}
	process.mu.Unlock()
}

func (process *Process) waitForExit(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-process.done:
		return true
	case <-ctx.Done():
		return false
	case <-timer.C:
		return false
	}
}

func (process *Process) finishStop() {
	process.cancel()
	process.mu.Lock()
	process.stopped = true
	process.pending = nil
	process.mu.Unlock()
}

func normalizeProcessOptions(options ProcessOptions) ProcessOptions {
	defaults := DefaultProcessOptions()
	if options.Clock == nil {
		options.Clock = defaults.Clock
	}
	if options.StartupTimeout <= 0 {
		options.StartupTimeout = defaults.StartupTimeout
	}
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = defaults.RequestTimeout
	}
	if options.ShutdownTimeout <= 0 {
		options.ShutdownTimeout = defaults.ShutdownTimeout
	}
	if options.TerminationTimeout <= 0 {
		options.TerminationTimeout = defaults.TerminationTimeout
	}
	if options.MaxLineBytes <= 0 {
		options.MaxLineBytes = defaults.MaxLineBytes
	}
	if options.DiagnosticBytes <= 0 {
		options.DiagnosticBytes = defaults.DiagnosticBytes
	}
	if options.DiagnosticLine <= 0 {
		options.DiagnosticLine = defaults.DiagnosticLine
	}
	return options
}

func newRequestID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	encoded := hex.EncodeToString(value[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", encoded[0:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:32]), nil
}

func safeRuntimeError(err error) error {
	var runtime *RuntimeError
	if errors.As(err, &runtime) {
		return &RuntimeError{Code: runtime.Code}
	}
	return runtimeError("internal_failure")
}

func jsonBytes(value []byte) io.Reader { return &byteReader{value: value} }

type byteReader struct {
	value  []byte
	offset int
}

func (reader *byteReader) Read(output []byte) (int, error) {
	if reader.offset >= len(reader.value) {
		return 0, io.EOF
	}
	count := copy(output, reader.value[reader.offset:])
	reader.offset += count
	return count, nil
}
