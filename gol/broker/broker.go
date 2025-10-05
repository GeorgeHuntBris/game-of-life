package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"sync"
	"uk.ac.bris.cs/gameoflife/gol/stubs"
)

var (
	currentBoard   [][]uint8
	boardWidth     int
	boardHeight    int
	completedTurns int
	paused         bool
	quit           bool
	mu             sync.Mutex
	cond           *sync.Cond
	servers        []*rpc.Client
)

/*var nodeAddress = []string{
	"18.209.56.139:8050", // node3
	"54.166.107.53:8050", // distlab2
	"3.88.45.63:8050",    // myservercsa
}*/

type Broker struct{}

func CountAliveCells(world [][]uint8, BoardWidth int, BoardHeight int) int {
	aliveCellCount := 0
	for y := 0; y < BoardHeight; y++ {
		for x := 0; x < BoardWidth; x++ {
			if world[y][x] == 255 { // Check if the cell is alive
				aliveCellCount++
			}
		}
	}
	return aliveCellCount
}

func makeCallServer(server1 *rpc.Client, world [][]uint8, boardWidth int, boardHeight int, startY int, endY int, result chan<- stubs.EvolveResponse) error {
	request := stubs.EvolveRequest{
		Board:       world,
		BoardWidth:  boardWidth,
		BoardHeight: boardHeight,
		StartY:      startY,
		EndY:        endY,
	}
	response := new(stubs.EvolveResponse)
	err := server1.Call(stubs.EvolveHandler, request, response)
	if err != nil {
		return nil
	}
	result <- *response
	return nil
}

func (b *Broker) CurrentBoardState(req stubs.CurrentBoardStateRequest, res *stubs.CurrentBoardStateResponse) error {
	mu.Lock()
	defer mu.Unlock()
	res.Board = currentBoard
	res.CompletedTurns = completedTurns
	copyCurrentBoard := currentBoard
	res.AliveCount = CountAliveCells(copyCurrentBoard, boardWidth, boardHeight)
	return nil
}

func (b *Broker) Pause(req stubs.PauseRequest, res *stubs.PauseResponse) error {
	mu.Lock() // Lock to modify state safely
	defer mu.Unlock()
	paused = true
	return nil
}

func (b *Broker) Resume(req stubs.ResumeRequest, res *stubs.ResumeResponse) error {
	mu.Lock() // Lock to modify state safely
	defer mu.Unlock()
	paused = false
	cond.Broadcast() // Send a message to anything that relies on the condition variable to wake up
	return nil
}

func (b *Broker) Quit(req stubs.QuitServerRequest, res *stubs.QuitServerResponse) error {
	mu.Lock()
	quit = true
	mu.Unlock()
	request := stubs.QuitServerRequest{}
	response := new(stubs.QuitServerResponse)

	for i := 0; i < 3; i++ {
		if servers[i] == nil {
			var err error
			server, err := rpc.Dial("tcp", fmt.Sprintf(":805%d", i))
			if err != nil {
				panic(err)
			}
			servers = append(servers, server)
		}
	}

	/*for i := 0; i < 3; i++ {
		if servers[i] == nil {
			var err error
			server, err := rpc.Dial("tcp", nodeAddress[i])
			if err != nil {
				panic(err)
			}
			servers = append(servers, server)
		}
	}*/

	for i := 0; i < 3; i++ {
		servers[i].Call(stubs.QuitServerHandler, request, response)
	}

	os.Exit(0)
	return nil
}

func (b *Broker) Broker(req stubs.BrokerRequest, res *stubs.BrokerResponse) error {

	/*for i := 0; i < 3; i++ {
		server, err := rpc.Dial("tcp", nodeAddress[i]) // Connect to 3 servers with ports 8050, 8051, 8052
		if err != nil {
			panic(err)
		}
		servers = append(servers, server)
	}*/

	for i := 0; i < 3; i++ {
		server, err := rpc.Dial("tcp", fmt.Sprintf(":805%d", i)) // Connect to 3 servers with ports 8050, 8051, 8052
		if err != nil {
			panic(err)
		}
		servers = append(servers, server)
	}

	mu.Lock() // Lock the state before evolving
	cond = sync.NewCond(&mu)
	currentBoard = req.Board
	boardWidth = req.BoardWidth
	boardHeight = req.BoardHeight
	mu.Unlock()
	for turn := 0; turn < req.Turns && !quit; turn++ {
		mu.Lock()
		for paused { // Wait while paused
			cond.Wait()
		}
		mu.Unlock()

		mu.Lock()
		copyCurrentBoard := currentBoard
		mu.Unlock()

		resultsChan := make([]chan stubs.EvolveResponse, 3)
		for i := range resultsChan {
			resultsChan[i] = make(chan stubs.EvolveResponse, 1)
		}
		startY := 0
		heightChunks := boardHeight / 3
		for i := 0; i < 3; i++ {
			endY := startY + heightChunks
			if i == 2 {
				endY = boardHeight
			}
			makeCallServer(servers[i], copyCurrentBoard, boardWidth, boardHeight, startY, endY, resultsChan[i])
			startY = endY
		}

		var nextWorld [][]uint8
		for i := 0; i < 3; i++ {
			nextWorld = append(nextWorld, (<-resultsChan[i]).NewBoard...)
		}

		mu.Lock()
		currentBoard = nextWorld
		completedTurns = turn + 1
		mu.Unlock()

	}
	mu.Lock()
	res.NewBoard = currentBoard
	mu.Unlock()
	return nil
}

func main() {
	pAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()

	// Register the Broker as an RPC service
	err := rpc.Register(&Broker{})
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
