package smuxnet

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"strings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const maxFrameSize = 1024 * 1024 * 10

type req struct {
	RequestMeta
	Frames int
	Frame  string
}

func sendReq(t *testing.T, r req, cli *Client, wg *sync.WaitGroup) {
	defer wg.Done()
	body, err := json.Marshal(&r)
	require.NoError(t, err)

	out, errs2 := cli.Request(body, time.Now().Add(time.Hour))
	go func() {
		fail := false
		for err := range errs2 {
			fail = true
			t.Error("Client error", err)
		}
		if fail {
			t.Fail()
		}
	}()

	counter := 0
	for frame := range out {
		counter++
		assert.Equal(t, len(frame), len(r.Frame), "frame len is incorrect")
	}
	assert.Equal(t, r.Frames, counter, "number of frames is incorrect")
}

func TestOk(t *testing.T) {
	r := req{
		Frames: 1,
		Frame:  strings.Repeat("1", 1024*1024*11),
	}
	r.RequestMeta.Timeout = 1
	cli := setupServerAndCli(t)
	wg := new(sync.WaitGroup)
	for i := 0; i < 1; i++ {
		wg.Add(1)
		go sendReq(t, r, cli, wg)
	}
	wg.Wait()
}

func setupServerAndCli(t *testing.T) *Client {
	handler := func(body []byte) <-chan []byte {
		out := make(chan []byte)
		go func() {
			var r req
			assert.NoError(t, json.Unmarshal(body, &r))
			for j := 0; j < r.Frames; j++ {
				out <- []byte(r.Frame)
			}
			close(out)
		}()
		return out
	}
	srv, err := NewServer(0, 0, maxFrameSize*2, maxFrameSize, ":9002")
	require.NoError(t, err)
	errs := srv.Serve(handler)
	go func() {
		fail := false
		for err := range errs {
			fail = true
			t.Error("Server error", err)
		}
		if fail {
			t.Fail()
		}
	}()
	time.Sleep(time.Millisecond)
	cli, err := NewClient("test", "tcp4", ":9002", 0, 0, maxFrameSize*2, maxFrameSize)
	require.NoError(t, err)
	return cli
}
