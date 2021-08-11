package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
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

		if string(b[:4]) == "HELO" {
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

			c.setVolume(50)
			go c.clock()
			time.Sleep(time.Second * 2)
			c.displayClock = true
		} else if string(b[:4]) == "STAT" {
			if string(b[8:12]) == "STMa" {
				playing = true
			}

			bytesReceived = int(binary.BigEndian.Uint64(b[23:31]))
			fullness = int(binary.BigEndian.Uint32(b[19:23]))
			lastStat = time.Now()
			fmt.Println("STAT", string(b[8:12]))
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
			c.setText(fmt.Sprintf("                %02d:%02d:%02d", h, m, s))
			c.render()
		}

		time.Sleep(time.Second)
	}
}

func (c *client) playSong(videoID string) {
	co := exec.Command("youtube-dl", "https://music.youtube.com/watch?v="+videoID, "-f", "bestaudio[ext=webm]", "-g")
	fmt.Println(co.String())
	b, err := co.CombinedOutput()
	fmt.Println(string(b))
	if err != nil {
		panic(err)
	}

	audioAssetsBytes.Reset()
	fcmd := exec.Command("ffmpeg", "-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "5", "-i",
		string(b), "-f", "wav", "-ar", "48000", "-ac", "2", "-loglevel", "warning", "-vn", "-")
	fcmd.Stdout = audioAssetsBytes
	fcmd.Stderr = os.Stderr

	err = fcmd.Start()
	if err != nil {
		panic(err)
	}

	time.Sleep(time.Millisecond * 1000)

	c.setVolume(25)

	header := "GET /assets/audio.wav HTTP/1.0\n\n"
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, uint16(28+len(header)))
	msg = append(msg, []byte("strm")...)
	msg = append(msg, 's', '1', 'p', '1', '4', '2', '1', 0xff, 0, 0, '0', 0, 0, 0, 0, 0, 0, 0, 35, 41, 0, 0, 0, 0)
	msg = append(msg, []byte(header)...)
	fmt.Println(len(msg))
	c.conn.Write(msg)
	fmt.Println(hex.Dump(msg))

	playing = true
}

func (c *client) setVolume(volume int) {
	// Volume is 0-100
	level := fmt.Sprintf("%05X", int(0x80000*math.Pow(float64(volume)/100, 2)))

	// out_LL            d0:0354	# volume output control: left->left gain
	// out_RR            d0:0357	# volume output control: right->right gain
	// VOLUME		  cwrite:0010	# volume

	// Digital volume control?
	i2c := c.makeI2C("d0", "0354", level)
	i2c = append(i2c, c.makeI2C("d0", "0357", level)...)
	i2c = append(i2c, c.makeI2C("cwrite", "0010", "7600")...)

	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, uint16(4+len(i2c)))
	msg = append(msg, []byte("i2cc")...)
	msg = append(msg, []byte(i2c)...)
	fmt.Println(len(msg))
	c.conn.Write(msg)
	fmt.Println(hex.Dump(msg))
}

func (c *client) makeI2C(bank, address, data string) []byte {
	a := nibbleise(address)
	d := nibbleise(data)

	var i2c []byte
	switch bank {
	case "d0":
		a32, _ := hex.DecodeString(a[3] + a[2])
		a10, _ := hex.DecodeString(a[1] + a[0])
		d4, _ := hex.DecodeString("0" + d[4])
		d32, _ := hex.DecodeString(d[3] + d[2])
		d10, _ := hex.DecodeString(d[1] + d[0])
		i2c = []byte{'s', 0x3e, 'w', 0x68, 'w', 0xe0, 'w', 0x00, 'w', 0x00, 'w', 0x01, 'w', a32[0], 'w', a10[0],
			'w', 0x00, 'w', d4[0], 'w', d32[0], 'p', d10[0]}

	case "cwrite":
		a32, _ := hex.DecodeString(a[3] + a[2])
		a10, _ := hex.DecodeString(a[1] + a[0])
		d32, _ := hex.DecodeString(d[3] + d[2])
		d10, _ := hex.DecodeString(d[1] + d[0])
		i2c = []byte{'s', 0x3e, 'w', 0x6c, 'w', a32[0], 'w', a10[0],
			'w', d32[0], 'p', d10[0]}
	}

	return i2c
}

// Nibbelise converts a hex string into nibbles. Least significant at [0]
func nibbleise(in string) []string {
	var out []string
	for k := range in {
		out = append(out, string(in[len(in)-1-k]))
	}

	return out
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
