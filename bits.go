package modbus

func packBits(values []bool) []byte {
	out := make([]byte, (len(values)+7)/8)
	for i, v := range values {
		if v {
			out[i/8] |= 1 << uint(i%8)
		}
	}
	return out
}

func unpackBits(data []byte, quantity uint16) []bool {
	out := make([]bool, quantity)
	for i := range out {
		out[i] = data[i/8]&(1<<uint(i%8)) != 0
	}
	return out
}

func registersToBytes(values []uint16) []byte {
	out := make([]byte, len(values)*2)
	for i, v := range values {
		putUint16(out[i*2:], v)
	}
	return out
}

func bytesToRegisters(data []byte) []uint16 {
	out := make([]uint16, len(data)/2)
	for i := range out {
		out[i] = uint16At(data[i*2:])
	}
	return out
}
