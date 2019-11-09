package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
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
func worker(p golParams, inputByte <-chan byte, startX, endX, startY, endY int, outputByte chan<- byte, group *sync.WaitGroup) {
	world := make([][]byte, endX-startX+2)
	for i := range world {
		world[i] = make([]byte, endY-startY)
	}

	for i := range world {
		for j := 0; j < p.imageWidth; j++ {
			world[i][j] = <-inputByte
		}
	}

	for i := 1; i < endX-startX+1; i++ {
		for j := startY; j < endY; j++ {
			switch getNewState(getAliveNeighbours(world, i, j, p.imageWidth), world[i][j] == 0xFF) {
			case -1:
				outputByte <- 0x00
			case 1:
				outputByte <- 0xFF
			case 0:
				outputByte <- world[i][j]
			}
		}
	}

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

func ioController(world [][]byte, p golParams, d distributorChans, keyChan <-chan rune, turns *int, paused *bool, resume chan<- bool, quit *bool) {
	for !*quit {
		select {
		case k := <-keyChan:
			if k == 's' {
				outputWorld(p, *turns, d, world)
			} else if k == 'p' {
				*paused = !(*paused)
				if *paused {
					fmt.Println("Pausing. The turn number", *turns, "is currently being processed.")
				} else {
					fmt.Println("Continuing.")
					resume <- true
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

	var inputByte = make([]chan byte, p.threads)
	var outputByte = make([]chan byte, p.threads)

	for i := 0; i < p.threads; i++ {
		inputByte[i] = make(chan byte, p.imageHeight/p.threads*p.imageWidth)
		outputByte[i] = make(chan byte, p.imageHeight/p.threads*p.imageWidth)
	}
  
	var turns int = 0
	var paused bool = false
	var quit bool = false
	var resume = make(chan bool)

	go ioController(world, p, d, keyChan, &turns, &paused, resume, &quit)


	// Calculate the new state of Game of Life after the given number of turns.
	for turns = 0; turns < p.turns; turns++ {

		var group sync.WaitGroup
		for t := 0; t < p.threads; t++ {

			group.Add(1)

			go worker(p, inputByte[t], p.imageHeight/p.threads*t, p.imageHeight/p.threads*(t+1), 0, p.imageWidth, outputByte[t], &group)

			for i := p.imageHeight/p.threads*t - 1; i < p.imageHeight/p.threads*(t+1)+1; i++ {
				for j := 0; j < p.imageWidth; j++ {
					inputByte[t] <- world[positiveModulo(i, p.imageHeight)][positiveModulo(j, p.imageWidth)]
				}
			}

		}

		group.Wait()

		for t := 0; t < p.threads; t++ {
			for x := 0; x < p.imageHeight/p.threads; x++ {
				for y := 0; y < p.imageWidth; y++ {
					world[x+p.imageHeight/p.threads*t][y] = <-outputByte[t]
				}
			}
		}<<<<<<< Stage_1b

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
