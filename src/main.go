package main

import (
	"flag"
	"fmt"
	"net"
)

// golParams provides the details of how to run the Game of Life and which image to load.
type golParams struct {
	turns       int
	threads     int
	imageWidth  int
	imageHeight int
}

// ioCommand allows requesting behaviour from the io (pgm) goroutine.
type ioCommand uint8

// This is a way of creating enums in Go.
// It will evaluate to:
//		ioOutput 	= 0
//		ioInput 	= 1
//		ioCheckIdle = 2
const (
	ioOutput ioCommand = iota
	ioInput
	ioCheckIdle
)

// cell is used as the return type for the testing framework.
type cell struct {
	x, y int
}

// distributorToIo defines all chans that the distributor goroutine will have to communicate with the io goroutine.
// Note the restrictions on chans being send-only or receive-only to prevent bugs.
type distributorToIo struct {
	command chan<- ioCommand
	idle    <-chan bool

	filename chan<- string
	inputVal <-chan uint8
	world    chan<- byte
}

// ioToDistributor defines all chans that the io goroutine will have to communicate with the distributor goroutine.
// Note the restrictions on chans being send-only or receive-only to prevent bugs.
type ioToDistributor struct {
	command <-chan ioCommand
	idle    chan<- bool

	filename <-chan string
	inputVal chan<- uint8
	world    <-chan byte
}

// distributorChans stores all the chans that the distributor goroutine will use.
type distributorChans struct {
	io distributorToIo
}

// ioChans stores all the chans that the io goroutine will use.
type ioChans struct {
	distributor ioToDistributor
}

// gameOfLife is the function called by the testing framework.
// It makes some channels and starts relevant goroutines.
// It places the created channels in the relevant structs.
// It returns an array of alive cells returned by the distributor.
func gameOfLife(p golParams, keyChan <-chan rune) []cell {
	var dChans distributorChans
	var ioChans ioChans

	ioCommand := make(chan ioCommand)
	dChans.io.command = ioCommand
	ioChans.distributor.command = ioCommand

	ioIdle := make(chan bool)
	dChans.io.idle = ioIdle
	ioChans.distributor.idle = ioIdle

	ioFilename := make(chan string)
	dChans.io.filename = ioFilename
	ioChans.distributor.filename = ioFilename

	inputVal := make(chan uint8)
	dChans.io.inputVal = inputVal
	ioChans.distributor.inputVal = inputVal

	worldChan := make(chan byte)
	dChans.io.world = worldChan
	ioChans.distributor.world = worldChan

	aliveCells := make(chan []cell)

	go distributor(p, dChans, aliveCells, keyChan)
	go pgmIo(p, ioChans)

	alive := <-aliveCells
	return alive
}

func listenForClients(clientNumber int, clients []net.Conn) {

	ln, err := net.Listen("tcp", ":4000")
	if err != nil {
		// handle error
	}

	if ln != nil {
		for i := 0; i < clientNumber; i++ {
			conn, _ := ln.Accept()
			fmt.Println("Accepted client number", i)
			clients[i] = conn
		}
	}
}

const clientNumber = 2

var clients = make([]net.Conn, clientNumber)

const (
	INIT     = 0
	INITDATA = 1
)

// main is the function called when starting Game of Life with 'make gol'
// Do not edit until Stage 2.
func main() {
	var params golParams

	flag.IntVar(
		&params.threads,
		"t",
		16,
		"Specify the number of worker threads to use. Defaults to 8.")

	flag.IntVar(
		&params.imageWidth,
		"w",
		512,
		"Specify the width of the image. Defaults to 512.")

	flag.IntVar(
		&params.imageHeight,
		"h",
		512,
		"Specify the height of the image. Defaults to 512.")

	flag.Parse()

	params.turns = 10

	listenForClients(clientNumber, clients)

	for _, c := range clients {
		fmt.Println(c.RemoteAddr().String())
	}

	startControlServer(params)
	keyChan := make(chan rune)
	go getKeyboardCommand(keyChan)
	gameOfLife(params, keyChan)
	StopControlServer()
}
