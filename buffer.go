package main

import (
	"bytes"
	"sync"
)

type audioBufferWrapper struct {
	b bytes.Buffer
	m sync.Mutex
}

func (b *audioBufferWrapper) Read(p []byte) (n int, err error) {
	b.m.Lock()
	defer b.m.Unlock()

	// Disregard EOF to prevent playback from stopping
	n, _ = b.b.Read(p)
	return n, nil
}

func (b *audioBufferWrapper) Write(p []byte) (n int, err error) {
	b.m.Lock()
	defer b.m.Unlock()
	return b.b.Write(p)
}

func (b *audioBufferWrapper) Len() int {
	b.m.Lock()
	defer b.m.Unlock()
	return b.b.Len()
}

func (b *audioBufferWrapper) Reset() {
	b.m.Lock()
	defer b.m.Unlock()
	b.b.Reset()
}
