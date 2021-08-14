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
)

type squeezebox2 struct {
	conn        *net.TCPConn
	font        psfFont
	framebuffer [1280]byte
}

func (s *squeezebox2) GetModel() int {
	return 2
}

func (s *squeezebox2) Listener() {
	// Load the font for this player
	f, err := os.Open("GohaClassic-16.psfu")
	if err != nil {
		panic(err)
	}

	font := readPSF(f)
	s.font = font
	f.Close()

	// Display init message
	s.DisplayText("SlimYTM")
	s.render()
	time.Sleep(time.Second * 2)

	// Set the volume to 1/2 intially
	s.SetVolume(80)

	// Start rendering routine
	go func() {
		for {
			s.render()
			time.Sleep(time.Millisecond * 100)
		}
	}()

	// Start receiving messages
	for {
		b := make([]byte, 1024)
		n, err := s.conn.Read(b)
		if err != nil {
			panic(err)
		}

		fmt.Println(b[:n])
		fmt.Println("DATA LEN:", n)

		if string(b[:4]) == "STAT" {
			// Status message from the squeezebox
			if string(b[8:12]) == "STMa" {
				playing = true
			}

			elapsedSeconds = int(binary.BigEndian.Uint32(b[45:49]))
			fmt.Println("********** STAT", string(b[8:12]))

		} else if string(b[:4]) == "IR  " {
			// IR command from the remote
			fmt.Println("IR Code", hex.EncodeToString(b[14:18]))
			sendXPL(xplMessage{
				messageType: "xpl-trig",
				target:      "*",
				schema:      "remote.basic",
				body: map[string]string{
					"keys":   hex.EncodeToString(b[14:18]),
					"device": "squeezebox",
					"zone":   "slimserver",
				},
			})
		}
	}
}

func (s *squeezebox2) DisplayText(text string) {
	if len(text) > 40 {
		panic("text too long to display")
	}

	for k, v := range text {
		// Set each character individually with an offset
		s.setChar(s.font.getChar(int(v)), k*8)
	}
}

func (s *squeezebox2) Play(videoID string) {
	// Get the player URL with youtube-dl
	co := exec.Command("youtube-dl", "https://music.youtube.com/watch?v="+videoID, "-f", "bestaudio[ext=webm]", "-g")
	fmt.Println(co.String())
	b, err := co.CombinedOutput()
	fmt.Println(string(b))
	if err != nil {
		panic(err)
	}

	// Start FFMPEG with the URL, piping stdout to our audio buffer
	audioBuffer.Reset()
	fcmd := exec.Command("ffmpeg", "-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "5", "-i",
		string(b), "-f", "wav", "-ar", "48000", "-ac", "2", "-loglevel", "warning", "-vn", "-")
	fcmd.Stdout = audioBuffer
	fcmd.Stderr = os.Stderr

	err = fcmd.Start()
	if err != nil {
		panic(err)
	}

	// Wait until with have at least AUDIO_PRELOAD seconds of audio in our buffer
	for {
		if audioBuffer.Len() > 48000*2*2*AUDIO_PRELOAD {
			break
		}
	}

	// Send the strm command to the Squeezebox
	header := "GET /assets/audio.wav HTTP/1.0\n\n"
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, uint16(28+len(header)))
	msg = append(msg, []byte("strm")...)
	msg = append(msg, 's', '1', 'p', '1', '4', '2', '1', 0xff, 0, 0, '0', 0, 0, 0, 0, 0, 0, 0, 35, 41, 0, 0, 0, 0)
	msg = append(msg, []byte(header)...)
	fmt.Println(len(msg))
	s.conn.Write(msg)
	fmt.Println(hex.Dump(msg))

	playing = true
}

func (s *squeezebox2) SetVolume(volume int) {
	// Set the volume (0-100)

	// Old gain for Squeezebox2 with firmware < 22
	oldGain := make([]byte, 4)
	binary.BigEndian.PutUint32(oldGain, uint32(float64(volume)/100*128))

	// New gain with fancy dB stuff
	newGain := make([]byte, 4)
	m := 50 / float64(100+1)
	db := m * (float64(volume) - 100)
	floatMult := math.Pow(10, db/20)

	if db >= -30 && db < 0 {
		binary.BigEndian.PutUint32(newGain, uint32(floatMult*(1<<8)+0.5)*(1<<8))
	} else {
		binary.BigEndian.PutUint32(newGain, uint32(floatMult*(1<<16)+0.5))
	}

	// Dispatch volume message
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, uint16(22))
	msg = append(msg, []byte("audg")...)
	msg = append(msg, oldGain...)
	msg = append(msg, oldGain...)
	msg = append(msg, 1, 255) // Always use digital volume and 255 preamp
	msg = append(msg, newGain...)
	msg = append(msg, newGain...)
	fmt.Println(len(msg))
	s.conn.Write(msg)
	fmt.Println(hex.Dump(msg))
}

func (s *squeezebox2) render() {
	// Send the current framebuffer to the Squeezebox
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, 1288)
	msg = append(msg, []byte("grfe")...)
	msg = append(msg, 0, 0, 'c', 'c')
	msg = append(msg, s.framebuffer[:]...)
	s.conn.Write(msg)

	s.framebuffer = [1280]byte{}
}

func (s *squeezebox2) setChar(chr []byte, offset int) {
	// Set the chr in the framebuffer
	i := 4 * offset * 8
	for col := 0; col < 8; col++ {
		for k := range chr {
			v := chr[len(chr)-1-(k+8)%16]
			mask := byte(0b10000000 >> col)
			if v&mask > 0 {
				squeezeMask := byte(1 << (i % 8))
				s.framebuffer[i/8] |= squeezeMask
			}

			i++
		}
		i += 16
	}
}
