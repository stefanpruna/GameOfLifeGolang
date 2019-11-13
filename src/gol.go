package main

import (
	"encoding/gob"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

type workerChannel struct {
	inputByte,
	outputByte chan byte
	inputHalo         [2]chan byte
	outputHalo        [2]chan byte
	distributorInput  chan int
	distributorOutput chan int
}

const (
	pause  = iota
	ping   = iota
	resume = iota
	quit   = iota
	save   = iota
)

// Modulus that only returns positive number
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

// Worker function
func worker(p golParams, channels workerChannel, startX, endX, startY, endY int) {

	world := make([][]byte, endX-startX+2)
	for i := range world {
		world[i] = make([]byte, endY-startY)
	}

	newWorld := make([][]byte, endX-startX+2)
	for i := range world {
		newWorld[i] = make([]byte, endY-startY)
	}

	for i := range world {
		for j := 0; j < p.imageWidth; j++ {
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

	for turn := 0; turn < p.turns; {

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
					for j := 1; j < p.imageWidth; j++ {
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
					for j := 1; j < p.imageWidth; j++ {
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
					switch getNewState(getAliveNeighbours(world, i, j, p.imageWidth), world[i][j] == 0xFF) {
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
						for j := 1; j < p.imageWidth; j++ {
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
						for j := 1; j < p.imageWidth; j++ {
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

// initialise worker channels
func initialiseChannels(workerChannels []workerChannel, threadsSmall, threadsSmallHeight, threadsLarge, threadsLargeHeight int, p golParams) {
	for i := 0; i < threadsSmall; i++ {
		workerChannels[i].inputByte = make(chan byte, threadsSmallHeight+2)
		workerChannels[i].outputByte = make(chan byte, threadsSmallHeight*p.imageWidth)
		workerChannels[i].inputHalo[0] = make(chan byte, p.imageWidth)
		workerChannels[i].inputHalo[1] = make(chan byte, p.imageWidth)
		workerChannels[i].distributorInput = make(chan int, 1)
		workerChannels[i].distributorOutput = make(chan int, 1)

		workerChannels[positiveModulo(i-1, p.threads)].outputHalo[1] = workerChannels[i].inputHalo[0]
		workerChannels[positiveModulo(i+1, p.threads)].outputHalo[0] = workerChannels[i].inputHalo[1]
	}

	for i := 0; i < threadsLarge; i++ {
		workerChannels[i+threadsSmall].inputByte = make(chan byte, threadsLargeHeight+2)
		workerChannels[i+threadsSmall].outputByte = make(chan byte, threadsLargeHeight*p.imageWidth)
		workerChannels[i+threadsSmall].inputHalo[0] = make(chan byte, p.imageWidth)
		workerChannels[i+threadsSmall].inputHalo[1] = make(chan byte, p.imageWidth)
		workerChannels[i+threadsSmall].distributorInput = make(chan int, 1)
		workerChannels[i+threadsSmall].distributorOutput = make(chan int, 1)

		workerChannels[positiveModulo(i+threadsSmall-1, p.threads)].outputHalo[1] = workerChannels[i+threadsSmall].inputHalo[0]
		workerChannels[positiveModulo(i+threadsSmall+1, p.threads)].outputHalo[0] = workerChannels[i+threadsSmall].inputHalo[1]
	}
}

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
				for i := 0; i < p.threads; i++ {
					workerChannels[i].distributorInput <- pause
				}
				for i := 0; i < p.threads; i++ {
					t := <-workerChannels[i].distributorOutput
					if t > stopAtTurn {
						stopAtTurn = t
					}
				}
				// Tell all workers to stop after turn stopAtTurn
				for i := 0; i < p.threads; i++ {
					workerChannels[i].distributorInput <- stopAtTurn
				}
				for i := 0; i < p.threads; i++ {
					r := <-workerChannels[i].distributorOutput
					if r != pause {
						fmt.Println("Something has gone wrong, r =", r)
					}
				}
				for i := 0; i < p.threads; i++ {
					workerChannels[i].distributorInput <- ping
				}
				for i := 0; i < p.threads; i++ {
					alive += <-workerChannels[i].distributorOutput
				}
				fmt.Println("There are", alive, "alive cells in the world.")
			}

			timer = time.NewTimer(2 * time.Second)
		case k := <-keyChan:
			if k == 'p' || k == 's' || k == 'q' {
				// If not already paused
				if !paused {
					// Get turn from all workers
					for i := 0; i < p.threads; i++ {
						workerChannels[i].distributorInput <- pause
					}
					// Compute turn to be stopped after
					for i := 0; i < p.threads; i++ {
						t := <-workerChannels[i].distributorOutput
						if t > stopAtTurn {
							stopAtTurn = t
						}
					}
					// Tell all workers to stop after turn stopAtTurn
					for i := 0; i < p.threads; i++ {
						workerChannels[i].distributorInput <- stopAtTurn
					}
					for i := 0; i < p.threads; i++ {
						r := <-workerChannels[i].distributorOutput
						if r != pause {
							fmt.Println("Something has gone wrong, r =", r)
						}
					}
					// Paused until resume
					if k == 'p' {
						fmt.Println("Pausing. The turn number", stopAtTurn+1, "is currently being processed.")
					}
				} else if k == 'p' { // If this was a pause command and we are already paused, resume
					// Resume all workers
					for i := 0; i < p.threads; i++ {
						workerChannels[i].distributorInput <- resume
					}
					fmt.Println("Continuing.")
				}
				// If this was a save or quit command
				if k == 's' || k == 'q' {
					if k == 's' {
						fmt.Println("Saving on turn", stopAtTurn)
					} else {
						fmt.Println("Saving and quitting on turn", stopAtTurn)
					}
					for i := 0; i < p.threads; i++ {
						workerChannels[i].distributorInput <- save
					}
					// If not saving while already paused
					if !paused {
						for i := 0; i < p.threads; i++ {
							workerChannels[i].distributorInput <- resume
						}
					}
					// Get the world and save it
					for t := 0; t < threadsSmall; t++ {
						startX := threadsSmallHeight * t
						for x := 0; x < threadsSmallHeight; x++ {
							for y := 0; y < p.imageWidth; y++ {
								world[x+startX][y] = <-workerChannels[t].outputByte
							}
						}
					}
					for t := 0; t < threadsLarge; t++ {
						startX := threadsSmallHeight*threadsSmall + threadsLargeHeight*t
						for x := 0; x < threadsLargeHeight; x++ {
							for y := 0; y < p.imageWidth; y++ {
								world[x+startX][y] = <-workerChannels[t+threadsSmall].outputByte
							}
						}
					}
					outputWorld(p, stopAtTurn, d, world)

					// Quit workers
					if k == 'q' {
						for i := 0; i < p.threads; i++ {
							workerChannels[i].distributorInput <- quit
							q = true
						}
					}
				}
				// If this was a pause command, actually pause
				if k == 'p' {
					paused = !paused
				}
			}
		case o := <-workerChannels[0].distributorOutput:
			if o != -1 {
				fmt.Println("Something has gone wrong, o =", o)
			}
			for i := 1; i < p.threads; i++ {
				<-workerChannels[i].distributorOutput
			}
			// Get the world and save it
			for t := 0; t < threadsSmall; t++ {
				startX := threadsSmallHeight * t
				for x := 0; x < threadsSmallHeight; x++ {
					for y := 0; y < p.imageWidth; y++ {
						world[x+startX][y] = <-workerChannels[t].outputByte
					}
				}
			}
			for t := 0; t < threadsLarge; t++ {
				startX := threadsSmallHeight*threadsSmall + threadsLargeHeight*t
				for x := 0; x < threadsLargeHeight; x++ {
					for y := 0; y < p.imageWidth; y++ {
						world[x+startX][y] = <-workerChannels[t+threadsSmall].outputByte
					}
				}
			}
			q = true
		}
	}
}

type initPackage struct {
	IpBefore, IpAfter string
}

func startWorkers(conn net.Conn, workers int, ipBefore, ipAfter string) {
	encoder := gob.NewEncoder(conn)

	_ = encoder.Encode(INIT)
	err := encoder.Encode(initPackage{ipBefore, ipAfter})
	if err != nil {
		fmt.Println("Err", err)
	}
	fmt.Println("done")

}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p golParams, d distributorChans, alive chan []cell, keyChan <-chan rune) {

	// Create the 2D slice to store the world.
	world := make([][]byte, p.imageHeight)
	for i := range world {
		world[i] = make([]byte, p.imageWidth)
	}

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
	initialiseChannels(workerChannels, threadsSmall, threadsSmallHeight, threadsLarge, threadsLargeHeight, p)

	// Threads per client
	//clientLarge := p.threads % clientNumber
	clientSmall := clientNumber - p.threads%clientNumber

	clientLargeWorkers := p.threads/clientNumber + 1
	clientSmallWorkers := p.threads / clientNumber

	// Start workers on remote machines
	for i := 0; i < clientNumber; i++ {
		host0, _, _ := net.SplitHostPort(clients[positiveModulo(i-1, clientNumber)].RemoteAddr().String())
		host1, _, _ := net.SplitHostPort(clients[positiveModulo(i+1, clientNumber)].RemoteAddr().String())
		if i < clientSmall {
			startWorkers(clients[i], clientSmallWorkers, host0, host1)
		} else {
			startWorkers(clients[i], clientLargeWorkers, host0, host1)
		}
	}

	// start workers
	for i := 0; i < threadsSmall; i++ {
		startX := threadsSmallHeight * i
		endX := threadsSmallHeight * (i + 1)
		go worker(p, workerChannels[i], startX, endX, 0, p.imageWidth)
	}

	for i := 0; i < threadsLarge; i++ {
		startX := threadsSmallHeight*threadsSmall + threadsLargeHeight*i
		endX := threadsSmallHeight*threadsSmall + threadsLargeHeight*(i+1)
		go worker(p, workerChannels[i+threadsSmall], startX, endX, 0, p.imageWidth)
	}

	// send data to workers
	for t := 0; t < threadsSmall; t++ {
		startX := threadsSmallHeight * t
		endX := threadsSmallHeight * (t + 1)

		for i := startX - 1; i < endX+1; i++ {
			for j := 0; j < p.imageWidth; j++ {
				workerChannels[t].inputByte <- world[positiveModulo(i, p.imageHeight)][positiveModulo(j, p.imageWidth)]
			}
		}
	}
	for t := 0; t < threadsLarge; t++ {
		startX := threadsSmallHeight*threadsSmall + threadsLargeHeight*t
		endX := threadsSmallHeight*threadsSmall + threadsLargeHeight*(t+1)

		for i := startX - 1; i < endX+1; i++ {
			for j := 0; j < p.imageWidth; j++ {
				workerChannels[t+threadsSmall].inputByte <- world[positiveModulo(i, p.imageHeight)][positiveModulo(j, p.imageWidth)]
			}
		}
	}

	// main worker controller function
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
