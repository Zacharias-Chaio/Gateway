package converter

import "testing"

func TestMapRegistersBoolUsesModbusRegisterBitOrder(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		bit  int
		want bool
	}{
		{name: "lowest bit", raw: []byte{0x00, 0x01}, bit: 0, want: true},
		{name: "highest bit", raw: []byte{0x80, 0x00}, bit: 15, want: true},
		{name: "unset bit", raw: []byte{0x00, 0x01}, bit: 1, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := MapRegisters(test.raw, PropMeta{DataType: "bool", StartBit: test.bit, EndBit: test.bit})
			if err != nil {
				t.Fatalf("MapRegisters returned an error: %v", err)
			}
			if got != test.want {
				t.Fatalf("MapRegisters(% x, bit %d) = %v, want %v", test.raw, test.bit, got, test.want)
			}
		})
	}
}
