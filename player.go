package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"time"

	"golang.org/x/text/encoding/charmap"
)

// player represents a squeezebox device
type player interface {
	GetID() int
	GetModel() string
	Listener()
	Heartbeat()

	// Display the clock. Outputs framebuffers to the channel
	DisplayClock() chan []byte
	// Display the text, scrolling if needed. Outputs framebuffers to the channel
	DisplayText(text string, ctx context.Context) chan []byte
	Render(buf []byte)

	Play(videoID string) (cancel func())
	Stop()

	SetVolume(level int)
	GetVolume() int

	Pause()
	Unpause()
}

const (
	AUDIO_PRELOAD      = 10 // Seconds of audio to load before playing
	VOLUME_INCREMENT   = 5
	IR_INTERVAL        = 200 * time.Millisecond // Prevents duplicate IR commands from ruining our day
	HEARTBEAT_INTERVAL = time.Second * 20       // Interval to request heartbeats at
)

// lastIR is global to prevent multiple players picking up the same signal
var lastIR time.Time

func tcpListener() {
	addr, err := net.ResolveTCPAddr("tcp4", ":3483")
	if err != nil {
		panic(err)
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		panic(err)
	}

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			panic(err)
		}

		fmt.Println("Received new tcp connection")

		b := make([]byte, 1024)
		_, err = conn.Read(b)
		if err != nil {
			panic(err)
		}

		if string(b[:4]) != "HELO" {
			log.Println("didn't receive a HELO")
		}

		fmt.Println("Squeezebox says HELO!")

		var c player
		queue := &Queue{
			Buffer: new(audioBufferWrapper),
		}

		if b[8] == 2 {
			fmt.Println("Connected to a Squeezebox v1")
			c = &squeezebox1{id: rand.Intn(100000), conn: conn, Queue: queue}
		} else if b[8] == 4 {
			fmt.Println("Connected to a Squeezebox v2")
			c = &squeezebox2{id: rand.Intn(100000), conn: conn, Queue: queue}
		} else {
			log.Println("non-squeezebox device tried to connect")
			log.Println("(choosing to continue, acting like it's a sbox2)")
			c = &squeezebox2{id: rand.Intn(100000), conn: conn, Queue: queue}
			// continue
		}

		fmt.Println("Firmware:", b[9])
		fmt.Println("MAC:", net.HardwareAddr(b[10:16]).String())

		queue.Player = c
		queues = append(queues, queue)

		go c.Listener()
		go c.Heartbeat()
		go queue.Watch()
	}
}

func udpListener() {
	addr, err := net.ResolveUDPAddr("udp4", ":3483")
	if err != nil {
		panic(err)
	}

	listener, err := net.ListenUDP("udp", addr)
	if err != nil {
		panic(err)
	}

	for {
		b := make([]byte, 1024)
		_, remote, err := listener.ReadFromUDP(b)
		if err != nil {
			panic(err)
		}

		if b[0] != 'd' {
			panic("received non-discovery request")
		}

		fmt.Println("Responding to discovery request from", remote.String())
		encoder := charmap.ISO8859_10.NewEncoder()
		resp, err := encoder.String("SlimYTM")
		if err != nil {
			panic(err)
		}
		resp = "D" + resp

		listener.WriteTo(append([]byte(resp), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0), remote)
	}

}
