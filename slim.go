package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

	"golang.org/x/text/encoding/charmap"
)

// client is thread-safe
type client struct {
	conn         *net.TCPConn
	displayClock bool
	font         psfFont
	framebuffer  [560]byte
}

var clients []*client

func (c *client) listener() {
	fmt.Println("Received new tcp connection")

	for {
		b := make([]byte, 1024)
		n, err := c.conn.Read(b)
		if err != nil {
			panic(err)
		}

		fmt.Println(b[:n])
		fmt.Println("DATA LEN:", n)

		if bytes.Equal(b[:4], []byte("HELO")) {
			fmt.Println("Squeezebox says HELO!")
			clients = append(clients, c)

			if b[8] != 2 {
				panic("non-squeezebox device connected")
			}

			fmt.Println("Firmware:", b[9])
			fmt.Println("MAC:", net.HardwareAddr(b[10:16]).String())

			f, err := os.Open("GohaClassic-16.psfu")
			if err != nil {
				panic(err)
			}

			font := readPSF(f)
			f.Close()

			c.font = font
			c.setText("SlimYTM")

			c.render()
			go c.clock()
			time.Sleep(time.Second * 2)
			c.displayClock = true
		}
	}
}

func (c *client) render() {
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, 566)
	msg = append(msg, []byte("grfd")...)
	msg = append(msg, 0x02, 0x30)
	msg = append(msg, c.framebuffer[:]...)
	c.conn.Write(msg)

	c.framebuffer = [560]byte{}
}

func (c *client) clock() {
	for {
		if c.displayClock {
			h, m, s := time.Now().Local().Clock()
			c.setText(fmt.Sprintf("            %02d:%02d:%02d", h, m, s))
			c.render()
		}

		time.Sleep(time.Second)
	}
}

func (c *client) playSong(videoID string) {
	os.Remove("assets/audio.webm")
	os.Remove("assets/audio.wav")

	co := exec.Command("youtube-dl", "https://music.youtube.com/watch?v="+videoID, "-f", "bestaudio[ext=webm]",
		"-o", "assets/audio.webm", "--external-downloader", "aria2c")
	fmt.Println(co.String())
	b, err := co.CombinedOutput()
	fmt.Println(string(b))
	if err != nil {
		panic(err)
	}

	wkdir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	_, err = exec.Command("ffmpeg", "-y", "-i", wkdir+"/assets/audio.webm", "-vn", wkdir+"/assets/audio.wav").Output()
	if err != nil {
		panic(err)
	}

	header := "GET /assets/audio.wav HTTP/1.0\n\n"
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, uint16(28+len(header)))
	msg = append(msg, []byte("strm")...)
	msg = append(msg, 's', '1', 'p', '1', '4', '2', '1', 1, 0, 1, '0', 0, 0, 0, 0, 0, 0, 0, 35, 40, 0, 0, 0, 0)
	msg = append(msg, []byte(header)...)
	fmt.Println(len(msg))
	c.conn.Write(msg)
	fmt.Println(hex.Dump(msg))
}

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

		client := &client{conn: conn}
		go client.listener()
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
	tcpListener()
}
