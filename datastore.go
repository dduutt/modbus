package modbus

import (
	"fmt"
	"sync"
)

type DataStore interface {
	ReadCoils(address, quantity uint16) ([]bool, error)
	ReadDiscreteInputs(address, quantity uint16) ([]bool, error)
	ReadHoldingRegisters(address, quantity uint16) ([]uint16, error)
	ReadInputRegisters(address, quantity uint16) ([]uint16, error)
	WriteCoils(address uint16, values []bool) error
	WriteHoldingRegisters(address uint16, values []uint16) error
}

type MemoryDataStore struct {
	mu               sync.RWMutex
	coils            []bool
	discreteInputs   []bool
	holdingRegisters []uint16
	inputRegisters   []uint16
}

func NewMemoryDataStore() *MemoryDataStore {
	return NewMemoryDataStoreSized(65536, 65536, 65536, 65536)
}

func NewMemoryDataStoreSized(coils, discreteInputs, holdingRegisters, inputRegisters int) *MemoryDataStore {
	return &MemoryDataStore{
		coils:            make([]bool, coils),
		discreteInputs:   make([]bool, discreteInputs),
		holdingRegisters: make([]uint16, holdingRegisters),
		inputRegisters:   make([]uint16, inputRegisters),
	}
}

func (s *MemoryDataStore) ReadCoils(address, quantity uint16) ([]bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return readBoolRange(s.coils, address, quantity)
}

func (s *MemoryDataStore) ReadDiscreteInputs(address, quantity uint16) ([]bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return readBoolRange(s.discreteInputs, address, quantity)
}

func (s *MemoryDataStore) ReadHoldingRegisters(address, quantity uint16) ([]uint16, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return readRegisterRange(s.holdingRegisters, address, quantity)
}

func (s *MemoryDataStore) ReadInputRegisters(address, quantity uint16) ([]uint16, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return readRegisterRange(s.inputRegisters, address, quantity)
}

func (s *MemoryDataStore) WriteCoils(address uint16, values []bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := checkRange(len(s.coils), address, uint16(len(values))); err != nil {
		return err
	}
	copy(s.coils[address:], values)
	return nil
}

func (s *MemoryDataStore) WriteDiscreteInputs(address uint16, values []bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := checkRange(len(s.discreteInputs), address, uint16(len(values))); err != nil {
		return err
	}
	copy(s.discreteInputs[address:], values)
	return nil
}

func (s *MemoryDataStore) WriteHoldingRegisters(address uint16, values []uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := checkRange(len(s.holdingRegisters), address, uint16(len(values))); err != nil {
		return err
	}
	copy(s.holdingRegisters[address:], values)
	return nil
}

func (s *MemoryDataStore) WriteInputRegisters(address uint16, values []uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := checkRange(len(s.inputRegisters), address, uint16(len(values))); err != nil {
		return err
	}
	copy(s.inputRegisters[address:], values)
	return nil
}

func readBoolRange(values []bool, address, quantity uint16) ([]bool, error) {
	if err := checkRange(len(values), address, quantity); err != nil {
		return nil, err
	}
	out := make([]bool, quantity)
	copy(out, values[address:uint32(address)+uint32(quantity)])
	return out, nil
}

func readRegisterRange(values []uint16, address, quantity uint16) ([]uint16, error) {
	if err := checkRange(len(values), address, quantity); err != nil {
		return nil, err
	}
	out := make([]uint16, quantity)
	copy(out, values[address:uint32(address)+uint32(quantity)])
	return out, nil
}

func checkRange(length int, address, quantity uint16) error {
	end := uint32(address) + uint32(quantity)
	if quantity == 0 || end > uint32(length) {
		return fmt.Errorf("%w: address=%d quantity=%d", ErrInvalidRequest, address, quantity)
	}
	return nil
}
