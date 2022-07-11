package main

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

type xplMessage struct {
	messageType string
	source      string
	target      string
	schema      string
	body        map[string]string
}

var xplPort *net.UDPConn
var sendAddr *net.UDPAddr

func xplInit() {
	var err error
	sendAddr, err = net.ResolveUDPAddr("udp4", "255.255.255.255:3865")
	if err != nil {
		logger.Panicw("unable to resolve xPL broadcast addr",
			"err", err)
	}

	xplPort, err = net.ListenUDP("udp4", nil)
	if err != nil {
		logger.Panicw("unable to open xPL listen addr",
			"err", err)
	}

	go xplHeartbeat()
}

func sendXPL(m xplMessage, source string) {
	out := compileXPL(m, source)
	xplPort.WriteToUDP([]byte(out), sendAddr)
}

func compileXPL(x xplMessage, source string) string {
	m := fmt.Sprintf(`%v
{
hop=1
source=%v
target=%v
}
%v
{
`, x.messageType, source, x.target, x.schema)

	for k, v := range x.body {
		m += k + "=" + v + "\n"
	}

	return m + "}\n"
}

func parseXPL(msg string) xplMessage {
	x := xplMessage{body: make(map[string]string)}
	split := strings.Split(msg, "\n")

	x.messageType = split[0]
	x.schema = split[6]

	for _, line := range split {
		sub := strings.SplitN(line, "=", 2)
		if len(sub) == 2 {
			x.body[sub[0]] = sub[1]
		}
	}

	x.source = x.body["source"]
	delete(x.body, "source")
	x.target = x.body["target"]
	delete(x.body, "target")
	delete(x.body, "hop")

	return x
}

func xplListener() {
	for {
		b := make([]byte, 1024)
		n, err := xplPort.Read(b)
		if err != nil {
			logger.Errorw("unable to read xPL message",
				"err", err)
			continue
		}

		x := parseXPL(string(b[:n]))
		target := strings.Split(x.target, ".")
		if target[0] == "slimdev-slimserv" {
			if x.schema == "osd.basic" {
				delay, ok := x.body["delay"]
				if !ok {
					delay = "5"
				}

				d, err := strconv.Atoi(delay)
				if err != nil {
					logger.Errorw("unable to convert osd.basic delay to integer",
						"err", err)
					d = 5
				}

				for _, v := range queues {
					if v.Player.GetName() == target[1] {
						cleaned := strings.TrimLeft(x.body["text"], "\\n")
						cleaned = strings.TrimLeft(cleaned, "\n")

						logger.Debugw("received xPL",
							"for", v.Player.GetName())
						ctx, _ := context.WithTimeout(context.Background(), time.Second*time.Duration(d))
						v.Texts = append(v.Texts, text{
							bufs: v.Player.DisplayText(cleaned, ctx),
							ctx:  ctx,
						})

						break
					}
				}
			}
		}
	}
}

func xplHeartbeat() {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		logger.Panicw("unable to get ip interfaces",
			"err", err)
	}

	var addr string
	for _, v := range addrs {
		a := v.(*net.IPNet).IP.To4()

		if a.IsGlobalUnicast() {
			addr = a.String()
			logger.Infow("setting xPL remote-ip",
				"addr", addr)
			break
		}
	}

	port := strings.Split(xplPort.LocalAddr().String(), ":")[1]

	for {
		for _, v := range persistent.Clients {
			sendXPL(xplMessage{
				messageType: "xpl-stat",
				target:      "*",
				schema:      "hbeat.app",
				body: map[string]string{
					"interval":  "1",
					"port":      port,
					"remote-ip": addr,
				},
			}, "slimdev-slimserv."+v.Name)
		}

		time.Sleep(time.Minute)
	}
}
