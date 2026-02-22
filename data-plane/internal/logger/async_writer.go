package logger

import (
	"bufio"
	"io"
	"sync"
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
		return len(p), nil
	}
}

func (aw *AsyncWriter) worker() {
	defer aw.wg.Done()
	bw := bufio.NewWriter(aw.inner)
	for data := range aw.buffer {
		_, _ = bw.Write(data)
		if len(aw.buffer) == 0 {
			_ = bw.Flush()
		}
	}
	_ = bw.Flush()
}

func (aw *AsyncWriter) Close() {
	close(aw.buffer)
	aw.wg.Wait()
}
