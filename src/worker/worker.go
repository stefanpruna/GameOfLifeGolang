package main

import (
	"encoding/gob"
	"fmt"
	"net"
)

const hostname = "3.133.100.193:"

const (
	INIT = 0
)

const (
	pause  = iota
	ping   = iota
	resume = iota
	quit   = iota
	save   = iota
)

type initPackage struct {
	Clients           int
	Workers           int
	IpBefore, IpAfter string
	Turns             int
	Width             int
}

type workerPackage struct {
	StartX int
	EndX   int
	World  [][]byte
	Index  int
}

type workerChannel struct {
	inputHalo        [2]chan byte
	outputHalo       [2]chan byte
	distributorInput chan int
	localDistributor chan byte
}

type distributorPackage struct {
	Index       int
	Type        int
	Data        int
	OutputWorld [][]byte
}

type controllerData struct {
	Index, Data int
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

func worker(p initPackage, channels workerChannel, wp workerPackage, encoder *gob.Encoder) {
	endX := wp.EndX
	startX := wp.StartX
	endY := p.Width
	startY := 0

	world := make([][]byte, endX-startX+2)
	for i := range world {
		world[i] = make([]byte, endY-startY)
	}

	newWorld := make([][]byte, endX-startX+2)
	for i := range world {
		newWorld[i] = make([]byte, endY-startY)
	}

	for i := range world {
		for j := 0; j < endY; j++ {
			newWorld[i][j] = wp.World[i][j]
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

	for turn := 0; turn < p.Turns; {
		fmt.Println("At turn", turn)

		if turn == stopAtTurn+1 {
			err := encoder.Encode(distributorPackage{
				Index:       wp.Index,
				Type:        0,
				Data:        pause,
				OutputWorld: nil,
			})
			if err != nil {
				fmt.Println("err", err)
			}
			for {
				r := <-channels.distributorInput
				if r == resume {
					break
				} else if r == save {
					err := encoder.Encode(distributorPackage{
						Index:       wp.Index,
						Type:        1,
						Data:        0,
						OutputWorld: newWorld[1 : endX-startX+1],
					})
					if err != nil {
						fmt.Println("err", err)
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
					err := encoder.Encode(distributorPackage{
						Index:       wp.Index,
						Type:        0,
						Data:        alive,
						OutputWorld: nil,
					})
					if err != nil {
						fmt.Println("err", err)
					}
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
					for j := 1; j < endY; j++ {
						world[0][j] = <-channels.inputHalo[0]
					}
					halo0 = true
				case <-channels.distributorInput:
					err := encoder.Encode(distributorPackage{
						Index:       wp.Index,
						Type:        0,
						Data:        turn,
						OutputWorld: nil,
					})
					if err != nil {
						fmt.Println("err", err)
					}
					stopAtTurn = <-channels.distributorInput
				}
			}
			if !halo1 {
				select {
				case c := <-channels.inputHalo[1]:
					world[endX-startX+1][0] = c
					for j := 1; j < endY; j++ {
						world[endX-startX+1][j] = <-channels.inputHalo[1]
					}
					halo1 = true
				case <-channels.distributorInput:
					err := encoder.Encode(distributorPackage{
						Index:       wp.Index,
						Type:        0,
						Data:        turn,
						OutputWorld: nil,
					})
					if err != nil {
						fmt.Println("err", err)
					}
					stopAtTurn = <-channels.distributorInput
				}
			}
		}

		// Move on to next turn
		if halo0 && halo1 {

			for i := 1; i < endX-startX+1; i++ {
				for j := startY; j < endY; j++ {
					switch getNewState(getAliveNeighbours(world, i, j, endY), world[i][j] == 0xFF) {
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
						for j := 1; j < endY; j++ {
							channels.outputHalo[0] <- newWorld[1][j]
						}
						out0 = true
					case <-channels.distributorInput:
						err := encoder.Encode(distributorPackage{
							Index:       wp.Index,
							Type:        0,
							Data:        turn,
							OutputWorld: nil,
						})
						if err != nil {
							fmt.Println("err", err)
						}
						stopAtTurn = <-channels.distributorInput
					}
				}
				if !out1 {
					select {
					case channels.outputHalo[1] <- newWorld[endX-startX][0]:
						for j := 1; j < endY; j++ {
							channels.outputHalo[1] <- newWorld[endX-startX][j]
						}
						out1 = true
					case <-channels.distributorInput:
						err := encoder.Encode(distributorPackage{
							Index:       wp.Index,
							Type:        0,
							Data:        turn,
							OutputWorld: nil,
						})
						if err != nil {
							fmt.Println("err", err)
						}
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

	err := encoder.Encode(distributorPackage{
		Index:       wp.Index,
		Type:        1,
		Data:        0,
		OutputWorld: newWorld[1 : endX-startX+1],
	})
	if err != nil {
		fmt.Println("err", err)
	}

	// Done
	err = encoder.Encode(distributorPackage{
		Index:       wp.Index,
		Type:        -1,
		Data:        -1,
		OutputWorld: nil,
	})
	if err != nil {
		fmt.Println("err", err)
	}
	channels.localDistributor <- 1

}

func initialiseChannels(workerChannels []workerChannel, workers, clients, imageWidth, endX, startX, i int) {
	workerChannels[i].inputHalo[0] = make(chan byte, imageWidth)
	workerChannels[i].inputHalo[1] = make(chan byte, imageWidth)
	workerChannels[i].localDistributor = make(chan byte)
	workerChannels[i].distributorInput = make(chan int, 1)

	if workers == 1 && clients > 1 {
		// Just one client
		workerChannels[0].outputHalo[0] = make(chan byte, imageWidth)
		workerChannels[0].outputHalo[1] = make(chan byte, imageWidth)
	} else if workers == 1 {
		workerChannels[0].outputHalo[0] = workerChannels[0].inputHalo[1]
		workerChannels[0].outputHalo[1] = workerChannels[0].inputHalo[0]
	} else if i == 0 {
		workerChannels[0].outputHalo[0] = make(chan byte, imageWidth)
		workerChannels[i+1].outputHalo[0] = workerChannels[i].inputHalo[1]
	} else {
		if i == workers-1 {
			workerChannels[workers-1].outputHalo[1] = make(chan byte, imageWidth)
			workerChannels[i-1].outputHalo[1] = workerChannels[i].inputHalo[0]
		} else {
			workerChannels[i-1].outputHalo[1] = workerChannels[i].inputHalo[0]
			workerChannels[i+1].outputHalo[0] = workerChannels[i].inputHalo[1]
		}

	}

}

func receiveFromDistributor(decoder *gob.Decoder, channels []workerChannel, exit chan byte) {
	for {
		var p controllerData

		err := decoder.Decode(&p)
		if err != nil {
			fmt.Println("err", err)
			break
		}

		if p.Index == -1 {
			fmt.Println("exited receiveFromDistributor")
			break
		}

		fmt.Println("index:", p.Index)
		channels[p.Index].distributorInput <- p.Data

	}
}

func waitForOtherClients(encoder *gob.Encoder, decoder *gob.Decoder) {
	// This client is ready to receive
	err := encoder.Encode(1)
	if err != nil {
		fmt.Println("waitForOtherClients enc err", err)
	}

	var p int
	err = decoder.Decode(&p)
	// All clients are ready to receive
	if err != nil {
		fmt.Println("waitForOtherClients err", err)
	}
	if p != 1 {
		fmt.Println("Error from distributor, p =", p)
	}

}

func distributor(encoder *gob.Encoder, decoder *gob.Decoder, exitThread []chan byte) {
	var haloClients = make([]net.Conn, 2)
	var done = make(chan net.Listener)

	var initP initPackage
	err := decoder.Decode(&initP)
	if err != nil {
		fmt.Println("initPackage err", err)
	}

	if initP.Clients != 1 {
		go waitForClients(haloClients, done)
	}

	workerChannel := make([]workerChannel, initP.Workers)
	workerPackages := make([]workerPackage, initP.Workers)
	for i := 0; i < initP.Workers; i++ {
		var w workerPackage
		err = decoder.Decode(&w)
		//fmt.Println(w)
		if err != nil {
			fmt.Println("worker for loop err", err)
			break
		}
		//
		//fmt.Println("Received worker package,", w.StartX, w.EndX)

		workerPackages[i] = w
		initialiseChannels(workerChannel, initP.Workers, initP.Clients, initP.Width, w.EndX, w.StartX, i)
	}

	waitForOtherClients(encoder, decoder)

	var ln net.Listener
	// Connect to external halo sockets
	if initP.Clients != 1 {
		go receiveFromClient(initP.IpBefore, workerChannel[0].inputHalo[0], initP.Width, initP.Turns, exitThread[0])
		go receiveFromClient(initP.IpAfter, workerChannel[initP.Workers-1].inputHalo[1], initP.Width, initP.Turns, exitThread[1])

		ln = <-done
	}

	go receiveFromDistributor(decoder, workerChannel, exitThread[2])

	if initP.Clients != 1 {
		ip0, _, _ := net.SplitHostPort(haloClients[0].RemoteAddr().String())
		ip1, _, _ := net.SplitHostPort(haloClients[1].RemoteAddr().String())
		if ip0 == initP.IpBefore && ip1 == initP.IpAfter {
			go serveToClient(haloClients[0], workerChannel[0].outputHalo[0], initP.Width, initP.Turns, exitThread[3])
			go serveToClient(haloClients[1], workerChannel[initP.Workers-1].outputHalo[1], initP.Width, initP.Turns, exitThread[4])
		} else if ip0 == initP.IpAfter && ip1 == initP.IpBefore {
			go serveToClient(haloClients[1], workerChannel[0].outputHalo[0], initP.Width, initP.Turns, exitThread[3])
			go serveToClient(haloClients[0], workerChannel[initP.Workers-1].outputHalo[1], initP.Width, initP.Turns, exitThread[4])
		} else {
			fmt.Println("IPs are mismatched")
		}
	} else {
		// Only local workers
		workerChannel[initP.Workers-1].outputHalo[1] = workerChannel[0].inputHalo[0]
		workerChannel[0].outputHalo[0] = workerChannel[initP.Workers-1].inputHalo[1]
	}

	fmt.Println("workers", initP.Workers)
	for i := 0; i < initP.Workers; i++ {
		go worker(initP, workerChannel[i], workerPackages[i], encoder)
	}

	fmt.Println("All workers active")
	for i := 0; i < initP.Workers; i++ {
		<-workerChannel[i].localDistributor
	}

	if initP.Clients != 1 {
		err = ln.Close()
		if err != nil {
			fmt.Println(err)
		}
	}
	fmt.Println("Done")
}

func serveToClient(conn net.Conn, c chan byte, width int, turns int, exit chan byte) {
	enc := gob.NewEncoder(conn)
	for i := 0; i < turns; i++ {
		var haloData = make([]byte, width)

		for i := 0; i < width; i++ {
			haloData[i] = <-c
		}

		err := enc.Encode(haloData)
		//fmt.Println("Sent halo to socket,", haloData)

		if err != nil {
			fmt.Println("err", err)
			break
		}
	}

	fmt.Println("exited serveToClient")
}

func waitForClients(clients []net.Conn, done chan net.Listener) {
	ln, err := net.Listen("tcp4", ":4001")
	if err != nil {
		fmt.Println("err", err)
	}

	if ln != nil {
		for i := 0; i < 2; i++ {
			conn, _ := ln.Accept()
			clients[i] = conn
		}
	}
	done <- ln
}

func receiveFromClient(ip string, c chan byte, width int, turns int, exit chan byte) {
	conn, err := net.Dial("tcp4", ip+":4001")
	if err != nil {
		fmt.Println("err", err)
	}

	dec := gob.NewDecoder(conn)
	for i := 0; i < turns; i++ {
		var haloData = make([]byte, width)
		err := dec.Decode(&haloData)
		//fmt.Println("Received from socket,", haloData)

		if err != nil {
			fmt.Println("err", err)
			break
		}

		// Take bytes from haloData row slice and put them in the channel
		for _, b := range haloData {
			c <- b
		}
	}

	fmt.Println("exited receiveFromClient")
}

func main() {
	conn, err := net.Dial("tcp4", hostname+"4000")
	if err != nil {
		fmt.Println("Server is offline")
		return
	}
	fmt.Println("Connected to server")

	exitThread := make([]chan byte, 5)
	for i := 0; i < 5; i++ {
		exitThread[i] = make(chan byte, 1)
	}

	dec := gob.NewDecoder(conn)
	enc := gob.NewEncoder(conn)

	i := 0
	for {
		var packetType int = 0
		err := dec.Decode(&packetType)
		if err != nil {
			fmt.Println("err", err)
			break
		}
		fmt.Println("packet type:", packetType)

		if packetType == INIT {
			fmt.Println("Starting distributor..")
			distributor(enc, dec, exitThread)
			for i := 0; i < 5; i++ {
				<-exitThread[i]
			}
			fmt.Println("Ran", i, "times.")
		}
	}
}
