package smuxnet

import (
	"testing"

	"encoding/json"

	"time"

	"sync"

	"strings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOkV2(t *testing.T) {
	r := req{
		Sources: 1,
		Frames:  4,
		Frame:   strings.Repeat("a", 1000*1000),
	}
	r.RequestMeta.Timeout = 1
	cli := setupServerV2AndCli(t)
	wg := new(sync.WaitGroup)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go sendReq(t, r, cli, wg)
	}
	wg.Wait()
}

func setupServerV2AndCli(t *testing.T) *Client {
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
	srv, err := NewServerV2(0, 0, 100, ":9002")
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
	cli, err := NewClient("test", "tcp4", ":9002", 0, 0, 100)
	require.NoError(t, err)
	return cli
}
