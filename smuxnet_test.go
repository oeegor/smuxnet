package smuxnet

import (
	"testing"

	"sync"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zenhotels/chanserv"
)

type sourceT struct {
	frames []chanserv.Frame
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

func createSource(t *testing.T, frames []chanserv.Frame) chanserv.Source {
	src := &sourceT{
		frames: frames,
		out:    make(chan chanserv.Frame),
	}
	go src.writeFrames(t)
	return src
}

func handler(t *testing.T) chanserv.SourceFunc {
	return func(reqBody []byte) <-chan chanserv.Source {
		respBody := append([]byte("resp: "), reqBody...)
		src := make(chan chanserv.Source)
		go func() {
			src <- createSource(t, []chanserv.Frame{frame(respBody)})
			close(src)
			t.Log("closed server chanserv.Source chan")
		}()
		return src
	}
}

func TestSmuxnet(t *testing.T) {
	testSmuxnet(t, []byte("my huge payload"))
}

func TestSmuxnetEmptyBody(t *testing.T) {
	testSmuxnet(t, nil)
}

func testSmuxnet(t *testing.T, reqBody []byte) {

	wg := new(sync.WaitGroup)
	srv, _ := NewServer(0, 0, 0)
	errs := srv.ListenAndServe(":20000", handler(t))

	wg.Add(1)
	go func() {
		defer wg.Done()

		cli, err := NewClient("id", "tcp4", ":20000", 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, "id", cli.ID())
		out, cerrs := cli.Request(reqBody, nil)

		wg.Add(1)
		go func() {
			defer wg.Done()
			for cerr := range cerrs {
				assert.NoError(t, cerr)
			}
			t.Log("client errors loop done")
		}()

		for frame := range out {
			t.Logf("got frame: %s", frame.Bytes())
		}
		t.Log("out channel closed")
		srv.GracefulStop()
		t.Log("server stopped")
	}()
	for err := range errs {
		assert.NoError(t, err)
	}

	t.Log("wg.Wait()")
	wg.Wait()
}
