package smuxnet

import (
	"net"
	"time"

	"io"

	"github.com/xtaci/smux"
	"github.com/zenhotels/chanserv"
)

func NewServer(readTimeout, writeTimeout time.Duration) (*server, error) {
	return &server{
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
	}, nil
}

type server struct {
	readTimeout  time.Duration
	writeTimeout time.Duration
}

func (s *server) ListenAndServe(addr string, srcFn chanserv.SourceFunc) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	conn, err := listener.Accept()

	// Setup server side of smux
	session, err := smux.Server(conn, nil)
	for {
		stream, err := session.AcceptStream()
		if err != nil {
			return err
		}
		go s.processStream(stream, srcFn)

	}
	return nil
}

func (s *server) processStream(stream *smux.Stream, srcFn chanserv.SourceFunc) {
	buf := make([]byte, 1024)
	io.ReadFull(stream, buf)
	for src := range srcFn(buf) {
		go func(s chanserv.Source) {
			for frame := range src.Out() {
				stream.Write(frame.Bytes())
			}
		}(src)
	}
}
