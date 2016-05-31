package storage

import (
	"bytes"
	"testing"
)

func TestZerosNewNodeIDWithPrefix(t *testing.T) {
	n := NewNodeIDWithPrefix(0, 0, 0, 64)
	if got, want := n.Path, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}; !bytes.Equal(got, want) {
		t.Fatalf("Expected Path of %v, but got %v", want, got)
	}
}

func TestNewNodeIDWithPrefix(t *testing.T) {
	n := NewNodeIDWithPrefix(0x12345678, 32, 32, 64)
	if got, want := n.Path, []byte{0x00, 0x00, 0x00, 0x00, 0x78, 0x56, 0x34, 0x12}; !bytes.Equal(got, want) {
		t.Fatalf("Expected Path of %v, but got %v", want, got)
	}

	n = NewNodeIDWithPrefix(0x345678, 24, 24, 24)
	if got, want := n.Path, []byte{0x78, 0x56, 0x34}; !bytes.Equal(got, want) {
		t.Fatalf("Expected Path of %v, but got %v", want, got)
	}
}

func TestNewNodeIDForTreeCoordsForZeros(t *testing.T) {
	n := NewNodeIDForTreeCoords(0, 0, 8)
	if got, want := n.Path, []byte{0x00}; !bytes.Equal(got, want) {
		t.Fatalf("Expected Path of %v, but got %v", want, got)
	}
	if got, want := n.String(), ""; got != want {
		t.Fatalf("Expected '%s', got '%s'", want, got)
	}
}

func TestNewNodeIDForTreeCoords(t *testing.T) {
	n := NewNodeIDForTreeCoords(11, 0x1234, 16)
	if got, want := n.Path, []byte{0x34, 0x12}; !bytes.Equal(got, want) {
		t.Fatalf("Expected Path of %v, but got %v", want, got)
	}
	if got, want := n.String(), "01000110100"; got != want {
		t.Fatalf("Expected '%s', got '%s'", want, got)
	}
}

func TestSetBit(t *testing.T) {
	n := NewNodeIDWithPrefix(0, 0, 0, 64)

	n.SetBit(27, 1)
	if got, want := n.Path, []byte{0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00}; !bytes.Equal(got, want) {
		t.Fatalf("Expected Path of %v, but got %v", want, got)
	}

	n.SetBit(27, 0)
	if got, want := n.Path, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}; !bytes.Equal(got, want) {
		t.Fatalf("Expected Path of %v, but got %v", want, got)
	}
}

func TestBit(t *testing.T) {
	// every 3rd bit set
	n := NewNodeIDWithPrefix(0x9249, 16, 16, 16)
	for x := 0; x < 16; x++ {
		want := 0
		if x%3 == 0 {
			want = 1
		}
		if got := n.Bit(x); got != uint(want) {
			t.Fatalf("Expected bit %d to be %d, but got %d", x, want, got)
		}
	}
}

func TestString(t *testing.T) {
	n := NewEmptyNodeID(32)
	if got, want := n.String(), ""; got != want {
		t.Fatalf("Expected '%s', got '%s'", want, got)
	}

	n = NewNodeIDWithPrefix(0x345678, 24, 32, 32)
	t.Logf("Node: %#v", n)
	if got, want := n.String(), "00110100010101100111100000000000"; got != want {
		t.Fatalf("Expected '%s', got '%s'", want, got)
	}
}
