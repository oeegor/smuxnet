package smuxnet

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/xtaci/smux"
	"github.com/zenhotels/chanserv"
)

func NewServer(readTimeout, writeTimeout time.Duration, minCompressLen int) (*server, error) {
	return &server{
		minCompressLen: minCompressLen,
		readTimeout:    readTimeout,
		writeTimeout:   writeTimeout,
		wg:             new(sync.WaitGroup),
		stop:           make(chan struct{}),
	}, nil
}

type server struct {
	minCompressLen int
	readTimeout    time.Duration
	writeTimeout   time.Duration
	// this channel is closed when server is asked to stop
	stop chan struct{}
	// this wg used to determine when all processing is finished
	wg *sync.WaitGroup
}

func (s *server) GracefulStop() {
	close(s.stop)
	s.wg.Wait()
}

func (s *server) Stop() {
	close(s.stop)
	s.wg.Wait()
}

func (s *server) ListenAndServe(addr string, srcFn chanserv.SourceFunc) chan error {
	errs := make(chan error, 1)
	go s.listenAndServe(addr, srcFn, errs)
	return errs
}

func (s *server) listenAndServe(addr string, srcFn chanserv.SourceFunc, errs chan error) {
	defer close(errs)

	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		errs <- fmt.Errorf("recolve tcp error: %v", err)
		return
	}
	listener, err := net.ListenTCP(tcpAddr.Network(), tcpAddr)
	if err != nil {
		errs <- fmt.Errorf("net.Listen error: %v", err)
		return
	}

ACCEPT_TCP_LOOP:
	for {
		select {
		case <-s.stop:
			break ACCEPT_TCP_LOOP
		default:
		}

		listener.SetDeadline(time.Now().Add(1e9))
		conn, err := listener.AcceptTCP()
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
				continue
			}
			errs <- fmt.Errorf("listener.Accept error: %v", err)
			return
		}
		s.wg.Add(1)
		go s.serve(conn, srcFn, errs)
	}
}

func (s *server) serve(conn net.Conn, srcFn chanserv.SourceFunc, errs chan error) {
	defer s.wg.Done()
	defer conn.Close()

	// Setup server side of smux
	session, err := smux.Server(conn, nil)
	if err != nil {
		errs <- fmt.Errorf("error creating session: %v", err)
		return
	}
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
			errs <- fmt.Errorf("error accepting stream: %v", err)
			break ACCEPT_LOOP
		}
		s.wg.Add(1)
		go s.processStream(stream, srcFn, errs)
	}
}

func (s *server) processStream(stream *smux.Stream, srcFn chanserv.SourceFunc, errs chan error) {
	defer s.wg.Done()
	defer stream.Close()
	defer fmt.Println("closing stream")

	if s.writeTimeout > 0 {
		stream.SetWriteDeadline(time.Now().Add(s.writeTimeout))
	}
	if s.readTimeout > 0 {
		stream.SetReadDeadline(time.Now().Add(s.readTimeout))
	}

	buf, err := readFrame(stream)
	if err != nil {
		errs <- fmt.Errorf("error reading request frame: %v", err)
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

				err = writeFrame(stream, frame.Bytes(), s.minCompressLen)
				if err != nil {
					errs <- fmt.Errorf("error writing to stream: %v", err)
				}
			}
		}(src)
	}
	wg.Wait()
}
