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
	if got, want := n.String(), "00010010001"; got != want {
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
	if got, want := n.String(), "00110100010101100111100000000000"; got != want {
		t.Fatalf("Expected '%s', got '%s'", want, got)
	}
}

func TestSiblings(t *testing.T) {
	l := 16
	n := NewNodeIDWithPrefix(0xabe4, l, l, l)
	expected := []string{
		"1010101111100101",
		"101010111110011",
		"10101011111000",
		"1010101111101",
		"101010111111",
		"10101011110",
		"1010101110",
		"101010110",
		"10101010",
		"1010100",
		"101011",
		"10100",
		"1011",
		"100",
		"11",
		"0"}

	sibs := n.Siblings()
	if got, want := len(sibs), len(expected); got != want {
		t.Fatalf("Expected %d siblings, got %d", want, got)
	}

	for i := 0; i < len(sibs); i++ {
		if want, got := expected[i], sibs[i].String(); want != got {
			t.Fatalf("Expected sib %d to be %v, got %v", i, want, got)
		}
	}
}

func TestNodeSelfEquivalent(t *testing.T) {
	l := 16
	n1 := NewNodeIDWithPrefix(0x1234, l, l, l)
	if !n1.Equivalent(n1) {
		t.Fatalf("%v not Equivalent to itself", n1)
	}
}

func TestNodeEquivalent(t *testing.T) {
	l := 16
	n1 := NewNodeIDWithPrefix(0x1234, l, l, l)
	n2 := NewNodeIDWithPrefix(0x1234, l, l, l)
	if !n1.Equivalent(n2) {
		t.Fatalf("%v not Equivalent with %v", n1, n2)
	}
}

func TestNodeNotEquivalentPrefixLen(t *testing.T) {
	l := 16
	n1 := NewNodeIDWithPrefix(0x1234, l, l, l)
	n2 := NewNodeIDWithPrefix(0x1234, l-1, l, l)
	if n1.Equivalent(n2) {
		t.Fatalf("%v incorrecly Equivalent with %v", n1, n2)
	}
}

func TestNodeNotEquivalentIDLen(t *testing.T) {
	l := 16
	n1 := NewNodeIDWithPrefix(0x1234, l, l, l)
	n2 := NewNodeIDWithPrefix(0x1234, l, l+1, l)
	if n1.Equivalent(n2) {
		t.Fatalf("%v incorrecly Equivalent with %v", n1, n2)
	}
}

func TestNodeNotEquivalentMaxLen(t *testing.T) {
	l := 16
	n1 := NewNodeIDWithPrefix(0x1234, l, l, l)
	// Different max len, but that's ok because the prefixes are identical
	n2 := NewNodeIDWithPrefix(0x1234, l, l, l*2)
	if !n1.Equivalent(n2) {
		t.Fatalf("%v not Equivalent with %v", n1, n2)
	}
}

func TestNodeNotEquivalentDifferentPrefix(t *testing.T) {
	l := 16
	n1 := NewNodeIDWithPrefix(0x1234, l, l, l)
	n2 := NewNodeIDWithPrefix(0x5432, l, l, l)
	if n1.Equivalent(n2) {
		t.Fatalf("%v incorrecly Equivalent with %v", n1, n2)
	}
}
