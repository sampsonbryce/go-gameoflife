package main

import (
	"bufio"
	"flag"
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/faiface/pixel"
	"github.com/faiface/pixel/imdraw"
	"github.com/faiface/pixel/pixelgl"
	"golang.org/x/image/colornames"
)

const PROCESS_FREQUENCY_MILLISECONDS = 50
const WORKER_COUNT = 4
const WINDOW_WIDTH = 1000
const WINDOW_HEIGHT = 1000
const CELL_SIZE = 20

type Cell struct {
	x int
	y int
}

type Viewport struct {
	offsetX float64
	offsetY float64
}

func (v *Viewport) inView(state *GameState, x int, y int) bool {
	positionX := float64(x) * state.cellSize
	positionY := float64(y) * state.cellSize
	if math.Abs(float64(positionX-v.offsetX)) > (WINDOW_WIDTH / 2) {
		return false
	}

	if math.Abs(float64(positionY-v.offsetY)) > (WINDOW_HEIGHT / 2) {
		return false
	}

	return true
}

type GameState struct {
	paused   bool
	viewport *Viewport
	window   *pixelgl.Window
	cellSize float64
	gridDraw *imdraw.IMDraw
	cellDraw *imdraw.IMDraw
}

func createGameState(window *pixelgl.Window) *GameState {
	return &GameState{
		paused:   true,
		viewport: &Viewport{offsetX: 0, offsetY: 0},
		window:   window,
		cellSize: CELL_SIZE,
		gridDraw: imdraw.New(nil),
		cellDraw: imdraw.New(nil),
	}
}

func getStartingCells() (map[string]*Cell, error) {
	isPatternInput := flag.Bool("pattern", false, "input is a pattern from gameoflife wiki")

	flag.Parse()

	if *isPatternInput {
		return readPattern()
	} else {
		return readCoordinates()
	}
}

func readPattern() (map[string]*Cell, error) {
	cells := make(map[string]*Cell)

	line := 0
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		text := scanner.Text()

		if strings.HasPrefix(text, "!") { // Comment
			continue
		}

		i := 0
		for _, char := range text {
			if string(char) == "O" {
				key := getCellKey(i, line)
				cells[key] = &Cell{x: i, y: line}
			}

			i++
		}

		line -= 1
	}

	if err := scanner.Err(); err != nil {
		return cells, err
	}

	return cells, nil
}

func readCoordinates() (map[string]*Cell, error) {

	cells := make(map[string]*Cell)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		text := scanner.Text()

		coords := strings.Split(strings.TrimSpace(text), " ")

		x, err := strconv.Atoi(coords[0])

		if err != nil {
			return cells, err
		}

		y, err := strconv.Atoi(coords[1])

		if err != nil {
			return cells, err
		}

		key := getCellKey(x, y)
		if _, ok := cells[key]; ok {
			return cells, fmt.Errorf("cell at coordinates %s already exists", key)
		}

		cells[key] = &Cell{x: x, y: y}
	}

	if err := scanner.Err(); err != nil {
		return cells, err
	}

	return cells, nil
}

func run(cellMap map[string]*Cell) {
	cfg := pixelgl.WindowConfig{
		Title:  "Game Of Life",
		Bounds: pixel.R(0, 0, WINDOW_WIDTH, WINDOW_HEIGHT),
		VSync:  true,
	}

	win, err := pixelgl.NewWindow(cfg)
	if err != nil {
		panic(err)
	}

	state := createGameState(win)

	// Draw initial pattern
	if !win.Closed() {
		draw(cellMap, state)
		win.Update()
	}

	startLoop(cellMap, state)
}

func main() {
	cellMap, err := getStartingCells()

	if err != nil {
		log.Fatal(err)
	}

	pixelgl.Run(func() {
		run(cellMap)
	})
}

func startLoop(cellMap map[string]*Cell, state *GameState) {
	newMaps := make(chan map[string]*Cell, 1)
	currentMap := cellMap // Should only be used for drawing. Don't write

	go startProcessLoop(cellMap, newMaps, state)

	for !state.window.Closed() {
		select {
		case newMap := <-newMaps:
			draw(newMap, state)
			currentMap = newMap
		default:
			// no new map
		}

		if state.window.JustPressed(pixelgl.KeyEnter) {
			state.paused = !state.paused
		}

		if state.window.JustPressed(pixelgl.KeyUp) {
			state.viewport.offsetY += math.Max(state.cellSize, 20)
			draw(currentMap, state)
		}

		if state.window.JustPressed(pixelgl.KeyDown) {
			state.viewport.offsetY -= math.Max(state.cellSize, 20)
			draw(currentMap, state)
		}

		if state.window.JustPressed(pixelgl.KeyLeft) {
			state.viewport.offsetX -= math.Max(state.cellSize, 20)
			draw(currentMap, state)
		}

		if state.window.JustPressed(pixelgl.KeyRight) {
			state.viewport.offsetX += math.Max(state.cellSize, 20)
			draw(currentMap, state)
		}

		// Zoom in
		if state.window.JustPressed(pixelgl.KeyI) {
			if state.cellSize < 50 {
				state.cellSize = state.cellSize * 2
				draw(currentMap, state)
			}
		}

		// Zoom out
		if state.window.JustPressed(pixelgl.KeyO) {
			if state.cellSize > 2 {
				state.cellSize = state.cellSize / 2
				draw(currentMap, state)
			}
		}

		state.window.Update()
	}
}

func startProcessLoop(startingMap map[string]*Cell, maps chan map[string]*Cell, state *GameState) {
	nextMapToProcess := startingMap

	for {
		time.Sleep(PROCESS_FREQUENCY_MILLISECONDS * time.Millisecond)

		if state.paused {
			continue
		}

		if nextMapToProcess == nil {
			continue
		}

		toProcess := nextMapToProcess
		nextMapToProcess = nil // Prevents double processing

		newMap := getNewCellMap(toProcess)

		// Copy to prevent map write during iteration
		nextMapToProcess = copyCellMap(newMap)
		maps <- copyCellMap(newMap)
	}
}

func draw(cellMap map[string]*Cell, state *GameState) {
	cellSpacer := state.cellSize / 2
	widthOffset := (WINDOW_WIDTH / 2) - (state.viewport.offsetX)
	heightOffset := (WINDOW_HEIGHT / 2) - (state.viewport.offsetY)

	state.window.Clear(colornames.Black)

	imd := state.cellDraw
	imd.Clear()

	for _, cell := range cellMap {
		if !state.viewport.inView(state, cell.x, cell.y) {
			continue
		}

		cellCenterX := (float64(cell.x) * state.cellSize) + widthOffset
		cellCenterY := (float64(cell.y) * state.cellSize) + heightOffset

		imd.Push(pixel.V(float64(cellCenterX-cellSpacer), float64(cellCenterY-cellSpacer)))
		imd.Push(pixel.V(float64(cellCenterX+cellSpacer), float64(cellCenterY-cellSpacer)))
		imd.Push(pixel.V(float64(cellCenterX+cellSpacer), float64(cellCenterY+cellSpacer)))
		imd.Push(pixel.V(float64(cellCenterX-cellSpacer), float64(cellCenterY+cellSpacer)))
		imd.Polygon(0)

	}

	imd.Draw(state.window)

	drawGrid(state)
}

func drawGrid(state *GameState) {
	imd := state.gridDraw
	imd.Clear()

	for i := state.cellSize / 2; i < WINDOW_WIDTH; i += state.cellSize {
		imd.Color = color.RGBA{0x30, 0x30, 0x30, 0xFF} // rgb(48, 48, 48)

		imd.Push(pixel.V(float64(i), 0))
		imd.Push(pixel.V(float64(i), WINDOW_HEIGHT))

		imd.Line(1)
	}

	for i := state.cellSize / 2; i < WINDOW_HEIGHT; i += state.cellSize {
		imd.Color = colornames.Gray

		imd.Push(pixel.V(0, float64(i)))
		imd.Push(pixel.V(WINDOW_WIDTH, float64(i)))

		imd.Line(1)
	}

	imd.Draw(state.window)
}

func getNewCellMap(currentMap map[string]*Cell) map[string]*Cell {
	cellCount := len(currentMap)
	newMap := make(map[string]*Cell)

	if cellCount < WORKER_COUNT {
		chunk := make([]*Cell, cellCount)
		i := 0
		for _, val := range currentMap {
			chunk[i] = val
			i++
		}

		processCells(currentMap, newMap, chunk)
	} else {
		chunks := chunkCells(currentMap, WORKER_COUNT)
		totalChunks := len(chunks)

		processedChunkMaps := make(chan map[string]*Cell, totalChunks)
		for _, chunk := range chunks {
			go processChunk(currentMap, chunk, processedChunkMaps)
		}

		completeChunks := 0
		for completeChunks < totalChunks {
			chunkMap := <-processedChunkMaps
			for k, v := range chunkMap {
				newMap[k] = v
			}
			completeChunks++
		}
	}

	return newMap
}

func processChunk(currentMap map[string]*Cell, currentChunk []*Cell, processedChunk chan map[string]*Cell) {
	chunkMap := make(map[string]*Cell)

	processCells(currentMap, chunkMap, currentChunk)

	processedChunk <- chunkMap
}

func processCells(currentMap map[string]*Cell, newMap map[string]*Cell, cells []*Cell) {
	for _, cell := range cells {
		// Due to round robin during chunking, some slice indexes might be 0
		if cell == nil {
			continue
		}

		if !shouldKillCell(currentMap, cell.x, cell.y) {
			key := getCellKey(cell.x, cell.y)
			newMap[key] = cell
		}

		reviveCells(currentMap, newMap, cell)
	}
}

func reviveCells(currentMap map[string]*Cell, newMap map[string]*Cell, cell *Cell) {
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			// Skip center
			if i == 0 && j == 0 {
				continue
			}

			currentX := cell.x + i
			currentY := cell.y + j

			key := getCellKey(currentX, currentY)

			// Already was alive
			if _, ok := currentMap[key]; ok {
				continue
			}

			// Already is alive in the new set, don't need to check
			if _, ok := newMap[key]; ok {
				continue
			}

			if shouldReviveCell(currentMap, currentX, currentY) {
				newMap[key] = &Cell{x: currentX, y: currentY}
			}
		}
	}
}

func shouldKillCell(cellMap map[string]*Cell, x int, y int) bool {
	neighborCount := getCellNeighborCount(cellMap, x, y)

	if neighborCount < 2 {
		return true
	}

	if neighborCount > 3 {
		return true
	}

	return false
}

func shouldReviveCell(cellMap map[string]*Cell, x int, y int) bool {
	neighborCount := getCellNeighborCount(cellMap, x, y)

	return neighborCount == 3
}

func getCellNeighborCount(cellMap map[string]*Cell, x int, y int) int {
	count := 0

	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			// Skip center
			if i == 0 && j == 0 {
				continue
			}

			currentX := x + i
			currentY := y + j

			key := getCellKey(currentX, currentY)
			if _, ok := cellMap[key]; ok {
				count++
			}
		}
	}

	return count
}

func chunkCells(cellMap map[string]*Cell, chunkCount int) [][]*Cell {
	cellCount := len(cellMap)
	chunkSize := (cellCount / chunkCount) + 1 // Add one to allow for remainder
	chunks := make([][]*Cell, chunkCount)

	chunkIndices := make([]int, chunkCount)
	for i := range chunkIndices {
		chunkIndices[i] = 0
	}

	for i := 0; i < chunkCount; i++ {
		chunks[i] = make([]*Cell, chunkSize)
	}

	currentChunk := 0
	for _, val := range cellMap {
		chunk := chunks[currentChunk]
		chunkIndex := chunkIndices[currentChunk]

		chunk[chunkIndex] = val

		chunkIndices[currentChunk]++
		currentChunk++
		currentChunk %= chunkCount
	}

	return chunks
}

func getCellKey(x int, y int) string {
	key := fmt.Sprintf("%d,%d", x, y)
	return key
}

func copyCellMap(cellMap map[string]*Cell) map[string]*Cell {
	newMap := make(map[string]*Cell)

	for k, v := range cellMap {
		newMap[k] = v
	}

	return newMap
}

func printCellMap(cellMap map[string]*Cell) {
	for k, v := range cellMap {
		fmt.Printf("%s = %v\n", k, v)
	}
}

func isMapEqual(map1 map[string]*Cell, map2 map[string]*Cell) bool {
	for k, v := range map1 {
		if val, ok := map2[k]; ok {
			if val != v {
				return false
			}
		} else {
			// Missing key
			return false
		}
	}

	return true
}
