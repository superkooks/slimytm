package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Jeffail/gabs/v2"
)

var queue []*gabs.Container
var queueIndex int
var playing bool
var elapsedSeconds int
var queueLock = new(sync.Mutex)

var playingClient = 0

func queueWatcher() {
	for {
		if playing && len(queue) > 0 {
			songTimes := strings.Split(queue[queueIndex].Path("duration").Data().(string), ":")
			mins, _ := strconv.Atoi(songTimes[0])
			secs, _ := strconv.Atoi(songTimes[1])

			fmt.Println("song time:", elapsedSeconds, "s")
			fmt.Println("duration:", mins*60+secs)

			if elapsedSeconds >= mins*60+secs-1 {
				fmt.Println("reached end of song")
				if queueIndex+1 < len(queue) {
					fmt.Println("playing next song")
					nextSong()
				} else {
					playing = false
				}
			}
		}

		time.Sleep(time.Millisecond * 100)
	}
}

func nextSong() {
	players[playingClient].Stop()
	if queueIndex >= len(queue)-1 {
		// Don't run off the end of the queue
		return
	}

	queueLock.Lock()
	queueIndex++
	playing = false
	queueLock.Unlock()

	players[playingClient].Play(queue[queueIndex].Path("videoId").Data().(string))
}

func previousSong() {
	players[playingClient].Stop()

	queueLock.Lock()
	if elapsedSeconds < 5 && queueIndex != 0 {
		queueIndex--
	}
	queueLock.Unlock()

	players[playingClient].Play(queue[queueIndex].Path("videoId").Data().(string))
}
