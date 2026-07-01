package modbus

import (
	"fmt"
	"strconv"
	"strings"
)

type Area string

const (
	AreaCoil            Area = "coil"
	AreaDiscreteInput   Area = "discrete-input"
	AreaHoldingRegister Area = "holding-register"
	AreaInputRegister   Area = "input-register"
)

type DataType string

const (
	TypeBool    DataType = "bool"
	TypeUInt16  DataType = "uint16"
	TypeInt16   DataType = "int16"
	TypeUInt32  DataType = "uint32"
	TypeInt32   DataType = "int32"
	TypeFloat32 DataType = "float32"
	TypeUInt64  DataType = "uint64"
	TypeInt64   DataType = "int64"
	TypeFloat64 DataType = "float64"
	TypeBytes   DataType = "bytes"
	TypeString  DataType = "string"
)

type ByteOrder string

const (
	ByteOrderBigEndian    ByteOrder = "big"
	ByteOrderLittleEndian ByteOrder = "little"
)

type WordOrder string

const (
	WordOrderHighFirst WordOrder = "high-first"
	WordOrderLowFirst  WordOrder = "low-first"
)

type Tag struct {
	Area      Area
	Address   uint16
	Quantity  uint16
	DataType  DataType
	UnitID    byte
	ByteOrder ByteOrder
	WordOrder WordOrder
}

func Coil(address uint16) Tag {
	return Tag{Area: AreaCoil, Address: address, Quantity: 1, DataType: TypeBool}
}

func DiscreteInput(address uint16) Tag {
	return Tag{Area: AreaDiscreteInput, Address: address, Quantity: 1, DataType: TypeBool}
}

func HoldingRegister(address uint16) Tag {
	return Tag{Area: AreaHoldingRegister, Address: address, Quantity: 1, DataType: TypeUInt16}
}

func InputRegister(address uint16) Tag {
	return Tag{Area: AreaInputRegister, Address: address, Quantity: 1, DataType: TypeUInt16}
}

func (t Tag) WithQuantity(quantity uint16) Tag {
	t.Quantity = quantity
	return t
}

func (t Tag) As(dataType DataType) Tag {
	t.DataType = dataType
	return t
}

func (t Tag) WithUnitID(unitID byte) Tag {
	t.UnitID = unitID
	return t
}

func (t Tag) WithOrder(byteOrder ByteOrder, wordOrder WordOrder) Tag {
	t.ByteOrder = byteOrder
	t.WordOrder = wordOrder
	return t
}

func (t Tag) Validate() error {
	if t.Quantity == 0 {
		return fmt.Errorf("%w: tag quantity must be greater than zero", ErrInvalidRequest)
	}
	switch t.Area {
	case AreaCoil, AreaDiscreteInput:
		if t.DataType != TypeBool && t.DataType != "" {
			return fmt.Errorf("%w: %s only supports bool", ErrInvalidRequest, t.Area)
		}
	case AreaHoldingRegister, AreaInputRegister:
		switch t.DataType {
		case "", TypeUInt16, TypeInt16, TypeUInt32, TypeInt32, TypeFloat32, TypeUInt64, TypeInt64, TypeFloat64, TypeBytes, TypeString:
		default:
			return fmt.Errorf("%w: unsupported register data type %s", ErrInvalidRequest, t.DataType)
		}
	default:
		return fmt.Errorf("%w: unsupported area %s", ErrInvalidRequest, t.Area)
	}
	return nil
}

func (t Tag) withDefaults(unitID byte) Tag {
	if t.UnitID == 0 {
		t.UnitID = unitID
	}
	if t.Quantity == 0 {
		t.Quantity = 1
	}
	if t.DataType == "" {
		if t.Area == AreaCoil || t.Area == AreaDiscreteInput {
			t.DataType = TypeBool
		} else {
			t.DataType = TypeUInt16
		}
	}
	if t.ByteOrder == "" {
		t.ByteOrder = ByteOrderBigEndian
	}
	if t.WordOrder == "" {
		t.WordOrder = WordOrderHighFirst
	}
	return t
}

func (t Tag) modbusQuantity() uint16 {
	switch t.DataType {
	case TypeUInt32, TypeInt32, TypeFloat32:
		return t.Quantity * 2
	case TypeUInt64, TypeInt64, TypeFloat64:
		return t.Quantity * 4
	default:
		return t.Quantity
	}
}

func ParseTag(s string) (Tag, error) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 4 {
		return Tag{}, fmt.Errorf("%w: invalid tag format", ErrInvalidRequest)
	}
	addr, err := strconv.ParseUint(parts[1], 10, 16)
	if err != nil {
		return Tag{}, fmt.Errorf("%w: invalid address", ErrInvalidRequest)
	}
	area, err := parseTagArea(parts[0])
	if err != nil {
		return Tag{}, err
	}
	tag := Tag{Area: area, Address: uint16(addr), Quantity: 1}
	if len(parts) >= 3 && parts[2] != "" {
		dataType, err := parseTagDataType(parts[2])
		if err != nil {
			return Tag{}, err
		}
		tag.DataType = dataType
	}
	if len(parts) == 4 {
		q, err := strconv.ParseUint(parts[3], 10, 16)
		if err != nil {
			return Tag{}, fmt.Errorf("%w: invalid quantity", ErrInvalidRequest)
		}
		tag.Quantity = uint16(q)
	}
	tag = tag.withDefaults(0)
	return tag, tag.Validate()
}

func parseTagArea(s string) (Area, error) {
	switch s {
	case "c":
		return AreaCoil, nil
	case "di":
		return AreaDiscreteInput, nil
	case "hr":
		return AreaHoldingRegister, nil
	case "ir":
		return AreaInputRegister, nil
	default:
		return "", fmt.Errorf("%w: unsupported tag area short name %q", ErrInvalidRequest, s)
	}
}

func parseTagDataType(s string) (DataType, error) {
	switch s {
	case "b":
		return TypeBool, nil
	case "u16":
		return TypeUInt16, nil
	case "i16":
		return TypeInt16, nil
	case "u32":
		return TypeUInt32, nil
	case "i32":
		return TypeInt32, nil
	case "f32":
		return TypeFloat32, nil
	case "u64":
		return TypeUInt64, nil
	case "i64":
		return TypeInt64, nil
	case "f64":
		return TypeFloat64, nil
	case "by":
		return TypeBytes, nil
	case "str":
		return TypeString, nil
	default:
		return "", fmt.Errorf("%w: unsupported tag data type short name %q", ErrInvalidRequest, s)
	}
}

func (a Area) String() string {
	return string(a)
}
