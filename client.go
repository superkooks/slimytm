package main

import (
	"encoding/json"
	"net/http"

	"github.com/Jeffail/gabs/v2"
	"github.com/gorilla/websocket"
)

type Event struct {
	Type   string          `json:"type"`
	Player int             `json:"player"`
	Data   json.RawMessage `json:"data"`
}

type PlayEvent struct {
	QueueType string          `json:"queueType"`
	QueueID   string          `json:"queueId"`
	StartSong json.RawMessage `json:"startSong"`
}

type Client struct {
	Conn *websocket.Conn
}

var clients []*Client

func (c *Client) Listener() {
	for {
		var e Event
		err := c.Conn.ReadJSON(&e)
		if err != nil {
			logger.Debugw("client disconnected",
				"err", err)
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

		err = json.Unmarshal(playlist.Path("tracks").Bytes(), &q.Songs)
		if err != nil {
			logger.Errorw("unable to get tracks from playlist info",
				"err", err)
			return
		}

		for k, v := range q.Songs {
			// Set the queue index to the start song
			if startSong.ID == v.ID {
				q.Index = k
			}
		}

	default:
		logger.Warn("unknown queue type")
	}
}
