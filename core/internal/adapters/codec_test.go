package adapters

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCodecExactLineBoundaryAndWrite(t *testing.T) {
	envelope := Envelope{
		ProtocolVersion: ProtocolV1, Kind: KindNotification, Operation: OperationReady,
		SentAt: time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC), Payload: json.RawMessage(`{}`),
	}
	encoded, _ := json.Marshal(envelope)
	input := append(append([]byte(nil), encoded...), '\n')
	codec := NewCodec(bytes.NewReader(input), nil, len(encoded))
	if _, err := codec.Read(); err != nil {
		t.Fatalf("read exact boundary: %v", err)
	}
	codec = NewCodec(bytes.NewReader(input), nil, len(encoded)-1)
	_, err := codec.Read()
	requireRuntimeCode(t, err, "line_too_large")

	var output bytes.Buffer
	codec = NewCodec(nil, &output, len(encoded))
	if err := codec.Write(envelope); err != nil {
		t.Fatalf("write exact boundary: %v", err)
	}
	if !bytes.Equal(output.Bytes(), input) {
		t.Fatalf("output differs: %q", output.Bytes())
	}
}

func TestCodecRejectsPartialAndOversizedLines(t *testing.T) {
	codec := NewCodec(strings.NewReader(`{"incomplete":true}`), nil, 1024)
	_, err := codec.Read()
	requireRuntimeCode(t, err, "partial_line")

	codec = NewCodec(strings.NewReader(strings.Repeat("x", 1025)+"\n"), nil, 1024)
	_, err = codec.Read()
	requireRuntimeCode(t, err, "line_too_large")
}
