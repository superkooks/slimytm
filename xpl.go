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

const xplSource = "superkooks-slimytm.mandoon"

var xplPort *net.UDPConn

func xplInit() {
	var err error
	xplPort, err = net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		panic(err)
	}

	go xplHeartbeat()
}

func sendXPL(m xplMessage) {
	xplPort.Write([]byte(compileXPL(m)))
}

func compileXPL(x xplMessage) string {
	m := fmt.Sprintf(`%v
{
hop=1
source=%v
target=%v
}
%v
{
`, x.messageType, xplSource, x.target, x.schema)

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
			panic(err)
		}

		x := parseXPL(string(b[:n]))
		if x.schema == "osd.basic" {
			delay, ok := x.body["delay"]
			if !ok {
				delay = "5"
			}

			d, err := strconv.Atoi(delay)
			if err != nil {
				panic("can't convert delay to integer")
			}

			for _, v := range queues {
				ctx, _ := context.WithTimeout(context.Background(), time.Second*time.Duration(d))
				v.Texts = append(v.Texts, text{
					bufs: v.Player.DisplayText(x.body["text"], ctx),
					ctx:  ctx,
				})
			}
		}
	}
}

func xplHeartbeat() {
	addr := strings.Split(xplPort.LocalAddr().String(), ":")

	for {
		sendXPL(xplMessage{
			messageType: "xpl-cmnd",
			target:      "*",
			schema:      "hbeat.app",
			body: map[string]string{
				"interval":  "1",
				"port":      addr[len(addr)-1],
				"remote-ip": strings.Join(addr[:len(addr)-1], ""),
			},
		})
		time.Sleep(time.Minute)
	}
}
