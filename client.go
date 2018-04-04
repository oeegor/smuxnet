package smuxnet

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/ofw/smux"
)

type ClientConfig struct {
	KeepAliveInterval time.Duration
	KeepAliveTimeout time.Duration
	MinCompressLen int
	MaxFrameSize int
}

func NewClient(
	id, network, addr string,
	conf ClientConfig,
) (*Client, error) {
	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}

	smuxConf := smux.DefaultConfig()
	if conf.KeepAliveTimeout > 0 {
		fmt.Println("set timeout",conf.KeepAliveTimeout)
		smuxConf.KeepAliveTimeout = conf.KeepAliveTimeout
	}
	if conf.KeepAliveInterval > 0 {
		smuxConf.KeepAliveInterval = conf.KeepAliveInterval
	}
	if conf.MaxFrameSize > 0 {
		smuxConf.MaxFrameSize = conf.MaxFrameSize
	}
	session, err := smux.Client(conn, smuxConf)
	if err != nil {
		return nil, err
	}
	return &Client{
		id:             id,
		minCompressLen: conf.MinCompressLen,
		session:        session,
		wg:             new(sync.WaitGroup),
	}, nil
}

type Client struct {
	id             string
	maxFrameSize   int
	minCompressLen int
	session        *smux.Session
	wg             *sync.WaitGroup
}

func (c *Client) ID() string {
	return c.id
}

func (c *Client) IsClosed() bool {
	return c.session.IsClosed()
}

func (c *Client) NumStreams() int {
	return c.session.NumStreams()
}

func (c *Client) GracefulClose() {
	c.wg.Wait()
	if !c.session.IsClosed() {
		c.session.Close()
	}
}

func (c *Client) Request(body []byte, deadline time.Time) (<-chan []byte, chan error) {
	c.wg.Add(1)

	errs := make(chan error, 2)
	src := make(chan []byte, 1024)
	go c.request(body, deadline, src, errs)
	return src, errs
}

func (c *Client) request(body []byte, deadline time.Time, out chan<- []byte, errs chan error) {
	defer c.wg.Done()
	defer close(errs)
	defer close(out)

	stream, err := c.session.OpenStream()
	if err != nil {
		errs <- fmt.Errorf("open stream error: %v", err)
		return
	}
	defer stream.Close()

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
			out <- buf
		} else {
			if err == io.EOF {
				break
			}
			errs <- fmt.Errorf("Client read frame error: %v", err)
			break
		}
	}
}
