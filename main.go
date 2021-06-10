package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/faiface/pixel"
	"github.com/faiface/pixel/imdraw"
	"github.com/faiface/pixel/pixelgl"
	"golang.org/x/image/colornames"
)

type Cell struct {
	x int
	y int
}

const PROCESS_FREQUENCY_MILLISECONDS = 50
const WORKER_COUNT = 4
const WINDOW_WIDTH = 1000
const WINDOW_HEIGHT = 1000

func getStartingCells() (map[string]*Cell, error) {
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
		fmt.Fprintln(os.Stderr, "reading standard input:", err)
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

	// Draw initial pattern
	if !win.Closed() {
		draw(cellMap, win)
		win.Update()
	}

	time.Sleep(2 * time.Second)

	startLoop(cellMap, win)
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

func startLoop(cellMap map[string]*Cell, win *pixelgl.Window) {

	newMaps := make(chan map[string]*Cell, 1)

	go startProcessLoop(cellMap, newMaps)

	for !win.Closed() {
		select {
		case newMap := <-newMaps:
			win.Clear(colornames.Black)
			draw(newMap, win)
		default:
			// pass
		}

		win.Update()
	}
}

func startProcessLoop(startingMap map[string]*Cell, maps chan map[string]*Cell) {
	nextMapToProcess := startingMap
	for {
		time.Sleep(PROCESS_FREQUENCY_MILLISECONDS * time.Millisecond)
		newMap := getNewCellMap(nextMapToProcess)

		// Copy to prevent map write during iteration
		nextMapToProcess = copyCellMap(newMap)
		maps <- copyCellMap(newMap)
	}
}

func draw(cellMap map[string]*Cell, win *pixelgl.Window) {
	cellSpacer := 5
	cellSize := 10
	widthOffset := WINDOW_WIDTH / 2
	heightOffset := WINDOW_HEIGHT / 2

	for _, cell := range cellMap {
		cellCenterX := (cell.x * cellSize) + widthOffset
		cellCenterY := (cell.y * cellSize) + heightOffset

		imd := imdraw.New(nil)

		imd.Push(pixel.V(float64(cellCenterX-cellSpacer), float64(cellCenterY-cellSpacer)))
		imd.Push(pixel.V(float64(cellCenterX+cellSpacer), float64(cellCenterY-cellSpacer)))
		imd.Push(pixel.V(float64(cellCenterX+cellSpacer), float64(cellCenterY+cellSpacer)))
		imd.Push(pixel.V(float64(cellCenterX-cellSpacer), float64(cellCenterY+cellSpacer)))
		imd.Polygon(0)

		imd.Draw(win)
	}
}

func getNewCellMap(currentMap map[string]*Cell) map[string]*Cell {
	cellCount := len(currentMap)
	newMap := make(map[string]*Cell)

	if cellCount < WORKER_COUNT {
		chunks := make([]*Cell, cellCount)
		i := 0
		for _, val := range currentMap {
			chunks[i] = val
			i++
		}

		processCells(currentMap, newMap, chunks)
	} else {
		chunks := chunkCells(currentMap, WORKER_COUNT)
		totalChunks := len(chunks)

		processedChunkMaps := make(chan map[string]*Cell, totalChunks)
		for _, chunk := range chunks {
			go processChunk(currentMap, chunk, processedChunkMaps)
		}

		completeChunks := 0
		for completeChunks < totalChunks {
			select {
			case chunkMap := <-processedChunkMaps:
				for k, v := range chunkMap {
					newMap[k] = v
				}
				completeChunks++
			case <-time.After(PROCESS_FREQUENCY_MILLISECONDS * time.Millisecond):
				panic("Took to long to process cells")
			}
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
