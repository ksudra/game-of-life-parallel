package gol

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

type GameOfLife struct {
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels, keyPresses <-chan rune) {

	world := buildWorld(p, c)
	ticker := time.NewTicker(2 * time.Second)
	finished := false
	// TODO: Create a 2D slice to store the world.

	turn := 0

	for turn = 0; turn < p.Turns; turn++ {
		go func() {
			for range ticker.C {
				alive := len(calculateAliveCells(world))
				c.events <- AliveCellsCount{
					CompletedTurns: turn,
					CellsCount:     alive,
				}
			}
		}()

		world = calculateNextState(p, world, turn, c)
		c.events <- TurnComplete{
			CompletedTurns: turn,
		}

		select {
		case input := <-keyPresses:
			switch input {
			case 's':
				sendWorld(p, c, world, turn)
			case 'q':
				sendWorld(p, c, world, turn)
				finished = true
			case 'p':
				c.events <- StateChange{
					CompletedTurns: turn,
					NewState:       Paused,
				}
				fmt.Printf("Current turn: %d\n", turn)
				ticker.Stop()
				for {
					input = <-keyPresses
					if input == 'p' {
						fmt.Println("Continuing")
						c.events <- StateChange{
							CompletedTurns: turn,
							NewState:       Executing,
						}
						ticker = time.NewTicker(2 * time.Second)
						break
					}
				}
			}
		default:
			break
		}

		if finished {
			break
		}
	}

	// TODO: Execute all turns of the Game of Life.
	sendWorld(p, c, world, turn)

	c.events <- FinalTurnComplete{
		CompletedTurns: turn,
		Alive:          calculateAliveCells(world),
	}
	ticker.Stop()

	c.events <- ImageOutputComplete{
		CompletedTurns: turn,
		Filename:       strings.Join([]string{strconv.Itoa(p.ImageWidth), strconv.Itoa(p.ImageHeight), strconv.Itoa(turn)}, "x"),
	}

	// TODO: Report the final state using FinalTurnCompleteEvent.

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

func buildWorld(p Params, c distributorChannels) [][]uint8 {
	c.ioCommand <- ioInput
	c.ioFilename <- strings.Join([]string{strconv.Itoa(p.ImageWidth), strconv.Itoa(p.ImageHeight)}, "x")

	world := make([][]uint8, p.ImageHeight)
	for y := range world {
		world[y] = make([]uint8, p.ImageWidth)
		for x := range world[y] {
			world[y][x] = <-c.ioInput
			if world[y][x] == 255 {
				c.events <- CellFlipped{
					CompletedTurns: 0,
					Cell:           util.Cell{X: x, Y: y},
				}
			}
		}
	}

	return world
}

func sendWorld(p Params, c distributorChannels, world [][]uint8, turn int) {
	c.ioCommand <- ioOutput
	c.ioFilename <- strings.Join([]string{strconv.Itoa(p.ImageWidth), strconv.Itoa(p.ImageHeight), strconv.Itoa(turn)}, "x")
	for y := range world {
		for x := range world[y] {
			c.ioOutput <- world[y][x]
		}
	}
}

func countNeighbours(p Params, x int, y int, world [][]uint8) int {
	neighbours := [8][2]int{
		{-1, -1},
		{-1, 0},
		{-1, 1},
		{0, -1},
		{0, 1},
		{1, -1},
		{1, 0},
		{1, 1},
	}

	count := 0

	for _, r := range neighbours {
		if world[(y+r[0]+p.ImageHeight)%p.ImageHeight][(x+r[1]+p.ImageWidth)%p.ImageWidth] == 255 {
			count++
		}
	}

	return count
}

func calculateNextState(p Params, world [][]uint8, turn int, c distributorChannels) [][]uint8 {
	tempWorld := make([][]uint8, len(world))
	for i := range world {
		tempWorld[i] = make([]uint8, len(world[i]))
		copy(tempWorld[i], world[i])
	}

	var wg sync.WaitGroup
	var remainder sync.WaitGroup

	for i := 0; i < p.Threads; i++ {
		start := i * (p.ImageHeight - p.ImageHeight%p.Threads) / p.Threads
		end := start + (p.ImageHeight-p.ImageHeight%p.Threads)/p.Threads
		wg.Add(1)
		go worker(&wg, start, end, p, tempWorld, world, turn, c)

	}
	wg.Wait()

	if p.ImageHeight%p.Threads > 0 {
		start := p.ImageHeight - p.ImageHeight%p.Threads
		remainder.Add(1)
		go worker(&remainder, start, p.ImageHeight, p, tempWorld, world, turn, c)
	}

	remainder.Wait()

	return tempWorld
}

func worker(wg *sync.WaitGroup, start int, end int, p Params, newWorld [][]uint8, world [][]uint8, turn int, c distributorChannels) {
	defer wg.Done()

	for y := start; y < end; y++ {
		for x := range newWorld {
			count := countNeighbours(p, x, y, world)

			if world[y][x] == 255 && (count < 2 || count > 3) {
				newWorld[y][x] = 0
				c.events <- CellFlipped{
					CompletedTurns: turn,
					Cell:           util.Cell{X: x, Y: y},
				}
			} else if world[y][x] == 0 && count == 3 {
				newWorld[y][x] = 255
				c.events <- CellFlipped{
					CompletedTurns: turn,
					Cell:           util.Cell{X: x, Y: y},
				}
			}
		}
	}
}

func calculateAliveCells(world [][]uint8) []util.Cell {
	var cells []util.Cell
	for y := range world {
		for x := range world[y] {
			if world[y][x] == 255 {
				cells = append(cells, util.Cell{X: x, Y: y})
			}
		}
	}
	return cells
}
