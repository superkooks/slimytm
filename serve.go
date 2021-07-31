package main

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/Jeffail/gabs/v2"
	"github.com/gorilla/mux"
)

// const YTM_URL = "https://music.youtube.com/watch?v=mTTB90p23qo"

var queue []*gabs.Container
var queueIndex int
var paused bool
var queueLock = new(sync.Mutex)

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
	queueIndex = 0
	queueLock.Unlock()

	go clients[0].playSong(queue[0].Path("videoId").Data().(string))

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

	fmt.Println("Serving api on :9001")
	r := mux.NewRouter()
	r.Use(corsMiddleware)
	r.Path("/play").HandlerFunc(playSongs)
	r.Path("/currentsong").HandlerFunc(currentSong)
	panic(http.ListenAndServe(":9001", r))
}
