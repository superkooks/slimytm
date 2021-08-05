package main

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/Jeffail/gabs/v2"
)

var queue []*gabs.Container
var queueIndex int
var playing bool
var lastStat time.Time
var nextSongLoaded bool
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

	if queueIndex+1 < len(queue) {
		fmt.Println(queue[queueIndex].Path("title").String())
		fmt.Println(queue[queueIndex+1].Path("title").String())
		preloadSong(queue[queueIndex+1].Path("videoId").Data().(string))
	}
}

func preloadSong(videoID string) {
	fmt.Println("preloading", videoID)

	os.Remove("assets/next.webm")
	os.Remove("assets/next.wav")

	co := exec.Command("youtube-dl", "https://music.youtube.com/watch?v="+videoID, "-f", "bestaudio[ext=webm]",
		"-o", "assets/next.webm", "--external-downloader", "aria2c")
	fmt.Println(co.String())
	b, err := co.CombinedOutput()
	fmt.Println(string(b))
	if err != nil {
		panic(err)
	}

	wkdir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	_, err = exec.Command("ffmpeg", "-y", "-i", wkdir+"/assets/next.webm", "-vn", wkdir+"/assets/next.wav").Output()
	if err != nil {
		panic(err)
	}

	nextSongLoaded = true
}
