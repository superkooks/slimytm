package main

import (
	"context"
	"net"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/text/encoding/charmap"
)

// player represents a squeezebox device
type player interface {
	GetID() string
	GetModel() string
	GetName() string
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

var metricConnectedPlayers = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "slimytm_connected_players",
	Help: "The number of players currently connected",
})

var metricLoadTime = promauto.NewHistogram(prometheus.HistogramOpts{
	Name:    "slimytm_load_time_seconds",
	Help:    "How long it takes to go from the command for next song to playing audio",
	Buckets: prometheus.LinearBuckets(0, 1.5, 15),
})

var metricPacketsTx = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "slimytm_packets_tx_total",
	Help: "The total number of packets sent",
}, []string{"player"})

var metricPacketsRx = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "slimytm_packets_rx_total",
	Help: "The total number of packets received",
}, []string{"player"})

func tcpListener() {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{Port: 3483})
	if err != nil {
		logger.Panicw("unable to start tcp listener",
			"port", 3483,
			"err", err)
	}

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			logger.Errorw("unable to accept tcp connection",
				"err", err)
			continue
		}

		logger.Debug("received new tcp connection")

		b := make([]byte, 1024)
		_, err = conn.Read(b)
		if err != nil {
			logger.Errorw("unable to read from connection",
				"err", err)
			continue
		}

		if string(b[:4]) != "HELO" {
			logger.DPanic("didn't receive a HELO")
		}

		logger.Debug("squeezebox says HELO!")

		var c player
		queue := &Queue{
			Buffer: new(audioBufferWrapper),
		}

		if b[8] == 2 {
			c = &squeezebox1{conn: conn, Queue: queue, mac: net.HardwareAddr(b[10:16])}
		} else if b[8] == 4 {
			c = &squeezebox2{conn: conn, Queue: queue, mac: net.HardwareAddr(b[10:16])}
		} else {
			logger.Warnw("non-squeebox device tried to connect. pretending it is a sbox2")
			c = &squeezebox2{conn: conn, Queue: queue, mac: net.HardwareAddr(b[10:16])}
			// continue
		}

		logger.Infow("connected to a new squeezebox",
			"assignedModel", c.GetModel(),
			"firmware", b[9],
			"mac", net.HardwareAddr(b[10:16]).String())

		queue.Player = c
		queues = append(queues, queue)

		go c.Listener()
		go c.Heartbeat()
		go queue.Watch()

		metricConnectedPlayers.Inc()
	}
}

func udpListener() {
	listener, err := net.ListenUDP("udp", &net.UDPAddr{Port: 3483})
	if err != nil {
		logger.Panicw("unable to start udp listener",
			"port", 3483,
			"err", err)
	}

	for {
		b := make([]byte, 1024)
		_, remote, err := listener.ReadFromUDP(b)
		if err != nil {
			logger.Errorw("unable to read udp packet",
				"err", err)
			return
		}

		if b[0] != 'd' {
			logger.Info("received non-discovery request")
			continue
		}

		logger.Debugw("responding to discovery request",
			"from", remote.String())
		encoder := charmap.ISO8859_10.NewEncoder()
		resp, err := encoder.String("SlimYTM")
		if err != nil {
			logger.DPanicw("unable to encode text",
				"err", err)
		}
		resp = "D" + resp

		listener.WriteTo(append([]byte(resp), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0), remote)
	}

}
