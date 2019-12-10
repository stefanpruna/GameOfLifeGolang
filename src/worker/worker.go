package main

import (
	"encoding/gob"
	"flag"
	"fmt"
	"io"
	"net"
)

const defaultHostname = "192.168.0.9"

const INIT = 0

const (
	pause  = iota
	ping   = iota
	resume = iota
	quit   = iota
	save   = iota
)

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

// Sent localDistributor packet
type outgoingDistPacket struct {
	Index       int
	Type        int
	Data        int
	OutputWorld [][]byte
}

// Received localDistributor packet
type incomingDistPacket struct {
	Index, Data int
}

type haloPacket struct {
	Index int
	Data  []byte
}

type workerData struct {
	inputHalo        [2]chan byte
	outputHalo       [2]chan byte
	distributorInput chan int
	localDistributor chan byte
}

// Makes a matrix slice
func makeMatrix(width, height int) [][]byte {
	M := make([][]byte, height)
	for i := range M {
		M[i] = make([]byte, width)
	}
	return M
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

func try(error error) {
	if error != nil {
		fmt.Println("Err", error)
	}
}

/* NETWORKING */

// Send external halos to another client
func serveToClient(conn net.Conn, index int, c chan byte, width int, turns int, exit chan byte) {
	enc := gob.NewEncoder(conn)
	for i := 0; i < turns; i++ {
		var haloData = make([]byte, width)

		for i := 0; i < width; i++ {
			haloData[i] = <-c
		}

		err := enc.Encode(haloPacket{index, haloData})
		if err != nil {
			fmt.Println("err", err)
			break
		}
	}

	exit <- 1
}

// Wait for 2 other clients to connect to this one
func waitForClients(clients []net.Conn, done chan net.Listener, listening chan byte) {
	ln, err := net.Listen("tcp4", ":4001")
	if err != nil {
		fmt.Println("err", err)
		return
	}
	listening <- 1

	if ln != nil {
		for i := 0; i < 2; i++ {
			conn, _ := ln.Accept()
			clients[i] = conn
		}
		done <- ln
	}
}

// Receive external halos from another client
func receiveFromClient(ip string, c [2]chan byte, turns int, exit chan byte) {
	conn, err := net.Dial("tcp4", ip+":4001")
	if err != nil {
		fmt.Println("err", err)
		exit <- 1
		return
	}

	dec := gob.NewDecoder(conn)
	for i := 0; i < turns; i++ {
		var haloP haloPacket

		err := dec.Decode(&haloP)
		if err != nil {
			fmt.Println("err", err)
			return
		}

		// Take bytes from haloData row slice and put them in the channel
		for _, b := range haloP.Data {
			c[haloP.Index] <- b
		}
	}

	exit <- 1
}

// Receive data from distributor server
func receiveFromDistributor(decoder *gob.Decoder, channels []workerData, exit chan byte) {
	for {
		var p incomingDistPacket
		try(decoder.Decode(&p))

		// Exit signal
		if p.Index == -1 {
			break
		}
		channels[p.Index].distributorInput <- p.Data
	}
	exit <- 1
}

// With the help of the distributor server, sync all clients to the same state
func syncWithOtherClients(encoder *gob.Encoder, decoder *gob.Decoder) {
	// This client is ready to receive
	try(encoder.Encode(1))

	var p int
	try(decoder.Decode(&p))
	// All clients now ready to receive
	if p != 1 {
		fmt.Println("Received mismatched value from distributor")
	}
}

/* ***** */

// Receive halo, or receive command from localDistributor
func receiveOrInterrupt(world [][]byte, data workerData, encoder *gob.Encoder, workerIndex, turn int, halo *bool, stopAtTurn *int, lineToReceive, haloIndex int) {
	select {
	case c := <-data.inputHalo[haloIndex]:
		length := len(world[lineToReceive])
		world[lineToReceive][0] = c
		for j := 1; j < length; j++ {
			world[lineToReceive][j] = <-data.inputHalo[haloIndex]
		}
		*halo = true
	case <-data.distributorInput:
		try(encoder.Encode(&outgoingDistPacket{workerIndex, 0, turn, nil}))
		*stopAtTurn = <-data.distributorInput
	}
}

// Send halo, or receive command from localDistributor
func sendOrInterrupt(world [][]byte, data workerData, encoder *gob.Encoder, workerIndex, turn int, out *bool, stopAtTurn *int, lineToSend, haloIndex int) {
	select {
	case data.outputHalo[haloIndex] <- world[lineToSend][0]:
		length := len(world[lineToSend])
		for j := 1; j < length; j++ {
			data.outputHalo[haloIndex] <- world[lineToSend][j]
		}
		*out = true
	case <-data.distributorInput:
		try(encoder.Encode(&outgoingDistPacket{workerIndex, 0, turn, nil}))
		*stopAtTurn = <-data.distributorInput
	}
}

func worker(p initPacket, data workerData, wp workerPacket, encoder *gob.Encoder) {
	endX := wp.EndX
	startX := wp.StartX
	endY := p.Width
	startY := 0

	world := makeMatrix(endY-startY, endX-startX+2)
	newWorld := makeMatrix(endY-startY, endX-startX+2)

	// Receive initial world from localDistributor
	for i := range world {
		for j := 0; j < endY; j++ {
			newWorld[i][j] = wp.World[i][j]
			world[i][j] = newWorld[i][j]
		}
	}

	halo0, halo1 := true, true
	stopAtTurn := -2

	for turn := 0; turn < p.Turns; {

		// This is the turn in which all workers synchronise and stop
		if turn == stopAtTurn+1 {
			try(encoder.Encode(&outgoingDistPacket{wp.Index, 0, pause, nil}))
			// Process IO
			for {
				r := <-data.distributorInput
				if r == resume {
					break
				} else if r == save {
					// Send the world to the localDistributor
					try(encoder.Encode(&outgoingDistPacket{wp.Index, 1, 0, newWorld[1 : endX-startX+1]}))
				} else if r == quit {
					data.localDistributor <- 1
					return
				} else if r == ping {
					// Send the number of alive cells to the localDistributor
					alive := 0
					for i := 1; i < endX-startX+1; i++ {
						for j := startY; j < endY; j++ {
							if newWorld[i][j] == 0xFF {
								alive++
							}
						}
					}
					try(encoder.Encode(&outgoingDistPacket{wp.Index, 0, alive, nil}))
					break
				}
			}
		}

		// Get halos or command
		if turn != 0 {
			// Either receive the top halo, or a command from localDistributor
			if !halo0 {
				receiveOrInterrupt(world, data, encoder, wp.Index, turn, &halo0, &stopAtTurn, 0, 0)
			}
			// Either receive the bottom halo, or a command from localDistributor
			if !halo1 {
				receiveOrInterrupt(world, data, encoder, wp.Index, turn, &halo1, &stopAtTurn, endX-startX+1, 1)
			}
		}

		// Move on to next turn
		if halo0 && halo1 {

			for i := 1; i < endX-startX+1; i++ {
				for j := startY; j < endY; j++ {
					// Compute alive neighbours
					aliveNeighbours := 0
					yp1 := (j + 1) % endY
					ym1 := j - 1
					if ym1 < 0 {
						ym1 += endY
					}

					aliveNeighbours = int(world[i+1][j]) + int(world[i-1][j]) +
						int(world[i][yp1]) + int(world[i][ym1]) +
						int(world[i+1][yp1]) + int(world[i+1][ym1]) +
						int(world[i-1][yp1]) + int(world[i-1][ym1])

					switch getNewState(aliveNeighbours/255, world[i][j] == 0xFF) {
					case -1:
						newWorld[i][j] = 0x00
					case 1:
						newWorld[i][j] = 0xFF
					case 0:
						newWorld[i][j] = world[i][j]
					}
				}
			}
			halo0, halo1 = false, false
			turn++

			// Try sending the halos, or a command from localDistributor
			out0, out1 := false, false
			for !(out0 && out1) {
				if !out0 {
					sendOrInterrupt(newWorld, data, encoder, wp.Index, turn, &out0, &stopAtTurn, 1, 0)
				}
				if !out1 {
					sendOrInterrupt(newWorld, data, encoder, wp.Index, turn, &out1, &stopAtTurn, endX-startX, 1)
				}
			}

			// Update old world
			for i := range world {
				copy(world[i], newWorld[i])
			}
		}

	}

	// Send the final world
	try(encoder.Encode(&outgoingDistPacket{wp.Index, 1, 0, newWorld[1 : endX-startX+1]}))
	// Done
	try(encoder.Encode(&outgoingDistPacket{wp.Index, -1, -1, nil}))
	data.localDistributor <- 0
}

// Initialise all worker channels
func initialiseChannels(data []workerData, workers, clients, imageWidth, i int) {
	data[i].inputHalo[0] = make(chan byte, imageWidth)
	data[i].inputHalo[1] = make(chan byte, imageWidth)
	data[i].localDistributor = make(chan byte)
	data[i].distributorInput = make(chan int, 1)

	if workers == 1 && clients > 1 { // Just one worker but more than one client => channels are external
		data[0].outputHalo[0] = make(chan byte, imageWidth)
		data[0].outputHalo[1] = make(chan byte, imageWidth)
	} else if workers == 1 { // Just one client, with just one worker => channels are internal
		data[0].outputHalo[0] = data[0].inputHalo[1]
		data[0].outputHalo[1] = data[0].inputHalo[0]
	} else if i == 0 { // >1 client & >1 worker AND this is the first channel => it is connected externally
		data[0].outputHalo[0] = make(chan byte, imageWidth)
		data[i+1].outputHalo[0] = data[i].inputHalo[1]
	} else if i == workers-1 { // >1 client & >1 worker AND this is the last channel => it is connected externally
		data[workers-1].outputHalo[1] = make(chan byte, imageWidth)
		data[i-1].outputHalo[1] = data[i].inputHalo[0]
	} else { // Normal internal channels
		data[i-1].outputHalo[1] = data[i].inputHalo[0]
		data[i+1].outputHalo[0] = data[i].inputHalo[1]
	}
}

func localDistributor(encoder *gob.Encoder, decoder *gob.Decoder, exitThread []chan byte) int {
	var haloClients = make([]net.Conn, 2)
	var done = make(chan net.Listener)
	var listening = make(chan byte)
	var initP initPacket
	var ln net.Listener = nil
	var r byte

	try(decoder.Decode(&initP))

	// Wait for external halo connections
	if initP.Clients != 1 {
		go waitForClients(haloClients, done, listening)
	}

	// Receive and initialise worker channels
	workerData := make([]workerData, initP.Workers)
	workerPackets := make([]workerPacket, initP.Workers)
	for i := 0; i < initP.Workers; i++ {
		var w workerPacket
		try(decoder.Decode(&w))

		workerPackets[i] = w
		initialiseChannels(workerData, initP.Workers, initP.Clients, initP.Width, i)
	}

	// Wait until the program binds to port, then sync to this point with all clients.
	// At this point all of them are listening and ready for connections
	if initP.Clients > 1 {
		<-listening
	}
	syncWithOtherClients(encoder, decoder)

	// Connect to external halo sockets
	if initP.Clients > 1 {
		go receiveFromClient(initP.IpBefore, [2]chan byte{workerData[0].inputHalo[0], workerData[initP.Workers-1].inputHalo[1]}, initP.Turns, exitThread[1])
		go receiveFromClient(initP.IpAfter, [2]chan byte{workerData[0].inputHalo[0], workerData[initP.Workers-1].inputHalo[1]}, initP.Turns, exitThread[2])

		ln = <-done
	}

	// Distributor server receiver thread
	go receiveFromDistributor(decoder, workerData, exitThread[0])

	// External halo serving functions
	if initP.Clients != 1 {
		ip0, _, _ := net.SplitHostPort(haloClients[0].RemoteAddr().String())
		ip1, _, _ := net.SplitHostPort(haloClients[1].RemoteAddr().String())
		if ip0 == initP.IpBefore && ip1 == initP.IpAfter {
			go serveToClient(haloClients[0], 1, workerData[0].outputHalo[0], initP.Width, initP.Turns, exitThread[3])
			go serveToClient(haloClients[1], 0, workerData[initP.Workers-1].outputHalo[1], initP.Width, initP.Turns, exitThread[4])
		} else if ip0 == initP.IpAfter && ip1 == initP.IpBefore {
			go serveToClient(haloClients[1], 1, workerData[0].outputHalo[0], initP.Width, initP.Turns, exitThread[3])
			go serveToClient(haloClients[0], 0, workerData[initP.Workers-1].outputHalo[1], initP.Width, initP.Turns, exitThread[4])
		} else {
			fmt.Println("IPs are mismatched")
		}
	} else {
		// Only local workers, no external halos
		workerData[initP.Workers-1].outputHalo[1] = workerData[0].inputHalo[0]
		workerData[0].outputHalo[0] = workerData[initP.Workers-1].inputHalo[1]
	}

	// Start all workers
	for i := 0; i < initP.Workers; i++ {
		go worker(initP, workerData[i], workerPackets[i], encoder)
	}
	fmt.Println("All workers active")

	// Wait for workers to finish work
	for i := 0; i < initP.Workers; i++ {
		r = <-workerData[i].localDistributor
	}
	fmt.Println("All workers finished")

	// Close connections, if open
	if ln != nil {
		try(ln.Close())
	}

	// Tell main what happened
	if r == 1 {
		return -1 // Quit command
	}
	if initP.Clients == 1 {
		return 1 // Just local client, no halo sockets
	}
	return 0 // Normal termination
}

// Game of Life client
func main() {
	// process hostname flag
	var hostname string
	flag.StringVar(&hostname, "hostname", defaultHostname, "The hostname of the server.")
	flag.Parse()

	// Connect to server
	fmt.Println("Connecting to", hostname)

	conn, err := net.Dial("tcp4", hostname+":4000")
	if err != nil {
		fmt.Println("Server is offline")
		return
	}
	fmt.Println("Connected to server")

	// Create channels used on networking thread exit
	exitThread := make([]chan byte, 5)
	for i := 0; i < 5; i++ {
		exitThread[i] = make(chan byte, 1)
	}

	// Main server encoder and decoder
	enc := gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)

	for {
		var packetType = 0
		err = dec.Decode(&packetType)
		if err != nil { // If server is no longer responding, our work is done.
			if err == io.EOF {
				fmt.Println("Connection closed, exiting worker.")
				return
			}
			fmt.Println("err", err)
			return
		}

		// Run game of life
		if packetType == INIT {
			fmt.Println("Starting workers..")
			result := localDistributor(enc, dec, exitThread)
			waitForX := 5

			if result == -1 || result == 1 {
				waitForX = 1
			}
			for i := 0; i < waitForX; i++ {
				<-exitThread[i]
			}

			if result == -1 { // If quit command, quit worker program
				return
			}
		}
	}
}
