package smuxnet

import (
	"testing"

	"fmt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zenhotels/chanserv"
)

type sourceT struct {
	frames []Frame
	out    chan chanserv.Frame
}

func (s *sourceT) Header() []byte {
	return nil
}

func (s *sourceT) Meta() chanserv.MetaData {
	return nil
}

func (s *sourceT) Out() <-chan chanserv.Frame {
	return s.out
}

func (s *sourceT) writeFrames(t *testing.T) {
	t.Log("write frames start")
	for _, frame := range s.frames {
		s.out <- frame
	}
	close(s.out)

	t.Log("closed sourceT out")
}

func createSource(t *testing.T, frames []Frame) chanserv.Source {

	src := &sourceT{
		frames: frames,
		out:    make(chan chanserv.Frame),
	}
	go src.writeFrames(t)
	return src
}

func TestSmuxnet(t *testing.T) {

	srcFn := func(reqBody []byte) <-chan chanserv.Source {
		t.Log("reqbody", string(reqBody))
		src := make(chan chanserv.Source)
		go func() {
			src <- createSource(t, nil)
			close(src)
		}()
		return src
	}

	fmt.Println("create srv")
	srv, _ := NewServer(0, 0, 0)
	errs := srv.ListenAndServe(":20000", srcFn)
	go func() {
		cli, err := NewClient("tcp4", ":20000", 0, 0, 0)
		require.NoError(t, err)
		fmt.Println("send req")
		out, cerrs := cli.Request(nil, nil)
		go func() {
			fmt.Println("cli errors wait")
			for cerr := range cerrs {
				assert.NoError(t, cerr)
			}
		}()

		fmt.Println("cli frames wait")
		for frame := range out {
			fmt.Println("got frame", string(frame.Bytes()))
		}
		fmt.Println("Stop server")
		srv.Stop()
	}()
	fmt.Println("waiting for errors")
	for err := range errs {
		assert.NoError(t, err)
	}
}
