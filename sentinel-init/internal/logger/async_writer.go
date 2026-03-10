package logger

import (
	"bufio"
	"fmt"
	"io"
	"sync"
	"time"
)

type AsyncWriter struct {
	inner  io.Writer
	buffer chan []byte
	wg     sync.WaitGroup
}

func NewAsyncWriter(inner io.Writer, queueSize int) *AsyncWriter {
	aw := &AsyncWriter{
		inner:  inner,
		buffer: make(chan []byte, queueSize),
	}
	aw.wg.Add(1)
	go aw.worker()
	return aw
}

func (aw *AsyncWriter) Write(p []byte) (n int, err error) {
	data := make([]byte, len(p))
	copy(data, p)
	select {
	case aw.buffer <- data:
		return len(p), nil
	default:
		return 0, fmt.Errorf("async buffer overflow, dropped %d bytes", len(p))
	}
}

func (aw *AsyncWriter) worker() {
	defer aw.wg.Done()
	bw := bufio.NewWriter(aw.inner)
	flushTicker := time.NewTicker(100 * time.Millisecond)
	defer flushTicker.Stop()

	for {
		select {
		case data, ok := <-aw.buffer:
			if !ok {
				_ = bw.Flush()
				return
			}
			_, _ = bw.Write(data)
		case <-flushTicker.C:
			_ = bw.Flush()
		}
	}
}

func (aw *AsyncWriter) Close() {
	close(aw.buffer)
	aw.wg.Wait()
}
