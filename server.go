package smuxnet

import (
	"fmt"
	"net"
	"sync"
	"time"

	"encoding/json"

	"github.com/xtaci/smux"
	"github.com/zenhotels/chanserv"
)

func NewServer(
	readTimeout, writeTimeout time.Duration,
	minCompressLen int,
) (*server, error) {
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

func (s *server) ListenAndServe(addr string, srcFn chanserv.SourceFunc) chan error {
	s.wg.Add(1)
	defer s.wg.Done()

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
	defer listener.Close()

ACCEPT_LOOP:
	for {
		select {
		case <-s.stop:
			break ACCEPT_LOOP
		default:
		}

		listener.SetDeadline(time.Now().Add(1e9))
		conn, err := listener.AcceptTCP()
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
				continue ACCEPT_LOOP
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
		go s.processStream(stream, srcFn, sessionWg, errs)
	}
	sessionWg.Wait()
}

func (s *server) processStream(
	stream *smux.Stream, srcFn chanserv.SourceFunc,
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

	writeLock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	for src := range srcFn(buf) {
		wg.Add(1)
		go func(cs chanserv.Source) {
			defer wg.Done()

		FRAME_LOOP:
			for frame := range cs.Out() {
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
				writeLock.Lock()
				err = writeFrame(stream, frame.Bytes(), s.minCompressLen)
				writeLock.Unlock()
				if err != nil {
					errs <- fmt.Errorf("error writing to stream: %v", err)
				}
			}
		}(src)
	}
	wg.Wait()
}

type RequestMeta struct {
	Timeout uint16
}
