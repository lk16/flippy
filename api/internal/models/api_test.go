package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBestMovesScan(t *testing.T) {
	tests := []struct {
		name       string
		input      interface{}
		wantErr    bool
		wantErrMsg string
		wantMoves  BestMoves
	}{
		{
			name:      "OK",
			input:     []byte("{1,2,3}"),
			wantErr:   false,
			wantMoves: BestMoves{1, 2, 3},
		},
		{
			name:       "InvalidType",
			input:      123, // passing an int instead of []byte
			wantErr:    true,
			wantErrMsg: "cannot scan int into BestMoves",
		},
		{
			name:       "NilBytes",
			input:      []byte(nil),
			wantErr:    true,
			wantErrMsg: "cannot scan nil into BestMoves",
		},
		{
			name:       "BrokenInt",
			input:      []byte("{1,abc,3}"),
			wantErr:    true,
			wantErrMsg: "cannot convert abc to int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var moves BestMoves
			err := moves.Scan(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrMsg != "" {
					if tt.name == "BrokenInt" {
						assert.Contains(t, err.Error(), tt.wantErrMsg)
					} else {
						assert.Equal(t, tt.wantErrMsg, err.Error())
					}
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantMoves, moves)
			}
		})
	}
}
