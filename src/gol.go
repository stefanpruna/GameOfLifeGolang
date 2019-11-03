package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	//"time"
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
func getAliveNeighbours(world [][]byte, x, y, imageHeight, imageWidth int) int {
	aliveNeighbours := 0
	//if the cell is on the left border
	dx := [8]int{-1, -1, 0, 1, 1, 1, 0, -1}
	dy := [8]int{0, 1, 1, 1, 0, -1, -1, -1}

	for i := 0; i < 8; i++ {
		newX := positiveModulo(x+dx[i], imageHeight)
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
func worker(p golParams, world [][]byte, startX, endX, startY, endY int, result chan [][]byte, group *sync.WaitGroup) {
	newWorld := make([][]byte, endX-startX)
	for i := range newWorld {
		newWorld[i] = make([]byte, endY-startY)
	}

	for i := startX; i < endX; i++ {
		for j := startY; j < endY; j++ {
			switch getNewState(getAliveNeighbours(world, i, j, p.imageHeight, p.imageWidth), world[i][j] == 0xFF) {
			case -1:
				newWorld[i-startX][j-startY] = 0x00
			case 1:
				newWorld[i-startX][j-startY] = 0xFF
			case 0:
				newWorld[i-startX][j-startY] = world[i][j]
			}
		}
	}

	group.Done()
	result <- newWorld
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

	// Make channels
	var chans = make([]chan [][]byte, p.threads)
	for i := 0; i < p.threads; i++ {
		chans[i] = make(chan [][]byte)
	}

	var turns int = 0
	var paused bool = false
	var quit bool = false
	var resume = make(chan bool)

	go eventController(world, p, d, keyChan, &turns, &paused, resume, &quit)

	// Thread calculations
	// 16x16 with 10 threads: 6 large threads with 2 height + 4 small threads with 1 height
	threadsLarge := p.imageHeight % p.threads
	threadsSmall := p.threads - p.imageHeight%p.threads

	threadsLargeHeight := p.imageHeight/p.threads + 1
	threadsSmallHeight := p.imageHeight / p.threads

	// Calculate the new state of Game of Life after the given number of turns.
	for turns = 0; turns < p.turns; turns++ {

		var group sync.WaitGroup

		for t := 0; t < threadsSmall; t++ {
			group.Add(1)
			go worker(p, world, threadsSmallHeight*t, threadsSmallHeight*(t+1), 0, p.imageWidth, chans[t], &group)
		}
		for t := 0; t < threadsLarge; t++ {
			group.Add(1)
			go worker(p, world, threadsSmallHeight*threadsSmall+threadsLargeHeight*t,
				threadsSmallHeight*threadsSmall+threadsLargeHeight*(t+1), 0, p.imageWidth, chans[threadsSmall+t], &group)
		}

		group.Wait()

		for t := 0; t < threadsSmall; t++ {
			channelOutput := <-chans[t]
			for x := 0; x < threadsSmallHeight; x++ {
				world[threadsSmallHeight*t+x] = channelOutput[x]
			}
		}
		for t := 0; t < threadsLarge; t++ {
			channelOutput := <-chans[threadsSmall+t]
			for x := 0; x < threadsLargeHeight; x++ {
				world[threadsSmallHeight*threadsSmall+threadsLargeHeight*t+x] = channelOutput[x]
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
