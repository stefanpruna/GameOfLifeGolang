package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
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

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p golParams, d distributorChans, alive chan []cell) {

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

	// Calculate the new state of Game of Life after the given number of turns.
	for turns := 0; turns < p.turns; turns++ {
		var group sync.WaitGroup

		for t := 0; t < p.threads; t++ {
			group.Add(1)
			go worker(p, world, p.imageHeight/p.threads*t, p.imageHeight/p.threads*(t+1), 0, p.imageWidth, chans[t], &group)
		}

		group.Wait()

		for t := 0; t < p.threads; t++ {
			channelOutput := <-chans[t]
			for x := 0; x < p.imageHeight/p.threads; x++ {
				world[x+p.imageHeight/p.threads*t] = channelOutput[x]
			}
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
