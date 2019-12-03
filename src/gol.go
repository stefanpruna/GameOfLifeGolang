package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Modulus that only returns positive number
func positiveModulo(x, m int) int {
	for x < 0 {
		x += m
	}
	return x % m
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
func worker(p golParams, inputByte <-chan byte, startX, endX, startY, endY int, outputByte chan<- byte) {
	world := make([][]byte, endX-startX+2)
	for i := range world {
		world[i] = make([]byte, endY-startY)
	}

	for {
		for i := range world {
			for j := 0; j < p.imageWidth; j++ {
				world[i][j] = <-inputByte
			}
		}

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
					outputByte <- 0x00
				case 1:
					outputByte <- 0xFF
				case 0:
					outputByte <- world[i][j]
				}
			}
		}
	}

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

	// Worker channels
	var inputByte = make([]chan byte, p.threads)
	var outputByte = make([]chan byte, p.threads)

	for i := 0; i < p.threads; i++ {
		inputByte[i] = make(chan byte, p.imageHeight/p.threads*p.imageWidth)
		outputByte[i] = make(chan byte, p.imageHeight/p.threads*p.imageWidth)

		// Start workers here for better performance
		go worker(p, inputByte[i], p.imageHeight/p.threads*i, p.imageHeight/p.threads*(i+1), 0, p.imageWidth, outputByte[i])
	}

	// Calculate the new state of Game of Life after the given number of turns.
	for turns := 0; turns < p.turns; turns++ {

		for t := 0; t < p.threads; t++ {
			for i := p.imageHeight/p.threads*t - 1; i < p.imageHeight/p.threads*(t+1)+1; i++ {
				for j := 0; j < p.imageWidth; j++ {
					inputByte[t] <- world[positiveModulo(i, p.imageHeight)][positiveModulo(j, p.imageWidth)]
				}
			}
		}

		for t := 0; t < p.threads; t++ {
			for x := 0; x < p.imageHeight/p.threads; x++ {
				for y := 0; y < p.imageWidth; y++ {
					world[x+p.imageHeight/p.threads*t][y] = <-outputByte[t]
				}
			}
		}

	}

	outputWorld(p, p.turns, d, world)

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
