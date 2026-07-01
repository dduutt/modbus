package modbus

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

type RawValue struct {
	Bits      []bool
	Registers []uint16
}

type Value struct {
	Data any
}

func (v Value) Bool() (bool, bool) {
	x, ok := v.Data.(bool)
	return x, ok
}

func (v Value) Bools() ([]bool, bool) {
	x, ok := v.Data.([]bool)
	if !ok {
		return nil, false
	}
	return append([]bool(nil), x...), true
}

func (v Value) UInt16() (uint16, bool) {
	x, ok := v.Data.(uint16)
	return x, ok
}

func (v Value) UInt16s() ([]uint16, bool) {
	x, ok := v.Data.([]uint16)
	if !ok {
		return nil, false
	}
	return append([]uint16(nil), x...), true
}

func (v Value) Int16() (int16, bool) {
	x, ok := v.Data.(int16)
	return x, ok
}

func (v Value) Int16s() ([]int16, bool) {
	x, ok := v.Data.([]int16)
	if !ok {
		return nil, false
	}
	return append([]int16(nil), x...), true
}

func (v Value) UInt32() (uint32, bool) {
	x, ok := v.Data.(uint32)
	return x, ok
}

func (v Value) UInt32s() ([]uint32, bool) {
	x, ok := v.Data.([]uint32)
	if !ok {
		return nil, false
	}
	return append([]uint32(nil), x...), true
}

func (v Value) Int32() (int32, bool) {
	x, ok := v.Data.(int32)
	return x, ok
}

func (v Value) Int32s() ([]int32, bool) {
	x, ok := v.Data.([]int32)
	if !ok {
		return nil, false
	}
	return append([]int32(nil), x...), true
}

func (v Value) Float32() (float32, bool) {
	x, ok := v.Data.(float32)
	return x, ok
}

func (v Value) Float32s() ([]float32, bool) {
	x, ok := v.Data.([]float32)
	if !ok {
		return nil, false
	}
	return append([]float32(nil), x...), true
}

func (v Value) UInt64() (uint64, bool) {
	x, ok := v.Data.(uint64)
	return x, ok
}

func (v Value) UInt64s() ([]uint64, bool) {
	x, ok := v.Data.([]uint64)
	if !ok {
		return nil, false
	}
	return append([]uint64(nil), x...), true
}

func (v Value) Int64() (int64, bool) {
	x, ok := v.Data.(int64)
	return x, ok
}

func (v Value) Int64s() ([]int64, bool) {
	x, ok := v.Data.([]int64)
	if !ok {
		return nil, false
	}
	return append([]int64(nil), x...), true
}

func (v Value) Float64() (float64, bool) {
	x, ok := v.Data.(float64)
	return x, ok
}

func (v Value) Float64s() ([]float64, bool) {
	x, ok := v.Data.([]float64)
	if !ok {
		return nil, false
	}
	return append([]float64(nil), x...), true
}

func (v Value) Bytes() ([]byte, bool) {
	x, ok := v.Data.([]byte)
	if !ok {
		return nil, false
	}
	return append([]byte(nil), x...), true
}

func (v Value) String() (string, bool) {
	x, ok := v.Data.(string)
	return x, ok
}

func DecodeValue(tag Tag, raw RawValue) (Value, error) {
	tag = tag.withDefaults(0)
	switch tag.DataType {
	case TypeBool:
		if len(raw.Bits) < int(tag.Quantity) {
			return Value{}, fmt.Errorf("%w: not enough bits", ErrInvalidResponse)
		}
		if tag.Quantity == 1 {
			return Value{Data: raw.Bits[0]}, nil
		}
		return Value{Data: append([]bool(nil), raw.Bits[:tag.Quantity]...)}, nil
	case TypeUInt16:
		if len(raw.Registers) < int(tag.Quantity) {
			return Value{}, fmt.Errorf("%w: not enough registers", ErrInvalidResponse)
		}
		if tag.Quantity == 1 {
			return Value{Data: raw.Registers[0]}, nil
		}
		return Value{Data: append([]uint16(nil), raw.Registers[:tag.Quantity]...)}, nil
	case TypeInt16:
		if len(raw.Registers) < int(tag.Quantity) {
			return Value{}, fmt.Errorf("%w: not enough registers", ErrInvalidResponse)
		}
		if tag.Quantity == 1 {
			return Value{Data: int16(raw.Registers[0])}, nil
		}
		out := make([]int16, tag.Quantity)
		for i := range out {
			out[i] = int16(raw.Registers[i])
		}
		return Value{Data: out}, nil
	case TypeUInt32, TypeInt32, TypeFloat32:
		return decode32(tag, raw.Registers)
	case TypeUInt64, TypeInt64, TypeFloat64:
		return decode64(tag, raw.Registers)
	case TypeBytes:
		out := make([]byte, len(raw.Registers)*2)
		for i, reg := range raw.Registers {
			binary.BigEndian.PutUint16(out[i*2:], reg)
		}
		return Value{Data: out}, nil
	case TypeString:
		out := make([]byte, len(raw.Registers)*2)
		for i, reg := range raw.Registers {
			binary.BigEndian.PutUint16(out[i*2:], reg)
		}
		return Value{Data: strings.TrimRight(string(out), "\x00")}, nil
	default:
		return Value{}, fmt.Errorf("%w: unsupported data type %s", ErrInvalidRequest, tag.DataType)
	}
}

func EncodeValue(tag Tag, value any) (RawValue, error) {
	tag = tag.withDefaults(0)
	switch tag.DataType {
	case TypeBool:
		switch v := value.(type) {
		case bool:
			return RawValue{Bits: []bool{v}}, nil
		case []bool:
			return RawValue{Bits: append([]bool(nil), v...)}, nil
		default:
			return RawValue{}, fmt.Errorf("%w: expected bool or []bool", ErrInvalidRequest)
		}
	case TypeUInt16:
		switch v := value.(type) {
		case uint16:
			return RawValue{Registers: []uint16{v}}, nil
		case []uint16:
			return RawValue{Registers: append([]uint16(nil), v...)}, nil
		default:
			return RawValue{}, fmt.Errorf("%w: expected uint16 or []uint16", ErrInvalidRequest)
		}
	case TypeInt16:
		switch v := value.(type) {
		case int16:
			return RawValue{Registers: []uint16{uint16(v)}}, nil
		case []int16:
			out := make([]uint16, len(v))
			for i := range v {
				out[i] = uint16(v[i])
			}
			return RawValue{Registers: out}, nil
		default:
			return RawValue{}, fmt.Errorf("%w: expected int16 or []int16", ErrInvalidRequest)
		}
	case TypeUInt32:
		return encode32(tag, value, func(v uint32) uint32 { return v })
	case TypeInt32:
		return encode32(tag, value, func(v uint32) uint32 { return v })
	case TypeFloat32:
		return encode32(tag, value, func(v uint32) uint32 { return v })
	case TypeUInt64:
		return encode64(tag, value)
	case TypeInt64:
		return encode64(tag, value)
	case TypeFloat64:
		return encode64(tag, value)
	case TypeBytes:
		b, ok := value.([]byte)
		if !ok {
			return RawValue{}, fmt.Errorf("%w: expected []byte", ErrInvalidRequest)
		}
		if len(b)%2 != 0 {
			return RawValue{}, fmt.Errorf("%w: byte slice length must be even", ErrInvalidRequest)
		}
		out := make([]uint16, len(b)/2)
		for i := range out {
			out[i] = binary.BigEndian.Uint16(b[i*2:])
		}
		return RawValue{Registers: out}, nil
	case TypeString:
		s, ok := value.(string)
		if !ok {
			return RawValue{}, fmt.Errorf("%w: expected string", ErrInvalidRequest)
		}
		b := []byte(s)
		maxBytes := int(tag.Quantity) * 2
		if tag.Quantity > 0 && len(b) > maxBytes {
			return RawValue{}, fmt.Errorf("%w: string exceeds tag quantity", ErrInvalidRequest)
		}
		if len(b)%2 != 0 {
			b = append(b, 0)
		}
		if tag.Quantity > 0 && len(b) < maxBytes {
			b = append(b, make([]byte, maxBytes-len(b))...)
		}
		out := make([]uint16, len(b)/2)
		for i := range out {
			out[i] = binary.BigEndian.Uint16(b[i*2:])
		}
		return RawValue{Registers: out}, nil
	default:
		return RawValue{}, fmt.Errorf("%w: unsupported data type %s", ErrInvalidRequest, tag.DataType)
	}
}

func decode32(tag Tag, registers []uint16) (Value, error) {
	need := int(tag.Quantity) * 2
	if len(registers) < need {
		return Value{}, fmt.Errorf("%w: not enough registers", ErrInvalidResponse)
	}
	one := func(i int) uint32 {
		a, b := registers[i], registers[i+1]
		if tag.WordOrder == WordOrderLowFirst {
			a, b = b, a
		}
		var bytes [4]byte
		if tag.ByteOrder == ByteOrderLittleEndian {
			binary.LittleEndian.PutUint16(bytes[0:], a)
			binary.LittleEndian.PutUint16(bytes[2:], b)
		} else {
			binary.BigEndian.PutUint16(bytes[0:], a)
			binary.BigEndian.PutUint16(bytes[2:], b)
		}
		return binary.BigEndian.Uint32(bytes[:])
	}
	if tag.Quantity == 1 {
		u := one(0)
		switch tag.DataType {
		case TypeUInt32:
			return Value{Data: u}, nil
		case TypeInt32:
			return Value{Data: int32(u)}, nil
		case TypeFloat32:
			return Value{Data: math.Float32frombits(u)}, nil
		}
	}
	switch tag.DataType {
	case TypeUInt32:
		out := make([]uint32, tag.Quantity)
		for i := range out {
			out[i] = one(i * 2)
		}
		return Value{Data: out}, nil
	case TypeInt32:
		out := make([]int32, tag.Quantity)
		for i := range out {
			out[i] = int32(one(i * 2))
		}
		return Value{Data: out}, nil
	case TypeFloat32:
		out := make([]float32, tag.Quantity)
		for i := range out {
			out[i] = math.Float32frombits(one(i * 2))
		}
		return Value{Data: out}, nil
	default:
		return Value{}, fmt.Errorf("%w: unsupported 32-bit type", ErrInvalidRequest)
	}
}

func decode64(tag Tag, registers []uint16) (Value, error) {
	need := int(tag.Quantity) * 4
	if len(registers) < need {
		return Value{}, fmt.Errorf("%w: not enough registers", ErrInvalidResponse)
	}
	one := func(i int) uint64 {
		words := [4]uint16{registers[i], registers[i+1], registers[i+2], registers[i+3]}
		if tag.WordOrder == WordOrderLowFirst {
			words[0], words[1], words[2], words[3] = words[3], words[2], words[1], words[0]
		}
		var bytes [8]byte
		for idx, word := range words {
			if tag.ByteOrder == ByteOrderLittleEndian {
				binary.LittleEndian.PutUint16(bytes[idx*2:], word)
			} else {
				binary.BigEndian.PutUint16(bytes[idx*2:], word)
			}
		}
		return binary.BigEndian.Uint64(bytes[:])
	}
	if tag.Quantity == 1 {
		u := one(0)
		switch tag.DataType {
		case TypeUInt64:
			return Value{Data: u}, nil
		case TypeInt64:
			return Value{Data: int64(u)}, nil
		case TypeFloat64:
			return Value{Data: math.Float64frombits(u)}, nil
		}
	}
	switch tag.DataType {
	case TypeUInt64:
		out := make([]uint64, tag.Quantity)
		for i := range out {
			out[i] = one(i * 4)
		}
		return Value{Data: out}, nil
	case TypeInt64:
		out := make([]int64, tag.Quantity)
		for i := range out {
			out[i] = int64(one(i * 4))
		}
		return Value{Data: out}, nil
	case TypeFloat64:
		out := make([]float64, tag.Quantity)
		for i := range out {
			out[i] = math.Float64frombits(one(i * 4))
		}
		return Value{Data: out}, nil
	default:
		return Value{}, fmt.Errorf("%w: unsupported 64-bit type", ErrInvalidRequest)
	}
}

func encode32[T ~uint32](tag Tag, value any, convert func(uint32) T) (RawValue, error) {
	var values []uint32
	switch tag.DataType {
	case TypeUInt32:
		switch v := value.(type) {
		case uint32:
			values = []uint32{v}
		case []uint32:
			values = append([]uint32(nil), v...)
		default:
			return RawValue{}, fmt.Errorf("%w: expected uint32 or []uint32", ErrInvalidRequest)
		}
	case TypeInt32:
		switch v := value.(type) {
		case int32:
			values = []uint32{uint32(v)}
		case []int32:
			values = make([]uint32, len(v))
			for i := range v {
				values[i] = uint32(v[i])
			}
		default:
			return RawValue{}, fmt.Errorf("%w: expected int32 or []int32", ErrInvalidRequest)
		}
	case TypeFloat32:
		switch v := value.(type) {
		case float32:
			values = []uint32{math.Float32bits(v)}
		case []float32:
			values = make([]uint32, len(v))
			for i := range v {
				values[i] = math.Float32bits(v[i])
			}
		default:
			return RawValue{}, fmt.Errorf("%w: expected float32 or []float32", ErrInvalidRequest)
		}
	}
	_ = convert
	out := make([]uint16, len(values)*2)
	for i, v := range values {
		var bytes [4]byte
		binary.BigEndian.PutUint32(bytes[:], v)
		a := binary.BigEndian.Uint16(bytes[0:])
		b := binary.BigEndian.Uint16(bytes[2:])
		if tag.ByteOrder == ByteOrderLittleEndian {
			a = binary.LittleEndian.Uint16(bytes[0:])
			b = binary.LittleEndian.Uint16(bytes[2:])
		}
		if tag.WordOrder == WordOrderLowFirst {
			a, b = b, a
		}
		out[i*2] = a
		out[i*2+1] = b
	}
	return RawValue{Registers: out}, nil
}

func encode64(tag Tag, value any) (RawValue, error) {
	var values []uint64
	switch tag.DataType {
	case TypeUInt64:
		switch v := value.(type) {
		case uint64:
			values = []uint64{v}
		case []uint64:
			values = append([]uint64(nil), v...)
		default:
			return RawValue{}, fmt.Errorf("%w: expected uint64 or []uint64", ErrInvalidRequest)
		}
	case TypeInt64:
		switch v := value.(type) {
		case int64:
			values = []uint64{uint64(v)}
		case []int64:
			values = make([]uint64, len(v))
			for i := range v {
				values[i] = uint64(v[i])
			}
		default:
			return RawValue{}, fmt.Errorf("%w: expected int64 or []int64", ErrInvalidRequest)
		}
	case TypeFloat64:
		switch v := value.(type) {
		case float64:
			values = []uint64{math.Float64bits(v)}
		case []float64:
			values = make([]uint64, len(v))
			for i := range v {
				values[i] = math.Float64bits(v[i])
			}
		default:
			return RawValue{}, fmt.Errorf("%w: expected float64 or []float64", ErrInvalidRequest)
		}
	}
	out := make([]uint16, len(values)*4)
	for i, v := range values {
		var bytes [8]byte
		binary.BigEndian.PutUint64(bytes[:], v)
		words := [4]uint16{
			binary.BigEndian.Uint16(bytes[0:]),
			binary.BigEndian.Uint16(bytes[2:]),
			binary.BigEndian.Uint16(bytes[4:]),
			binary.BigEndian.Uint16(bytes[6:]),
		}
		if tag.ByteOrder == ByteOrderLittleEndian {
			words = [4]uint16{
				binary.LittleEndian.Uint16(bytes[0:]),
				binary.LittleEndian.Uint16(bytes[2:]),
				binary.LittleEndian.Uint16(bytes[4:]),
				binary.LittleEndian.Uint16(bytes[6:]),
			}
		}
		if tag.WordOrder == WordOrderLowFirst {
			words[0], words[1], words[2], words[3] = words[3], words[2], words[1], words[0]
		}
		copy(out[i*4:], words[:])
	}
	return RawValue{Registers: out}, nil
}
