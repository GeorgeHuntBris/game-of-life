package main

import (
	"flag"
	"net"
	"net/rpc"
	"os"
	"sync"
	"uk.ac.bris.cs/gameoflife/gol/stubs"
)

var (
	currentBoard [][]uint8
	mutexS       sync.Mutex
)

type Server struct{}

func UpdateBoard(world [][]uint8, width int, height int, startY int, endY int) [][]uint8 {

	heightChunk := endY - startY
	nextWorld := make([][]uint8, heightChunk)
	for i := range nextWorld {
		nextWorld[i] = make([]uint8, width)
	}

	for y := startY; y < endY; y++ {
		for x := 0; x < width; x++ {
			liveNeighbours := 0

			left := (x - 1 + width) % width
			right := (x + 1) % width
			above := (y - 1 + height) % height
			below := (y + 1) % height

			// Count live neighbors
			if world[y][left] == 255 {
				liveNeighbours++
			}
			if world[y][right] == 255 {
				liveNeighbours++
			}
			if world[above][x] == 255 {
				liveNeighbours++
			}
			if world[below][x] == 255 {
				liveNeighbours++
			}
			if world[below][left] == 255 {
				liveNeighbours++
			}
			if world[below][right] == 255 {
				liveNeighbours++
			}
			if world[above][left] == 255 {
				liveNeighbours++
			}
			if world[above][right] == 255 {
				liveNeighbours++
			}

			localY := y - startY
			if world[y][x] == 255 {
				if liveNeighbours < 2 || liveNeighbours > 3 {
					nextWorld[localY][x] = 0
				} else {
					nextWorld[localY][x] = 255
				}
			} else {
				if liveNeighbours == 3 {
					nextWorld[localY][x] = 255
				} else {
					nextWorld[localY][x] = 0
				}
			}
		}
	}
	return nextWorld
}

func (s *Server) QuitServer(req stubs.QuitServerRequest, res *stubs.QuitServerResponse) error {
	os.Exit(0)
	return nil
}

func worker(height int, width int, world [][]uint8, startY int, endY int, result chan<- [][]uint8) {

	nextWorld := UpdateBoard(world, width, height, startY, endY)
	result <- nextWorld
}

func (s *Server) Evolve(req stubs.EvolveRequest, res *stubs.EvolveResponse) error {
	mutexS.Lock() // Lock the state before evolving
	currentBoard = req.Board
	mutexS.Unlock()

	mutexS.Lock()
	copyCurrentBoard := currentBoard
	mutexS.Unlock()

	result := make([]chan [][]uint8, 3) // 3 worker threads
	for i := range result {
		result[i] = make(chan [][]uint8)
	}
	startY := 0
	heightSpaces := req.BoardHeight / 3
	for i := 0; i < 3; i++ {
		endY := startY + heightSpaces
		if i == 2 {
			endY = req.BoardHeight
		}
		go worker(req.BoardHeight, req.BoardWidth, copyCurrentBoard, startY, endY, result[i])
		startY = endY
	}

	var nextWorld [][]uint8

	for i := 0; i < 3; i++ {
		nextWorld = append(nextWorld, <-result[i]...)
	}

	mutexS.Lock()
	currentBoard = nextWorld
	mutexS.Unlock()

	mutexS.Lock()
	res.NewBoard = currentBoard
	mutexS.Unlock()
	return nil
}

func main() {
	pAddr := flag.String("port", "8050", "Port to listen on")
	flag.Parse()

	// Register the GameOfLifeOperations service
	err := rpc.Register(&Server{})
	if err != nil {
		panic(err)
	}

	// Start listening for incoming connections
	listener, err := net.Listen("tcp", ":"+*pAddr)
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	for {
		rpc.Accept(listener)
	}

}
