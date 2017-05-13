package smuxnet

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/xtaci/smux"
	"github.com/zenhotels/chanserv"
)

type Client interface {
	ID() string
	IsClosed() bool
	Request(body []byte, deadline time.Time) (<-chan chanserv.Frame, chan error)
}

func NewClient(
	id, network, addr string,
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
	conf.MaxFrameSize = 65000
	conf.MaxReceiveBuffer = 1024 * 1024 * 1024
	session, err := smux.Client(conn, conf)
	if err != nil {
		return nil, err
	}
	return &client{
		id:             id,
		minCompressLen: minCompressLen,
		session:        session,
		wg:             new(sync.WaitGroup),
	}, nil
}

type client struct {
	id             string
	minCompressLen int
	session        *smux.Session
	wg             *sync.WaitGroup
}

func (c *client) ID() string {
	return c.id
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

func (c *client) Request(body []byte, deadline time.Time) (<-chan chanserv.Frame, chan error) {
	c.wg.Add(1)

	errs := make(chan error, 2)
	out := make(chan chanserv.Frame, 1024)
	go c.request(body, deadline, out, errs)
	return out, errs

}

func (c *client) request(body []byte, deadline time.Time, out chan<- chanserv.Frame, errs chan error) {
	defer c.wg.Done()
	defer close(errs)
	defer close(out)

	stream, err := c.session.OpenStream()
	if err != nil {
		errs <- fmt.Errorf("open stream error: %v", err)
		return
	}

	err = stream.SetDeadline(deadline)
	if err != nil {
		errs <- fmt.Errorf("set deadline error: %v", err)
		return
	}

	if err := writeFrame(stream, body, c.minCompressLen, nil); err != nil {
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
