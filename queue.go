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

			if elapsedSeconds >= mins*60+secs-3 {
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
	queueLock.Lock()
	queueIndex++
	playing = false

	players[playingClient].Play(queue[queueIndex].Path("videoId").Data().(string))
	queueLock.Unlock()
}
