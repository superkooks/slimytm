package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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

var metricQueueLength = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "slimytm_queue_length",
	Help: "The current length of the queue",
}, []string{"player"})

var metricQueueIndex = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "slimytm_queue_index",
	Help: "The current index of the queue",
}, []string{"player"})

var metricBufferLength = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "slimytm_buffer_length_bytes",
	Help: "The current length of the audio buffer in bytes",
}, []string{"player"})

var metricPlayState = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "slimytm_play_state",
	Help: "The current state of play. 3=loading, 2=paused, 1=playing, 0=not_playing",
}, []string{"player"})

var metricFrameTiming = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "slimytm_frame_timing_seconds",
	Help:    "How long it takes to generate and send a frame",
	Buckets: prometheus.ExponentialBuckets(0.025, 1.5, 7),
}, []string{"player"})

var metricSecondsPlayed = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "slimytm_played_audio_seconds_total",
	Help: "The total number of seconds of audio that has been played",
}, []string{"player"})

func (q *Queue) Watch() {
	for {
		// Update metrics
		metricQueueLength.WithLabelValues(q.Player.GetName()).Set(float64(len(q.Songs)))
		metricQueueIndex.WithLabelValues(q.Player.GetName()).Set(float64(q.Index))

		if q.Buffer != nil {
			metricBufferLength.WithLabelValues(q.Player.GetName()).Set(float64(q.Buffer.Len()))
		} else {
			metricBufferLength.WithLabelValues(q.Player.GetName()).Set(0)
		}

		if q.Loading {
			metricPlayState.WithLabelValues(q.Player.GetName()).Set(3)
		} else if q.Paused {
			metricPlayState.WithLabelValues(q.Player.GetName()).Set(2)
		} else if q.Playing {
			metricPlayState.WithLabelValues(q.Player.GetName()).Set(1)
		} else {
			metricPlayState.WithLabelValues(q.Player.GetName()).Set(0)
		}

		// Check whether we have finished a song
		if q.Playing && len(q.Songs) > 0 && q.Index >= 0 && q.Index < len(q.Songs) {
			metricSecondsPlayed.WithLabelValues(q.Player.GetName()).Add(0.1)

			songTimes := strings.Split(q.Songs[q.Index].Duration, ":")
			mins, _ := strconv.Atoi(songTimes[0])
			secs, _ := strconv.Atoi(songTimes[1])

			if q.ElapsedSecs >= mins*60+secs-1 {
				logger.Debug("reached end of song")
				if q.Index+1 < len(q.Songs) {
					logger.Debug("will play next song")
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
	logger.Debugw("next song called",
		"index", q.Index,
		"queueLen", len(q.Songs))

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
		logger.Debug("loading next song")
		q.Loading = true
		q.UpdateClients()
		q.CancelPlaying = q.Player.Play(q.Songs[q.Index].ID)
	} else {
		logger.Debug("no more songs left")
		q.Reset()
	}
}

func (q *Queue) Previous() {
	logger.Debugw("previous song called",
		"index", q.Index,
		"queueLen", len(q.Songs))

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
		logger.Debug("queue unpaused")
	} else {
		q.Player.Pause()
		logger.Debug("queue paused")
	}

	q.Paused = !q.Paused
	q.Playing = !q.Playing
	q.UpdateClients()
}

func (q *Queue) Reset() {
	logger.Debug("queue reset")

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

	return []byte(fmt.Sprintf(`{"id": "%v", "name": "%v", "type": "%v", "song": %v, "paused": %v, "loading": %v, "volume": %v}`,
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

	frameTime := time.Now()
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
		metricFrameTiming.WithLabelValues(q.Player.GetName()).Observe(float64(time.Since(frameTime)) / float64(time.Second))
		frameTime = time.Now()

		// Animate the screen at 30 fps
		time.Sleep(time.Millisecond * 33)
	}
}
