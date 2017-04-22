package smuxnet

import (
	"net"
	"time"

	"fmt"

	"sync"

	"github.com/xtaci/smux"
	"github.com/zenhotels/chanserv"
)

func NewServer(readTimeout, writeTimeout time.Duration) (*server, error) {
	return &server{
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
		wg:           new(sync.WaitGroup),
		stop:         make(chan struct{}),
	}, nil
}

type server struct {
	readTimeout  time.Duration
	writeTimeout time.Duration
	// this channel is closed when server is asked to stop
	stop chan struct{}
	// this wg used to determine when all processing is finished
	wg *sync.WaitGroup
}

func (s server) Stop() {
	close(s.stop)
	s.Wait()
}

func (s server) Wait() {
	s.wg.Wait()

}

func (s *server) ListenAndServe(addr string, srcFn chanserv.SourceFunc) error {
	s.wg.Add(1)
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
		s.wg.Add(1)
		go s.serve(conn, srcFn)
	}
	s.wg.Done()
	return nil
}

func (s *server) serve(conn net.Conn, srcFn chanserv.SourceFunc) {
	defer s.wg.Done()
	defer conn.Close()

	// Setup server side of smux
	session, err := smux.Server(conn, nil)
	if err != nil {
		fmt.Println("error creating session", err)
		return
	}
	defer fmt.Println("session close")
	defer session.Close()

ACCEPT_LOOP:
	for {
		select {
		case <-s.stop:
			break ACCEPT_LOOP
		default:
		}
		stream, err := session.AcceptStream()
		if err != nil {
			fmt.Println("error accepting stream", err)
			break
		}
		s.wg.Add(1)
		go s.processStream(stream, srcFn)
	}
}

func (s *server) processStream(stream *smux.Stream, srcFn chanserv.SourceFunc) {
	defer fmt.Println("stream close")
	defer s.wg.Done()
	defer stream.Close()
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
	wg := new(sync.WaitGroup)

	for src := range srcFn(buf) {
		wg.Add(1)
		go func(cs chanserv.Source) {
			defer wg.Done()

		FRAME_LOOP:
			for frame := range cs.Out() {
				select {
				case <-s.stop:
					continue FRAME_LOOP
				default:
				}

				fmt.Println("write", string(frame.Bytes()))
				writeFrame(stream, frame.Bytes())
				if err != nil {
					fmt.Println("error writing to stream", err)
				}
			}
		}(src)
	}
	wg.Wait()
}
