package main

import (
	"encoding/gob"
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
func gameOfLife(p golParams, keyChan <-chan rune, clientNumber int, clients []clientData) []cell {
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

	if p.threads < clientNumber {
		p.threads = clientNumber
	}
	p.threads = 4

	go distributor(p, dChans, aliveCells, keyChan, clients, clientNumber)
	go pgmIo(p, ioChans)

	alive := <-aliveCells
	return alive
}

func processClients(clientNumber int) []clientData {

	clients := make([]clientData, clientNumber)

	ln, err := net.Listen("tcp4", ":4000")
	if err != nil {
		fmt.Println("Could not listen to port 4000", err)
	}

	if ln != nil {
		for i := 0; i < clientNumber; i++ {
			conn, err := ln.Accept()
			if err != nil {
				fmt.Println(err)
			}
			fmt.Println("Client number", i, "/", clientNumber, "connected")

			clients[i].encoder = gob.NewEncoder(conn)
			clients[i].decoder = gob.NewDecoder(conn)
			clients[i].ip, _, _ = net.SplitHostPort(conn.RemoteAddr().String())
		}
	}

	return clients
}

const clientNumber = 2

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

	params.turns = 500

	fmt.Println("Waiting for", clientNumber, "clients to connect.")
	clients := processClients(clientNumber)

	startControlServer(params)
	keyChan := make(chan rune)
	go getKeyboardCommand(keyChan)

	gameOfLife(params, keyChan, clientNumber, clients)
	gameOfLife(params, keyChan, clientNumber, clients)
	StopControlServer()
}
