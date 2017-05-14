package smuxnet

import (
	"encoding/binary"
	"io"

	"fmt"

	"hash/crc32"

	"sync"

	"errors"

	"github.com/bkaradzic/go-lz4"
)

const (
	bodyTypeRaw byte = iota
	bodyTypeCompressedLz4
)

type header [9]byte

func (h *header) GetCRC() uint32 {
	return binary.LittleEndian.Uint32(h[:4])
}

func (h *header) SetCRC(crc uint32) {
	binary.LittleEndian.PutUint32(h[:4], crc)
}

func (h *header) GetPayloadSize() uint32 {
	return binary.LittleEndian.Uint32(h[4:8])
}

func (h *header) SetPayloadSize(sz uint32) {
	binary.LittleEndian.PutUint32(h[4:8], sz)
}

func (h *header) GetBodyType() byte {
	return h[8]
}

func (h *header) SetBodyType(bt byte) {
	h[8] = bt
}

func writeFrame(wr io.Writer, frame []byte, minCompressLen int, lock *sync.Mutex) (err error) {
	bodyType := bodyTypeRaw
	if len(frame) > minCompressLen {
		bodyType = bodyTypeCompressedLz4
		frame, err = lz4.Encode(nil, frame)
		if err != nil {
			return err
		}
	}
	header := new(header)
	header.SetBodyType(bodyType)
	header.SetCRC(crc32.ChecksumIEEE(frame))
	header.SetPayloadSize(uint32(len(frame)))

	if lock != nil {
		lock.Lock()
		defer lock.Unlock()
	}
	if _, err = wr.Write(header[:]); err != nil {
		return err
	}
	_, err = wr.Write(frame)
	return err
}

func readFrame(r io.Reader) ([]byte, error) {
	header := header{}
	if _, err := io.ReadFull(r, header[:]); err != nil {
		if err == io.EOF {
			return nil, err
		}
		return nil, fmt.Errorf("readFrameHeader: %v", err)
	}
	data := make([]byte, header.GetPayloadSize())
	if _, err := io.ReadFull(r, data); err != nil {
		if err == io.EOF {
			return nil, err
		}
		return nil, fmt.Errorf("readFrame: %v", err)
	}

	if crc32.ChecksumIEEE(data) != header.GetCRC() {
		return nil, errors.New("invalid crc")
	}

	switch header.GetBodyType() {
	case bodyTypeCompressedLz4:
		data, err := lz4.Decode(nil, data)
		if err != nil {
			return nil, fmt.Errorf("lz4: %v", err)
		}
		return data, nil
	}
	return data, nil
}
