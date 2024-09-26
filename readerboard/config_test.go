package readerboard

import (
	"encoding/json"
	"testing"
)

func TestMarshalDevMap(t *testing.T) {
	for i, tcase := range []struct {
		input         string
		expected      DevMap
		errorExpected bool
	}{
		{"{\"1\":{\"DeviceType\":\"Busylight1.x\",\"NetworkID\":\"net1\",\"Description\":\"Some device\",\"Serial\":\"123\"}}",
			DevMap{
				1: DeviceDescription{
					DeviceType:  Busylight1,
					NetworkID:   "net1",
					Description: "Some device",
					Serial:      "123",
				},
			}, false},
		{"{\"x1\":{\"DeviceType\":\"Busylight1.x\",\"NetworkID\":\"net1\",\"Description\":\"Some device\",\"Serial\":\"123\"}}",
			DevMap{
				1: DeviceDescription{
					DeviceType:  Busylight1,
					NetworkID:   "net1",
					Description: "Some device",
					Serial:      "123",
				},
			}, true},
		{"{\"123\":{\"DeviceType\":\"Busylight2.x\",\"NetworkID\":\"net1\",\"Serial\":\"123\"},\"456\":{\"NetworkID\":\"net2\",\"DeviceType\":\"Readerboard\"}}",
			DevMap{
				123: DeviceDescription{
					DeviceType:  Busylight2,
					NetworkID:   "net1",
					Serial:      "123",
					Description: "",
				},
				456: DeviceDescription{
					DeviceType:  Readerboard3RGB,
					NetworkID:   "net2",
					Serial:      "",
					Description: "",
				},
			}, false},
	} {
		var actual DevMap
		if err := json.Unmarshal([]byte(tcase.input), &actual); err != nil {
			if !tcase.errorExpected {
				t.Fatalf("test case %d: unmarshal failed: %v", i, err)
			}
			continue
		} else if tcase.errorExpected {
			t.Fatalf("test case %d: error expected but none found", i)
		}

		if len(actual) != len(tcase.expected) {
			t.Fatalf("test case %d, got %d item(s), expected %d", i, len(actual), len(tcase.expected))
		}
		for k, v := range tcase.expected {
			vv, ok := actual[k]
			if !ok {
				t.Fatalf("test case %d, expected key %v missing from actual result", i, k)
			}
			if v != vv {
				t.Fatalf("test case %d, expected %v, got %v", i, v, vv)
			}
		}
	}
}

func TestMarshalNetworkType(t *testing.T) {
	for i, tcase := range []struct {
		input         string
		expected      NetworkType
		normalized    string
		errorExpected bool
	}{
		{"\"RS-485\"", RS485Network, "\"RS-485\"", false},
		{"\"RS485\"", RS485Network, "\"RS-485\"", false},
		{"\"rs485\"", RS485Network, "\"RS-485\"", false},
		{"\"rs-485\"", RS485Network, "\"RS-485\"", false},
		{"\"485\"", RS485Network, "\"RS-485\"", false},
		{"\"USB\"", USBDirect, "\"USB\"", false},
		{"\"usb\"", USBDirect, "\"USB\"", false},
		{"\"rs-232\"", USBDirect, "", true},
		{"\"carrierpidgeon\"", USBDirect, "", true},
	} {
		var actual NetworkType
		if err := json.Unmarshal([]byte(tcase.input), &actual); err != nil {
			if !tcase.errorExpected {
				t.Fatalf("test case %d: unmarshal failed: %v", i, err)
			}
			continue
		} else if tcase.errorExpected {
			t.Fatalf("test case %d: error expected but none found", i)
		}
		if actual != tcase.expected {
			t.Fatalf("test case %d, expected %v, got %v", i, tcase.expected, actual)
		}

		b, err := json.Marshal(actual)
		if err != nil {
			if !tcase.errorExpected {
				t.Fatalf("test case %d: marshal failed: %v", i, err)
			}
			continue
		} else if tcase.errorExpected {
			t.Fatalf("test case %d: marshal error expected but none found", i)
		}
		if string(b) != tcase.normalized {
			t.Fatalf("test case %d: marshalled to %q, but expected %q", i, string(b), tcase.normalized)
		}
	}
}

func TestMarshalHardwareModel(t *testing.T) {
	for i, tcase := range []struct {
		input         string
		expected      HardwareModel
		normalized    string
		errorExpected bool
	}{
		{"\"Busylight1\"", Busylight1, "\"Busylight1\"", false},
		{"\"Busylight2\"", Busylight2, "\"Busylight2\"", false},
		{"\"Busylight2.0\"", Busylight2, "\"Busylight2\"", false},
		{"\"Busylight2.1\"", Busylight2, "\"Busylight2\"", false},
		{"\"Busylight2\"", Busylight2, "\"Busylight2\"", false},
		{"\"Busylight\"", Busylight2, "\"Busylight2\"", false},
		{"\"Readerboard3RGB\"", Readerboard3RGB, "\"Readerboard3_RGB\"", false},
		{"\"Readerboard3\"", Readerboard3RGB, "\"Readerboard3_RGB\"", false},
		{"\"Readerboard3Mono\"", Readerboard3Mono, "\"Readerboard3_Monochrome\"", false},
		{"\"Rboard\"", Readerboard3RGB, "", true},
	} {
		var actual HardwareModel
		if err := json.Unmarshal([]byte(tcase.input), &actual); err != nil {
			if !tcase.errorExpected {
				t.Fatalf("test case %d: unmarshal failed: %v", i, err)
			}
			continue
		} else if tcase.errorExpected {
			t.Fatalf("test case %d: error expected but none found", i)
		}
		if actual != tcase.expected {
			t.Fatalf("test case %d, expected %v, got %v", i, tcase.expected, actual)
		}

		b, err := json.Marshal(actual)
		if err != nil {
			if !tcase.errorExpected {
				t.Fatalf("test case %d: marshal failed: %v", i, err)
			}
			continue
		} else if tcase.errorExpected {
			t.Fatalf("test case %d: marshal error expected but none found", i)
		}
		if string(b) != tcase.normalized {
			t.Fatalf("test case %d: marshalled to %q, but expected %q", i, string(b), tcase.normalized)
		}
	}
}
