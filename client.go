package main

import (
	"encoding/json"
	"math/rand"
	"net/http"

	"github.com/Jeffail/gabs/v2"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Event struct {
	Type   string          `json:"type"`
	Player string          `json:"player"`
	Data   json.RawMessage `json:"data"`
}

type PlayEvent struct {
	QueueType string          `json:"queueType"`
	QueueID   string          `json:"queueId"`
	StartSong json.RawMessage `json:"startSong"`
	Shuffle   bool            `json:"shuffle"`
}

type Client struct {
	Conn *websocket.Conn
}

var clients []*Client

var metricConnectedClients = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "slimytm_connected_clients",
	Help: "The number of clients currently connected",
})

func (c *Client) Listener() {
	for {
		var e Event
		err := c.Conn.ReadJSON(&e)
		if err != nil {
			logger.Debugw("client disconnected",
				"err", err)
			metricConnectedClients.Dec()
			return
		}

		// Find the correct queue
		var queue *Queue
		for _, v := range queues {
			if v.Player.GetID() == e.Player {
				queue = v
			}
		}

		if e.Type == "PLAY" {
			var p PlayEvent
			err := json.Unmarshal(e.Data, &p)
			if err != nil {
				logger.Warnw("unable to unmarshal event",
					"err", err)
				continue
			}

			c.PlaySongs(queue, p)
		} else if e.Type == "NEXT" {
			queue.Next()
		} else if e.Type == "PREVIOUS" {
			queue.Previous()
		} else if e.Type == "PAUSE" {
			queue.Pause()
		} else if e.Type == "VOLUME" {
			var v int
			err := json.Unmarshal(e.Data, &v)
			if err != nil {
				logger.Warnw("unable to unmarshal event",
					"err", err)
				continue
			}

			queue.Player.SetVolume(v)
			queue.UpdateClients()
		} else {
			logger.Warnw("received unknown event from web client",
				"event", e.Type)
		}
	}
}

func (c *Client) PlaySongs(q *Queue, p PlayEvent) {
	if q == nil {
		logger.Warnw("unknown player for event, dropping")
		return
	}

	// Add the start song to the queue
	var startSong Song
	err := json.Unmarshal(p.StartSong, &startSong)
	if err != nil {
		logger.Warnw("unable to unmarshal event",
			"err", err)
		return
	}

	q.Songs = []Song{startSong}
	q.Index = -1
	q.Next()

	// Retrieve the rest of the songs and enqueue them
	var songs []Song
	switch p.QueueType {
	case "playlist":
		resp, err := http.Get("http://localhost:9000/api/playlist/" + p.QueueID)
		if err != nil {
			logger.Errorw("unable to retrieve playlist",
				"err", err)
			return
		}

		playlist, err := gabs.ParseJSONBuffer(resp.Body)
		if err != nil {
			logger.Errorw("unable to parse json playlist info",
				"err", err)
			return
		}

		err = json.Unmarshal(playlist.Path("tracks").Bytes(), &songs)
		if err != nil {
			logger.Errorw("unable to get tracks from playlist info",
				"err", err)
			return
		}

	default:
		logger.Warn("unknown queue type")
		return
	}

	if p.Shuffle {
		logger.Debug("shuffling playlist")
		rand.Shuffle(len(songs), func(i, j int) { songs[i], songs[j] = songs[j], songs[i] })

		// Remove the already playing song from the list
		for k, v := range songs {
			if startSong.ID == v.ID {
				songs = append(songs[:k], songs[k+1:]...)
				break
			}
		}

		q.Songs = append(q.Songs, songs...)

	} else {
		q.Songs = songs

		for k, v := range q.Songs {
			// Set the queue index to the start song
			if startSong.ID == v.ID {
				q.Index = k
				break
			}
		}
	}
}
