package main

import (
	"encoding/gob"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

type workerData struct {
	outputWorld       chan [][]byte
	distributorOutput chan int
	encoder           *gob.Encoder
	index             int
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
func initialiseChannels(workerChannels []workerData, threadsSmall, threadsSmallHeight, threadsLarge, threadsLargeHeight int, p golParams) {
	for i := 0; i < threadsSmall; i++ {
		workerChannels[i].outputWorld = make(chan [][]byte, 1)

		workerChannels[i].distributorOutput = make(chan int, 1)
	}

	for i := 0; i < threadsLarge; i++ {
		workerChannels[i+threadsSmall].outputWorld = make(chan [][]byte, 1)

		workerChannels[i+threadsSmall].distributorOutput = make(chan int, 1)
	}
}

type controllerData struct {
	Index, Data int
}

func encodeData(worker workerData, data int) {
	p := controllerData{worker.index, data}
	err := worker.encoder.Encode(&p)
	if err != nil {
		fmt.Println(err)
	}
}

type clientEncDec struct {
	encoder *gob.Encoder
	decoder *gob.Decoder
}

func workerController(p golParams, world [][]byte, workerData []workerData, d distributorChans, keyChan <-chan rune, threadsSmall, threadsSmallHeight, threadsLarge, threadsLargeHeight int) {
	stopAtTurn := 0
	paused := false
	timer := time.NewTimer(1111 * time.Second)
	for q := false; q != true; {
		select {
		case <-timer.C:
			// Get alive cells
			alive := 0

			if !paused {
				for i := 0; i < p.threads; i++ {
					encodeData(workerData[i], pause)
				}
				for i := 0; i < p.threads; i++ {
					t := <-workerData[i].distributorOutput
					if t > stopAtTurn {
						stopAtTurn = t
					}
				}
				// Tell all workers to stop after turn stopAtTurn
				for i := 0; i < p.threads; i++ {
					encodeData(workerData[i], stopAtTurn)
				}
				for i := 0; i < p.threads; i++ {
					r := <-workerData[i].distributorOutput
					if r != pause {
						fmt.Println("Something has gone wrong, r =", r)
					}
				}
				for i := 0; i < p.threads; i++ {
					encodeData(workerData[i], ping)
				}
				for i := 0; i < p.threads; i++ {
					alive += <-workerData[i].distributorOutput
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
						fmt.Println(i)
						encodeData(workerData[i], pause)
					}
					// Compute turn to be stopped after
					for i := 0; i < p.threads; i++ {
						t := <-workerData[i].distributorOutput
						if t > stopAtTurn {
							stopAtTurn = t
						}
					}
					// Tell all workers to stop after turn stopAtTurn
					for i := 0; i < p.threads; i++ {
						encodeData(workerData[i], stopAtTurn)
					}
					for i := 0; i < p.threads; i++ {
						r := <-workerData[i].distributorOutput
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
						encodeData(workerData[i], resume)
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
						encodeData(workerData[i], save)
					}
					// If not saving while already paused
					if !paused {
						for i := 0; i < p.threads; i++ {
							encodeData(workerData[i], resume)
						}
					}
					// Get the world and save it
					for t := 0; t < threadsSmall; t++ {
						tw := <-workerData[t].outputWorld
						startX := threadsSmallHeight * t
						for x := 0; x < threadsSmallHeight; x++ {
							for y := 0; y < p.imageWidth; y++ {
								world[x+startX][y] = tw[x][y]
							}
						}
					}
					for t := 0; t < threadsLarge; t++ {
						tw := <-workerData[t+threadsSmall].outputWorld
						startX := threadsSmallHeight*threadsSmall + threadsLargeHeight*t
						for x := 0; x < threadsLargeHeight; x++ {
							for y := 0; y < p.imageWidth; y++ {
								world[x+startX][y] = tw[x][y]
							}
						}
					}
					outputWorld(p, stopAtTurn, d, world)

					// Quit workers
					if k == 'q' {
						for i := 0; i < p.threads; i++ {
							encodeData(workerData[i], quit)
							q = true
						}
					}
				}
				// If this was a pause command, actually pause
				if k == 'p' {
					paused = !paused
				}
			}
		case o := <-workerData[0].distributorOutput:
			fmt.Println("received quit signal!!!!!")
			if o != -1 {
				fmt.Println("Something has gone wrong, o =", o)
			}
			for i := 1; i < p.threads; i++ {
				<-workerData[i].distributorOutput
			}
			// Get the world and save it
			for t := 0; t < threadsSmall; t++ {
				tw := <-workerData[t].outputWorld
				startX := threadsSmallHeight * t
				for x := 0; x < threadsSmallHeight; x++ {
					for y := 0; y < p.imageWidth; y++ {
						world[x+startX][y] = tw[x][y]
					}
				}
			}
			for t := 0; t < threadsLarge; t++ {
				tw := <-workerData[t+threadsSmall].outputWorld
				startX := threadsSmallHeight*threadsSmall + threadsLargeHeight*t
				for x := 0; x < threadsLargeHeight; x++ {
					for y := 0; y < p.imageWidth; y++ {
						world[x+startX][y] = tw[x][y]
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
	Index  int
}

type distributorPackage struct {
	Index       int
	Type        int
	Data        int
	OutputWorld [][]byte
}

func listenToWorker(decoder *gob.Decoder, channel []workerData) {
	for {
		var p distributorPackage
		err := decoder.Decode(&p)
		if err != nil {
			fmt.Println("Err", err)
		}

		if p.Type == 1 {
			channel[p.Index].outputWorld <- p.OutputWorld
			fmt.Println("Received world from worker")
		} else {
			channel[p.Index].distributorOutput <- p.Data
			fmt.Println("Received data from worker")
		}
	}
}

func startWorkers(client clientEncDec, initP initPackage, workerP []workerPackage, workerData []workerData) {
	// The next packet is an init package
	err := client.encoder.Encode(INIT)
	if err != nil {
		fmt.Println("Err", err)
	}

	// Send the init package
	err = client.encoder.Encode(initP)

	// Send worker packages
	for i, p := range workerP {
		workerData[i].encoder = client.encoder
		workerData[i].index = i
		_ = client.encoder.Encode(p)
	}

	if err != nil {
		fmt.Println("Err", err)
	}
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
	workerData := make([]workerData, p.threads)
	initialiseChannels(workerData, threadsSmall, threadsSmallHeight, threadsLarge, threadsLargeHeight, p)

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
			borderedWorld[(threadsSmallHeight)*i : (threadsSmallHeight)*(i+1)+2],
			t,
		}
		t++
	}
	for i := 0; i < threadsLarge; i++ {
		workerBounds[t] = workerPackage{
			threadsSmallHeight*threadsSmall + threadsLargeHeight*i,
			threadsSmallHeight*threadsSmall + threadsLargeHeight*(i+1),
			borderedWorld[threadsSmallHeight*threadsSmall+threadsLargeHeight*i : threadsSmallHeight*threadsSmall+threadsLargeHeight*(i+1)+2],
			t,
		}
		t++
	}
	fmt.Println("aaa")

	clientsGOB := make([]clientEncDec, clientNumber)
	for i := 0; i < clientNumber; i++ {
		clientsGOB[i].encoder = gob.NewEncoder(clients[i])
		clientsGOB[i].decoder = gob.NewDecoder(clients[i])
	}

	t = 0
	// Start workers on remote machines
	for i := 0; i < clientNumber; i++ {
		host0, _, _ := net.SplitHostPort(clients[positiveModulo(i-1, clientNumber)].RemoteAddr().String())
		host1, _, _ := net.SplitHostPort(clients[positiveModulo(i+1, clientNumber)].RemoteAddr().String())
		if i < clientSmall {
			fmt.Println(clientSmallWorkers, "workers started on client", i)
			go startWorkers(clientsGOB[i], initPackage{clientSmallWorkers, host0, host1, p.turns, p.imageWidth},
				workerBounds[t:t+clientSmallWorkers], workerData[t:t+clientSmallWorkers])
			t += clientSmallWorkers
		} else {
			fmt.Println(clientLargeWorkers, "workers started on client", i)
			go startWorkers(clientsGOB[i], initPackage{clientLargeWorkers, host0, host1, p.turns, p.imageWidth},
				workerBounds[t:t+clientLargeWorkers], workerData[t:t+clientSmallWorkers])
			t += clientLargeWorkers
		}
	}

	for i := 0; i < clientNumber; i++ {
		var p int
		err := clientsGOB[i].decoder.Decode(&p)

		if err != nil {
			fmt.Println(err)
		}

		if p != 1 {
			fmt.Println("Ready package mismatch")
		}

		go listenToWorker(clientsGOB[i].decoder, workerData)
	}
	for i := 0; i < clientNumber; i++ {
		p := 1
		err := clientsGOB[i].encoder.Encode(&p)

		if err != nil {
			fmt.Println(err)
		}
	}

	fmt.Println("before controller")
	// main worker controller function
	workerController(p, world, workerData, d, keyChan, threadsSmall, threadsSmallHeight, threadsLarge, threadsLargeHeight)
	fmt.Println("WorkerController ended")

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

	outputWorld(p, p.turns, d, world)

	// Make sure that the Io has finished any output before exiting.
	d.io.command <- ioCheckIdle
	<-d.io.idle
	// Return the coordinates of cells that are still alive.
	alive <- finalAlive
}
