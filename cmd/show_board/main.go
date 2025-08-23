package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/lk16/flippy/api/internal/models"
)

func main() {
	boardString := flag.String("board", "", "the board to show")
	flag.Parse()

	board, err := models.NewBoardFromString(*boardString)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	board.Print()
}
