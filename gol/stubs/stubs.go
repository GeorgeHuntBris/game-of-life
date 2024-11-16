package stubs

var BrokerHandler = "Broker.Broker"
var CurrentBoardStateHandler = "Broker.CurrentBoardState"
var PauseHandler = "Broker.Pause"
var ResumeHandler = "Broker.Resume"
var QuitHandler = "Broker.Quit"
var EvolveHandler = "Server.Evolve"
var QuitServerHandler = "Server.QuitServer"

type BrokerRequest struct {
	BoardWidth  int
	BoardHeight int
	Board       [][]uint8
	Turns       int
}

type BrokerResponse struct {
	NewBoard [][]uint8
}

type EvolveRequest struct {
	BoardWidth  int
	BoardHeight int
	Board       [][]uint8
	StartY      int
	EndY        int
}

type EvolveResponse struct {
	NewBoard [][]uint8
}

type CurrentBoardStateRequest struct{}

type CurrentBoardStateResponse struct {
	Board          [][]uint8
	CompletedTurns int
	AliveCount     int
}

type PauseRequest struct{}
type PauseResponse struct{}

type ResumeRequest struct{}
type ResumeResponse struct{}

type QuitRequest struct{}
type QuitResponse struct{}

type QuitServerRequest struct{}
type QuitServerResponse struct{}
