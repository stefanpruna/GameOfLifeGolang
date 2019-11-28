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

		workerChannels[i].distributorInput = make(chan int, 1)
		workerChannels[i].distributorOutput = make(chan int, 1)
	}

	for i := 0; i < threadsLarge; i++ {
		workerChannels[i+threadsSmall].inputByte = make(chan byte, threadsLargeHeight+2)
		workerChannels[i+threadsSmall].outputByte = make(chan byte, threadsLargeHeight*p.imageWidth)

		workerChannels[i+threadsSmall].distributorInput = make(chan int, 1)
		workerChannels[i+threadsSmall].distributorOutput = make(chan int, 1)
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

func startWorkers(conn net.Conn, initP initPackage, workerP []workerPackage) {
	encoder := gob.NewEncoder(conn)

	// The next packet is an init package
	_ = encoder.Encode(INIT)

	// Send the init package
	err := encoder.Encode(initP)
	fmt.Println(initP)

	// Send worker packages
	for _, p := range workerP {
		fmt.Println(p)
		_ = encoder.Encode(p)
	}

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

	workerBounds := make([]workerPackage, p.threads)
	t := 0

	// Copy of world, but with extra 2 lines (one at the start, one at the end)
	borderedWorld := make([][]byte, p.imageHeight+2)
	for i := range world {
		borderedWorld[i+1] = world[i]
	}
	borderedWorld[0] = world[p.imageHeight-1]
	borderedWorld[p.imageHeight+1] = world[0]

	// start workers
	for i := 0; i < threadsSmall; i++ {
		workerBounds[t] = workerPackage{
			threadsSmallHeight * i,
			threadsSmallHeight * (i + 1),
			borderedWorld[(threadsSmallHeight)*i : (threadsSmallHeight)*(i+1)+2]}
		t++
	}
	for i := 0; i < threadsLarge; i++ {
		workerBounds[t] = workerPackage{
			threadsSmallHeight*threadsSmall + threadsLargeHeight*i,
			threadsSmallHeight*threadsSmall + threadsLargeHeight*(i+1),
			borderedWorld[threadsSmallHeight*threadsSmall+threadsLargeHeight*i : threadsSmallHeight*threadsSmall+threadsLargeHeight*(i+1)+2]}
		t++
	}

	t = 0
	// Start workers on remote machines
	for i := 0; i < clientNumber; i++ {
		host0, _, _ := net.SplitHostPort(clients[positiveModulo(i-1, clientNumber)].RemoteAddr().String())
		host1, _, _ := net.SplitHostPort(clients[positiveModulo(i+1, clientNumber)].RemoteAddr().String())
		if i < clientSmall {
			startWorkers(clients[i], initPackage{clientSmallWorkers, host0, host1, p.turns, p.imageWidth}, workerBounds[t:t+clientSmallWorkers])
			t += clientSmallWorkers
		} else {
			startWorkers(clients[i], initPackage{clientLargeWorkers, host0, host1, p.turns, p.imageWidth}, workerBounds[t:t+clientLargeWorkers])
			t += clientLargeWorkers
		}
	}

	// send data to workers
	for t := 0; t < threadsSmall; t++ {
		startX := threadsSmallHeight * t
		endX := threadsSmallHeight * (t + 1)

		for i := startX - 1; i < endX+1; i++ {
			for j := 0; j < p.imageWidth; j++ {
				workerChannels[t].inputByte <- world[positiveModulo(i, p.imageHeight)][j]
			}
		}
	}
	for t := 0; t < threadsLarge; t++ {
		startX := threadsSmallHeight*threadsSmall + threadsLargeHeight*t
		endX := threadsSmallHeight*threadsSmall + threadsLargeHeight*(t+1)

		for i := startX - 1; i < endX+1; i++ {
			for j := 0; j < p.imageWidth; j++ {
				workerChannels[t+threadsSmall].inputByte <- world[positiveModulo(i, p.imageHeight)][j]
			}
		}
	}

	// main worker controller function
	//workerController(p, world, workerChannels, d, keyChan, threadsSmall, threadsSmallHeight, threadsLarge, threadsLargeHeight)

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
