package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type workerChannel struct {
	inputByte,
	outputByte chan byte
	inputHalo,
	outputHalo [2]chan byte
	distributorInput,
	distributorOutput chan int
}

const (
	pause  = iota
	ping   = iota
	resume = iota
	quit   = iota
	save   = iota
)

func positiveModulo(x, m int) int {
	for x < 0 {
		x += m
	}
	return x % m
}

// Returns the new state of a cell from the number of alive neighbours and current state
func getNewState(numberOfAlive int, cellState bool) int {
	if cellState == true {
		if numberOfAlive < 2 || numberOfAlive > 3 {
			return -1
		}
		if numberOfAlive <= 3 {
			return 0
		}
	} else {
		if numberOfAlive == 3 {
			return 1
		}
	}
	return 0
}

// Makes a matrix slice
func makeMatrix(width, height int) [][]byte {
	M := make([][]byte, height)
	for i := range M {
		M[i] = make([]byte, width)
	}
	return M
}

// Receive halo, or receive command from distributor
func receiveOrInterrupt(world [][]byte, channels workerChannel, turn int, halo *bool, stopAtTurn *int, lineToReceive, haloIndex int) {
	select {
	case c := <-channels.inputHalo[haloIndex]:
		length := len(world[lineToReceive])
		world[lineToReceive][0] = c
		for j := 1; j < length; j++ {
			world[lineToReceive][j] = <-channels.inputHalo[haloIndex]
		}
		*halo = true
	case <-channels.distributorInput:
		channels.distributorOutput <- turn
		*stopAtTurn = <-channels.distributorInput
	}
}

// Send halo, or receive command from distributor
func sendOrInterrupt(world [][]byte, channels workerChannel, turn int, out *bool, stopAtTurn *int, lineToSend, haloIndex int) {
	select {
	case channels.outputHalo[haloIndex] <- world[lineToSend][0]:
		length := len(world[lineToSend])
		for j := 1; j < length; j++ {
			channels.outputHalo[haloIndex] <- world[lineToSend][j]
		}
		*out = true
	case <-channels.distributorInput:
		channels.distributorOutput <- turn
		*stopAtTurn = <-channels.distributorInput
	}
}

// Worker function
func worker(p golParams, channels workerChannel, startX, endX, startY, endY int) {

	world := makeMatrix(endY-startY, endX-startX+2)
	newWorld := makeMatrix(endY-startY, endX-startX+2)

	// Receive initial world from distributor
	for i := range world {
		for j := 0; j < p.imageWidth; j++ {
			newWorld[i][j] = <-channels.inputByte
			world[i][j] = newWorld[i][j]
		}
	}

	halo0, halo1 := true, true
	stopAtTurn := -2

	for turn := 0; turn < p.turns; {

		// This is the turn in which all workers synchronise and stop
		if turn == stopAtTurn+1 {
			channels.distributorOutput <- pause
			// Process IO
			for {
				r := <-channels.distributorInput
				if r == resume {
					break
				} else if r == save {
					// Send the world to the distributor
					for i := 1; i < endX-startX+1; i++ {
						for j := startY; j < endY; j++ {
							channels.outputByte <- newWorld[i][j]
						}
					}
				} else if r == quit {
					return
				} else if r == ping {
					// Send the number of alive cells to the distributor
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
				}
			}
		}

		// Get halos or command
		if turn != 0 {
			// Either receive the top halo, or a command from distributor
			if !halo0 {
				receiveOrInterrupt(world, channels, turn, &halo0, &stopAtTurn, 0, 0)
			}
			// Either receive the bottom halo, or a command from distributor
			if !halo1 {
				receiveOrInterrupt(world, channels, turn, &halo1, &stopAtTurn, endX-startX+1, 1)
			}
		}

		// Move on to next turn, if both halos are present
		if halo0 && halo1 {
			// Execute turn
			for i := 1; i < endX-startX+1; i++ {
				for j := startY; j < endY; j++ {
					// Compute alive neighbours
					aliveNeighbours := 0

					yp1 := (j + 1) % p.imageWidth
					ym1 := j - 1
					if ym1 < 0 {
						ym1 += p.imageWidth
					}

					aliveNeighbours = int(world[i+1][j]) + int(world[i-1][j]) +
						int(world[i][yp1]) + int(world[i][ym1]) +
						int(world[i+1][yp1]) + int(world[i+1][ym1]) +
						int(world[i-1][yp1]) + int(world[i-1][ym1])

					switch getNewState(aliveNeighbours/255, world[i][j] == 0xFF) {
					case -1:
						newWorld[i][j] = 0x00
					case 1:
						newWorld[i][j] = 0xFF
					case 0:
						newWorld[i][j] = world[i][j]
					}
				}
			}
			halo0, halo1 = false, false
			turn++

			// Try sending the halos, or a command from distributor
			out0, out1 := false, false
			for !(out0 && out1) {
				if !out0 {
					sendOrInterrupt(newWorld, channels, turn, &out0, &stopAtTurn, 1, 0)
				}
				if !out1 {
					sendOrInterrupt(newWorld, channels, turn, &out1, &stopAtTurn, endX-startX, 1)
				}
			}

			// Update old world
			for i := range world {
				copy(world[i], newWorld[i])
			}
		}
	}

	// Send the world to the distributor
	for i := 1; i < endX-startX+1; i++ {
		for j := startY; j < endY; j++ {
			channels.outputByte <- newWorld[i][j]
		}
	}
	// Done
	channels.distributorOutput <- -1
}

// Sends world to output
func outputWorld(p golParams, state int, d distributorChans, world [][]byte) {
	d.io.command <- ioOutput
	d.io.filename <- strings.Join([]string{strconv.Itoa(p.imageWidth), strconv.Itoa(p.imageHeight)}, "x") + "_state_" + strconv.Itoa(state)
	for i := range world {
		for j := range world[i] {
			d.io.world <- world[i][j]
		}
	}
}

// Initialise worker channels
func initialiseChannels(workerChannels []workerChannel, threadsSmall, threadsSmallHeight, threadsLargeHeight int, p golParams) {
	threadHeight := threadsSmallHeight
	for i := 0; i < p.threads; i++ {
		if i == threadsSmall {
			threadHeight = threadsLargeHeight
		}
		workerChannels[i].inputByte = make(chan byte, threadHeight+2)
		workerChannels[i].outputByte = make(chan byte, threadHeight*p.imageWidth)
		workerChannels[i].inputHalo[0] = make(chan byte, p.imageWidth)
		workerChannels[i].inputHalo[1] = make(chan byte, p.imageWidth)
		workerChannels[i].distributorInput = make(chan int, 1)
		workerChannels[i].distributorOutput = make(chan int, 1)

		// Link channels
		workerChannels[positiveModulo(i-1, p.threads)].outputHalo[1] = workerChannels[i].inputHalo[0]
		workerChannels[positiveModulo(i+1, p.threads)].outputHalo[0] = workerChannels[i].inputHalo[1]
	}
}

func pauseWorkers(workerChannels []workerChannel, stopAtTurn *int) {
	// Pause and get current turns
	for _, channel := range workerChannels {
		channel.distributorInput <- pause
	}
	for _, channel := range workerChannels {
		t := <-channel.distributorOutput
		if t > *stopAtTurn {
			*stopAtTurn = t
		}
	}

	// Tell all workers to stop after turn stopAtTurn
	for _, channel := range workerChannels {
		channel.distributorInput <- *stopAtTurn
	}
	for _, channel := range workerChannels {
		r := <-channel.distributorOutput
		if r != pause {
			fmt.Println("Something has gone wrong, r =", r)
		}
	}
}

func receiveWorld(world [][]byte, workerChannels []workerChannel, threadsSmall, threadsSmallHeight, threadsLargeHeight int) {
	startX := 0
	endX := threadsSmallHeight
	for i, channel := range workerChannels {
		for x := 0; x < endX-startX; x++ {
			length := len(world[x])
			for y := 0; y < length; y++ {
				world[x+startX][y] = <-channel.outputByte
			}
		}
		// New startX, endX
		startX = endX
		if i < threadsSmall-1 {
			endX += threadsSmallHeight
		} else {
			endX += threadsLargeHeight
		}
	}
}

// Sends data over all worker channels
func sendToWorkers(workerChannels []workerChannel, data int) {
	for _, channel := range workerChannels {
		channel.distributorInput <- data
	}
}

// Controls IO
func workerController(p golParams, world [][]byte, workerChannels []workerChannel, d distributorChans, keyChan <-chan rune, threadsSmall, threadsSmallHeight, threadsLarge, threadsLargeHeight int) {
	stopAtTurn := 0
	paused := false
	timer := time.NewTimer(2 * time.Second)
	for q := false; q != true; {
		select {
		case <-timer.C:
			// Get alive cells
			alive := 0
			if !paused {
				pauseWorkers(workerChannels, &stopAtTurn)
				// Ping unpauses workers
				sendToWorkers(workerChannels, ping)
				for _, channel := range workerChannels {
					alive += <-channel.distributorOutput
				}
				fmt.Println("There are", alive, "alive cells in the world.")
			}

			timer = time.NewTimer(2 * time.Second)
		case k := <-keyChan:
			if k == 'p' || k == 's' || k == 'q' {
				// If not already paused
				if !paused {
					pauseWorkers(workerChannels, &stopAtTurn)
					// Paused until resume
					if k == 'p' {
						fmt.Println("Pausing. The turn number", stopAtTurn+1, "is currently being processed.")
					}
				} else if k == 'p' { // If this was a pause command and we are already paused, resume
					// Resume all workers
					sendToWorkers(workerChannels, resume)
					fmt.Println("Continuing.")
				}
				// If this was a save or quit command
				if k == 's' || k == 'q' {
					if k == 's' {
						fmt.Println("Saving on turn", stopAtTurn)
					} else {
						fmt.Println("Saving and quitting on turn", stopAtTurn)
					}
					sendToWorkers(workerChannels, save)

					// If paused just to save, unpause. If quit, don't unpause
					if !paused && k == 's' {
						sendToWorkers(workerChannels, resume)
					}

					// Receive and output world
					receiveWorld(world, workerChannels, threadsSmall, threadsSmallHeight, threadsLargeHeight)
					outputWorld(p, stopAtTurn, d, world)

					// Quit workers
					if k == 'q' {
						q = true
						sendToWorkers(workerChannels, quit)
					}
				}
				// If this was a pause command, actually pause
				if k == 'p' {
					paused = !paused
				}
			}
		case o := <-workerChannels[0].distributorOutput: // Workers are starting to finish
			if o != -1 {
				fmt.Println("Something has gone wrong, o =", o)
			}
			for i := 1; i < p.threads; i++ {
				<-workerChannels[i].distributorOutput
			}
			// Receive the world and quit
			receiveWorld(world, workerChannels, threadsSmall, threadsSmallHeight, threadsLargeHeight)
			q = true
		}
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p golParams, d distributorChans, alive chan []cell, keyChan <-chan rune) {

	// Create the 2D slice to store the world.
	world := makeMatrix(p.imageWidth, p.imageHeight)

	// Request the io goroutine to read in the image with the given filename.
	d.io.command <- ioInput
	d.io.filename <- strings.Join([]string{strconv.Itoa(p.imageWidth), strconv.Itoa(p.imageHeight)}, "x")

	// The io goroutine sends the requested image byte by byte, in rows.
	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			val := <-d.io.inputVal
			if val != 0 {
				fmt.Println("Alive cell at", x, y)
				world[y][x] = val
			}
		}
	}

	// Make channels
	var chans = make([]chan [][]byte, p.threads)
	for i := 0; i < p.threads; i++ {
		chans[i] = make(chan [][]byte)
	}

	// Thread calculations
	// 16x16 with 10 threads: 6 large threads with 2 height + 4 small threads with 1 height
	threadsLarge := p.imageHeight % p.threads
	threadsSmall := p.threads - p.imageHeight%p.threads
	threadsLargeHeight := p.imageHeight/p.threads + 1
	threadsSmallHeight := p.imageHeight / p.threads

	// Worker channels
	workerChannels := make([]workerChannel, p.threads)
	initialiseChannels(workerChannels, threadsSmall, threadsSmallHeight, threadsLargeHeight, p)

	// Start workers
	startX := 0
	endX := threadsSmallHeight
	for i := 0; i < p.threads; i++ {
		go worker(p, workerChannels[i], startX, endX, 0, p.imageWidth)
		// Send initial world to worker
		for x := startX - 1; x < endX+1; x++ {
			for y := 0; y < p.imageWidth; y++ {
				workerChannels[i].inputByte <- world[positiveModulo(x, p.imageHeight)][positiveModulo(y, p.imageWidth)]
			}
		}
		// New startX, endX
		startX = endX
		if i < threadsSmall-1 {
			endX += threadsSmallHeight
		} else {
			endX += threadsLargeHeight
		}
	}

	// Process IO and control workers
	workerController(p, world, workerChannels, d, keyChan, threadsSmall, threadsSmallHeight, threadsLarge, threadsLargeHeight)

	// Create an empty slice to store coordinates of cells that are still alive after p.turns are done.
	var finalAlive []cell
	// Go through the world and append the cells that are still alive.
	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			if world[y][x] != 0 {
				finalAlive = append(finalAlive, cell{x: x, y: y})
			}
		}
	}

	// Make sure that the Io has finished any output before exiting.
	d.io.command <- ioCheckIdle
	<-d.io.idle

	// Return the coordinates of cells that are still alive.
	alive <- finalAlive
}
