package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/Jeffail/gabs/v2"
	"github.com/gorilla/mux"
)

type audioBufferWrapper struct {
	b bytes.Buffer
	m sync.Mutex
}

func (b *audioBufferWrapper) Read(p []byte) (n int, err error) {
	b.m.Lock()
	defer b.m.Unlock()
	return b.b.Read(p)
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

var audioBuffer = new(audioBufferWrapper)

func playSongs(w http.ResponseWriter, r *http.Request) {
	body, err := gabs.ParseJSONBuffer(r.Body)
	if err != nil {
		panic(err)
	}
	r.Body.Close()

	// Add the start song to the queue
	queueLock.Lock()
	startSong := body.Path("startSong")
	queue = []*gabs.Container{startSong}
	queueIndex = -1
	queueLock.Unlock()
	nextSong()

	// Retrieve the rest of the songs and enqueue them
	queueType := body.Path("queueType").Data().(string)
	queueID := body.Path("queueId").Data().(string)

	switch queueType {
	case "playlist":
		resp, err := http.Get("http://localhost:9000/api/playlist/" + queueID)
		if err != nil {
			panic(err)
		}

		playlist, err := gabs.ParseJSONBuffer(resp.Body)
		if err != nil {
			panic(err)
		}

		queueLock.Lock()
		queue = playlist.Path("tracks").Children()
		for k, v := range queue {
			// Set the queue index to the start song
			if body.Path("startSong.videoId").Data().(string) == v.Path("videoId").Data().(string) {
				queueIndex = k
			}
		}

		fmt.Println("index @", queueIndex)
		fmt.Println("length:", len(queue))
		// fmt.Println("next song:", queue[queueIndex+1].Path("title").String())
		queueLock.Unlock()
	default:
		panic("unknown queue type")
	}
}

func currentSong(w http.ResponseWriter, r *http.Request) {
	queueLock.Lock()
	if queueIndex < len(queue) && len(queue) > 0 {
		w.Write([]byte(queue[queueIndex].String()))
	} else {
		w.Write([]byte("{}"))
	}
	queueLock.Unlock()
}

func audio(w http.ResponseWriter, r *http.Request) {
	fmt.Println("****** New audio request")
	fmt.Println("Buffer is currently", audioBuffer.Len(), "bytes long")
	fmt.Println("     =", audioBuffer.Len()/48000/2/2, "s")
	io.Copy(w, audioBuffer)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" {
			w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	go startSqueezebox()
	go queueWatcher()

	xplInit()
	go xplListener()

	fmt.Println("Serving api on :9001")
	r := mux.NewRouter()
	r.Use(corsMiddleware)
	r.Path("/play").HandlerFunc(playSongs)
	r.Path("/currentsong").HandlerFunc(currentSong)
	r.Path("/assets/audio.wav").HandlerFunc(audio)

	// f, _ := os.Open("wa.wav")
	// n, err := io.Copy(audioAssetsBytes, f)
	// fmt.Println("copied", n, "bytes")
	// if err != nil {
	// 	panic(err)
	// }
	// f.Close()

	panic(http.ListenAndServe(":9001", r))
}
