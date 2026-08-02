package othello

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseField(t *testing.T) {
	tests := []struct {
		field   string
		want    int
		wantErr bool
	}{
		{field: "a1", want: 0},
		{field: "h1", want: 7},
		{field: "a8", want: 56},
		{field: "h8", want: 63},
		{field: "d3", want: 19},
		{field: "D3", want: 19},
		{field: "--", want: PassMove},
		{field: "ps", want: PassMove},
		{field: "PA", want: PassMove},
		{field: "i1", wantErr: true},
		{field: "a9", wantErr: true},
		{field: "a", wantErr: true},
		{field: "abc", wantErr: true},
		{field: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			got, err := ParseField(tt.field)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
