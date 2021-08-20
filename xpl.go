package main

import (
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
	b, err := net.ResolveUDPAddr("udp4", "0.0.0.0:3865")
	if err != nil {
		panic(err)
	}

	xplPort, err = net.ListenUDP("udp", b)
	if err != nil {
		panic(err)
	}
}

func sendXPL(m xplMessage) {
	bAddr, err := net.ResolveUDPAddr("udp4", "192.168.73.255:3865")
	if err != nil {
		panic(err)
	}

	xplPort.WriteTo([]byte(compileXPL(m)), bAddr)
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
			displayAllPlayers(x.body["text"])
			delay, ok := x.body["delay"]
			if !ok {
				delay = "5"
			}

			d, err := strconv.Atoi(delay)
			if err != nil {
				panic("can't convert delay to integer")
			}

			textDelay = time.Second * time.Duration(d)
		}
	}
}
