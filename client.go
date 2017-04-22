package smuxnet

import (
	"net"
	"time"

	"fmt"

	"sync"

	"github.com/xtaci/smux"
)

type Frame interface {
	Bytes() []byte
}

type Client interface {
	Request(body []byte, timeout <-chan struct{}) (<-chan Frame, error)
}

func NewClient(network, addr string, keepAliveInterval, keepAliveTimeout time.Duration) (*client, error) {
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
		session: session,
		wg:      new(sync.WaitGroup),
	}, nil
}

type client struct {
	session *smux.Session
	wg      *sync.WaitGroup
}

func (c *client) GracefulClose() {
	c.wg.Wait()
	if !c.session.IsClosed() {
		c.session.Close()
	}
}

func (c *client) Request(body []byte, timeout <-chan struct{}) (<-chan Frame, error) {
	c.wg.Add(1)

	var stream *smux.Stream
	done := make(chan struct{})
	go func() {
		defer c.wg.Done()

		select {
		case <-timeout:
			fmt.Println("smux request timeouted")
		case <-done:
		}
		if stream != nil {
			if err := stream.Close(); err != nil {
				fmt.Println("close stream error:", err)
			}
		}
	}()

	stream, err := c.session.OpenStream()
	if err != nil {
		return nil, err
	}
	if err := writeFrame(stream, body); err != nil {
		close(done)
		return nil, err
	}
	out := make(chan Frame, 1024)
	c.wg.Add(1)
	go c.readStream(stream, out, done)
	return out, nil
}

func (c *client) readStream(stream *smux.Stream, out chan<- Frame, done chan struct{}) {
	defer c.wg.Done()
	defer close(done)

	for {
		buf, err := readFrame(stream)
		if err != nil {
			break
		}
		out <- frame(buf)
	}
}

type frame []byte

func (f frame) Bytes() []byte {
	return []byte(f)
}
