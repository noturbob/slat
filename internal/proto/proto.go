package proto

import (
	"encoding/binary"
	"errors"
	"io"
)

// FrameType identifies the kind of message in a client<->daemon frame.
// Only used for the client->daemon direction (input, resize, hello).
// The daemon->client direction is an unframed raw byte stream, since it's
// already fully-formed terminal output with no message boundaries needed.
type FrameType byte

const (
	TypeHello  FrameType = 0x01 // client->daemon: initial terminal size
	TypeInput  FrameType = 0x02 // client->daemon: raw keystrokes/paste bytes
	TypeResize FrameType = 0x03 // client->daemon: new terminal size
)

type Frame struct {
	Type    FrameType
	Payload []byte
}

var ErrFrameTooLarge = errors.New("proto: frame too large")

const maxFrameSize = 1 << 20 // 1MB safety cap against a corrupt stream

// WriteFrame writes a length-prefixed frame: [1 byte type][4 byte BE len][payload].
func WriteFrame(w io.Writer, t FrameType, payload []byte) error {
	header := make([]byte, 5)
	header[0] = byte(t)
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	if _, err := w.Write(header); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	_, err := w.Write(payload)
	return err
}

// ReadFrame blocks until a full frame has been read from r.
func ReadFrame(r io.Reader) (Frame, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		return Frame{}, err
	}
	t := FrameType(header[0])
	n := binary.BigEndian.Uint32(header[1:])
	if n > maxFrameSize {
		return Frame{}, ErrFrameTooLarge
	}
	payload := make([]byte, n)
	if n > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return Frame{}, err
		}
	}
	return Frame{Type: t, Payload: payload}, nil
}

// EncodeSize/DecodeSize pack a terminal size into a 4-byte payload.
func EncodeSize(cols, rows int) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint16(b[0:2], uint16(cols))
	binary.BigEndian.PutUint16(b[2:4], uint16(rows))
	return b
}

func DecodeSize(b []byte) (cols, rows int) {
	if len(b) < 4 {
		return 80, 24
	}
	cols = int(binary.BigEndian.Uint16(b[0:2]))
	rows = int(binary.BigEndian.Uint16(b[2:4]))
	return
}