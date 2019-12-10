package main

import (
	"encoding/gob"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const INIT = 0

const (
	pause  = iota
	ping   = iota
	resume = iota
	quit   = iota
	save   = iota
)

type workerData struct {
	outputWorld       chan [][]byte
	distributorOutput chan int
	encoder           *gob.Encoder
	index             int
}

// Client information
type clientData struct {
	encoder *gob.Encoder
	decoder *gob.Decoder
	ip      string
}

// User interaction data packet
type controllerPacket struct {
	Index, Data int
}

// Client initialization packet
type initPacket struct {
	Clients           int
	Workers           int
	IpBefore, IpAfter string
	Turns             int
	Width             int
}

// Individual worker initialisation packet
type workerPacket struct {
	StartX int
	EndX   int
	World  [][]byte
	Index  int
}

// General client inbout packet
type distributorPacket struct {
	Index       int
	Type        int
	Data        int
	OutputWorld [][]byte
}

func try(error error) {
	if error != nil {
		fmt.Println("Err", error)
	}
}

func positiveModulo(x, m int) int {
	for x < 0 {
		x += m
	}
	return x % m
}

// Makes a matrix slice
func makeMatrix(width, height int) [][]byte {
	M := make([][]byte, height)
	for i := range M {
		M[i] = make([]byte, width)
	}
	return M
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

// Pauses and synchronises all workers
func pauseWorkers(workerData []workerData, stopAtTurn *int) {
	// Pause and get current turns
	for _, worker := range workerData {
		try(worker.encoder.Encode(controllerPacket{worker.index, pause}))
	}
	for _, worker := range workerData {
		t := <-worker.distributorOutput
		if t > *stopAtTurn {
			*stopAtTurn = t
		}
	}

	// Tell all workers to stop after turn stopAtTurn
	for _, worker := range workerData {
		try(worker.encoder.Encode(controllerPacket{worker.index, *stopAtTurn}))
	}
	for _, channel := range workerData {
		r := <-channel.distributorOutput
		if r != pause {
			fmt.Println("Something has gone wrong, r =", r)
		}
	}
}

// Receive the world from all workers
func receiveWorld(world [][]byte, workerData []workerData, threadsSmall, threadsSmallHeight, threadsLargeHeight int) {
	startX := 0
	endX := threadsSmallHeight
	for i, worker := range workerData {
		tw := <-worker.outputWorld
		for i := range tw {
			copy(world[startX+i], tw[i])
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
func sendToWorkers(workerData []workerData, data int) {
	for _, worker := range workerData {
		try(worker.encoder.Encode(controllerPacket{worker.index, data}))
	}
}

// Controlls user interaction
func workerController(p golParams, world [][]byte, workerData []workerData, d distributorChans, keyChan <-chan rune, threadsSmall, threadsSmallHeight, threadsLargeHeight int) {
	stopAtTurn := 0
	paused := false
	timer := time.NewTimer(2 * time.Second)
	for q := false; q != true; {
		select {
		case <-timer.C:
			// Get alive cells
			alive := 0
			if !paused {
				pauseWorkers(workerData, &stopAtTurn)
				// Ping unpauses workers
				sendToWorkers(workerData, ping)
				for _, channel := range workerData {
					alive += <-channel.distributorOutput
				}
				fmt.Println("There are", alive, "alive cells in the world.")
			}

			timer = time.NewTimer(2 * time.Second)
		case k := <-keyChan:
			if k == 'p' || k == 's' || k == 'q' {
				// If not already paused
				if !paused {
					pauseWorkers(workerData, &stopAtTurn)
					// Paused until resume
					if k == 'p' {
						fmt.Println("Pausing. The turn number", stopAtTurn+1, "is currently being processed.")
					}
				} else if k == 'p' { // If this was a pause command and we are already paused, resume
					// Resume all workers
					sendToWorkers(workerData, resume)
					fmt.Println("Continuing.")
				}
				// If this was a save or quit command
				if k == 's' || k == 'q' {
					if k == 's' {
						fmt.Println("Saving on turn", stopAtTurn)
					} else {
						fmt.Println("Saving and quitting on turn", stopAtTurn)
					}
					sendToWorkers(workerData, save)

					// If paused just to save, unpause. If quit, don't unpause
					if !paused && k == 's' {
						sendToWorkers(workerData, resume)
					}

					// Receive and output world
					receiveWorld(world, workerData, threadsSmall, threadsSmallHeight, threadsLargeHeight)
					outputWorld(p, stopAtTurn, d, world)

					// Quit workers
					if k == 'q' {
						q = true
						sendToWorkers(workerData, quit)
					}
				}
				// If this was a pause command, actually pause
				if k == 'p' {
					paused = !paused
				}
			}
		case o := <-workerData[0].distributorOutput: // Workers are starting to finish
			if o != -1 {
				fmt.Println("Something has gone wrong, o =", o)
			}
			for i := 1; i < p.threads; i++ {
				<-workerData[i].distributorOutput
			}
			// Receive the world and quit
			receiveWorld(world, workerData, threadsSmall, threadsSmallHeight, threadsLargeHeight)
			q = true
		}
	}
}

// Listens to a client's connection and transfers received packets to intended channels
func listenToWorker(decoder *gob.Decoder, channel []workerData, workersServed int) {
	// This for terminates when all workers sent their final data
	for i := 0; i < workersServed; {
		var p distributorPacket
		err := decoder.Decode(&p)
		if err == io.EOF {
			fmt.Println("Client disconnected.")
		} else {
			try(err)
		}

		if p.Type == 1 { // a worker sent the world
			channel[p.Index].outputWorld <- p.OutputWorld
		} else if p.Type == 0 { // a worker sent some data
			channel[p.Index].distributorOutput <- p.Data
		} else if p.Type == -1 { // a worker sent final data, so no more data will be received from it
			channel[p.Index].distributorOutput <- p.Data
			i++ // we are finished with this worker
		}
	}
}

// Initialises workers on the clients
func startWorkers(client clientData, initP initPacket, workerP []workerPacket, workerData []workerData) {
	// Let the client know we are about to send an init packet
	try(client.encoder.Encode(INIT))

	// Send the init packet
	try(client.encoder.Encode(initP))

	// Send worker packets
	for i, p := range workerP {
		workerData[i].encoder = client.encoder
		workerData[i].index = i
		try(client.encoder.Encode(p))
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p golParams, d distributorChans, alive chan []cell, keyChan <-chan rune, clients []clientData, clientNumber int) {

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
	//threadsLarge := p.imageHeight % p.threads
	threadsSmall := p.threads - p.imageHeight%p.threads
	threadsLargeHeight := p.imageHeight/p.threads + 1
	threadsSmallHeight := p.imageHeight / p.threads

	// Worker channel initialisation
	workerData := make([]workerData, p.threads)
	for i := 0; i < p.threads; i++ {
		workerData[i].outputWorld = make(chan [][]byte, 1)
		workerData[i].distributorOutput = make(chan int, 1)
	}

	// Threads per client
	//clientLarge := p.threads % clientNumber
	clientSmall := clientNumber - p.threads%clientNumber

	clientLargeWorkers := p.threads/clientNumber + 1
	clientSmallWorkers := p.threads / clientNumber

	workerBounds := make([]workerPacket, p.threads)

	// Copy of world, but with extra 2 lines (one at the start, one at the end)
	borderedWorld := make([][]byte, p.imageHeight+2)
	for i := range world {
		borderedWorld[i+1] = world[i]
	}
	borderedWorld[0] = world[p.imageHeight-1]
	borderedWorld[p.imageHeight+1] = world[0]

	// Create worker initialisation packets
	startX := 0
	endX := threadsSmallHeight
	for i := 0; i < p.threads; i++ {
		workerBounds[i] = workerPacket{startX, endX,
			borderedWorld[startX : endX+2],
			i}

		startX = endX
		if i < threadsSmall-1 {
			endX += threadsSmallHeight
		} else {
			endX += threadsLargeHeight
		}
	}

	// Start workers on remote clients
	t := 0
	for i := 0; i < clientNumber; i++ {
		host0 := clients[positiveModulo(i-1, clientNumber)].ip
		host1 := clients[positiveModulo(i+1, clientNumber)].ip
		workers := clientSmallWorkers
		if i >= clientSmall {
			workers = clientLargeWorkers
		}
		fmt.Println(workers, "workers started on client", i)
		startWorkers(clients[i],
			initPacket{clientNumber, workers, host0, host1, p.turns, p.imageWidth},
			workerBounds[t:t+workers], workerData[t:t+workers])
		t += workers
	}

	// Client synchronisation
	for i := 0; i < clientNumber; i++ {
		var p int
		try(clients[i].decoder.Decode(&p))

		if p != 1 {
			fmt.Println("Ready packet mismatch")
		}
	}
	for i := 0; i < clientNumber; i++ {
		p := 1
		try(clients[i].encoder.Encode(&p))

		if i < clientSmall {
			go listenToWorker(clients[i].decoder, workerData, clientSmallWorkers)
		} else {
			go listenToWorker(clients[i].decoder, workerData, clientLargeWorkers)
		}
	}

	// Process IO and control workers
	workerController(p, world, workerData, d, keyChan, threadsSmall, threadsSmallHeight, threadsLargeHeight)

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

	// Tell workers to exit listening functions
	for i := 0; i < clientNumber; i++ {
		try(clients[i].encoder.Encode(controllerPacket{-1, 0}))
	}

	//outputWorld(p, p.turns, d, world)

	// Make sure that the Io has finished any output before exiting.
	d.io.command <- ioCheckIdle
	<-d.io.idle
	// Return the coordinates of cells that are still alive.
	alive <- finalAlive

}
