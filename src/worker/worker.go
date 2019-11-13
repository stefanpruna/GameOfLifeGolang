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
const (
	pause  = iota
	ping   = iota
	resume = iota
	quit   = iota
	save   = iota
)

type initPackage struct {
	workers           int
	IpBefore, IpAfter string
	turns             int
	width             int
}

type workerPackage struct {
	startX int
	endX   int
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

func initialiseChannels(workerChannels []workerChannel, workers, imageWidth, endX, startX int) {
	height := endX - startX + 1
	for i := 0; i < workers; i++ {

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
			for i := 0; i < p.workers; i++ {
				var w initPackage
				err = dec.Decode(&w)
				if err != nil {
					fmt.Println("err", err)
					break
				}
				//initialiseChannels()
				//go worker()
			}
			fmt.Printf("Received : %+v", p)
			conn.Close()
		}
	}
}
