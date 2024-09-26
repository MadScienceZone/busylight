package readerboard

import (
	"bytes"
	"testing"
)

func TestBaudRateCode(t *testing.T) {
	for i, tcase := range []struct {
		input         byte
		expected      int
		errorExpected bool
	}{
		{'0', 300, false},
		{'3', 2400, false},
		{'C', 115200, false},
		{'D', 0, true},
	} {
		actual, err := parseBaudRateCode(tcase.input)
		if tcase.errorExpected {
			if err == nil {
				t.Fatalf("test case %d: expected error but got none", i)
			}
			continue
		}
		if err != nil {
			t.Fatalf("test case %d: %v", i, err)
		}
		if actual != tcase.expected {
			t.Fatalf("test case %d: input %c, expected %d but got %d", i, tcase.input, tcase.expected, actual)
		}
	}
}

func TestParseLEDSequence(t *testing.T) {
	for i, tcase := range []struct {
		input         []byte
		expected      LEDSequence
		errorExpected bool
	}{
		{[]byte("S_"), LEDSequence{IsRunning: false, Position: 0, Sequence: []byte{}}, false},
		{[]byte("R_"), LEDSequence{IsRunning: true, Position: 0, Sequence: []byte{}}, false},
		{[]byte("R0@ABC"), LEDSequence{IsRunning: true, Position: 0, Sequence: []byte("ABC")}, false},
		{[]byte("R3@ABC"), LEDSequence{IsRunning: true, Position: 3, Sequence: []byte("ABC")}, false},
		{[]byte("S<@ABC"), LEDSequence{IsRunning: false, Position: 12, Sequence: []byte("ABC")}, false},
		{[]byte("S<ABC"), LEDSequence{IsRunning: false, Position: 12, Sequence: []byte("ABC")}, true},
		{nil, LEDSequence{IsRunning: false, Position: 12, Sequence: []byte("ABC")}, true},
	} {
		actual, err := parseLEDSequence(tcase.input)
		if tcase.errorExpected {
			if err == nil {
				t.Fatalf("test case %d: expected error but got none", i)
			}
			continue
		}
		if err != nil {
			t.Fatalf("test case %d: %v", i, err)
		}
		if actual.IsRunning != tcase.expected.IsRunning ||
			actual.Position != tcase.expected.Position ||
			!bytes.Equal(actual.Sequence, tcase.expected.Sequence) {
			t.Fatalf("test case %d: expected %v but got %v", i, tcase.expected, actual)
		}
	}
}

func TestParseLightList(t *testing.T) {
	for i, tcase := range []struct {
		input         []byte
		expected      []byte
		errorExpected bool
	}{
		{[]byte("ABC"), []byte("ABC"), false},
		{nil, []byte(""), false},
		{[]byte("AAa"), []byte("AAa"), false},
		{[]byte("0"), []byte("0"), false},
	} {
		actual, err := parseLightList(tcase.input)
		if tcase.errorExpected {
			if err == nil {
				t.Fatalf("test case %d: expected error but got none", i)
			}
			continue
		}
		if err != nil {
			t.Fatalf("test case %d: %v", i, err)
		}
		if !bytes.Equal(actual, tcase.expected) {
			t.Fatalf("test case %d: expected %v but got %v", i, tcase.expected, actual)
		}
	}
}
