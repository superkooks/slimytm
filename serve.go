package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var logger *zap.SugaredLogger

// Handle players downloading audio
func audio(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	for _, v := range queues {
		if fmt.Sprint(v.Player.GetID()) == vars["id"] {
			logger.Debugw("new audio request",
				"bufLen", v.Buffer.Len(),
				"bufSecs", v.Buffer.Len()/48000/2/2)
			io.Copy(w, v.Buffer)
			return
		}
	}
}

// Handle clients getting all the players
func getPlayers(w http.ResponseWriter, r *http.Request) {
	// Inlining a struct seems like the simplest solution
	type resp struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
		// Thumbnail string `json:"thumbnail"`
	}

	var out []resp
	for _, v := range queues {
		out = append(out, resp{
			ID:   v.Player.GetID(),
			Type: v.Player.GetModel(),
			Name: v.Player.GetName(),
		})
	}

	b, err := json.Marshal(out)
	if err != nil {
		logger.Errorw("unable to encode player information",
			"err", err)
		return
	}

	w.Write(b)
}

// Handle the websocket connection from any client
func ws(w http.ResponseWriter, r *http.Request) {
	// Upgrade the connection to ws
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Errorw("unable to upgrade connection to websocket",
			"err", err)
		return
	}

	// Add the client to our list
	c := &Client{Conn: conn}
	clients = append(clients, c)
	metricConnectedClients.Inc()

	// Set the close handler on the ws connection
	conn.SetCloseHandler(func(code int, text string) error {
		// Boilerplate close handler
		message := websocket.FormatCloseMessage(code, "")
		conn.WriteControl(websocket.CloseMessage, message, time.Now().Add(time.Second))

		// Remove the client from our list
		for k, v := range clients {
			if v == c {
				clients = append(clients[:k], clients[k+1:]...)
			}
		}

		return nil
	})

	// Update the client with all player states
	for _, v := range queues {
		v.UpdateClients()
	}

	go c.Listener()
}

// A middleware to cope for any CORS requests
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" {
			w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Entrypoint
func main() {
	// Load persistent data
	LoadPersistent()

	// Start the logger
	var conf zap.Config
	conf = zap.NewProductionConfig()
	conf.OutputPaths = append(conf.OutputPaths, persistent.LogLocations...)
	conf.Development = true
	conf.Level.SetLevel(zap.DebugLevel)
	conf.EncoderConfig = zap.NewProductionEncoderConfig()
	conf.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	l, err := conf.Build()
	if err != nil {
		panic(err)
	}

	defer l.Sync()
	logger = l.Sugar()
	logger.Info("slimytm is starting")

	// Start slimproto listeners
	go udpListener()
	go tcpListener()

	// Start xpl
	xplInit()
	go xplListener()

	// Start webserver
	r := mux.NewRouter()
	r.Use(corsMiddleware)
	r.Path("/players").HandlerFunc(getPlayers)
	r.Path("/player/{id}/audio.wav").HandlerFunc(audio)
	r.Path("/ws").HandlerFunc(ws)
	r.Path("/metrics").Handler(promhttp.Handler())

	logger.Panicw("unable to start http server",
		"port", 9001,
		"err", http.ListenAndServe(":9001", r))
}
