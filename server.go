package smuxnet

import (
	"net"
	"time"

	"fmt"

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
	for {
		// new node has connected to us
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go s.serve(conn, srcFn)
	}
	return nil
}

func (s *server) serve(conn net.Conn, srcFn chanserv.SourceFunc) {
	// Setup server side of smux
	session, err := smux.Server(conn, nil)
	if err != nil {
		fmt.Println("error creating session", err)
		return
	}
	for {
		stream, err := session.AcceptStream()
		if err != nil {
			fmt.Println("error accepting stream", err)
			break
		}
		go s.processStream(stream, srcFn)
	}
	session.Close()
}

func (s *server) processStream(stream *smux.Stream, srcFn chanserv.SourceFunc) {
	if s.writeTimeout > 0 {
		stream.SetWriteDeadline(time.Now().Add(s.writeTimeout))
	}
	if s.readTimeout > 0 {
		stream.SetReadDeadline(time.Now().Add(s.readTimeout))
	}

	buf, err := readFrame(stream)
	if err != nil {
		fmt.Println("error reading request", err)
		return
	}
	for src := range srcFn(buf) {
		go func(s chanserv.Source) {
			for frame := range src.Out() {
				_, err := stream.Write(frame.Bytes())
				if err != nil {
					fmt.Println("error writing to stream", err)
				}
			}
		}(src)
	}
	stream.Close()
}
