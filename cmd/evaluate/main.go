package main

import (
	"fmt"
	"syscall/js"

	"github.com/lk16/flippy/api/internal/models"
	"github.com/lk16/flippy/api/internal/wasm/evaluate"
)

// evaluatePosition is a dummy implementation that always returns 69
func evaluatePosition(this js.Value, args []js.Value) interface{} {

	if len(args) != 2 {
		fmt.Println("evaluatePosition called with wrong number of args")
		return map[string]interface{}{
			"score": 0,
		}
	}

	if args[0].Type() != js.TypeString {
		fmt.Println("evaluatePosition called with wrong type of arg 0")
		return map[string]interface{}{
			"score": 0,
		}
	}

	if args[1].Type() != js.TypeNumber {
		fmt.Println("evaluatePosition called with wrong type of arg 1")
		return map[string]interface{}{
			"score": 0,
		}
	}

	positionString := args[0].String()
	depth := args[1].Int()

	nPos, err := models.NewNormalizedPositionFromString(positionString)
	if err != nil {
		fmt.Println("error creating normalized position:", err)
		return map[string]interface{}{
			"score": 0,
		}
	}

	return map[string]interface{}{
		"score": evaluate.Evaluate(nPos, depth),
	}
}

func main() {
	// Register the evaluatePosition function in the global scope
	js.Global().Set("evaluatePosition", js.FuncOf(evaluatePosition))

	// Keep the program running
	select {}
}
