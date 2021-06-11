# Game of life

[Conway's Game of Life](https://en.wikipedia.org/wiki/Conway%27s_Game_of_Life)

## Usage

By default, the program reads coordinates of cells

```
go run .
1 1
1 2
1 3
^D
```

You can pipe a test file with coordinates into the program

```bash
cat test.txt | go run .
```

You can also pipe a pattern file with a plaintext pattern (like those found on the game of life wiki) with the `-pattern` flag

Example [wiki page](https://www.conwaylife.com/wiki/Breeder_1)

Example [pattern file](https://www.conwaylife.com/patterns/breeder1.cells)

```bash
cat pattern.txt | go run . -pattern
```

### Commands

- Press enter to start and stop the game
- Move the view around with the arrow keys
- Use `i` and `o` to zoom in and zoom out respectively
