package renderproc

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"testing"
)

type benchmarkFrame struct {
	Head *string `json:"head,omitempty"`
	Body *string `json:"body,omitempty"`
	Done bool    `json:"done,omitempty"`
}

func benchmarkFrames() []benchmarkFrame {
	frames := []benchmarkFrame{{Head: ptr("<title>Benchmark</title>")}}
	for range 32 {
		frames = append(frames, benchmarkFrame{Body: ptr(string(bytes.Repeat([]byte("x"), 1024)))})
	}
	return append(frames, benchmarkFrame{Done: true})
}

func ptr(value string) *string { return &value }

func BenchmarkNDJSONFrames(b *testing.B) {
	var wire bytes.Buffer
	encoder := json.NewEncoder(&wire)
	for _, frame := range benchmarkFrames() {
		_ = encoder.Encode(frame)
	}
	data := wire.Bytes()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		decoder := json.NewDecoder(bytes.NewReader(data))
		for {
			var frame benchmarkFrame
			if err := decoder.Decode(&frame); err != nil {
				if err != io.EOF {
					b.Fatal(err)
				}
				break
			}
		}
	}
}

func BenchmarkBinaryFrames(b *testing.B) {
	var wire bytes.Buffer
	for _, frame := range benchmarkFrames() {
		kind := byte(3)
		payload := ""
		if frame.Head != nil {
			kind, payload = 1, *frame.Head
		}
		if frame.Body != nil {
			kind, payload = 2, *frame.Body
		}
		wire.WriteByte(kind)
		_ = binary.Write(&wire, binary.BigEndian, uint32(len(payload)))
		wire.WriteString(payload)
	}
	data := wire.Bytes()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		reader := bytes.NewReader(data)
		for reader.Len() > 0 {
			_, _ = reader.ReadByte()
			var size uint32
			_ = binary.Read(reader, binary.BigEndian, &size)
			_, _ = reader.Seek(int64(size), io.SeekCurrent)
		}
	}
}
