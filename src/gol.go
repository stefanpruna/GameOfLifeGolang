package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

type workerChannel struct {
	inputByte,
	outputByte chan byte
	inputHalo  [2]chan byte
	outputHalo [2]chan byte
}

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
func worker(p golParams, channels workerChannel, startX, endX, startY, endY int, group *sync.WaitGroup) {
	world := make([][]byte, endX-startX+2)
	for i := range world {
		world[i] = make([]byte, endY-startY)
	}

	for i := range world {
		for j := 0; j < p.imageWidth; j++ {
			world[i][j] = <-channels.inputByte
			fmt.Print(world[i][j], " ")
		}
		fmt.Println()
	}
	fmt.Println("MUIE")

	// TODO add pause
	for it := 0; it < p.turns; it++ {
		// Receive new halo lines
		if it != 0 {
			for j := 0; j < p.imageWidth; j++ {
				world[0][j] = <-channels.inputHalo[0]
				world[endX-startX+1][j] = <-channels.inputHalo[1]
			}
		}

		for i := 1; i < endX-startX+1; i++ {
			for j := startY; j < endY; j++ {
				switch getNewState(getAliveNeighbours(world, i, j, p.imageWidth), world[i][j] == 0xFF) {
				case -1:
					world[i][j] = 0x00
				case 1:
					world[i][j] = 0xFF
				}
			}
		}

		// Send new halo lines
		for j := 0; j < p.imageWidth; j++ {
			channels.outputHalo[0] <- world[1][j]
			channels.outputHalo[1] <- world[endX-startX][j]
		}

	}

	for i := 1; i < endX-startX+1; i++ {
		for j := startY; j < endY; j++ {
			channels.outputByte <- world[i][j]
		}
	}
	fmt.Println("done")
	group.Done()
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

// Returns number of alive cells in the world.
func getAlive(world [][]byte) int {
	r := 0
	for i := range world {
		for j := range world[i] {
			if world[i][j] == 0x00 {
				r++
			}
		}
	}
	return r
}

func eventController(world [][]byte, p golParams, d distributorChans, keyChan <-chan rune, turns *int, paused *bool, resume chan<- bool, quit *bool) {
	timer := time.NewTimer(2 * time.Second)
	for !*quit {
		select {
		case <-timer.C:
			fmt.Println("There are", getAlive(world), "alive cells in the world.")
			if !*paused {
				timer = time.NewTimer(2 * time.Second)
			}
		case k := <-keyChan:
			if k == 's' {
				outputWorld(p, *turns, d, world)
			} else if k == 'p' {
				*paused = !(*paused)
				if *paused {
					fmt.Println("Pausing. The turn number", *turns, "is currently being processed.")
					timer.Stop()
				} else {
					fmt.Println("Continuing.")
					resume <- true
					timer = time.NewTimer(2 * time.Second)
				}
			} else if k == 'q' {
				fmt.Println("Quitting simulation and outputting final state of the world.")
				if *paused {
					*paused = false
					resume <- true
				}
				*quit = true
			}
		}
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

	// Wait group
	var group sync.WaitGroup

	// Make channels
	var chans = make([]chan [][]byte, p.threads)
	for i := 0; i < p.threads; i++ {
		chans[i] = make(chan [][]byte)
	}

	//
	p.threads = 1
	//

	// Thread calculations
	// 16x16 with 10 threads: 6 large threads with 2 height + 4 small threads with 1 height
	threadsLarge := p.imageHeight % p.threads
	threadsSmall := p.threads - p.imageHeight%p.threads

	threadsLargeHeight := p.imageHeight/p.threads + 1
	threadsSmallHeight := p.imageHeight / p.threads

	// Worker channels
	workerChannels := make([]workerChannel, p.threads)

	for i := 0; i < threadsSmall; i++ {
		workerChannels[i].inputByte = make(chan byte, threadsSmallHeight+2)
		workerChannels[i].outputByte = make(chan byte, threadsSmallHeight*p.imageWidth)
		workerChannels[i].inputHalo[0] = make(chan byte, p.imageWidth)
		workerChannels[i].inputHalo[1] = make(chan byte, p.imageWidth)

		workerChannels[positiveModulo(i-1, p.threads)].outputHalo[1] = workerChannels[i].inputHalo[0]
		workerChannels[positiveModulo(i+1, p.threads)].outputHalo[0] = workerChannels[i].inputHalo[1]

		startX := threadsSmallHeight * i
		endX := threadsSmallHeight * (i + 1)
		// Start workers here for better performance
		go worker(p, workerChannels[i], startX, endX, 0, p.imageWidth, &group)
	}

	for i := 0; i < threadsLarge; i++ {
		workerChannels[i+threadsSmall].inputByte = make(chan byte, threadsLargeHeight+2)
		workerChannels[i+threadsSmall].outputByte = make(chan byte, threadsLargeHeight*p.imageWidth)
		workerChannels[i+threadsSmall].inputHalo[0] = make(chan byte, p.imageWidth)
		workerChannels[i+threadsSmall].inputHalo[1] = make(chan byte, p.imageWidth)

		workerChannels[positiveModulo(i+threadsSmall-1, p.threads)].outputHalo[1] = workerChannels[i+threadsSmall].inputHalo[0]
		workerChannels[positiveModulo(i+threadsSmall+1, p.threads)].outputHalo[0] = workerChannels[i+threadsSmall].inputHalo[1]

		startX := threadsSmallHeight*threadsSmall + threadsLargeHeight*i
		endX := threadsSmallHeight*threadsSmall + threadsLargeHeight*(i+1)
		// Start workers here for better performance
		go worker(p, workerChannels[i+threadsSmall], startX, endX, 0, p.imageWidth, &group)
	}

	var turns = 0
	var paused = false
	var quit = false
	var resume = make(chan bool)

	go eventController(world, p, d, keyChan, &turns, &paused, resume, &quit)

	// Calculate the new state of Game of Life after the given number of turns.
	for turns = 0; turns < 1; turns++ {

		for t := 0; t < threadsSmall; t++ {

			group.Add(1)

			startX := threadsSmallHeight * t
			endX := threadsSmallHeight * (t + 1)

			for i := startX - 1; i < endX+1; i++ {
				for j := 0; j < p.imageWidth; j++ {
					workerChannels[t].inputByte <- world[positiveModulo(i, p.imageHeight)][positiveModulo(j, p.imageWidth)]
				}
			}
		}

		for t := 0; t < threadsLarge; t++ {

			group.Add(1)

			startX := threadsSmallHeight*threadsSmall + threadsLargeHeight*t
			endX := threadsSmallHeight*threadsSmall + threadsLargeHeight*(t+1)

			for i := startX - 1; i < endX+1; i++ {
				for j := 0; j < p.imageWidth; j++ {
					workerChannels[t+threadsSmall].inputByte <- world[positiveModulo(i, p.imageHeight)][positiveModulo(j, p.imageWidth)]
				}
			}
		}

		group.Wait()

		for t := 0; t < threadsSmall; t++ {
			startX := threadsSmallHeight * t
			for x := 0; x < threadsSmallHeight; x++ {
				for y := 0; y < p.imageWidth; y++ {
					world[x+startX][y] = <-workerChannels[t].outputByte
				}
			}
		}

		for i := range world {
			for j := range world[i] {
				fmt.Print(world[i][j], " ")
			}
			fmt.Println()
		}
		fmt.Println("MUIE2")

		for t := 0; t < threadsLarge; t++ {
			startX := threadsSmallHeight*threadsSmall + threadsLargeHeight*t

			for x := 0; x < threadsLargeHeight; x++ {
				for y := 0; y < p.imageWidth; y++ {
					world[x+startX][y] = <-workerChannels[t+threadsSmall].outputByte
				}
			}
		}

		if paused {
			<-resume
		}
		if quit {
			outputWorld(p, turns, d, world)
			break
		}

	}

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
