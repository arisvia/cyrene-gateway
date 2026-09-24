package provider

import (
	"encoding/json"
	"testing"
)

func TestToFloat(t *testing.T) {
	cases := []struct {
		input any
		want  float64
	}{
		{input: 1.5, want: 1.5},
		{input: float32(2.5), want: 2.5},
		{input: 8, want: 8.0},
		{input: int64(10), want: 10.0},
		{input: json.Number("3.14"), want: 3.14},
		{input: "invalid", want: 0.0},
		{input: nil, want: 0.0},
	}

	for _, c := range cases {
		got := toFloat(c.input)
		if got != c.want {
			t.Errorf("toFloat(%v) = %f, want %f", c.input, got, c.want)
		}
	}
}
