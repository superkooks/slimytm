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

type squeezebox2 struct {
	Queue *Queue

	conn   *net.TCPConn
	font   psfFont
	volume int
	mac    net.HardwareAddr
}

func (s *squeezebox2) GetID() string {
	return s.mac.String()
}

func (s *squeezebox2) GetModel() string {
	return "Squeezebox 2"
}

func (s *squeezebox2) GetName() string {
	if v, ok := persistent.Clients[s.mac.String()]; ok {
		return v.Name
	}

	return s.mac.String()
}

func (s *squeezebox2) Listener() {
	// Load the font for this player
	f, err := os.Open("ter-132n.psf")
	if err != nil {
		logger.Panicw("unable read font",
			"err", err)
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
			// Client has timed out, remove its queue
			for k, v := range queues {
				if v.Player.GetID() == s.GetID() {
					queues = append(queues[:k], queues[k+1:]...)
				}
			}

			logger.DPanic("player has timed out")
			s.conn.Close()
			metricConnectedPlayers.Dec()
			return
		} else if err != nil {
			logger.Errorw("unable to read from connection",
				"err", err)
			return
		}

		logger.Debugw("new packet",
			"len", n,
			"data", b[:n])

		metricPacketsRx.WithLabelValues(s.GetName()).Inc()

		if string(b[:4]) == "STAT" {
			// Status message from the squeezebox
			if string(b[8:12]) == "STMa" {
				s.Queue.Playing = true
				s.Queue.Loading = false
				s.Queue.UpdateClients()
			}

			s.Queue.ElapsedSecs = int(binary.BigEndian.Uint32(b[45:49]))
			logger.Debugw("STAT",
				"type", string(b[8:12]))

		} else if string(b[:4]) == "IR  " {
			if time.Since(lastIR) < IR_INTERVAL {
				// Prevent duplicate IR commands from ruining our day
				continue
			}

			// IR command from the remote
			irCode := hex.EncodeToString(b[14:18])
			logger.Debugw("ir event",
				"code", irCode)
			sendXPL(xplMessage{
				messageType: "xpl-trig",
				target:      "*",
				schema:      "remote.basic",
				body: map[string]string{
					"keys":   irCode,
					"device": s.GetName(),
					"zone":   "slimserver",
					"power":  "on",
				},
			}, "slimdev-slimserv."+s.GetName())

			if irCode == "7689807f" {
				// Volume UP
				s.SetVolume(s.volume + 5)
				ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
				s.Queue.Texts = append(s.Queue.Texts, text{
					bufs: s.DisplayText(fmt.Sprintf("Volume = %v/100", s.volume), ctx),
					ctx:  ctx,
				})
			} else if irCode == "768900ff" {
				// Volume DOWN
				s.SetVolume(s.volume - 5)
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

func (s *squeezebox2) Heartbeat() {
	for {
		// Send the strm t command to request status
		msg := make([]byte, 2)
		binary.BigEndian.PutUint16(msg, uint16(28))
		msg = append(msg, []byte("strm")...)
		msg = append(msg, 't', '0', 'm', '?', '?', '?', '?', 0, 0, 0, '0', 0, 0, 0, 0, 0, 0, 0, 35, 41, 0, 0, 0, 0)
		logger.Debugw("sending heartbeat",
			"len", len(msg),
			"data", msg)
		_, err := s.conn.Write(msg)
		if err != nil {
			// Client probably dropped conn
			logger.DPanicw("could not send heartbeat",
				"err", err)
			return
		}
		metricPacketsTx.WithLabelValues(s.GetName()).Inc()

		time.Sleep(HEARTBEAT_INTERVAL)
	}
}

func (s *squeezebox2) DisplayClock() chan []byte {
	out := make(chan []byte)

	go func() {
		for {
			buf := make([]byte, 1280)
			h, m, sec := time.Now().Local().Clock()
			for k, v := range fmt.Sprintf("      %02d:%02d:%02d", h, m, sec) {
				// Set each character individually with an offset
				s.setChar(s.font.getChar(int(v)), k*8, buf)
			}
			out <- buf
		}
	}()

	return out
}

// Return a channel of framebuffers, scrolling the text if needed.
func (s *squeezebox2) DisplayText(text string, ctx context.Context) chan []byte {
	out := make(chan []byte)

	if len(text) > 20 {
		// Scroll text across screen
		text += "    "
		variableFrame := make([]byte, 4*16*len(text))
		for k, v := range text {
			s.setChar(s.font.getChar(int(v)), k*8, variableFrame)
		}

		go s.scrollBuffer(variableFrame, ctx, out)
	} else {
		buf := make([]byte, 1280)
		for k, v := range text {
			// Set each character individually with an offset
			s.setChar(s.font.getChar(int(v)), k*8, buf)
		}

		go func() {
			// Output the framebuffer until the context expires
			for {
				select {
				case <-ctx.Done():
					return
				case out <- buf:
				}
			}
		}()
	}

	return out
}

func (s *squeezebox2) Play(videoID string) (cancel func()) {
	start := time.Now()

	// Get the player URL with youtube-dl
	co := exec.Command("youtube-dl", "https://music.youtube.com/watch?v="+videoID, "-f", "bestaudio[ext=webm]", "-g")
	logger.Debugw("getting audio download url",
		"cmd", co.String())
	b, err := co.CombinedOutput()
	logger.Debugw("youtube-dl command output", "output", string(b))
	if err != nil {
		logger.Errorw("unable to get audio download url",
			"err", err)
		return
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
		logger.Errorw("unable to start ffmpeg stream",
			"err", err)
		return
	}

	// Wait until with have at least AUDIO_PRELOAD seconds of audio in our buffer
	for {
		time.Sleep(50 * time.Millisecond)
		if s.Queue.Buffer.Len() > 48000*2*2*AUDIO_PRELOAD {
			break
		}
	}

	// Send the strm command to the Squeezebox
	header := fmt.Sprintf("GET /player/%v/audio.wav HTTP/1.0\n\n", s.GetID())
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, uint16(28+len(header)))
	msg = append(msg, []byte("strm")...)
	msg = append(msg, 's', '1', 'p', '1', '4', '2', '1', 0xff, 0, 0, '0', 0, 0, 0, 0, 0, 0, 0, 35, 41, 0, 0, 0, 0)
	msg = append(msg, []byte(header)...)
	logger.Debugw("sending play",
		"len", len(msg),
		"data", msg,
		"elapsedMs", time.Since(start)/time.Millisecond)
	s.conn.Write(msg)
	metricPacketsTx.WithLabelValues(s.GetName()).Inc()

	s.Queue.Playing = true
	s.Queue.Loading = false
	s.Queue.UpdateClients()

	metricLoadTime.Observe(float64(time.Since(start)) / float64(time.Second))

	return cancel
}

func (s *squeezebox2) Stop() {
	// Attempts to fix issue where squeezebox won't start next song
	s.stop()
	time.Sleep(100 * time.Millisecond)
	s.stop()
}

func (s *squeezebox2) stop() {
	// Send the strm command to the Squeezebox
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, uint16(28))
	msg = append(msg, []byte("strm")...)
	msg = append(msg, 'q', '1', 'p', '1', '4', '2', '1', 0xff, 0, 0, '0', 0, 0, 0, 0, 0, 0, 0, 35, 41, 0, 0, 0, 0)
	logger.Debugw("sending stop",
		"len", len(msg),
		"data", msg)
	s.conn.Write(msg)
	metricPacketsTx.WithLabelValues(s.GetName()).Inc()
}

func (s *squeezebox2) Pause() {
	// Send the strm command to the Squeezebox
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, uint16(28))
	msg = append(msg, []byte("strm")...)
	msg = append(msg, 'p', '0', 'm', '?', '?', '?', '?', 0, 0, 0, '0', 0, 0, 0, 0, 0, 0, 0, 35, 41, 0, 0, 0, 0)
	logger.Debugw("sending pause",
		"len", len(msg),
		"data", msg)
	s.conn.Write(msg)
	metricPacketsTx.WithLabelValues(s.GetName()).Inc()
}

func (s *squeezebox2) Unpause() {
	// Send the strm command to the Squeezebox
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, uint16(28))
	msg = append(msg, []byte("strm")...)
	msg = append(msg, 'u', '0', 'm', '?', '?', '?', '?', 0, 0, 0, '0', 0, 0, 0, 0, 0, 0, 0, 35, 41, 0, 0, 0, 0)
	logger.Debugw("sending unpause",
		"len", len(msg),
		"data", msg)
	s.conn.Write(msg)
	metricPacketsTx.WithLabelValues(s.GetName()).Inc()
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
	logger.Debugw("sending volume",
		"len", len(msg),
		"data", msg)
	s.conn.Write(msg)
	metricPacketsTx.WithLabelValues(s.GetName()).Inc()

	s.volume = volume
}

func (s *squeezebox2) GetVolume() int {
	return s.volume
}

func (s *squeezebox2) Render(buf []byte) {
	if len(buf) != 1280 {
		logger.Panic("framebuffer has incorrect length")
	}

	// Send the current framebuffer to the Squeezebox
	msg := make([]byte, 2)
	binary.BigEndian.PutUint16(msg, 1288)
	msg = append(msg, []byte("grfe")...)
	msg = append(msg, 0, 0, 'c', 'c')
	msg = append(msg, buf...)
	s.conn.Write(msg)
	metricPacketsTx.WithLabelValues(s.GetName()).Inc()
}

// Displays the whole buffer forever until it cancelled
func (s *squeezebox2) scrollBuffer(varBuffer []byte, ctx context.Context, out chan []byte) {
	for {
		// Wait until we are being composited before starting the timer
		out <- varBuffer[:1280]
		out <- varBuffer[:1280]
		stationary := time.NewTimer(time.Second * 3)

	outer:
		for {
			// Display the first 40 characters for 3 seconds
			select {
			case out <- varBuffer[:1280]:
			case <-stationary.C:
				break outer
			case <-ctx.Done():
				// Exit if we have been cancelled (by twitter)
				return
			}
		}

		// Scroll text across until we reach the start
		for i := 0; i < len(varBuffer); i += 12 {
			var frame []byte
			if i > len(varBuffer)-1280 {
				// Current frame overlaps end of buffer
				frame = append(varBuffer[i:], varBuffer[0:1280+i-len(varBuffer)]...)
			} else {
				frame = varBuffer[i : i+1280]
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

func (s *squeezebox2) setChar(chr []byte, offset int, buffer []byte) {
	// Form chars into uint16 (font is 16 pix wide)
	var wideChr []uint16
	for i := 0; i < len(chr); i += 2 {
		wideChr = append(wideChr, uint16(chr[i])<<8|uint16(chr[i+1]))
	}

	// Set the chr in the framebuffer
	i := 4 * offset * 16
	for col := 0; col < 16; col++ {
		for _, v := range wideChr {
			mask := uint16(0b1000000000000000 >> col)
			if v&mask > 0 {
				squeezeMask := byte(1 << (7 - i%8))
				buffer[i/8] |= squeezeMask
			}

			i++
		}
	}
}
