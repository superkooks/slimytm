package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"os/exec"
	"time"
)

type squeezebox1 struct {
	Queue *Queue

	id     int
	conn   *net.TCPConn
	font   psfFont
	volume int
}

func (s *squeezebox1) GetID() int {
	return s.id
}

func (s *squeezebox1) GetModel() string {
	return "Squeezebox 1"
}

func (s *squeezebox1) Listener() {
	// Load the font for this player
	f, err := os.Open("GohaClassic-16.psfu")
	if err != nil {
		panic(err)
	}

	font := readPSF(f)
	s.font = font
	f.Close()

	// Display init message
	ctx, _ := context.WithTimeout(context.Background(), time.Second)
	buf := <-s.DisplayText("SlimYTM", ctx)
	s.Render(buf)
	time.Sleep(time.Second * 2)
	go s.Queue.Composite()

	// Set the volume to 1/2 intially
	s.SetVolume(50)

	// Start receiving messages
	for {
		b := make([]byte, 1024)
		s.conn.SetReadDeadline(time.Now().Add(HEARTBEAT_INTERVAL * 3))
		n, err := s.conn.Read(b)
		if errors.Is(err, os.ErrDeadlineExceeded) {
			// Client has timed out, remove it from available players
			for k, v := range queues {
				if v.Player.GetID() == s.GetID() {
					queues = append(queues[:k], queues[k+1:]...)
				}
			}

			fmt.Println("**** Player has timed out")
			s.conn.Close()
			return
		} else if err != nil {
			panic(err)
		}

		fmt.Println(b[:n])
		fmt.Println("DATA LEN:", n)

		if string(b[:4]) == "STAT" {
			// Status message from the squeezebox
			if string(b[8:12]) == "STMa" {
				s.Queue.Playing = true
				s.Queue.Loading = false
				s.Queue.UpdateClients()
			}

			bytesPlayed := int(binary.BigEndian.Uint64(b[23:31])) - int(binary.BigEndian.Uint32(b[19:23]))
			s.Queue.ElapsedSecs = bytesPlayed / 48000 / 2 / 2
			fmt.Println("********** STAT", string(b[8:12]))

		} else if string(b[:4]) == "IR  " {
			if time.Since(lastIR) < IR_INTERVAL {
				// Prevent duplicate IR commands from ruining our day
				continue
			}

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
				s.SetVolume(s.volume + VOLUME_INCREMENT)
				ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
				s.Queue.Texts = append(s.Queue.Texts, text{
					bufs: s.DisplayText(fmt.Sprintf("Volume = %v/100", s.volume), ctx),
					ctx:  ctx,
				})
			} else if irCode == "768900ff" {
				// Volume DOWN
				s.SetVolume(s.volume - VOLUME_INCREMENT)
				ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
				s.Queue.Texts = append(s.Queue.Texts, text{
					bufs: s.DisplayText(fmt.Sprintf("Volume = %v/100", s.volume), ctx),
					ctx:  ctx,
				})
			} else if irCode == "7689a05f" {
				// NEXT Song
				s.Queue.Next()
			} else if irCode == "7689c03f" {
				// PREVIOUS Song
				s.Queue.Previous()
			} else if irCode == "768920df" {
				// PAUSE/UNPAUSE Song
				s.Queue.Pause()
			} else if irCode == "768940bf" {
				// RESET Queue
				s.Queue.Reset()
			}

			lastIR = time.Now()
		}
	}
}

func (s *squeezebox1) Heartbeat() {
	for {
		// Send the strm t command to request status
		msg := make([]byte, 2)
		binary.BigEndian.PutUint16(msg, uint16(28))
		msg = append(msg, []byte("strm")...)
		msg = append(msg, 't', '0', 'm', '?', '?', '?', '?', 0, 0, 0, '0', 0, 0, 0, 0, 0, 0, 0, 35, 41, 0, 0, 0, 0)
		fmt.Println(len(msg))
		fmt.Println(hex.Dump(msg))
		_, err := s.conn.Write(msg)
		if err != nil {
			// Client probably dropped conn
			fmt.Println("**** Heartbeat err:", err)
			return
		}

		time.Sleep(HEARTBEAT_INTERVAL)
	}
}

func (s *squeezebox1) DisplayClock() chan []byte {
	out := make(chan []byte)

	go func() {
		for {
			buf := make([]byte, 1280)
			h, m, sec := time.Now().Local().Clock()
			for k, v := range fmt.Sprintf("                %02d:%02d:%02d", h, m, sec) {
				// Set each character individually with an offset
				s.setChar(s.font.getChar(int(v)), k*8, buf)
			}

			out <- buf
		}
	}()

	return out
}

// Return a channel of framebuffers, scrolling the text if needed.
func (s *squeezebox1) DisplayText(text string, ctx context.Context) chan []byte {
	out := make(chan []byte)

	if len(text) > 35 {
		// Scroll text across screen
		text += "    "
		variableFrame := make([]byte, 2*8*len(text))
		for k, v := range text {
			s.setChar(s.font.getChar(int(v)), k*8, variableFrame)
		}

		go s.scrollBuffer(variableFrame, ctx, out)
	} else {
		buf := make([]byte, 560)
		for k, v := range text {
			// Set each character individually with an offset
			s.setChar(s.font.getChar(int(v)), k*8, buf)
		}

		go func() {
			// Output the framebuffer until the context expires
			for {
				select {
				case <-ctx.Done():
				case out <- buf:
				}
			}
		}()
	}

	return out
}

func (s *squeezebox1) Play(videoID string) (cancel func()) {
	// Get the player URL with youtube-dl
	co := exec.Command("youtube-dl", "https://music.youtube.com/watch?v="+videoID, "-f", "bestaudio[ext=webm]", "-g")
	fmt.Println(co.String())
	b, err := co.CombinedOutput()
	fmt.Println(string(b))
	if err != nil {
		panic(err)
	}

	// Start FFMPEG with the URL, piping stdout to our audio buffer
	s.Queue.Buffer.Reset()
	ctx, cancel := context.WithCancel(context.Background())
	fcmd := exec.CommandContext(ctx, "ffmpeg", "-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "5", "-i",
		string(b), "-f", "wav", "-ar", "48000", "-ac", "2", "-loglevel", "warning", "-vn", "-")
	fcmd.Stdout = s.Queue.Buffer
	fcmd.Stderr = os.Stderr

	err = fcmd.Start()
	if err != nil {
		panic(err)
	}

	// Wait until with have at least AUDIO_PRELOAD seconds of audio in our buffer
	for {
		time.Sleep(50 * time.Millisecond)
		if s.Queue.Buffer.Len() > 48000*2*2*AUDIO_PRELOAD {
			break
		}
	}

	// Send the strm command to the Squeezebox
	header := fmt.Sprintf("GET /player/%v/audio.wav HTTP/1.0\n\n", s.id)
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, uint16(28+len(header)))
	msg = append(msg, []byte("strm")...)
	msg = append(msg, 's', '1', 'p', '1', '4', '2', '1', 0xff, 0, 0, '0', 0, 0, 0, 0, 0, 0, 0, 35, 41, 0, 0, 0, 0)
	msg = append(msg, []byte(header)...)
	fmt.Println(len(msg))
	s.conn.Write(msg)
	fmt.Println(hex.Dump(msg))

	return cancel
}

func (s *squeezebox1) Stop() {
	// Send the strm command to the Squeezebox
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, uint16(28))
	msg = append(msg, []byte("strm")...)
	msg = append(msg, 'q', '1', 'p', '1', '4', '2', '1', 0xff, 0, 0, '0', 0, 0, 0, 0, 0, 0, 0, 35, 41, 0, 0, 0, 0)
	fmt.Println(len(msg))
	s.conn.Write(msg)
	fmt.Println(hex.Dump(msg))
}

func (s *squeezebox1) Pause() {
	// Send the strm command to the Squeezebox
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, uint16(28))
	msg = append(msg, []byte("strm")...)
	msg = append(msg, 'p', '0', 'm', '?', '?', '?', '?', 0, 0, 0, '0', 0, 0, 0, 0, 0, 0, 0, 35, 41, 0, 0, 0, 0)
	fmt.Println(len(msg))
	s.conn.Write(msg)
	fmt.Println(hex.Dump(msg))
}

func (s *squeezebox1) Unpause() {
	// Send the strm command to the Squeezebox
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, uint16(28))
	msg = append(msg, []byte("strm")...)
	msg = append(msg, 'u', '0', 'm', '?', '?', '?', '?', 0, 0, 0, '0', 0, 0, 0, 0, 0, 0, 0, 35, 41, 0, 0, 0, 0)
	fmt.Println(len(msg))
	s.conn.Write(msg)
	fmt.Println(hex.Dump(msg))
}

func (s *squeezebox1) SetVolume(volume int) {
	// Set the volume (0-100) on the MAS35x9 with I2C
	if volume < 0 {
		volume = 0
	} else if volume > 100 {
		volume = 100
	}

	level := fmt.Sprintf("%05X", int(0x80000*math.Pow(float64(volume)/100, 2)))

	// out_LL            d0:0354	# volume output control: left->left gain
	// out_RR            d0:0357	# volume output control: right->right gain
	// VOLUME		  cwrite:0010	# volume

	// Digital volume control?
	i2c := s.makeI2C("d0", "0354", level)
	i2c = append(i2c, s.makeI2C("d0", "0357", level)...)
	i2c = append(i2c, s.makeI2C("cwrite", "0010", "7600")...)

	// Send I2C command
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, uint16(4+len(i2c)))
	msg = append(msg, []byte("i2cc")...)
	msg = append(msg, []byte(i2c)...)
	fmt.Println(len(msg))
	s.conn.Write(msg)
	fmt.Println(hex.Dump(msg))

	s.volume = volume
}

func (s *squeezebox1) GetVolume() int {
	return s.volume
}

func (s *squeezebox1) Render(buf []byte) {
	if len(buf) != 560 {
		panic("framebuffer has incorrect length")
	}

	// Send the current framebuffer to the Squeezebox
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, 566)
	msg = append(msg, []byte("grfd")...)
	msg = append(msg, 0x02, 0x30)
	msg = append(msg, buf...)
	s.conn.Write(msg)
}

// Displays the whole buffer forever until it cancelled
func (s *squeezebox1) scrollBuffer(varBuffer []byte, ctx context.Context, out chan []byte) {
	for {
		// Wait until we are being composited before starting the timer
		out <- make([]byte, 1280)
		out <- make([]byte, 1280)
		stationary := time.NewTimer(time.Second * 3)

	outer:
		for {
			// Display the first 35 characters for 3 seconds
			select {
			case out <- varBuffer[:560]:
			case <-stationary.C:
				break outer
			case <-ctx.Done():
				// Exit if we have been cancelled (by twitter)
				return
			}
		}

		// Scroll text across until we reach the start
		for i := 0; i < len(varBuffer); i += 6 {
			var frame []byte
			if i > len(varBuffer)-560 {
				// Current frame overlaps end of buffer
				frame = append(varBuffer[i:], varBuffer[0:560+i-len(varBuffer)]...)
			} else {
				frame = varBuffer[i : i+560]
			}

			select {
			case out <- frame:
			case <-ctx.Done():
				return
			}
		}

		// Continue doing this until we are cancelled
	}
}

// makeI2C generates an I2C command string
func (s *squeezebox1) makeI2C(bank, address, data string) []byte {
	// Nibbleise(tm) the address and data
	a := nibbleise(address)
	d := nibbleise(data)

	// Follow fixed formats for each bank
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

func (s *squeezebox1) setChar(chr []byte, offset int, buffer []byte) {
	// Set the chr in the framebuffer
	i := 2 * offset * 8
	for col := 0; col < 8; col++ {
		for _, v := range chr {
			mask := byte(0b10000000 >> col)
			if v&mask > 0 {
				squeezeMask := byte(1 << (7 - i%8))
				buffer[i/8] |= squeezeMask
			}

			i++
		}
	}
}

// Nibbelise converts a hex string into nibbles. Least significant at [0]
func nibbleise(in string) []string {
	var out []string
	for k := range in {
		out = append(out, string(in[len(in)-1-k]))
	}

	return out
}
