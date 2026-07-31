package adapters

import (
	"bytes"
	"sync"
)

const (
	MaxDiagnosticBytes = 64 * 1024
	MaxDiagnosticLine  = 16 * 1024
)

type DiagnosticBuffer struct {
	mu      sync.Mutex
	entries [][]byte
	pending []byte
	discard bool
	size    int
	maximum int
	lineMax int
	dropped int
	secrets []string
}

func NewDiagnosticBuffer(maximum, lineMaximum int) *DiagnosticBuffer {
	if maximum <= 0 {
		maximum = MaxDiagnosticBytes
	}
	if lineMaximum <= 0 {
		lineMaximum = MaxDiagnosticLine
	}
	return &DiagnosticBuffer{maximum: maximum, lineMax: lineMaximum}
}

func (buffer *DiagnosticBuffer) SetSecrets(secrets []string) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	buffer.secrets = append([]string(nil), secrets...)
}

func (buffer *DiagnosticBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	for _, value := range data {
		if value == '\n' {
			buffer.commitLocked()
			continue
		}
		if buffer.discard {
			continue
		}
		if len(buffer.pending) >= buffer.lineMax {
			buffer.discard = true
			buffer.dropped++
			continue
		}
		buffer.pending = append(buffer.pending, value)
	}
	return len(data), nil
}

func (buffer *DiagnosticBuffer) AddLine(line string) {
	_, _ = buffer.Write(append([]byte(line), '\n'))
}

func (buffer *DiagnosticBuffer) Snapshot() (string, int) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	var output bytes.Buffer
	for _, entry := range buffer.entries {
		output.Write(entry)
		output.WriteByte('\n')
	}
	return Redact(output.String(), buffer.secrets), buffer.dropped
}

func (buffer *DiagnosticBuffer) commitLocked() {
	line := append([]byte(nil), buffer.pending...)
	buffer.pending = buffer.pending[:0]
	buffer.discard = false
	if len(line) == 0 {
		return
	}
	line = []byte(Redact(string(line), buffer.secrets))
	for buffer.size+len(line)+1 > buffer.maximum && len(buffer.entries) > 0 {
		buffer.size -= len(buffer.entries[0]) + 1
		buffer.entries = buffer.entries[1:]
		buffer.dropped++
	}
	if len(line)+1 > buffer.maximum {
		buffer.dropped++
		return
	}
	buffer.entries = append(buffer.entries, line)
	buffer.size += len(line) + 1
}
