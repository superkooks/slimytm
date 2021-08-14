package main

import (
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
	DisplayText(text string)
	Play(videoID string)
	SetVolume(level int)
}

const (
	AUDIO_PRELOAD = 5 // Seconds of audio to load before playing
)

var players []player
var displayClock = true
var xplOSDText string

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
			c = &squeezebox1{conn: conn}
		} else if b[8] == 4 {
			fmt.Println("Connected to a Squeezebox v2")
			c = &squeezebox2{conn: conn}
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

	for {
		for _, p := range players {
			if displayClock {
				h, m, sec := time.Now().Local().Clock()
				p.DisplayText(fmt.Sprintf("                %02d:%02d:%02d", h, m, sec))
			} else {
				p.DisplayText(xplOSDText)
			}
		}

		time.Sleep(time.Millisecond * 100)
	}
}
