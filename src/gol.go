package main

import (
	"fmt"
	"strconv"
	"strings"
)

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

	oldWorld := make([][]byte, p.imageHeight)
	for i := range oldWorld {
		oldWorld[i] = make([]byte, p.imageWidth)
	}

	// Calculate the new state of Game of Life after the given number of turns.
	for turns := 0; turns < p.turns; turns++ {
		for i := range oldWorld {
			copy(oldWorld[i], world[i])
		}
		for x := 0; x < p.imageHeight; x++ {
			for y := 0; y < p.imageWidth; y++ {
				// Compute alive neighbours
				aliveNeighbours := 0

				yp1 := (y + 1) % p.imageWidth
				ym1 := y - 1
				if ym1 < 0 {
					ym1 += p.imageWidth
				}

				xp1 := (x + 1) % p.imageHeight
				xm1 := x - 1
				if xm1 < 0 {
					xm1 += p.imageHeight
				}

				aliveNeighbours = int(oldWorld[xp1][y]) + int(oldWorld[xm1][y]) +
					int(oldWorld[x][yp1]) + int(oldWorld[x][ym1]) +
					int(oldWorld[xp1][yp1]) + int(oldWorld[xp1][ym1]) +
					int(oldWorld[xm1][yp1]) + int(oldWorld[xm1][ym1])

				switch getNewState(aliveNeighbours/255, oldWorld[x][y] == 0xFF) {
				case -1:
					world[x][y] = 0x00
				case 1:
					world[x][y] = 0xFF
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
