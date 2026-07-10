package modbus

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"time"
)

type Client struct {
	transport Transport
	unitID    byte
	timeout   time.Duration
}

func NewClient(transport Transport, opts ...Option) *Client {
	cfg := clientOptions{unitID: 1, timeout: 5 * time.Second}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Client{
		transport: transport,
		unitID:    cfg.unitID,
		timeout:   cfg.timeout,
	}
}

func NewTCPClient(address string, opts ...Option) *Client {
	return NewClient(NewTCPTransport(address), opts...)
}

func NewRTUClient(conn io.ReadWriteCloser, opts ...Option) *Client {
	return NewClient(NewRTUTransport(conn), opts...)
}

// Connect establishes the underlying connection when the transport supports
// explicit connection. Transports without a separate connect step return nil.
func (c *Client) Connect(ctx context.Context) error {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	if transport, ok := c.transport.(connector); ok {
		return transport.Connect(ctx)
	}
	return nil
}

func (c *Client) Close() error {
	return c.transport.Close()
}

func (c *Client) do(ctx context.Context, req PDU) (PDU, error) {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	resp, err := c.transport.Do(ctx, c.unitID, req)
	if err != nil {
		return PDU{}, err
	}
	if err := parseException(resp, req.Function); err != nil {
		return PDU{}, err
	}
	return resp, nil
}

func (c *Client) ReadCoils(ctx context.Context, address, quantity uint16) ([]bool, error) {
	return c.readBits(ctx, FuncReadCoils, address, quantity)
}

func (c *Client) ReadDiscreteInputs(ctx context.Context, address, quantity uint16) ([]bool, error) {
	return c.readBits(ctx, FuncReadDiscreteInputs, address, quantity)
}

func (c *Client) readBits(ctx context.Context, fn FunctionCode, address, quantity uint16) ([]bool, error) {
	if err := quantityRange(quantity, 2000); err != nil {
		return nil, err
	}
	req := PDU{Function: fn, Data: make([]byte, 4)}
	putUint16(req.Data[0:], address)
	putUint16(req.Data[2:], quantity)

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	byteCount := int((quantity + 7) / 8)
	if len(resp.Data) != 1+byteCount || int(resp.Data[0]) != byteCount {
		return nil, fmt.Errorf("%w: invalid bit read byte count", ErrInvalidResponse)
	}
	return unpackBits(resp.Data[1:], quantity), nil
}

func (c *Client) ReadHoldingRegisters(ctx context.Context, address, quantity uint16) ([]uint16, error) {
	return c.readRegisters(ctx, FuncReadHoldingRegisters, address, quantity)
}

func (c *Client) ReadInputRegisters(ctx context.Context, address, quantity uint16) ([]uint16, error) {
	return c.readRegisters(ctx, FuncReadInputRegisters, address, quantity)
}

func (c *Client) readRegisters(ctx context.Context, fn FunctionCode, address, quantity uint16) ([]uint16, error) {
	if err := quantityRange(quantity, 125); err != nil {
		return nil, err
	}
	req := PDU{Function: fn, Data: make([]byte, 4)}
	putUint16(req.Data[0:], address)
	putUint16(req.Data[2:], quantity)

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	byteCount := int(quantity * 2)
	if len(resp.Data) != 1+byteCount || int(resp.Data[0]) != byteCount {
		return nil, fmt.Errorf("%w: invalid register read byte count", ErrInvalidResponse)
	}
	return bytesToRegisters(resp.Data[1:]), nil
}

func (c *Client) WriteSingleCoil(ctx context.Context, address uint16, value bool) error {
	req := PDU{Function: FuncWriteSingleCoil, Data: make([]byte, 4)}
	putUint16(req.Data[0:], address)
	if value {
		putUint16(req.Data[2:], 0xFF00)
	} else {
		putUint16(req.Data[2:], 0x0000)
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return err
	}
	if len(resp.Data) != 4 || !bytes.Equal(resp.Data, req.Data) {
		return fmt.Errorf("%w: write single coil echo mismatch", ErrInvalidResponse)
	}
	return nil
}

func (c *Client) WriteSingleRegister(ctx context.Context, address, value uint16) error {
	req := PDU{Function: FuncWriteSingleRegister, Data: make([]byte, 4)}
	putUint16(req.Data[0:], address)
	putUint16(req.Data[2:], value)
	resp, err := c.do(ctx, req)
	if err != nil {
		return err
	}
	if len(resp.Data) != 4 || !bytes.Equal(resp.Data, req.Data) {
		return fmt.Errorf("%w: write single register echo mismatch", ErrInvalidResponse)
	}
	return nil
}

func (c *Client) Diagnostic(ctx context.Context, subFunction, data uint16) (uint16, error) {
	resp, err := c.do(ctx, PDU{Function: FuncDiagnostic, Data: buildDiagnosticData(subFunction, data)})
	if err != nil {
		return 0, err
	}
	respSubFunction, respData, err := parseDiagnosticData(resp.Data)
	if err != nil {
		return 0, err
	}
	if respSubFunction != subFunction {
		return 0, fmt.Errorf("%w: diagnostic sub-function mismatch", ErrInvalidResponse)
	}
	return respData, nil
}

func (c *Client) ReadExceptionStatus(ctx context.Context) (byte, error) {
	resp, err := c.do(ctx, PDU{Function: FuncReadExceptionStatus})
	if err != nil {
		return 0, err
	}
	if len(resp.Data) != 1 {
		return 0, fmt.Errorf("%w: read exception status response length mismatch", ErrInvalidResponse)
	}
	return resp.Data[0], nil
}

func (c *Client) GetCommEventCounter(ctx context.Context) (CommEventCounter, error) {
	resp, err := c.do(ctx, PDU{Function: FuncGetCommEventCounter})
	if err != nil {
		return CommEventCounter{}, err
	}
	return parseCommEventCounterData(resp.Data)
}

func (c *Client) GetCommEventLog(ctx context.Context) (CommEventLog, error) {
	resp, err := c.do(ctx, PDU{Function: FuncGetCommEventLog})
	if err != nil {
		return CommEventLog{}, err
	}
	return parseCommEventLogData(resp.Data)
}

func (c *Client) ReportServerID(ctx context.Context) ([]byte, error) {
	resp, err := c.do(ctx, PDU{Function: FuncReportServerID})
	if err != nil {
		return nil, err
	}
	return parseReportServerIDData(resp.Data)
}

func (c *Client) ReadTag(ctx context.Context, tag Tag) (Value, error) {
	tag = tag.withDefaults(c.unitID)
	client := c.withUnitID(tag.UnitID)
	raw, err := client.readTagRaw(ctx, tag)
	if err != nil {
		return Value{}, err
	}
	return DecodeValue(tag, raw)
}

func (c *Client) WriteTag(ctx context.Context, tag Tag, value any) error {
	tag = tag.withDefaults(c.unitID)
	client := c.withUnitID(tag.UnitID)
	raw, err := EncodeValue(tag, value)
	if err != nil {
		return err
	}
	switch tag.Area {
	case AreaCoil:
		return client.WriteMultipleCoils(ctx, tag.Address, raw.Bits)
	case AreaHoldingRegister:
		return client.WriteMultipleRegisters(ctx, tag.Address, raw.Registers)
	default:
		return fmt.Errorf("%w: area %s is read-only", ErrInvalidRequest, tag.Area)
	}
}

func (c *Client) WriteTags(ctx context.Context, values map[string]TagValue) error {
	if len(values) == 0 {
		return nil
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)

	groups := make(map[tagBatchKey][]tagWritePlan)
	for _, name := range names {
		tagValue := values[name]
		tag := tagValue.Tag.withDefaults(c.unitID)
		if err := tag.Validate(); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if tag.Area != AreaCoil && tag.Area != AreaHoldingRegister {
			return fmt.Errorf("%s: %w: area %s is read-only", name, ErrInvalidRequest, tag.Area)
		}
		raw, err := EncodeValue(tag, tagValue.Value)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		quantity, err := rawWriteQuantity(tag.Area, raw)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		end := uint32(tag.Address) + uint32(quantity)
		if end > 65536 {
			return fmt.Errorf("%s: %w: address range exceeds 65535", name, ErrInvalidRequest)
		}
		key := tagBatchKey{unitID: tag.UnitID, area: tag.Area}
		groups[key] = append(groups[key], tagWritePlan{
			name:     name,
			tag:      tag,
			raw:      raw,
			start:    tag.Address,
			quantity: quantity,
			end:      end,
		})
	}
	for key, plans := range groups {
		sort.SliceStable(plans, func(i, j int) bool {
			if plans[i].start == plans[j].start {
				return plans[i].name < plans[j].name
			}
			return plans[i].start < plans[j].start
		})
		ranges := mergeTagWritePlans(plans, maxWriteQuantity(key.area))
		client := c.withUnitID(key.unitID)
		for _, writeRange := range ranges {
			if err := client.writeAreaRaw(ctx, key.area, writeRange.start, writeRange.raw); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Client) withUnitID(unitID byte) *Client {
	if unitID == c.unitID {
		return c
	}
	clone := *c
	clone.unitID = unitID
	return &clone
}

func (c *Client) ReadTags(ctx context.Context, tags map[string]Tag) (map[string]Value, error) {
	out := make(map[string]Value, len(tags))
	if len(tags) == 0 {
		return out, nil
	}
	groups := make(map[tagBatchKey][]tagReadPlan)
	for name, tag := range tags {
		tag = tag.withDefaults(c.unitID)
		if err := tag.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		quantity := tag.modbusQuantity()
		end := uint32(tag.Address) + uint32(quantity)
		if end > 65536 {
			return nil, fmt.Errorf("%s: %w: address range exceeds 65535", name, ErrInvalidRequest)
		}
		key := tagBatchKey{unitID: tag.UnitID, area: tag.Area}
		groups[key] = append(groups[key], tagReadPlan{
			name:     name,
			tag:      tag,
			start:    tag.Address,
			quantity: quantity,
			end:      end,
		})
	}
	for key, plans := range groups {
		sort.Slice(plans, func(i, j int) bool {
			if plans[i].start == plans[j].start {
				return plans[i].quantity < plans[j].quantity
			}
			return plans[i].start < plans[j].start
		})
		ranges := mergeTagReadPlans(plans, maxReadQuantity(key.area))
		client := c.withUnitID(key.unitID)
		for _, readRange := range ranges {
			raw, err := client.readAreaRaw(ctx, key.area, readRange.start, readRange.quantity)
			if err != nil {
				return nil, err
			}
			for _, plan := range readRange.plans {
				offset := int(plan.start - readRange.start)
				rawPart := sliceRawValue(key.area, raw, offset, int(plan.quantity))
				value, err := DecodeValue(plan.tag, rawPart)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", plan.name, err)
				}
				out[plan.name] = value
			}
		}
	}
	return out, nil
}

type tagBatchKey struct {
	unitID byte
	area   Area
}

type tagReadPlan struct {
	name     string
	tag      Tag
	start    uint16
	quantity uint16
	end      uint32
}

type mergedTagReadPlan struct {
	start    uint16
	quantity uint16
	end      uint32
	plans    []tagReadPlan
}

type TagValue struct {
	Tag   Tag
	Value any
}

type tagWritePlan struct {
	name     string
	tag      Tag
	raw      RawValue
	start    uint16
	quantity uint16
	end      uint32
}

type mergedTagWritePlan struct {
	start    uint16
	quantity uint16
	end      uint32
	raw      RawValue
}

func mergeTagReadPlans(plans []tagReadPlan, maxQuantity uint16) []mergedTagReadPlan {
	if len(plans) == 0 {
		return nil
	}
	merged := make([]mergedTagReadPlan, 0, len(plans))
	current := mergedTagReadPlan{
		start:    plans[0].start,
		quantity: plans[0].quantity,
		end:      plans[0].end,
		plans:    []tagReadPlan{plans[0]},
	}
	for _, plan := range plans[1:] {
		canMerge := uint32(plan.start) <= current.end && plan.end-uint32(current.start) <= uint32(maxQuantity)
		if canMerge {
			if plan.end > current.end {
				current.end = plan.end
				current.quantity = uint16(current.end - uint32(current.start))
			}
			current.plans = append(current.plans, plan)
			continue
		}
		merged = append(merged, current)
		current = mergedTagReadPlan{
			start:    plan.start,
			quantity: plan.quantity,
			end:      plan.end,
			plans:    []tagReadPlan{plan},
		}
	}
	merged = append(merged, current)
	return merged
}

func mergeTagWritePlans(plans []tagWritePlan, maxQuantity uint16) []mergedTagWritePlan {
	if len(plans) == 0 {
		return nil
	}
	merged := make([]mergedTagWritePlan, 0, len(plans))
	current := newMergedTagWritePlan(plans[0])
	for _, plan := range plans[1:] {
		canMerge := uint32(plan.start) <= current.end && plan.end-uint32(current.start) <= uint32(maxQuantity)
		if canMerge {
			if plan.end > current.end {
				growMergedWritePlan(&current, plan.end)
			}
			overlayMergedWritePlan(&current, plan)
			continue
		}
		merged = append(merged, current)
		current = newMergedTagWritePlan(plan)
	}
	merged = append(merged, current)
	return merged
}

func newMergedTagWritePlan(plan tagWritePlan) mergedTagWritePlan {
	raw := cloneRawValue(plan.raw)
	return mergedTagWritePlan{
		start:    plan.start,
		quantity: plan.quantity,
		end:      plan.end,
		raw:      raw,
	}
}

func growMergedWritePlan(plan *mergedTagWritePlan, end uint32) {
	add := int(end - plan.end)
	if len(plan.raw.Bits) > 0 {
		plan.raw.Bits = append(plan.raw.Bits, make([]bool, add)...)
	}
	if len(plan.raw.Registers) > 0 {
		plan.raw.Registers = append(plan.raw.Registers, make([]uint16, add)...)
	}
	plan.end = end
	plan.quantity = uint16(plan.end - uint32(plan.start))
}

func overlayMergedWritePlan(merged *mergedTagWritePlan, plan tagWritePlan) {
	offset := int(plan.start - merged.start)
	if len(plan.raw.Bits) > 0 {
		copy(merged.raw.Bits[offset:], plan.raw.Bits)
	}
	if len(plan.raw.Registers) > 0 {
		copy(merged.raw.Registers[offset:], plan.raw.Registers)
	}
}

func cloneRawValue(raw RawValue) RawValue {
	return RawValue{
		Bits:      append([]bool(nil), raw.Bits...),
		Registers: append([]uint16(nil), raw.Registers...),
	}
}

func maxReadQuantity(area Area) uint16 {
	switch area {
	case AreaCoil, AreaDiscreteInput:
		return 2000
	default:
		return 125
	}
}

func maxWriteQuantity(area Area) uint16 {
	switch area {
	case AreaCoil:
		return 1968
	default:
		return 123
	}
}

func rawWriteQuantity(area Area, raw RawValue) (uint16, error) {
	switch area {
	case AreaCoil:
		if len(raw.Bits) == 0 || len(raw.Bits) > 1968 {
			return 0, fmt.Errorf("%w: coil count must be 1..1968", ErrInvalidRequest)
		}
		return uint16(len(raw.Bits)), nil
	case AreaHoldingRegister:
		if len(raw.Registers) == 0 || len(raw.Registers) > 123 {
			return 0, fmt.Errorf("%w: register count must be 1..123", ErrInvalidRequest)
		}
		return uint16(len(raw.Registers)), nil
	default:
		return 0, fmt.Errorf("%w: area %s is read-only", ErrInvalidRequest, area)
	}
}

func sliceRawValue(area Area, raw RawValue, offset, quantity int) RawValue {
	switch area {
	case AreaCoil, AreaDiscreteInput:
		return RawValue{Bits: raw.Bits[offset : offset+quantity]}
	default:
		return RawValue{Registers: raw.Registers[offset : offset+quantity]}
	}
}

func (c *Client) writeAreaRaw(ctx context.Context, area Area, address uint16, raw RawValue) error {
	switch area {
	case AreaCoil:
		return c.WriteMultipleCoils(ctx, address, raw.Bits)
	case AreaHoldingRegister:
		return c.WriteMultipleRegisters(ctx, address, raw.Registers)
	default:
		return fmt.Errorf("%w: area %s is read-only", ErrInvalidRequest, area)
	}
}

func (c *Client) readAreaRaw(ctx context.Context, area Area, address, quantity uint16) (RawValue, error) {
	switch area {
	case AreaCoil:
		bits, err := c.ReadCoils(ctx, address, quantity)
		return RawValue{Bits: bits}, err
	case AreaDiscreteInput:
		bits, err := c.ReadDiscreteInputs(ctx, address, quantity)
		return RawValue{Bits: bits}, err
	case AreaHoldingRegister:
		registers, err := c.ReadHoldingRegisters(ctx, address, quantity)
		return RawValue{Registers: registers}, err
	case AreaInputRegister:
		registers, err := c.ReadInputRegisters(ctx, address, quantity)
		return RawValue{Registers: registers}, err
	default:
		return RawValue{}, fmt.Errorf("%w: unknown area", ErrInvalidRequest)
	}
}

func (c *Client) readTagRaw(ctx context.Context, tag Tag) (RawValue, error) {
	if err := tag.Validate(); err != nil {
		return RawValue{}, err
	}
	switch tag.Area {
	case AreaCoil:
		bits, err := c.ReadCoils(ctx, tag.Address, tag.modbusQuantity())
		return RawValue{Bits: bits}, err
	case AreaDiscreteInput:
		bits, err := c.ReadDiscreteInputs(ctx, tag.Address, tag.modbusQuantity())
		return RawValue{Bits: bits}, err
	case AreaHoldingRegister:
		registers, err := c.ReadHoldingRegisters(ctx, tag.Address, tag.modbusQuantity())
		return RawValue{Registers: registers}, err
	case AreaInputRegister:
		registers, err := c.ReadInputRegisters(ctx, tag.Address, tag.modbusQuantity())
		return RawValue{Registers: registers}, err
	default:
		return RawValue{}, fmt.Errorf("%w: unknown area", ErrInvalidRequest)
	}
}

func (c *Client) WriteMultipleCoils(ctx context.Context, address uint16, values []bool) error {
	if len(values) == 0 || len(values) > 1968 {
		return fmt.Errorf("%w: coil count must be 1..1968", ErrInvalidRequest)
	}
	payload := packBits(values)
	req := PDU{Function: FuncWriteMultipleCoils, Data: make([]byte, 5+len(payload))}
	putUint16(req.Data[0:], address)
	putUint16(req.Data[2:], uint16(len(values)))
	req.Data[4] = byte(len(payload))
	copy(req.Data[5:], payload)
	return c.writeMultiple(ctx, req, address, uint16(len(values)))
}

func (c *Client) WriteMultipleRegisters(ctx context.Context, address uint16, values []uint16) error {
	if len(values) == 0 || len(values) > 123 {
		return fmt.Errorf("%w: register count must be 1..123", ErrInvalidRequest)
	}
	payload := registersToBytes(values)
	req := PDU{Function: FuncWriteMultipleRegisters, Data: make([]byte, 5+len(payload))}
	putUint16(req.Data[0:], address)
	putUint16(req.Data[2:], uint16(len(values)))
	req.Data[4] = byte(len(payload))
	copy(req.Data[5:], payload)
	return c.writeMultiple(ctx, req, address, uint16(len(values)))
}

func (c *Client) ReadWriteMultipleRegisters(ctx context.Context, readAddress, readQuantity, writeAddress uint16, values []uint16) ([]uint16, error) {
	if err := quantityRange(readQuantity, 125); err != nil {
		return nil, err
	}
	if len(values) == 0 || len(values) > 121 {
		return nil, fmt.Errorf("%w: write register count must be 1..121", ErrInvalidRequest)
	}
	payload := registersToBytes(values)
	req := PDU{Function: FuncReadWriteMultipleRegisters, Data: make([]byte, 9+len(payload))}
	putUint16(req.Data[0:], readAddress)
	putUint16(req.Data[2:], readQuantity)
	putUint16(req.Data[4:], writeAddress)
	putUint16(req.Data[6:], uint16(len(values)))
	req.Data[8] = byte(len(payload))
	copy(req.Data[9:], payload)

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	byteCount := int(readQuantity * 2)
	if len(resp.Data) != 1+byteCount || int(resp.Data[0]) != byteCount {
		return nil, fmt.Errorf("%w: invalid read/write register byte count", ErrInvalidResponse)
	}
	return bytesToRegisters(resp.Data[1:]), nil
}

func (c *Client) ReadFileRecords(ctx context.Context, requests []FileRecordRequest) ([]FileRecord, error) {
	data, err := buildReadFileRecordRequestData(requests)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, PDU{Function: FuncReadFileRecord, Data: data})
	if err != nil {
		return nil, err
	}
	return parseReadFileRecordResponseData(resp.Data, requests)
}

func (c *Client) WriteFileRecords(ctx context.Context, records []FileRecord) error {
	data, err := buildWriteFileRecordData(records)
	if err != nil {
		return err
	}
	req := PDU{Function: FuncWriteFileRecord, Data: data}
	resp, err := c.do(ctx, req)
	if err != nil {
		return err
	}
	if !bytes.Equal(resp.Data, req.Data) {
		return fmt.Errorf("%w: write file record echo mismatch", ErrInvalidResponse)
	}
	return nil
}

func (c *Client) ReadFIFOQueue(ctx context.Context, address uint16) ([]uint16, error) {
	req := PDU{Function: FuncReadFIFOQueue, Data: make([]byte, 2)}
	putUint16(req.Data, address)
	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	return parseFIFOQueueResponse(resp.Data)
}

func (c *Client) ReadDeviceIdentification(ctx context.Context, code ReadDeviceIDCode) (DeviceIdentification, error) {
	objectID, err := startDeviceIdentificationObjectID(code)
	if err != nil {
		return DeviceIdentification{}, err
	}
	return c.readDeviceIdentification(ctx, code, objectID)
}

func (c *Client) ReadDeviceIdentificationObject(ctx context.Context, objectID byte) (DeviceIdentification, error) {
	return c.readDeviceIdentification(ctx, ReadDeviceIDCodeSpecific, objectID)
}

func (c *Client) readDeviceIdentification(ctx context.Context, code ReadDeviceIDCode, objectID byte) (DeviceIdentification, error) {
	var merged DeviceIdentification
	seen := make(map[byte]struct{})
	for {
		if _, ok := seen[objectID]; ok {
			return DeviceIdentification{}, fmt.Errorf("%w: device identification pagination loop at object 0x%02x", ErrInvalidResponse, objectID)
		}
		seen[objectID] = struct{}{}

		page, err := c.readDeviceIdentificationPage(ctx, code, objectID)
		if err != nil {
			return DeviceIdentification{}, err
		}
		if merged.Objects == nil {
			merged = page
			merged.Objects = make(map[byte][]byte, len(page.Objects))
		}
		for id, value := range page.Objects {
			merged.Objects[id] = append([]byte(nil), value...)
		}
		merged.MoreFollows = page.MoreFollows
		merged.NextObjectID = page.NextObjectID
		if !page.MoreFollows || page.NextObjectID == 0 {
			return merged, nil
		}
		objectID = page.NextObjectID
	}
}

func (c *Client) readDeviceIdentificationPage(ctx context.Context, code ReadDeviceIDCode, objectID byte) (DeviceIdentification, error) {
	req := PDU{
		Function: FuncReadDeviceIdentification,
		Data:     []byte{meiTypeReadDeviceIdentification, byte(code), objectID},
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return DeviceIdentification{}, err
	}
	info, err := parseDeviceIdentificationResponse(resp.Data)
	if err != nil {
		return DeviceIdentification{}, err
	}
	if info.ReadDeviceIDCode != code {
		return DeviceIdentification{}, fmt.Errorf("%w: device identification code mismatch", ErrInvalidResponse)
	}
	return info, nil
}

func (c *Client) MaskWriteRegister(ctx context.Context, address, andMask, orMask uint16) error {
	req := PDU{Function: FuncMaskWriteRegister, Data: make([]byte, 6)}
	putUint16(req.Data[0:], address)
	putUint16(req.Data[2:], andMask)
	putUint16(req.Data[4:], orMask)
	resp, err := c.do(ctx, req)
	if err != nil {
		return err
	}
	if len(resp.Data) != 6 || !bytes.Equal(resp.Data, req.Data) {
		return fmt.Errorf("%w: mask write register echo mismatch", ErrInvalidResponse)
	}
	return nil
}

func (c *Client) writeMultiple(ctx context.Context, req PDU, address, quantity uint16) error {
	resp, err := c.do(ctx, req)
	if err != nil {
		return err
	}
	if len(resp.Data) != 4 || uint16At(resp.Data[0:]) != address || uint16At(resp.Data[2:]) != quantity {
		return fmt.Errorf("%w: write multiple echo mismatch", ErrInvalidResponse)
	}
	return nil
}
