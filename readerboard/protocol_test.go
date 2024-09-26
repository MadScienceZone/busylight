package readerboard

import (
	"net/url"
	"testing"
)

func TestEscape485(t *testing.T) {
	for i, tcase := range []struct {
		input    []byte
		expected []byte
	}{
		{[]byte{0x00}, []byte{0x00}},
		{[]byte{0x01}, []byte{0x01}},
		{[]byte{0x7d}, []byte{0x7d}},
		{[]byte{0x7e}, []byte{0x7f, 0x7e}},
		{[]byte{0x7f}, []byte{0x7f, 0x7f}},
		{[]byte{0x80}, []byte{0x7e, 0x00}},
		{[]byte{0x81}, []byte{0x7e, 0x01}},
		{[]byte{0xfd}, []byte{0x7e, 0x7d}},
		{[]byte{0xfe}, []byte{0x7e, 0x7e}},
		{[]byte{0xff}, []byte{0x7e, 0x7f}},
	} {
		actual := Escape485(tcase.input)
		if len(actual) != len(tcase.expected) {
			t.Fatalf("test case %d: output %v length %d, expected %v (length %d)", i, actual, len(actual), tcase.expected, len(tcase.expected))
		}
		for j := 0; j < len(actual); j++ {
			if actual[j] != tcase.expected[j] {
				t.Fatalf("test case %d: output differs at byte %d (%d vs %d); actual %v, expected %v",
					i, j, actual[j], tcase.expected[j], actual, tcase.expected)
			}
		}
	}
}

func TestUnescape485(t *testing.T) {
	for i, tcase := range []struct {
		input    []byte
		expected []byte
	}{
		{[]byte{0x00}, []byte{0x00}},
		{[]byte{0x01}, []byte{0x01}},
		{[]byte{0x7d}, []byte{0x7d}},
		{[]byte{0x7f, 0x7e}, []byte{0x7e}},
		{[]byte{0x7f, 0x7f}, []byte{0x7f}},
		{[]byte{0x7e, 0x00}, []byte{0x80}},
		{[]byte{0x7e, 0x01}, []byte{0x81}},
		{[]byte{0x7e, 0x7d}, []byte{0xfd}},
		{[]byte{0x7e, 0x7e}, []byte{0xfe}},
		{[]byte{0x7e, 0x7f}, []byte{0xff}},
	} {
		actual := Unescape485(tcase.input)
		if len(actual) != len(tcase.expected) {
			t.Fatalf("test case %d: output %v length %d, expected %v (length %d)", i, actual, len(actual), tcase.expected, len(tcase.expected))
		}
		for j := 0; j < len(actual); j++ {
			if actual[j] != tcase.expected[j] {
				t.Fatalf("test case %d: output differs at byte %d (%d vs %d); actual %v, expected %v",
					i, j, actual[j], tcase.expected[j], actual, tcase.expected)
			}
		}
	}
}

func TestHTTPCommands(t *testing.T) {
	for i, tcase := range []struct {
		handler            func(url.Values, HardwareModel) ([]byte, error)
		addrs              []int
		data               url.Values
		expected485        []byte
		expectedUSB        []byte
		expectError        bool
		expectNetworkError bool
		model              HardwareModel
	}{
		// 0
		{AllLightsOff, []int{0}, nil, []byte{0x80}, []byte("C\004X\004"), false, false, Readerboard3RGB},
		{AllLightsOff, []int{42}, nil, []byte{0x80}, []byte("C\004X\004"), false, true, Readerboard3RGB},
		{AllLightsOff, []int{2}, nil, []byte{0x82}, []byte("C\004X\004"), false, false, Readerboard3RGB},
		// 3
		{Bitmap, []int{1}, map[string][]string{"merge": []string{""}, "pos": []string{"0"}, "trans": []string{">"},
			"image": []string{"12345678$a1a2a3a4$abcdef01$0000"}}, []byte("\x91IM0>12345678$a1a2a3a4$abcdef01$0000$"), []byte("IM0>12345678$a1a2a3a4$abcdef01$0000$\004"), false, false, Readerboard3RGB},
		{Bitmap, []int{1}, map[string][]string{"merge": []string{""}, "pos": []string{"0"}, "trans": []string{">"},
			"image": []string{"12345678$a1a2a3a4$abcdef010000"}}, []byte("\x91IM0>12345678$a1a2a3a4$abcdef010000$"), []byte("IM0>12345678$a1a2a3a4$abcdef010000$\004"), true, false, Readerboard3RGB},
		{Bitmap, []int{1}, map[string][]string{"merge": []string{""}, "pos": []string{"0"}, "trans": []string{">"},
			"image": []string{"12345678$a1a2a3a4$"}}, []byte("\x91IM0>12345678$a1a2a3a4$abcdef01$$"), []byte("IM0>12345678$a1a2a3a4$abcdef01$$\004"), true, false, Readerboard3RGB},
		{Bitmap, []int{1}, map[string][]string{"merge": []string{""}, "pos": []string{"0"}, "trans": []string{">"},
			"image": []string{"12345678a1a2a3a4$00"}}, []byte("\x91IM0>12345678$a1a2a3a4$abcdef01$00$"), []byte("IM0>12345678$a1a2a3a4$abcdef01$00$\004"), true, false, Readerboard3RGB},
		{Bitmap, []int{1}, map[string][]string{"merge": []string{""}, "pos": []string{"0"}, "trans": []string{">"},
			"image": []string{"12345678$a1a2a3a4$abcd$00"}}, []byte("\x91IM0>12345678$a1a2a3a4$abcd$00$"), []byte("IM0>12345678$a1a2a3a4$abcd$00$\004"), false, false, Readerboard3RGB},
		{Bitmap, []int{1}, map[string][]string{"merge": []string{""}, "pos": []string{"0"}, "trans": []string{">"},
			"image": []string{"12345678$ff"}}, []byte("\x91IM0>12345678$ff$"), []byte("IM0>12345678$ff$\004"), false, false, Readerboard3Mono},
		{Bitmap, []int{1}, map[string][]string{"merge": []string{"no"}, "pos": []string{"~"}, "trans": []string{"<"},
			"image": []string{"12345678$a1a2a3a4$abcdef01$ff"}}, []byte("\x91I.\x7f~<12345678$a1a2a3a4$abcdef01$ff$"), []byte("I.~<12345678$a1a2a3a4$abcdef01$ff$\004"), false, false, Readerboard3RGB},
		// 10
		{Clear, []int{2}, nil, []byte{0x92, 'C'}, []byte("C\004"), false, false, Readerboard3RGB},
		// 11
		{Off, []int{2}, nil, []byte{0x92, 'X'}, []byte("X\004"), false, false, Readerboard3RGB},
		// 12
		{Color, []int{2}, map[string][]string{"color": []string{"4"}}, []byte{0x92, 'K', '4'}, []byte("K4\004"), false, false, Readerboard3RGB},
		{Color, []int{2}, map[string][]string{"color": []string{"42"}}, []byte{0x92, 'K', '4'}, []byte("K4\004"), true, false, Readerboard3RGB},
		{Color, []int{2}, map[string][]string{"color": []string{""}}, []byte{0x92, 'K', '1'}, []byte("K1\004"), false, false, Readerboard3RGB},
		// 15
		{Flash, []int{5}, map[string][]string{"l": []string{"AB_C"}}, []byte{0x95, 'F', 'A', 'B', '_', 'C', '$'}, []byte("FAB_C$\004"), false, false, Readerboard3RGB},
		{Flash, []int{5, 23}, map[string][]string{"l": []string{"AB_C"}}, []byte{0xBF, 0x02, 5, 23, 'F', 'A', 'B', '_', 'C', '$'}, []byte("FAB_C$\004"), false, false, Readerboard3RGB},
		{Flash, []int{5, 23}, map[string][]string{"l": []string{"AB$C"}}, []byte{0xBF, 0x02, 5, 23, 'F', 'A', 'B', '_', 'C', '$'}, []byte("FAB_C$\004"), true, false, Readerboard3RGB},
		// 18
		{Font, []int{1}, map[string][]string{"idx": []string{"0"}}, []byte{0x91, 'A', '0'}, []byte("A0\004"), false, false, Readerboard3RGB},
		{Font, []int{1}, map[string][]string{"idx": []string{"1"}}, []byte{0x91, 'A', '1'}, []byte("A1\004"), false, false, Readerboard3RGB},
		{Font, []int{1}, map[string][]string{"idx": []string{"x"}}, []byte{0x91, 'A', '1'}, []byte("A1\004"), true, false, Readerboard3RGB},
		// 21
		{Graph, []int{1}, map[string][]string{"v": []string{"12"}}, []byte{0x91, 'H', '8'}, []byte("H8\004"), false, false, Readerboard3RGB},
		{Graph, []int{1}, map[string][]string{"v": []string{"2"}}, []byte{0x91, 'H', '2'}, []byte("H2\004"), false, false, Readerboard3RGB},
		{Graph, []int{1}, map[string][]string{"colors": []string{"2"}}, []byte{0x91, 'H', '2'}, []byte("H2\004"), true, false, Readerboard3RGB},
		{Graph, []int{1}, map[string][]string{"colors": []string{"2222222"}}, []byte{0x91, 'H', '2'}, []byte("H2\004"), true, false, Readerboard3RGB},
		{Graph, []int{1}, map[string][]string{"colors": []string{"222222222"}}, []byte{0x91, 'H', '2'}, []byte("H2\004"), true, false, Readerboard3RGB},
		{Graph, []int{1}, map[string][]string{"colors": []string{"12345671"}}, []byte("\x91HK12345671"), []byte("HK12345671\004"), false, false, Readerboard3RGB},
		// 27
		{Move, []int{1}, map[string][]string{"pos": []string{"o"}}, []byte{0x91, '@', 'o'}, []byte("@o\004"), false, false, Readerboard3RGB},
		{Move, []int{1}, map[string][]string{"pos": []string{"p"}}, []byte{0x91, '@', 'o'}, []byte("@o\004"), true, false, Readerboard3RGB},
		{Move, []int{1}, map[string][]string{"pos": []string{"!"}}, []byte{0x91, '@', 'o'}, []byte("@o\004"), true, false, Readerboard3RGB},
		{Move, []int{1}, map[string][]string{"pos": []string{"~"}}, []byte{0x91, '@', 0x7f, '~'}, []byte("@~\004"), false, false, Readerboard3RGB},
		{Move, []int{1}, map[string][]string{"pos": []string{">"}}, []byte{0x91, '@', '>'}, []byte("@>\004"), false, false, Readerboard3RGB},
		// 32
		{Light, []int{5}, map[string][]string{"l": []string{"A"}}, []byte{0x95, 'S', 'A'}, []byte("SA\004"), false, false, Readerboard3RGB},
		{Light, []int{5}, map[string][]string{"l": []string{"AB_C"}}, []byte{0x95, 'L', 'A', 'B', '_', 'C', '$'}, []byte("LAB_C$\004"), false, false, Readerboard3RGB},
		{Light, []int{5, 23}, map[string][]string{"l": []string{"AB_C"}}, []byte{0xBF, 0x02, 5, 23, 'L', 'A', 'B', '_', 'C', '$'}, []byte("LAB_C$\004"), false, false, Readerboard3RGB},
		// 35
		{Strobe, []int{5}, map[string][]string{"l": []string{"AB_C"}}, []byte{0x95, '*', 'A', 'B', '_', 'C', '$'}, []byte("*AB_C$\004"), false, false, Readerboard3RGB},
		{Strobe, []int{5, 23}, map[string][]string{"l": []string{"AB_C"}}, []byte{0xBF, 0x02, 5, 23, '*', 'A', 'B', '_', 'C', '$'}, []byte("*AB_C$\004"), false, false, Readerboard3RGB},
		// 37
		{Scroll, []int{14}, map[string][]string{"loop": []string{"false"}, "t": []string{"Hello, $World!"}}, []byte{0x9e, '<', '.', 'H', 'e', 'l', 'l', 'o', ',', ' ', '$', 'W', 'o', 'r', 'l', 'd', '!', 0x1b}, []byte("<.Hello, $World!\033\004"), false, false, Readerboard3RGB},
		{Scroll, []int{14}, map[string][]string{"loop": []string{""}, "t": []string{"Hello, $World!"}}, []byte{0x9e, '<', 'L', 'H', 'e', 'l', 'l', 'o', ',', ' ', '$', 'W', 'o', 'r', 'l', 'd', '!', 0x1b}, []byte("<LHello, $World!\033\004"), false, false, Readerboard3RGB},
		{Scroll, []int{14}, map[string][]string{"loop": []string{"true"}, "t": []string{"Hello, $World!"}}, []byte{0x9e, '<', 'L', 'H', 'e', 'l', 'l', 'o', ',', ' ', '$', 'W', 'o', 'r', 'l', 'd', '!', 0x1b}, []byte("<LHello, $World!\033\004"), false, false, Readerboard3RGB},
		{Scroll, []int{14}, map[string][]string{"t": []string{"Hello, $World!"}}, []byte{0x9e, '<', '.', 'H', 'e', 'l', 'l', 'o', ',', ' ', '$', 'W', 'o', 'r', 'l', 'd', '!', 0x1b}, []byte("<.Hello, $World!\033\004"), false, false, Readerboard3RGB},
		{Scroll, []int{14}, map[string][]string{"t": []string{"Hello, $W\004orld!"}}, []byte{0x9e, '<', '.', 'H', 'e', 'l', 'l', 'o', ',', ' ', '$', 'W', 'o', 'r', 'l', 'd', '!', 0x1b}, []byte("<.Hello, $World!\033\004"), true, false, Readerboard3RGB},
		{Scroll, []int{14}, map[string][]string{"t": []string{"Hello, $W\033orld!"}}, []byte{0x9e, '<', '.', 'H', 'e', 'l', 'l', 'o', ',', ' ', '$', 'W', 'o', 'r', 'l', 'd', '!', 0x1b}, []byte("<.Hello, $World!\033\004"), true, false, Readerboard3RGB},
		// 43
		{Text, []int{14}, map[string][]string{"merge": []string{"false"}, "t": []string{"Hello, $World!"}}, []byte{0x9e, 'T', '.', '<', '.', 'H', 'e', 'l', 'l', 'o', ',', ' ', '$', 'W', 'o', 'r', 'l', 'd', '!', 0x1b}, []byte("T.<.Hello, $World!\033\004"), false, false, Readerboard3RGB},
		{Text, []int{14}, map[string][]string{"merge": []string{""}, "align": []string{"|"}, "trans": []string{">"}, "t": []string{"Hello, $World!"}}, []byte{0x9e, 'T', 'M', '|', '>', 'H', 'e', 'l', 'l', 'o', ',', ' ', '$', 'W', 'o', 'r', 'l', 'd', '!', 0x1b}, []byte("TM|>Hello, $World!\033\004"), false, false, Readerboard3RGB},
	} {
		directConnection := DirectDriver{BaseNetworkDriver{GlobalAddress: 15}}
		serialNetwork := RS485Driver{BaseNetworkDriver{GlobalAddress: 15}}

		command, err := tcase.handler(tcase.data, tcase.model)
		if tcase.expectError {
			if err == nil {
				t.Fatalf("test case %d: handler error expected but none returned", i)
			}
			continue
		}
		if err != nil {
			t.Fatalf("test case %d: handler error: %v", i, err)
		}

		var o485, oUSB []byte
		if command[0] == 0xff {
			o485, err = serialNetwork.AllLightsOffBytes(tcase.addrs, command[1:])
		} else {
			o485, err = serialNetwork.Bytes(tcase.addrs, command)
		}
		if tcase.expectNetworkError {
			if err == nil {
				t.Fatalf("test case %d: handler RS-485 network error expected but none returned", i)
			}
			continue
		}
		if err != nil {
			t.Fatalf("test case %d: rs-485 network error: %v", i, err)
		}

		if command[0] == 0xff {
			oUSB, err = directConnection.AllLightsOffBytes(tcase.addrs, command[1:])
		} else {
			oUSB, err = directConnection.Bytes(tcase.addrs, command)
		}
		if tcase.expectNetworkError {
			if err == nil {
				t.Fatalf("test case %d: handler direct network error expected but none returned", i)
			}
			continue
		}
		if err != nil {
			t.Fatalf("test case %d: usb connection error: %v", i, err)
		}

		if len(o485) != len(tcase.expected485) {
			t.Fatalf("test case %d: 485 output %v length %d, expected %v (length %d)", i, o485, len(o485), tcase.expected485, len(tcase.expected485))
		}
		for j := 0; j < len(o485); j++ {
			if o485[j] != tcase.expected485[j] {
				t.Fatalf("test case %d: 485 output differs at byte %d (%d vs %d); actual %v, expected %v",
					i, j, o485[j], tcase.expected485[j], o485, tcase.expected485)
			}
		}
		if len(tcase.expectedUSB) != len(oUSB) {
			t.Fatalf("test case %d: USB output was %q, expected %q", i, oUSB, tcase.expectedUSB)
		}
		for j := 0; j < len(oUSB); j++ {
			if oUSB[j] != tcase.expectedUSB[j] {
				t.Fatalf("test case %d: 485 output differs at byte %d (%d vs %d); actual %v, expected %v",
					i, j, oUSB[j], tcase.expectedUSB[j], oUSB, tcase.expectedUSB)
			}
		}
	}
}
