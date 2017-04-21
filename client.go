package smuxnet

import (
	"net"
	"time"

	"fmt"

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
	}, nil
}

type client struct {
	session *smux.Session
}

func (c *client) Request(body []byte, timeout <-chan struct{}) (<-chan Frame, error) {
	var stream *smux.Stream
	done := make(chan struct{})
	go func() {
		select {
		case <-timeout:
			fmt.Println("smux request timeouted")
			if !stream.IsClosed() {
				stream.Close()
			}
		case <-done:
		}
	}()

	stream, err := c.session.OpenStream()
	if err != nil {
		return nil, err
	}
	if _, err := stream.Write(body); err != nil {
		close(done)
		return nil, err
	}
	out := make(chan Frame, 1024)
	go c.readStream(stream, out, done)
	return out, nil
}

func (c *client) readStream(stream *smux.Stream, out chan<- Frame, done chan struct{}) {
	for {
		buf, err := readFrame(stream)
		if err != nil {
			break
		}
		out <- frame(buf)
	}
	close(done)
}

type frame []byte

func (f frame) Bytes() []byte {
	return []byte(f)
}
