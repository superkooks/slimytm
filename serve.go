package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Handle players downloading audio
func audio(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	for _, v := range queues {
		if fmt.Sprint(v.Player.GetID()) == vars["id"] {
			fmt.Println("****** New audio request")
			fmt.Println("Buffer is currently", v.Buffer.Len(), "bytes long")
			fmt.Println("     =", v.Buffer.Len()/48000/2/2, "s")
			io.Copy(w, v.Buffer)
			return
		}
	}
}

// Handle clients getting all the players
func getPlayers(w http.ResponseWriter, r *http.Request) {
	// Inlining a struct seems like the simplest solution
	type resp struct {
		ID int `json:"id"`
		// Name string `json:"name"`
		Type string `json:"type"`
		// Thumbnail string `json:"thumbnail"`
	}

	var out []resp
	for _, v := range queues {
		out = append(out, resp{
			ID:   v.Player.GetID(),
			Type: v.Player.GetModel(),
		})
	}

	b, err := json.Marshal(out)
	if err != nil {
		panic(err)
	}

	w.Write(b)
}

// Handle the websocket connection from any client
func ws(w http.ResponseWriter, r *http.Request) {
	// Upgrade the connection to ws
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		panic(err)
	}

	// Add the client to our list
	c := &Client{Conn: conn}
	clients = append(clients, c)

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
	go udpListener()
	go tcpListener()

	xplInit()
	go xplListener()

	fmt.Println("Serving api on :9001")
	r := mux.NewRouter()
	r.Use(corsMiddleware)
	r.Path("/players").HandlerFunc(getPlayers)
	r.Path("/player/{id}/audio.wav").HandlerFunc(audio)
	r.Path("/ws").HandlerFunc(ws)

	// f, _ := os.Open("wa.wav")
	// n, err := io.Copy(audioAssetsBytes, f)
	// fmt.Println("copied", n, "bytes")
	// if err != nil {
	// 	panic(err)
	// }
	// f.Close()

	panic(http.ListenAndServe(":9001", r))
}
