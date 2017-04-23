package smuxnet

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/bkaradzic/go-lz4"
)

const rawHeader = "raw!"
const compressionHeader = "lz4!"

func writeFrame(wr io.Writer, frame []byte, minCompressLen int) (err error) {
	header := rawHeader
	if len(frame) > minCompressLen {
		header = compressionHeader
		frame, err = lz4.Encode(nil, frame)
		if err != nil {
			return err
		}
	}

	buf := make([]byte, 12)
	binary.LittleEndian.PutUint64(buf[:8], uint64(len(frame)))
	copy(buf[8:12], header)
	if _, err = wr.Write(buf); err != nil {
		return
	}
	_, err = io.Copy(wr, bytes.NewReader(frame))
	return
}

func readFrame(r io.Reader) ([]byte, error) {

	buf := make([]byte, 12)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	frameSize := binary.LittleEndian.Uint64(buf[:8])
	framebuf := bytes.NewBuffer(make([]byte, 0, frameSize))
	if _, err := io.CopyN(framebuf, r, int64(frameSize)); err != nil {
		return nil, err
	}

	data := framebuf.Bytes()
	if bytes.Equal([]byte(compressionHeader), buf[8:]) {
		return lz4.Decode(nil, data)
	}
	return data, nil
}
