package main

import (
	"encoding/gob"
	"fmt"
	"net"
)

const hostname = "127.0.0.1:"

const (
	INIT     = 0
	INITDATA = 1
)

const (
	pause  = iota
	ping   = iota
	resume = iota
	quit   = iota
	save   = iota
)

type initPackage struct {
	Workers           int
	IpBefore, IpAfter string
	Turns             int
	Width             int
}

type workerPackage struct {
	StartX int
	EndX   int
	World  [][]byte
}

type workerChannel struct {
	inputByte,
	outputByte chan byte
	inputHalo         [2]chan byte
	outputHalo        [2]chan byte
	distributorInput  chan int
	distributorOutput chan int
}

func positiveModulo(x, m int) int {
	if x > 0 {
		return x % m
	} else {
		for x < 0 {
			x += m
		}
		return x % m
	}
}

// Return the number of alive neighbours
func getAliveNeighbours(world [][]byte, x, y, imageWidth int) int {
	aliveNeighbours := 0

	dx := [8]int{-1, -1, 0, 1, 1, 1, 0, -1}
	dy := [8]int{0, 1, 1, 1, 0, -1, -1, -1}

	for i := 0; i < 8; i++ {
		newX := x + dx[i]
		newY := positiveModulo(y+dy[i], imageWidth)
		if world[newX][newY] == 0xFF {
			aliveNeighbours++
		}
	}
	return aliveNeighbours
}

// Returns the new state of a cell from the number of alive neighbours and current state
func getNewState(numberOfAlive int, cellState bool) int {
	if cellState == true {
		if numberOfAlive < 2 {
			return -1
		}
		if numberOfAlive <= 3 {
			return 0
		}
		if numberOfAlive > 3 {
			return -1
		}
	} else {
		if numberOfAlive == 3 {
			return 1
		}
	}
	return 0
}

func worker(imageWidth int, turns int, channels workerChannel, startX, endX, startY, endY int) {

	world := make([][]byte, endX-startX+2)
	for i := range world {
		world[i] = make([]byte, endY-startY)
	}

	newWorld := make([][]byte, endX-startX+2)
	for i := range world {
		newWorld[i] = make([]byte, endY-startY)
	}

	for i := range world {
		for j := 0; j < imageWidth; j++ {
			newWorld[i][j] = <-channels.inputByte
		}
	}

	for i := range world {
		for j := range world[i] {
			world[i][j] = newWorld[i][j]
		}
	}

	halo0 := true
	halo1 := true
	stopAtTurn := -2

	for turn := 0; turn < turns; {

		if turn == stopAtTurn+1 {
			channels.distributorOutput <- pause
			for {
				r := <-channels.distributorInput
				if r == resume {
					break
				} else if r == save {
					for i := 1; i < endX-startX+1; i++ {
						for j := startY; j < endY; j++ {
							channels.outputByte <- newWorld[i][j]
						}
					}
				} else if r == quit {
					return
				} else if r == ping {
					alive := 0
					for i := 1; i < endX-startX+1; i++ {
						for j := startY; j < endY; j++ {
							if newWorld[i][j] == 0xFF {
								alive++
							}
						}
					}
					channels.distributorOutput <- alive
					break
				} else {
					fmt.Println("Something went wrong, r = ", r)
				}
			}
		}

		// Process something
		if turn != 0 {
			if !halo0 {
				select {
				case c := <-channels.inputHalo[0]:
					world[0][0] = c
					for j := 1; j < imageWidth; j++ {
						world[0][j] = <-channels.inputHalo[0]
					}
					halo0 = true
				case <-channels.distributorInput:
					channels.distributorOutput <- turn
					stopAtTurn = <-channels.distributorInput
				}
			}
			if !halo1 {
				select {
				case c := <-channels.inputHalo[1]:
					world[endX-startX+1][0] = c
					for j := 1; j < imageWidth; j++ {
						world[endX-startX+1][j] = <-channels.inputHalo[1]
					}
					halo1 = true
				case <-channels.distributorInput:
					channels.distributorOutput <- turn
					stopAtTurn = <-channels.distributorInput
				}
			}
		}

		// Move on to next turn
		if halo0 && halo1 {

			for i := 1; i < endX-startX+1; i++ {
				for j := startY; j < endY; j++ {
					switch getNewState(getAliveNeighbours(world, i, j, imageWidth), world[i][j] == 0xFF) {
					case -1:
						newWorld[i][j] = 0x00
					case 1:
						newWorld[i][j] = 0xFF
					case 0:
						newWorld[i][j] = world[i][j]
					}
				}
			}
			halo0 = false
			halo1 = false
			turn++

			out0 := false
			out1 := false
			for !(out0 && out1) {
				if !out0 {
					select {
					case channels.outputHalo[0] <- newWorld[1][0]:
						for j := 1; j < imageWidth; j++ {
							channels.outputHalo[0] <- newWorld[1][j]
						}
						out0 = true
					case <-channels.distributorInput:
						channels.distributorOutput <- turn
						stopAtTurn = <-channels.distributorInput
					}
				}
				if !out1 {
					select {
					case channels.outputHalo[1] <- newWorld[endX-startX][0]:
						for j := 1; j < imageWidth; j++ {
							channels.outputHalo[1] <- newWorld[endX-startX][j]
						}
						out1 = true
					case <-channels.distributorInput:
						channels.distributorOutput <- turn
						stopAtTurn = <-channels.distributorInput
					}
				}
			}

			for i := range world {
				for j := range world[i] {
					world[i][j] = newWorld[i][j]
				}
			}
		}

	}

	for i := 1; i < endX-startX+1; i++ {
		for j := startY; j < endY; j++ {
			channels.outputByte <- newWorld[i][j]
		}
	}

	// Done
	channels.distributorOutput <- -1

}

func initialiseChannels(workerChannels []workerChannel, workers, imageWidth, endX, startX, i int) {
	height := endX - startX + 1

	workerChannels[i].inputByte = make(chan byte, height+2)
	workerChannels[i].outputByte = make(chan byte, height*imageWidth)
	workerChannels[i].inputHalo[0] = make(chan byte, imageWidth)
	workerChannels[i].inputHalo[1] = make(chan byte, imageWidth)
	workerChannels[i].distributorInput = make(chan int, 1)
	workerChannels[i].distributorOutput = make(chan int, 1)
	if i == 0 {
		workerChannels[0].outputHalo[0] = make(chan byte, imageWidth)
	} else {
		if i == workers-1 {
			workerChannels[workers-1].outputHalo[1] = make(chan byte, imageWidth)
		} else {
			workerChannels[i-1].outputHalo[1] = workerChannels[i].inputHalo[0]
			workerChannels[i+1].outputHalo[0] = workerChannels[i].inputHalo[1]
		}

	}

}

func distributor(encoder *gob.Encoder, decoder *gob.Decoder) {
	var haloClients = make([]net.Conn, 2)
	var done = make(chan byte)
	go serve(haloClients, done)

	var p initPackage
	err := decoder.Decode(&p)
	fmt.Println(p)

	if err != nil {
		fmt.Println("err", err)
	}
	workerChannel := make([]workerChannel, p.Workers)
	workerPackages := make([]workerPackage, p.Workers)
	for i := 0; i < p.Workers; i++ {
		var w workerPackage
		err = decoder.Decode(&w)
		fmt.Println(w)
		if err != nil {
			fmt.Println("err", err)
			break
		}
		//
		fmt.Println("Received worker package,", w.StartX, w.EndX)

		workerPackages[i] = w
		initialiseChannels(workerChannel, p.Workers, p.Width, w.EndX, w.StartX, i)
	}

	// Connect to external halo sockets
	listenToSocket(p.IpBefore, workerChannel[0].inputHalo[0], p.Width)
	listenToSocket(p.IpAfter, workerChannel[p.Workers-1].inputHalo[1], p.Width)

	<-done
	ip0, _, _ := net.SplitHostPort(haloClients[0].RemoteAddr().String())
	ip1, _, _ := net.SplitHostPort(haloClients[1].RemoteAddr().String())
	if ip0 == p.IpBefore && ip1 == p.IpAfter {
		sendToSocket(haloClients[0], workerChannel[0].outputHalo[0], p.Width)
		sendToSocket(haloClients[1], workerChannel[p.Workers-1].outputHalo[1], p.Width)
	} else if ip0 == p.IpAfter && ip1 == p.IpBefore {
		sendToSocket(haloClients[1], workerChannel[0].outputHalo[0], p.Width)
		sendToSocket(haloClients[0], workerChannel[p.Workers-1].outputHalo[1], p.Width)
	} else {
		fmt.Println("IPs are mismatched")
	}

	for i := 0; i < p.Workers; i++ {
		worker(p.Width, p.Turns, workerChannel[i], workerPackages[i].StartX, workerPackages[i].EndX, 0, p.Width)
	}

}

func sendToSocket(conn net.Conn, c chan byte, width int) {
	enc := gob.NewEncoder(conn)
	for {
		var haloData = make([]byte, width)

		for i := 0; i < width; i++ {
			haloData[i] = <-c
		}

		err := enc.Encode(haloData)

		if err != nil {
			fmt.Println("err", err)
			break
		}
	}
}

func serve(clients []net.Conn, done chan byte) {
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
	done <- 1
}

func listenToSocket(ip string, c chan byte, width int) {
	conn, _ := net.Dial("tcp", ip+":4001")
	dec := gob.NewDecoder(conn)
	for {
		var haloData = make([]byte, width)
		err := dec.Decode(&haloData)
		if err != nil {
			fmt.Println("err", err)
			break
		}
		for _, b := range haloData {
			c <- b
		}
	}
}

func main() {
	conn, _ := net.Dial("tcp", hostname+"4000")

	dec := gob.NewDecoder(conn)
	enc := gob.NewEncoder(conn)

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

			distributor(enc, dec)

			conn.Close()
		}
	}
}
