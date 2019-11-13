package main

import (
	"encoding/gob"
	"fmt"
	"net"
)

const hostname = "localhost:"

const (
	INIT = 0
)

type initPackage struct {
	IpBefore, IpAfter string
}

func serve(clients []net.Conn) {
	ln, err := net.Listen("tcp", ":4001")
	if err != nil {
		// handle error
	}

	if ln != nil {
		for i := 0; i < 2; i++ {
			conn, _ := ln.Accept()
			clients[i] = conn
		}
	}
}

func main() {
	conn, _ := net.Dial("tcp", hostname+"4000")
	dec := gob.NewDecoder(conn)

	const clientNumber = 2
	//var clients = make([]net.Conn, clientNumber)

	for {

		var packetType int = 0
		err := dec.Decode(&packetType)
		if err != nil {
			fmt.Println("err", err)
			break
		}
		fmt.Println("packet type:", packetType)

		if packetType == INIT {
			var p initPackage
			err = dec.Decode(&p)
			if err != nil {
				fmt.Println("err", err)
				break
			}
			fmt.Printf("Received : %+v", p)
			conn.Close()
		}
	}
}
