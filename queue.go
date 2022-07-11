package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type Song struct {
	ID         string      `json:"videoId"`
	Title      string      `json:"title"`
	Artists    []Artist    `json:"artists"`
	Album      Album       `json:"album"`
	Duration   string      `json:"duration"`
	Thumbnails []Thumbnail `json:"thumbnails"`
}

type Artist struct {
	Name string `json:"name"`
}

type Album struct {
	Name string `json:"name"`
}

type Thumbnail struct {
	URL string `json:"url"`
}

type text struct {
	bufs     chan []byte
	ctx      context.Context
	disabled func() bool
}

type Queue struct {
	Player player
	Buffer *audioBufferWrapper
	Texts  []text

	Songs []Song
	Index int

	CancelPlaying func()
	Playing       bool
	Loading       bool
	Paused        bool
	ElapsedSecs   int
}

var queues []*Queue

func (q *Queue) Watch() {
	for {
		if q.Playing && len(q.Songs) > 0 && q.Index >= 0 && q.Index < len(q.Songs) {
			songTimes := strings.Split(q.Songs[q.Index].Duration, ":")
			mins, _ := strconv.Atoi(songTimes[0])
			secs, _ := strconv.Atoi(songTimes[1])

			if q.ElapsedSecs >= mins*60+secs-1 {
				logger.Debug("reached end of song")
				if q.Index+1 < len(q.Songs) {
					logger.Debug("playing next song")
					q.Next()
				} else {
					logger.Debug("reached end of queue")
					q.Playing = false
				}
			}
		}

		time.Sleep(time.Millisecond * 100)
	}
}

func (q *Queue) Next() {
	q.Player.Stop()
	if q.CancelPlaying != nil {
		q.CancelPlaying()
	}

	q.Buffer.Reset()
	q.Playing = false
	q.Paused = false
	q.Index++

	// Don't run over the end of the queue
	if q.Index < len(q.Songs) {
		q.Loading = true
		q.UpdateClients()
		q.CancelPlaying = q.Player.Play(q.Songs[q.Index].ID)
	} else {
		q.Reset()
	}
}

func (q *Queue) Previous() {
	q.Player.Stop()
	if q.CancelPlaying != nil {
		q.CancelPlaying()
	}

	q.Buffer.Reset()
	q.Playing = false
	q.Paused = false

	// Don't run off the end of the queue
	if q.ElapsedSecs < 5 && q.Index > 0 {
		q.Index--
	}

	q.Loading = true
	q.UpdateClients()
	q.CancelPlaying = q.Player.Play(q.Songs[q.Index].ID)
}

func (q *Queue) Pause() {
	if q.Paused {
		q.Player.Unpause()
	} else {
		q.Player.Pause()
	}

	q.Paused = !q.Paused
	q.Playing = !q.Playing
	q.UpdateClients()
}

func (q *Queue) Reset() {
	q.Player.Stop()
	if q.CancelPlaying != nil {
		q.CancelPlaying()
	}
	q.Buffer.Reset()

	q.Songs = []Song{}
	q.Index = 0
	q.Paused = false
	q.Playing = false
	q.Loading = false
	q.ElapsedSecs = 0
	q.UpdateClients()
}

// Returns the JSON representation of the current song
func (q *Queue) CurrentSongJSON() []byte {
	var song string
	if q.Index < len(q.Songs) && len(q.Songs) > 0 && (q.Playing || q.Paused) {
		b, _ := json.Marshal(q.Songs[q.Index])
		song = string(b)
	} else {
		song = "{}"
	}

	return []byte(fmt.Sprintf(`{"id": %v, "name": "%v", "type": "%v", "song": %v, "paused": %v, "loading": %v, "volume": %v}`,
		q.Player.GetID(), q.Player.GetName(), q.Player.GetModel(), song, q.Paused, q.Loading, q.Player.GetVolume(),
	))
}

// Returns buffers with the current song name
func (q *Queue) CurrentSongBuf() chan []byte {
	var curText string
	var curBuf chan []byte
	out := make(chan []byte)

	go func() {
		for {
			if len(q.Songs) == 0 || q.Index < 0 || q.Index >= len(q.Songs) {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			songsStr := fmt.Sprintf("%v from %v by %v",
				q.Songs[q.Index].Title,
				q.Songs[q.Index].Album.Name,
				q.Songs[q.Index].Artists[0].Name,
			)

			if curText != songsStr {
				curBuf = q.Player.DisplayText(songsStr, context.Background())
				curText = songsStr
			}

			out <- <-curBuf
		}
	}()

	return out
}

// Update all clients
func (q *Queue) UpdateClients() {
	s := q.CurrentSongJSON()
	for _, v := range clients {
		v.Conn.WriteMessage(websocket.TextMessage, s)
	}
}

func (q *Queue) Composite() {
	// Clock will always be displayed with the lowest priority, queue starts disabled
	q.Texts = []text{
		{
			bufs: q.Player.DisplayClock(),
			ctx:  context.Background(),
		},
		{
			bufs:     q.Player.DisplayText("Loading...", context.Background()),
			ctx:      context.Background(),
			disabled: func() bool { return !q.Loading },
		},
		{
			bufs:     q.CurrentSongBuf(),
			ctx:      context.Background(),
			disabled: func() bool { return !q.Playing },
		},
	}

	for {
		top := q.Texts[len(q.Texts)-1]

		// Find the top enabled element
		i := 1
		for top.disabled != nil && top.disabled() {
			i++
			top = q.Texts[len(q.Texts)-i]
		}

		if top.ctx.Err() != nil {
			// The context has been cancelled/timed out, remove it and try again
			q.Texts = append(q.Texts[:len(q.Texts)-i], q.Texts[len(q.Texts)-i+1:]...)
			continue
		}

		// Render the top buffer
		q.Player.Render(<-top.bufs)

		// Animate the screen at 30 fps
		time.Sleep(time.Millisecond * 33)
	}
}
