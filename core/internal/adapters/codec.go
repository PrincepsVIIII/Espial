package adapters

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

const MaxLineBytes = 1 << 20

type Codec struct {
	reader *bufio.Reader
	writer io.Writer
	write  sync.Mutex
	max    int
}

func NewCodec(reader io.Reader, writer io.Writer, maximum int) *Codec {
	if maximum <= 0 {
		maximum = MaxLineBytes
	}
	return &Codec{reader: bufio.NewReaderSize(reader, maximum+2), writer: writer, max: maximum}
}

func (codec *Codec) Read() (Envelope, error) {
	line, err := codec.reader.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) {
		for errors.Is(err, bufio.ErrBufferFull) {
			_, err = codec.reader.ReadSlice('\n')
		}
		return Envelope{}, runtimeError("line_too_large")
	}
	if errors.Is(err, io.EOF) && len(line) > 0 {
		return Envelope{}, runtimeError("partial_line")
	}
	if err != nil {
		return Envelope{}, err
	}
	line = bytes.TrimSuffix(line, []byte{'\n'})
	line = bytes.TrimSuffix(line, []byte{'\r'})
	if len(line) == 0 || len(line) > codec.max {
		if len(line) > codec.max {
			return Envelope{}, runtimeError("line_too_large")
		}
		return Envelope{}, runtimeError("invalid_envelope")
	}
	return DecodeEnvelope(line)
}

func (codec *Codec) Write(envelope Envelope) error {
	if err := ValidateEnvelope(envelope); err != nil {
		return err
	}
	encoded, err := json.Marshal(envelope)
	if err != nil || len(encoded) > codec.max {
		return runtimeError("line_too_large")
	}
	codec.write.Lock()
	defer codec.write.Unlock()
	if _, err := codec.writer.Write(append(encoded, '\n')); err != nil {
		return runtimeError("write_failed")
	}
	return nil
}
