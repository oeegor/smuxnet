package smuxnet

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/pierrec/lz4"
)

const compressionHeader = "lz4!"

func writeFrame(wr io.Writer, frame []byte) (err error) {
	comp := make([]byte, lz4.CompressBlockBound(len(frame)))
	size, err := lz4.CompressBlock(frame, comp, 0)
	if err != nil {
		return err
	}
	if size >= len(frame) {
		// discard compressed results
		return writeFrame(wr, frame)
	}
	comp = comp[:size]
	frameSize := size + len(compressionHeader) + 8
	buf := make([]byte, 8+len(compressionHeader)+8)

	binary.LittleEndian.PutUint64(buf, uint64(frameSize))
	copy(buf[8:], compressionHeader)
	binary.LittleEndian.PutUint64(buf[12:], uint64(len(frame)))
	if _, err = wr.Write(buf); err != nil {
		return
	}
	_, err = io.Copy(wr, bytes.NewReader(comp))
	return
}

func readFrame(r io.Reader) ([]byte, error) {
	buf := make([]byte, 8)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	frameSize := binary.LittleEndian.Uint64(buf)
	framebuf := bytes.NewBuffer(make([]byte, 0, frameSize))
	if _, err := io.CopyN(framebuf, r, int64(frameSize)); err != nil {
		return nil, err
	}

	data := framebuf.Bytes()
	if !bytes.Equal([]byte(compressionHeader), data[:4]) {
		// doesn't have a compression header
		return data, nil
	}
	uncompressedSize := binary.LittleEndian.Uint64(data[4:])
	uncompressed := make([]byte, uncompressedSize)
	size, err := lz4.UncompressBlock(data[12:], uncompressed, 0)
	if err != nil {
		return nil, err
	}
	uncompressed = uncompressed[:size]
	return uncompressed, nil
}
