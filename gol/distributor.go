package gol

import (
	"flag"
	"fmt"
	"log"
	"net/rpc"
	"os"
	"sync"
	"time"
	"uk.ac.bris.cs/gameoflife/gol/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	keyPresses <-chan rune
}

func makeCall(client *rpc.Client, world [][]uint8, turns int, boardWidth int, boardHeight int, result chan<- stubs.BrokerResponse) error {
	request := stubs.BrokerRequest{
		Board:       world,
		Turns:       turns,
		BoardWidth:  boardWidth,
		BoardHeight: boardHeight,
	}
	response := new(stubs.BrokerResponse)
	err := client.Call(stubs.BrokerHandler, request, response)
	if err != nil {
		return err
	}
	result <- *response
	return nil
}

func makeCallCurrentBoardState(client *rpc.Client, result chan<- stubs.CurrentBoardStateResponse) error {
	request := stubs.CurrentBoardStateRequest{}
	response := new(stubs.CurrentBoardStateResponse)
	err := client.Call(stubs.CurrentBoardStateHandler, request, response)
	if err != nil {
		return err
	}
	result <- *response
	return nil
}

func makeCallResume(client *rpc.Client) error {
	request := stubs.ResumeRequest{}
	response := new(stubs.ResumeResponse)
	err := client.Call(stubs.ResumeHandler, request, response)
	if err != nil {
		return err
	}
	return nil
}

func makeCallPause(client *rpc.Client) error {
	request := stubs.PauseRequest{}
	response := new(stubs.PauseResponse)
	err := client.Call(stubs.PauseHandler, request, response)
	if err != nil {
		return err
	}
	return nil
}

func pgmFile(world [][]uint8, c distributorChannels, p Params, completedTurns int) {
	c.ioCommand <- ioOutput
	pgmFileName := fmt.Sprintf("%dx%dx%d", p.ImageWidth, p.ImageHeight, completedTurns)
	c.ioFilename <- pgmFileName
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			c.ioOutput <- world[y][x]
		}
	}
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	c.events <- ImageOutputComplete{CompletedTurns: completedTurns, Filename: pgmFileName}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	world := make([][]uint8, p.ImageHeight)
	for i, _ := range world {
		world[i] = make([]uint8, p.ImageWidth)
	}
	c.ioCommand <- ioInput
	filename := fmt.Sprintf("%dx%d", p.ImageWidth, p.ImageHeight)
	c.ioFilename <- filename
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			world[y][x] = <-c.ioInput
		}
	}

	var broker *string
	// Check if the "server" flag is already defined
	if flag.Lookup("broker") == nil {
		broker = flag.String("broker", ":8030", "IP:port string to connect to as server")
	} else {
		// If the flag is already defined, retrieve its value
		b := flag.Lookup("broker").Value.String()
		broker = &b
	}
	flag.Parse()

	client, err := rpc.Dial("tcp", *broker)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer client.Close()

	aliveCountChan := make(chan stubs.CurrentBoardStateResponse, 1)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			err := makeCallCurrentBoardState(client, aliveCountChan)
			if err != nil {
				panic(err)
			}
			aliveCount := <-aliveCountChan
			if aliveCount.CompletedTurns == p.Turns {
				ticker.Stop()
			}
			c.events <- AliveCellsCount{CompletedTurns: aliveCount.CompletedTurns, CellsCount: aliveCount.AliveCount}
		}
	}()

	var (
		mutex sync.Mutex
		pause = false
	)
	go func() {
		for {
			key := <-c.keyPresses
			currentBoardStateChan := make(chan stubs.CurrentBoardStateResponse, 1)
			err := makeCallCurrentBoardState(client, currentBoardStateChan)
			if err != nil {
				panic(err)
			}
			currentBoardState := <-currentBoardStateChan
			switch key {
			case 's':
				pgmFile(currentBoardState.Board, c, p, currentBoardState.CompletedTurns)
			case 'q':
				os.Exit(0)
			case 'k':
				ticker.Stop()
				pgmFile(currentBoardState.Board, c, p, currentBoardState.CompletedTurns)
				request := stubs.QuitRequest{}
				response := new(stubs.QuitResponse)
				client.Call(stubs.QuitHandler, request, response)
				c.events <- StateChange{CompletedTurns: currentBoardState.CompletedTurns, NewState: Quitting}
				os.Exit(0)
			case 'p':
				mutex.Lock()
				pause = !pause
				mutex.Unlock()
				if pause == true {
					ticker.Stop()
					err := makeCallPause(client)
					if err != nil {
						panic(err)
					}
					c.events <- StateChange{CompletedTurns: currentBoardState.CompletedTurns, NewState: Paused}
				} else {
					err := makeCallResume(client)
					if err != nil {
						panic(err)
					}
					ticker.Reset(2 * time.Second)
					c.events <- StateChange{CompletedTurns: currentBoardState.CompletedTurns, NewState: Executing}
				}
			}
		}
	}()

	// Make the RPC call
	resultChan := make(chan stubs.BrokerResponse)
	go func() {
		c.events <- StateChange{CompletedTurns: 0, NewState: Executing}
		err5 := makeCall(client, world, p.Turns, p.ImageWidth, p.ImageHeight, resultChan)
		if err5 != nil {
			log.Println("RPC call failed:", err)
			return
		}
	}()

	newWorld := <-resultChan
	pgmFile(newWorld.NewBoard, c, p, p.Turns)
	var aliveCells []util.Cell
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			if newWorld.NewBoard[y][x] == 255 { // Check if the cell is alive
				aliveCells = append(aliveCells, util.Cell{X: x, Y: y})
			}
		}
	}

	c.events <- FinalTurnComplete{CompletedTurns: p.Turns, Alive: aliveCells}

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{p.Turns, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)

}
