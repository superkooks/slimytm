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
var lastStat time.Time
var elapsedSeconds int
var queueLock = new(sync.Mutex)

var bytesReceived int
var fullness int

func queueWatcher() {
	for {
		if playing && len(queue) > 0 {
			songTimes := strings.Split(queue[queueIndex].Path("duration").Data().(string), ":")
			mins, _ := strconv.Atoi(songTimes[0])
			secs, _ := strconv.Atoi(songTimes[1])

			var songTime int
			if elapsedSeconds != 0 {
				songTime = elapsedSeconds
			} else {
				// Byte rate should be constant for all songs (I hope)
				byteRate := 48000 * 2 * 2 // per second
				bytesPlayed := bytesReceived - fullness
				songTime = bytesPlayed / byteRate
			}

			fmt.Println("song time:", songTime, "s")
			fmt.Println("duration:", mins*60+secs)

			if songTime >= mins*60+secs-3 {
				fmt.Println("reached end of song")
				if queueIndex+1 < len(queue) {
					fmt.Println("playing next song")
					nextSong()
				} else {
					playing = false
				}
			} else if time.Since(lastStat).Milliseconds() > 1100 {
				fmt.Println("presumed buffer underrun")
				fmt.Println("buffer len:", audioBuffer.Len())
				fmt.Println("    =", audioBuffer.Len()/48000/2/2, "s")
			}
		}

		time.Sleep(time.Millisecond * 100)
	}
}

func nextSong() {
	queueLock.Lock()
	queueIndex++
	playing = false

	clients[0].playSong(queue[queueIndex].Path("videoId").Data().(string))
	queueLock.Unlock()
}
