package smuxnet

import (
	"fmt"
	"net"
	"sync"
	"time"
	"encoding/json"

	"github.com/ofw/smux"
)

type Handler func([]byte) <-chan []byte

func NewServer(
	readTimeout, writeTimeout time.Duration,
	minCompressLen int, addr string,
) (*Server, error) {

	listener, err := createListener(addr)
	if err != nil {
		return nil, err
	}

	server := &Server{
		listener:       listener,
		minCompressLen: minCompressLen,
		readTimeout:    readTimeout,
		writeTimeout:   writeTimeout,
		wg:             new(sync.WaitGroup),
		stop:           make(chan struct{}),
	}
	return server, nil
}

type Server struct {
	listener       *net.TCPListener
	minCompressLen int
	readTimeout    time.Duration
	writeTimeout   time.Duration
	// this channel is closed when Server is asked to stop
	stop chan struct{}
	// this wg used to determine when all processing is finished
	wg *sync.WaitGroup
}

func createListener(addr string) (*net.TCPListener, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("recolve tcp error: %v", err)

	}
	listener, err := net.ListenTCP(tcpAddr.Network(), tcpAddr)
	if err != nil {
		return nil, fmt.Errorf("net.Listen error: %v", err)
	}
	return listener, nil
}

func (s *Server) GracefulStop() {
	close(s.stop)
	s.wg.Wait()
}

func (s *Server) Serve(handler Handler) chan error {
	s.wg.Add(1)

	errs := make(chan error, 1)
	go s.serve(handler, errs)
	return errs
}

func (s *Server) serve(handler Handler, errs chan error) {
	defer s.wg.Done()
	defer close(errs)
	defer s.listener.Close()

ACCEPT_LOOP:
	for {
		select {
		case <-s.stop:
			break ACCEPT_LOOP
		default:
		}

		s.listener.SetDeadline(time.Now().Add(1e9))
		conn, err := s.listener.AcceptTCP()
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
				continue ACCEPT_LOOP
			}
			errs <- fmt.Errorf("listener.Accept error: %v", err)
			return
		}
		s.wg.Add(1)
		go s.serveSession(conn, handler, errs)
	}
}

func (s *Server) serveSession(conn net.Conn, handler Handler, errs chan error) {
	defer s.wg.Done()
	defer conn.Close()

	// Setup Server side of smux
	conf := smux.DefaultConfig()
	conf.MaxFrameSize = 65000
	session, err := smux.Server(conn, conf)
	if err != nil {
		errs <- fmt.Errorf("error creating session: %v", err)
		return
	}
	defer session.Close()

	sessionWg := new(sync.WaitGroup)
SESSION_LOOP:
	for {
		select {
		case <-s.stop:
			break SESSION_LOOP
		default:
		}

		session.SetDeadline(time.Now().Add(1e9))
		stream, err := session.AcceptStream()
		if err != nil {
			if err.Error() == "i/o timeout" {
				continue SESSION_LOOP
			}

			errs <- fmt.Errorf("error accepting stream: %v", err)
			break SESSION_LOOP
		}
		sessionWg.Add(1)
		go s.processStream(stream, handler, sessionWg, errs)
	}
	sessionWg.Wait()
}

func (s *Server) processStream(
	stream *smux.Stream, handler Handler,
	sessionWg *sync.WaitGroup, errs chan error,
) {
	defer sessionWg.Done()
	defer stream.Close()

	started := time.Now()
	if s.readTimeout > 0 {
		stream.SetReadDeadline(started.Add(s.readTimeout))
	}

	buf, err := readFrame(stream)
	if err != nil {
		errs <- fmt.Errorf("error reading request frame: %v", err)
		return
	}

	var req RequestMeta
	json.Unmarshal(buf, &req)

	if req.Timeout > 0 {
		stream.SetWriteDeadline(started.Add(time.Duration(req.Timeout) * time.Second))
	} else if s.writeTimeout > 0 {
		stream.SetWriteDeadline(time.Now().Add(s.writeTimeout))
	}

FRAME_LOOP:
	for frame := range handler(buf) {
		if err != nil {
			// if error is not empty shouldn't write anymore frames
			// they may be written despite deadline has passed
			// because of channels in stream.go
			continue
		}
		select {
		case <-s.stop:
			continue FRAME_LOOP
		default:
		}
		err = writeFrame(stream, frame, s.minCompressLen, nil)
		if err != nil {
			errs <- fmt.Errorf("error writing to stream: %v", err)
		}
	}
}

type RequestMeta struct {
	Timeout uint16
}
