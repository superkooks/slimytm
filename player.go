package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"golang.org/x/text/encoding/charmap"
)

// player represents a squeezebox device
type player interface {
	GetModel() int
	Listener()
	DisplayText(text string, ctx context.Context)

	Play(videoID string)
	Stop()

	SetVolume(level int)
	GetVolume() int

	Pause()
	Unpause()
}

type text struct {
	text   string
	expiry time.Time
}

const (
	AUDIO_PRELOAD    = 10 // Seconds of audio to load before playing
	VOLUME_INCREMENT = 5
)

var players []player
var textStack []text
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
		if b[8] == 2 {
			fmt.Println("Connected to a Squeezebox v1")
			c = &squeezebox1{conn: conn, framebuffer: make([]byte, 560)}
		} else if b[8] == 4 {
			fmt.Println("Connected to a Squeezebox v2")
			c = &squeezebox2{conn: conn, framebuffer: make([]byte, 1280)}
		} else {
			log.Println("non-squeezebox device tried to connect")
			continue
		}

		fmt.Println("Firmware:", b[9])
		fmt.Println("MAC:", net.HardwareAddr(b[10:16]).String())

		players = append(players, c)
		go c.Listener()
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
		resp, err := encoder.String("SuperKooks")
		if err != nil {
			panic(err)
		}
		resp = "D" + resp

		listener.WriteTo(append([]byte(resp), 0, 0, 0, 0, 0, 0, 0), remote)
	}

}

func startSqueezebox() {
	go udpListener()
	go tcpListener()

	var currentText text
	var cancel context.CancelFunc
	var checkStack bool
	for {
		if len(textStack) == 0 {
			h, m, sec := time.Now().Local().Clock()
			for _, p := range players {
				p.DisplayText(fmt.Sprintf("                %02d:%02d:%02d", h, m, sec), context.Background())
			}
		} else {
			if time.Until(currentText.expiry) > 0 && !checkStack {
				// Watch the stack for new text
				if textStack[len(textStack)-1].text != currentText.text {
					// If we find new text, then cancel the current text and display the new text
					cancel()
					checkStack = true
					continue
				}
			} else {
				checkStack = false
				currentText = textStack[len(textStack)-1]
				for time.Since(currentText.expiry) > 0 {
					// Don't display text that has expired
					textStack = textStack[:len(textStack)-1]
					if len(textStack) == 0 {
						break
					}
					currentText = textStack[len(textStack)-1]
				}

				if len(textStack) == 0 {
					continue
				}

				var ctx context.Context
				ctx, cancel = context.WithTimeout(context.Background(), time.Until(currentText.expiry))
				for _, p := range players {
					p.DisplayText(currentText.text, ctx)
				}
			}
		}

		time.Sleep(time.Millisecond * 100)
	}
}
