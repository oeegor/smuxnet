package smuxnet

import (
	"net"
	"time"

	"github.com/xtaci/smux"
	"github.com/zenhotels/chanserv"
)

type Client interface {
	Request(body []byte) (<-chan chanserv.Frame, error)
}

func NewClient(network, addr string) (*client, error) {
	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}

	session, err := smux.Client(conn, nil)
	if err != nil {
		return nil, err
	}
	return &client{
		session: session,
	}, nil
}

type client struct {
	readTimeout  time.Duration
	timeout      time.Duration
	writeTimeout time.Duration
	session      *smux.Session
}

func (c *client) Request(body []byte) (<-chan chanserv.Frame, error) {
	stream, err := c.session.OpenStream()
	if err != nil {
		return nil, err
	}
	if c.timeout > 0 {
		stream.SetDeadline(time.Now().Add(c.timeout))
	}
	if c.writeTimeout > 0 {
		stream.SetWriteDeadline(time.Now().Add(c.writeTimeout))
	}
	if _, err := stream.Write(body); err != nil {
		return nil, err
	}
	out := make(chan chanserv.Frame, 1024)
	go c.readStream(stream, out)
	return out, nil
}

func (c *client) readStream(stream *smux.Stream, out chan<- chanserv.Frame) {
	for {
		if c.readTimeout > 0 {
			stream.SetReadDeadline(time.Now().Add(c.readTimeout))
		}
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
