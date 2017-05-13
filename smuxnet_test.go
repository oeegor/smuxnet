package smuxnet

import (
	"testing"

	"encoding/json"

	"time"

	"sync"

	"fmt"

	"github.com/stretchr/testify/require"
	"github.com/zenhotels/chanserv"
)

type req struct {
	Sources int
	Frames  int
	Frame   string
}

func TestOk(t *testing.T) {

	r := req{
		Sources: 1,
		Frames:  1,
		Frame:   "foo",
	}
	cli := setupServerAndCli(t)
	wg := new(sync.WaitGroup)
	for i := 0; i < 1; i++ {
		wg.Add(1)
		go sendReq(t, r, cli, wg)
	}
	fmt.Println("wait")
	wg.Wait()
	fmt.Println("wait doe")
}

func sendReq(t *testing.T, r req, cli Client, wg *sync.WaitGroup) {
	defer wg.Done()
	body, err := json.Marshal(&r)
	require.NoError(t, err)

	out, errs2 := cli.Request(body, time.Now().Add(time.Hour))
	go func() {
		for err := range errs2 {
			require.NoError(t, err)
		}
	}()

	counter := 0
	fmt.Println("read out")
	for frame := range out {
		fmt.Println("read out 1")
		counter++
		require.Equal(t, len(frame.Bytes()), len(r.Frame))
	}
	fmt.Println("read out doe")
	require.Equal(t, r.Frames*r.Sources, counter)
}

func setupServerAndCli(t *testing.T) Client {
	srcFunc := func(body []byte) <-chan chanserv.Source {
		fmt.Println("body", string(body))
		out := make(chan chanserv.Source)

		go func() {
			var r req
			require.NoError(t, json.Unmarshal(body, &r))
			fmt.Println("ddd")
			for i := 0; i < r.Sources; i++ {
				frames := make([]frame, r.Frames)
				for j := 0; j < r.Frames; j++ {
					frames[j] = frame(r.Frame)
				}
				src := &source{frames: frames, out: make(chan chanserv.Frame)}
				go src.writeFrames(t)
				out <- src
			}
			close(out)
		}()
		return out
	}

	srv, _ := NewServer(0, 0, 100)
	errs := srv.ListenAndServe(":9001", srcFunc)
	go func() {
		for err := range errs {
			require.NoError(t, err)
		}
	}()
	cli, err := NewClient("test", "tcp4", ":9001", 0, 0, 100)
	require.NoError(t, err)
	return cli
}

type source struct {
	frames []frame
	header []byte
	out    chan chanserv.Frame
}

func (s *source) Header() []byte {
	return s.header
}

func (s *source) Meta() chanserv.MetaData {
	return nil
}

func (s *source) Out() <-chan chanserv.Frame {
	return s.out
}

func (s *source) writeFrames(t *testing.T) {
	fmt.Println("wr1")
	for _, frame := range s.frames {
		s.out <- &frame
	}
	fmt.Println("wr2")
	close(s.out)
}
