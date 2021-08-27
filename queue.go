package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Jeffail/gabs/v2"
	"github.com/gorilla/websocket"
)

var queue []*gabs.Container
var queueIndex int
var playing bool
var paused bool
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
					resetPlayingText(false)
					playing = false
				}
			}
		}

		time.Sleep(time.Millisecond * 100)
	}
}

// Returns the JSON representation of the current song
func getCurrentSong() []byte {
	queueLock.Lock()
	defer queueLock.Unlock()

	var song string
	if queueIndex < len(queue) && len(queue) > 0 && playing {
		song = queue[queueIndex].String()
	} else {
		song = "{}"
	}

	return []byte(fmt.Sprintf(`{"song": %v, "paused": %v, "volume": %v}`,
		song, paused, players[playingClient].GetVolume(),
	))
}

func nextSong() {
	defer updateWebClients()
	players[playingClient].Stop()
	if queueIndex >= len(queue)-1 {
		// Don't run off the end of the queue
		resetPlayingText(false)
		playing = false
		return
	}

	queueLock.Lock()
	queueIndex++
	playing = false
	queueLock.Unlock()

	players[playingClient].Play(queue[queueIndex].Path("videoId").Data().(string))
	resetPlayingText(true)
}

func previousSong() {
	defer updateWebClients()
	if queueIndex == 0 {
		// Don't run off the end of the queue
		return
	}

	players[playingClient].Stop()

	queueLock.Lock()
	if elapsedSeconds < 5 && queueIndex != 0 {
		queueIndex--
	}
	queueLock.Unlock()

	players[playingClient].Play(queue[queueIndex].Path("videoId").Data().(string))
	resetPlayingText(true)
}

func togglePause() {
	defer updateWebClients()
	if paused {
		players[playingClient].Unpause()
	} else {
		players[playingClient].Pause()
	}
	paused = !paused
	playing = !playing
}

func resetQueue() {
	defer updateWebClients()
	players[playingClient].Stop()
	queueLock.Lock()
	queue = []*gabs.Container{}
	queueIndex = 0
	playing = false
	paused = false
	elapsedSeconds = 0
	queueLock.Unlock()
	resetPlayingText(false)
}

func resetPlayingText(set bool) {
	// Clear text stack of currently playing text
	for k, v := range textStack {
		if v.note == "playing" {
			textStack = append(textStack[:k], textStack[k+1:]...)
			break
		}
	}
	checkStack = true

	if set {
		// Set currently playing text
		song := queue[queueIndex].Path("title").String()
		artist := queue[queueIndex].Path("artists.0.name").String()
		album := queue[queueIndex].Path("album.name").String()
		textStack = append(textStack, text{
			text:   fmt.Sprintf("%v from %v by %v", song, album, artist),
			note:   "playing",
			expiry: time.Now().Add(time.Hour), // Effectively never expire, we will clear ourselves
		})
	}
}

func updateWebClients() {
	s := getCurrentSong()
	for _, v := range webClients {
		v.WriteMessage(websocket.TextMessage, s)
	}
}
