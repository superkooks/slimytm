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
	framebuffer []byte
	volume      int
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
	s.SetVolume(50)

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
			irCode := hex.EncodeToString(b[14:18])
			fmt.Println("IR Code", irCode)
			sendXPL(xplMessage{
				messageType: "xpl-trig",
				target:      "*",
				schema:      "remote.basic",
				body: map[string]string{
					"keys":   irCode,
					"device": "squeezebox",
					"zone":   "slimserver",
				},
			})

			if irCode == "7689807f" {
				// Volume UP
				s.SetVolume(s.volume + 5)
				s.DisplayText(fmt.Sprintf("Volume = %v/100", s.volume))
				textDelay = time.Second * 2
			} else if irCode == "768900ff" {
				// Volume DOWN
				s.SetVolume(s.volume - 5)
				s.DisplayText(fmt.Sprintf("Volume = %v/100", s.volume))
				textDelay = time.Second * 2
			} else if irCode == "7689a05f" && time.Since(lastIR) > time.Second {
				// NEXT Song
				nextSong()
			} else if irCode == "7689c03f" && time.Since(lastIR) > time.Second {
				// PREVIOUS Song
				previousSong()
			} else if irCode == "768920df" && time.Since(lastIR) > time.Second {
				// PAUSE/UNPAUSE Song
				togglePause()
			}

			lastIR = time.Now()
		}
	}
}

func (s *squeezebox2) DisplayText(text string) {
	if len(text) > 40 {
		// Scroll text across screen
		text += "  "
		variableFrame := make([]byte, 4*8*len(text))
		for k, v := range text {
			s.setChar(s.font.getChar(int(v)), k*8, variableFrame)
		}

		go s.scrollBuffer(variableFrame)
	} else {
		for k, v := range text {
			// Set each character individually with an offset
			s.setChar(s.font.getChar(int(v)), k*8, s.framebuffer)
		}

		s.render()
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

func (s *squeezebox2) Stop() {
	// Send the strm command to the Squeezebox
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, uint16(28))
	msg = append(msg, []byte("strm")...)
	msg = append(msg, 'q', '1', 'p', '1', '4', '2', '1', 0xff, 0, 0, '0', 0, 0, 0, 0, 0, 0, 0, 35, 41, 0, 0, 0, 0)
	fmt.Println(len(msg))
	s.conn.Write(msg)
	fmt.Println(hex.Dump(msg))
}

func (s *squeezebox2) Pause() {
	// Send the strm command to the Squeezebox
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, uint16(28))
	msg = append(msg, []byte("strm")...)
	msg = append(msg, 'p', '1', 'p', '1', '4', '2', '1', 0xff, 0, 0, '0', 0, 0, 0, 0, 0, 0, 0, 35, 41, 0, 0, 0, 0)
	fmt.Println(len(msg))
	s.conn.Write(msg)
	fmt.Println(hex.Dump(msg))
}

func (s *squeezebox2) Unpause() {
	// Send the strm command to the Squeezebox
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, uint16(28))
	msg = append(msg, []byte("strm")...)
	msg = append(msg, 'u', '1', 'p', '1', '4', '2', '1', 0xff, 0, 0, '0', 0, 0, 0, 0, 0, 0, 0, 35, 41, 0, 0, 0, 0)
	fmt.Println(len(msg))
	s.conn.Write(msg)
	fmt.Println(hex.Dump(msg))
}

func (s *squeezebox2) SetVolume(volume int) {
	// Set the volume (0-100)
	if volume < 0 {
		volume = 0
	} else if volume > 100 {
		volume = 100
	}

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

	if volume == 0 {
		newGain = []byte{0, 0, 0, 0}
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

	s.volume = volume
}

func (s *squeezebox2) GetVolume() int {
	return s.volume
}

func (s *squeezebox2) render() {
	if len(s.framebuffer) != 1280 {
		panic("framebuffer has incorrect length")
	}

	// Send the current framebuffer to the Squeezebox
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, 1288)
	msg = append(msg, []byte("grfe")...)
	msg = append(msg, 0, 0, 'c', 'c')
	msg = append(msg, s.framebuffer...)
	s.conn.Write(msg)

	s.framebuffer = make([]byte, 1280)
}

func (s *squeezebox2) scrollBuffer(varBuffer []byte) {
	// Display intial 40 chars
	s.framebuffer = varBuffer[:1280]
	s.render()
	time.Sleep(time.Second * 3)

	// Scroll text across until we reach the start
	for i := 0; i < len(varBuffer); i += 12 {
		if i > len(varBuffer)-1280 {
			// Current frame overlaps end of buffer
			s.framebuffer = varBuffer[i:]
			s.framebuffer = append(s.framebuffer, varBuffer[0:1280+i-len(varBuffer)]...)
		} else {
			s.framebuffer = varBuffer[i : i+1280]
		}

		// Animate at 30 fps
		s.render()
		time.Sleep(time.Millisecond * 33)
	}

	// Display the first frame for another second
	s.framebuffer = varBuffer[:1280]
	s.render()
	time.Sleep(time.Second)
}

func (s *squeezebox2) setChar(chr []byte, offset int, buffer []byte) {
	// Set the chr in the framebuffer
	i := 4 * offset * 8
	for col := 0; col < 8; col++ {
		for k := range chr {
			v := chr[len(chr)-1-(k+8)%16]
			mask := byte(0b10000000 >> col)
			if v&mask > 0 {
				squeezeMask := byte(1 << (i % 8))
				buffer[i/8] |= squeezeMask
			}

			i++
		}
		i += 16
	}
}
