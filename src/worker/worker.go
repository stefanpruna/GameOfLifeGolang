package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
)

const hostname = "localhost:"

const (
	INIT = 0
)

func intToBytes(x int) []byte {
	var result = make([]byte, 4)
	result[0] = byte(x & 0xFF)
	result[1] = byte((x >> 8) & 0xFF)
	result[2] = byte((x >> 16) & 0xFF)
	result[3] = byte((x >> 24) & 0xFF)
	return result
}

func bytesToInt(b []byte) int {
	return (int(b[3]) << 24) | (int(b[2]) << 16) | (int(b[1]) << 8) | int(b[0])
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

	c := bufio.NewReader(conn)

	const clientNumber = 2
	//var clients = make([]net.Conn, clientNumber)

	for {
		var lenBuff = make([]byte, 4)
		_, err := io.ReadFull(c, lenBuff)

		if err != nil {
			fmt.Println("error:", err)
			break
		}

		packetLen := bytesToInt(lenBuff)

		fmt.Println("len:", packetLen)

		packetType, err := c.ReadByte()

		if packetType == INIT {
			buff := make([]byte, packetLen-1)
			_, err := io.ReadFull(c, buff)

			if err != nil {
				fmt.Println("error:", err)
				break
			}

			lenIpBefore := buff[0]

			var ipBefore = make([]byte, lenIpBefore)
			copy(ipBefore, buff[1:1+lenIpBefore])

			lenIpAfter := buff[1+lenIpBefore]

			var ipAfter = make([]byte, lenIpAfter)
			copy(ipAfter, buff[1+lenIpBefore+1:1+lenIpBefore+1+lenIpAfter])

			fmt.Println(string(ipBefore), string(ipAfter))
		}
	}
}
