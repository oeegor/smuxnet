package smuxnet

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/xtaci/smux"
)

type Frame interface {
	Bytes() []byte
}

type Client interface {
	IsClosed() bool
	Request(body []byte, timeout <-chan struct{}) (<-chan Frame, chan error)
}

func NewClient(
	network, addr string,
	keepAliveInterval, keepAliveTimeout time.Duration,
	minCompressLen int,
) (Client, error) {
	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}

	conf := smux.DefaultConfig()
	if keepAliveTimeout > 0 {
		conf.KeepAliveTimeout = keepAliveTimeout
	}
	if keepAliveInterval > 0 {
		conf.KeepAliveInterval = keepAliveInterval
	}
	session, err := smux.Client(conn, conf)
	if err != nil {
		return nil, err
	}
	return &client{
		minCompressLen: minCompressLen,
		session:        session,
		wg:             new(sync.WaitGroup),
	}, nil
}

type client struct {
	minCompressLen int
	session        *smux.Session
	wg             *sync.WaitGroup
}

func (c *client) IsClosed() bool {
	return c.session.IsClosed()
}

func (c *client) GracefulClose() {
	c.wg.Wait()
	if !c.session.IsClosed() {
		c.session.Close()
	}
}

func (c *client) Request(body []byte, timeout <-chan struct{}) (<-chan Frame, chan error) {
	c.wg.Add(1)

	errs := make(chan error, 2)
	out := make(chan Frame, 1024)
	c.wg.Add(1)
	go c.request(body, timeout, out, errs)
	return out, errs

}

func (c *client) request(body []byte, timeout <-chan struct{}, out chan<- Frame, errs chan error) {
	done := make(chan struct{})
	defer close(done)

	var stream *smux.Stream
	go func() {
		defer c.wg.Done()
		defer close(errs)
		defer close(out)

		select {
		case <-timeout:
			errs <- errors.New("request timeouted")
		case <-done:
		}
	}()

	stream, err := c.session.OpenStream()
	if err != nil {
		errs <- fmt.Errorf("open stream error: %v", err)
		return
	}

	if err := writeFrame(stream, body, c.minCompressLen); err != nil {
		errs <- fmt.Errorf("write frame error: %v", err)
		return
	}

	for {
		if buf, err := readFrame(stream); err == nil {
			out <- frame(buf)
		} else {
			if err == io.EOF {
				break
			}
			errs <- fmt.Errorf("client read frame error: %v", err)
			break
		}
	}
}

type frame []byte

func (f frame) Bytes() []byte {
	return []byte(f)
}
