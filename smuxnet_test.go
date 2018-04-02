package smuxnet

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"strings"

	"github.com/stretchr/testify/require"
	"bytes"
	"fmt"
)

const maxFrameSize = 1024 * 1024 * 10

type req struct {
	RequestMeta
	Frames int
	Frame  string
}

func handler(t require.TestingT) Handler {
	return 	func(body []byte) <-chan []byte {
		out := make(chan []byte)
		go func() {
			var r req
			require.NoError(t, json.Unmarshal(body, &r))
			for j := 0; j < r.Frames; j++ {
				out <- []byte(r.Frame)
			}
			close(out)
		}()
		return out
	}
}

func sendReq(t require.TestingT, r req, cli *Client) {
	body, err := json.Marshal(&r)
	require.NoError(t, err)

	out, errs2 := cli.Request(body, time.Now().Add(time.Hour))
	go func() {
		for err := range errs2 {
			require.NoError(t, err,"client error")
		}
	}()

	var buf bytes.Buffer
	for frame := range out {
		buf.Write(frame)
	}
	require.Equal(t, r.Frame, buf.String())
}

func TestOk(t *testing.T) {
	started := time.Now()
	r := req{
		Frames: 1,
		Frame:  strings.Repeat("1", 1<<6),
	}
	r.RequestMeta.Timeout = 1
	srv := setupServer(t)
	errs := srv.Serve(handler(t))
	go func() {
		for err := range errs {
			continue
			require.NoError(t, err, "Server error")
		}
	}()

	fmt.Println("[test] server setup done", time.Since(started))
	defer func(){fmt.Println("test done", time.Since(started))}()
	defer srv.Stop()
	// time.Sleep(time.Millisecond)
	cli := setupCli(t)
	fmt.Println("[test] client setup done", time.Since(started))
	wg := new(sync.WaitGroup)
	for i := 0; i < 1; i++ {
		wg.Add(1)
		go func(){
			fmt.Println("[test] send req", i, time.Since(started))
			sendReq(t, r, cli)
			fmt.Println("[test] send req done", i, time.Since(started))
			wg.Done()
		}()
	}
	fmt.Println("[test] wait wg", time.Since(started))
	wg.Wait()
	fmt.Println("[test] wait wg done", time.Since(started))
	require.False(t, cli.IsClosed())
}

func BenchmarkSMUX(b *testing.B) {
	const concurrent = 5
	r := req{
		Frames: 1,
		Frame:  strings.Repeat("1", 1<<5),
	}
	r.RequestMeta.Timeout = 10
	srv := setupServer(b)
	srv.Serve(handler(b))
	defer srv.Stop()
	cli := setupCli(b)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		wg := new(sync.WaitGroup)
		wg.Add(concurrent)
		for j := 0; j < concurrent; j++ {
			go func(){
				sendReq(b, r, cli)
				wg.Done()
			}()
		}
		wg.Wait()
	}
}

func setupCli(t require.TestingT) *Client {
	cli, err := NewClient("test", "tcp4", ":9002", 0, 0, maxFrameSize*2, maxFrameSize)
	require.NoError(t, err)
	return cli
}

func setupServer(t require.TestingT) *Server {
	srv, err := NewServer(0, 0, maxFrameSize*2, maxFrameSize, ":9002")
	require.NoError(t, err)
	return srv
}
