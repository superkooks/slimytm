package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/Jeffail/gabs/v2"
)

var queue []*gabs.Container
var queueIndex int
var playing bool
var lastStat time.Time
var queueLock = new(sync.Mutex)

func queueWatcher() {
	for {
		if playing && len(queue) > 0 {
			// songTimes := strings.Split(queue[queueIndex].Path("duration").Data().(string), ":")
			// mins, _ := strconv.Atoi(songTimes[0])
			// secs, _ := strconv.Atoi(songTimes[1])
			// songDuration := time.Duration(mins*int(time.Minute) + secs*int(time.Second))

			if time.Since(lastStat) > 1100*time.Millisecond {
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

	clients[0].playSong(queue[queueIndex].Path("videoId").Data().(string))
	queueLock.Unlock()
}
