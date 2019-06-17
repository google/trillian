package dynamic

// Binary serialization and de-serialization for dynamic messages

import (
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"

	"github.com/jhump/protoreflect/desc"
)

// defaultDeterminism, if true, will mean that calls to Marshal will produce
// deterministic output. This is used to make the output of proto.Marshal(...)
// deterministic (since there is no way to have that convey determinism intent).
// **This is only used from tests.**
var defaultDeterminism = false

// Marshal serializes this message to bytes, returning an error if the operation
// fails. The resulting bytes are in the standard protocol buffer binary format.
func (m *Message) Marshal() ([]byte, error) {
	var b codedBuffer
	if err := m.marshal(&b, defaultDeterminism); err != nil {
		return nil, err
	}
	return b.buf, nil
}

// MarshalAppend behaves exactly the same as Marshal, except instead of allocating a
// new byte slice to marshal into, it uses the provided byte slice. The backing array
// for the returned byte slice *may* be the same as the one that was passed in, but
// it's not guaranteed as a new backing array will automatically be allocated if
// more bytes need to be written than the provided buffer has capacity for.
func (m *Message) MarshalAppend(b []byte) ([]byte, error) {
	codedBuf := codedBuffer{buf: b}
	if err := m.marshal(&codedBuf, defaultDeterminism); err != nil {
		return nil, err
	}
	return codedBuf.buf, nil
}

// MarshalDeterministic serializes this message to bytes in a deterministic way,
// returning an error if the operation fails. This differs from Marshal in that
// map keys will be sorted before serializing to bytes. The protobuf spec does
// not define ordering for map entries, so Marshal will use standard Go map
// iteration order (which will be random). But for cases where determinism is
// more important than performance, use this method instead.
func (m *Message) MarshalDeterministic() ([]byte, error) {
	var b codedBuffer
	if err := m.marshal(&b, true); err != nil {
		return nil, err
	}
	return b.buf, nil
}

func (m *Message) marshal(b *codedBuffer, deterministic bool) error {
	if err := m.marshalKnownFields(b, deterministic); err != nil {
		return err
	}
	return m.marshalUnknownFields(b)
}

func (m *Message) marshalKnownFields(b *codedBuffer, deterministic bool) error {
	for _, tag := range m.knownFieldTags() {
		itag := int32(tag)
		val := m.values[itag]
		fd := m.FindFieldDescriptor(itag)
		if fd == nil {
			panic(fmt.Sprintf("Couldn't find field for tag %d", itag))
		}
		if err := marshalField(itag, fd, val, b, deterministic); err != nil {
			return err
		}
	}
	return nil
}

func (m *Message) marshalUnknownFields(b *codedBuffer) error {
	for _, tag := range m.unknownFieldTags() {
		itag := int32(tag)
		sl := m.unknownFields[itag]
		for _, u := range sl {
			if err := b.encodeTagAndWireType(itag, u.Encoding); err != nil {
				return err
			}
			switch u.Encoding {
			case proto.WireBytes:
				if err := b.encodeRawBytes(u.Contents); err != nil {
					return err
				}
			case proto.WireStartGroup:
				b.buf = append(b.buf, u.Contents...)
				if err := b.encodeTagAndWireType(itag, proto.WireEndGroup); err != nil {
					return err
				}
			case proto.WireFixed32:
				if err := b.encodeFixed32(u.Value); err != nil {
					return err
				}
			case proto.WireFixed64:
				if err := b.encodeFixed64(u.Value); err != nil {
					return err
				}
			case proto.WireVarint:
				if err := b.encodeVarint(u.Value); err != nil {
					return err
				}
			default:
				return proto.ErrInternalBadWireType
			}
		}
	}
	return nil
}

func marshalField(tag int32, fd *desc.FieldDescriptor, val interface{}, b *codedBuffer, deterministic bool) error {
	if fd.IsMap() {
		mp := val.(map[interface{}]interface{})
		entryType := fd.GetMessageType()
		keyType := entryType.FindFieldByNumber(1)
		valType := entryType.FindFieldByNumber(2)
		var entryBuffer codedBuffer
		if deterministic {
			keys := make([]interface{}, 0, len(mp))
			for k := range mp {
				keys = append(keys, k)
			}
			sort.Sort(sortable(keys))
			for _, k := range keys {
				v := mp[k]
				entryBuffer.reset()
				if err := marshalFieldElement(1, keyType, k, &entryBuffer, deterministic); err != nil {
					return err
				}
				if err := marshalFieldElement(2, valType, v, &entryBuffer, deterministic); err != nil {
					return err
				}
				if err := b.encodeTagAndWireType(tag, proto.WireBytes); err != nil {
					return err
				}
				if err := b.encodeRawBytes(entryBuffer.buf); err != nil {
					return err
				}
			}
		} else {
			for k, v := range mp {
				entryBuffer.reset()
				if err := marshalFieldElement(1, keyType, k, &entryBuffer, deterministic); err != nil {
					return err
				}
				if err := marshalFieldElement(2, valType, v, &entryBuffer, deterministic); err != nil {
					return err
				}
				if err := b.encodeTagAndWireType(tag, proto.WireBytes); err != nil {
					return err
				}
				if err := b.encodeRawBytes(entryBuffer.buf); err != nil {
					return err
				}
			}
		}
		return nil
	} else if fd.IsRepeated() {
		sl := val.([]interface{})
		wt, err := getWireType(fd.GetType())
		if err != nil {
			return err
		}
		if isPacked(fd) && len(sl) > 1 &&
			(wt == proto.WireVarint || wt == proto.WireFixed32 || wt == proto.WireFixed64) {
			// packed repeated field
			var packedBuffer codedBuffer
			for _, v := range sl {
				if err := marshalFieldValue(fd, v, &packedBuffer, deterministic); err != nil {
					return err
				}
			}
			if err := b.encodeTagAndWireType(tag, proto.WireBytes); err != nil {
				return err
			}
			return b.encodeRawBytes(packedBuffer.buf)
		} else {
			// non-packed repeated field
			for _, v := range sl {
				if err := marshalFieldElement(tag, fd, v, b, deterministic); err != nil {
					return err
				}
			}
			return nil
		}
	} else {
		return marshalFieldElement(tag, fd, val, b, deterministic)
	}
}

func isPacked(fd *desc.FieldDescriptor) bool {
	opts := fd.AsFieldDescriptorProto().GetOptions()
	// if set, use that value
	if opts != nil && opts.Packed != nil {
		return opts.GetPacked()
	}
	// if unset: proto2 defaults to false, proto3 to true
	return fd.GetFile().IsProto3()
}

// sortable is used to sort map keys. Values will be integers (int32, int64, uint32, and uint64),
// bools, or strings.
type sortable []interface{}

func (s sortable) Len() int {
	return len(s)
}

func (s sortable) Less(i, j int) bool {
	vi := s[i]
	vj := s[j]
	switch reflect.TypeOf(vi).Kind() {
	case reflect.Int32:
		return vi.(int32) < vj.(int32)
	case reflect.Int64:
		return vi.(int64) < vj.(int64)
	case reflect.Uint32:
		return vi.(uint32) < vj.(uint32)
	case reflect.Uint64:
		return vi.(uint64) < vj.(uint64)
	case reflect.String:
		return vi.(string) < vj.(string)
	case reflect.Bool:
		return vi.(bool) && !vj.(bool)
	default:
		panic(fmt.Sprintf("cannot compare keys of type %v", reflect.TypeOf(vi)))
	}
}

func (s sortable) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func marshalFieldElement(tag int32, fd *desc.FieldDescriptor, val interface{}, b *codedBuffer, deterministic bool) error {
	wt, err := getWireType(fd.GetType())
	if err != nil {
		return err
	}
	if err := b.encodeTagAndWireType(tag, wt); err != nil {
		return err
	}
	if err := marshalFieldValue(fd, val, b, deterministic); err != nil {
		return err
	}
	if wt == proto.WireStartGroup {
		return b.encodeTagAndWireType(tag, proto.WireEndGroup)
	}
	return nil
}

func marshalFieldValue(fd *desc.FieldDescriptor, val interface{}, b *codedBuffer, deterministic bool) error {
	switch fd.GetType() {
	case descriptor.FieldDescriptorProto_TYPE_BOOL:
		v := val.(bool)
		if v {
			return b.encodeVarint(1)
		} else {
			return b.encodeVarint(0)
		}

	case descriptor.FieldDescriptorProto_TYPE_ENUM,
		descriptor.FieldDescriptorProto_TYPE_INT32:
		v := val.(int32)
		return b.encodeVarint(uint64(v))

	case descriptor.FieldDescriptorProto_TYPE_SFIXED32:
		v := val.(int32)
		return b.encodeFixed32(uint64(v))

	case descriptor.FieldDescriptorProto_TYPE_SINT32:
		v := val.(int32)
		return b.encodeVarint(encodeZigZag32(v))

	case descriptor.FieldDescriptorProto_TYPE_UINT32:
		v := val.(uint32)
		return b.encodeVarint(uint64(v))

	case descriptor.FieldDescriptorProto_TYPE_FIXED32:
		v := val.(uint32)
		return b.encodeFixed32(uint64(v))

	case descriptor.FieldDescriptorProto_TYPE_INT64:
		v := val.(int64)
		return b.encodeVarint(uint64(v))

	case descriptor.FieldDescriptorProto_TYPE_SFIXED64:
		v := val.(int64)
		return b.encodeFixed64(uint64(v))

	case descriptor.FieldDescriptorProto_TYPE_SINT64:
		v := val.(int64)
		return b.encodeVarint(encodeZigZag64(v))

	case descriptor.FieldDescriptorProto_TYPE_UINT64:
		v := val.(uint64)
		return b.encodeVarint(v)

	case descriptor.FieldDescriptorProto_TYPE_FIXED64:
		v := val.(uint64)
		return b.encodeFixed64(v)

	case descriptor.FieldDescriptorProto_TYPE_DOUBLE:
		v := val.(float64)
		return b.encodeFixed64(math.Float64bits(v))

	case descriptor.FieldDescriptorProto_TYPE_FLOAT:
		v := val.(float32)
		return b.encodeFixed32(uint64(math.Float32bits(v)))

	case descriptor.FieldDescriptorProto_TYPE_BYTES:
		v := val.([]byte)
		return b.encodeRawBytes(v)

	case descriptor.FieldDescriptorProto_TYPE_STRING:
		v := val.(string)
		return b.encodeRawBytes(([]byte)(v))

	case descriptor.FieldDescriptorProto_TYPE_MESSAGE:
		m := val.(proto.Message)
		if bytes, err := proto.Marshal(m); err != nil {
			return err
		} else {
			return b.encodeRawBytes(bytes)
		}

	case descriptor.FieldDescriptorProto_TYPE_GROUP:
		// just append the nested message to this buffer
		dm, ok := val.(*Message)
		if ok {
			return dm.marshal(b, deterministic)
		} else {
			m := val.(proto.Message)
			return b.encodeMessage(m)
		}
		// whosoever writeth start-group tag (e.g. caller) is responsible for writing end-group tag

	default:
		return fmt.Errorf("unrecognized field type: %v", fd.GetType())
	}
}

func getWireType(t descriptor.FieldDescriptorProto_Type) (int8, error) {
	switch t {
	case descriptor.FieldDescriptorProto_TYPE_ENUM,
		descriptor.FieldDescriptorProto_TYPE_BOOL,
		descriptor.FieldDescriptorProto_TYPE_INT32,
		descriptor.FieldDescriptorProto_TYPE_SINT32,
		descriptor.FieldDescriptorProto_TYPE_UINT32,
		descriptor.FieldDescriptorProto_TYPE_INT64,
		descriptor.FieldDescriptorProto_TYPE_SINT64,
		descriptor.FieldDescriptorProto_TYPE_UINT64:
		return proto.WireVarint, nil

	case descriptor.FieldDescriptorProto_TYPE_FIXED32,
		descriptor.FieldDescriptorProto_TYPE_SFIXED32,
		descriptor.FieldDescriptorProto_TYPE_FLOAT:
		return proto.WireFixed32, nil

	case descriptor.FieldDescriptorProto_TYPE_FIXED64,
		descriptor.FieldDescriptorProto_TYPE_SFIXED64,
		descriptor.FieldDescriptorProto_TYPE_DOUBLE:
		return proto.WireFixed64, nil

	case descriptor.FieldDescriptorProto_TYPE_BYTES,
		descriptor.FieldDescriptorProto_TYPE_STRING,
		descriptor.FieldDescriptorProto_TYPE_MESSAGE:
		return proto.WireBytes, nil

	case descriptor.FieldDescriptorProto_TYPE_GROUP:
		return proto.WireStartGroup, nil

	default:
		return 0, proto.ErrInternalBadWireType
	}
}

// Unmarshal de-serializes the message that is present in the given bytes into
// this message. It first resets the current message. It returns an error if the
// given bytes do not contain a valid encoding of this message type.
func (m *Message) Unmarshal(b []byte) error {
	m.Reset()
	if err := m.UnmarshalMerge(b); err != nil {
		return err
	}
	return m.Validate()
}

// UnmarshalMerge de-serializes the message that is present in the given bytes
// into this message. Unlike Unmarshal, it does not first reset the message,
// instead merging the data in the given bytes into the existing data in this
// message.
func (m *Message) UnmarshalMerge(b []byte) error {
	return m.unmarshal(newCodedBuffer(b), false)
}

func (m *Message) unmarshal(buf *codedBuffer, isGroup bool) error {
	for !buf.eof() {
		tagNumber, wireType, err := buf.decodeTagAndWireType()
		if err != nil {
			return err
		}
		if wireType == proto.WireEndGroup {
			if isGroup {
				// finished parsing group
				return nil
			} else {
				return proto.ErrInternalBadWireType
			}
		}
		fd := m.FindFieldDescriptor(tagNumber)
		if fd == nil {
			err := m.unmarshalUnknownField(tagNumber, wireType, buf)
			if err != nil {
				return err
			}
		} else {
			err := m.unmarshalKnownField(fd, wireType, buf)
			if err != nil {
				return err
			}
		}
	}
	if isGroup {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func unmarshalSimpleField(fd *desc.FieldDescriptor, v uint64) (interface{}, error) {
	switch fd.GetType() {
	case descriptor.FieldDescriptorProto_TYPE_BOOL:
		return v != 0, nil
	case descriptor.FieldDescriptorProto_TYPE_UINT32,
		descriptor.FieldDescriptorProto_TYPE_FIXED32:
		if v > math.MaxUint32 {
			return nil, NumericOverflowError
		}
		return uint32(v), nil

	case descriptor.FieldDescriptorProto_TYPE_INT32,
		descriptor.FieldDescriptorProto_TYPE_ENUM:
		s := int64(v)
		if s > math.MaxInt32 || s < math.MinInt32 {
			return nil, NumericOverflowError
		}
		return int32(s), nil

	case descriptor.FieldDescriptorProto_TYPE_SFIXED32:
		if v > math.MaxUint32 {
			return nil, NumericOverflowError
		}
		return int32(v), nil

	case descriptor.FieldDescriptorProto_TYPE_SINT32:
		if v > math.MaxUint32 {
			return nil, NumericOverflowError
		}
		return decodeZigZag32(v), nil

	case descriptor.FieldDescriptorProto_TYPE_UINT64,
		descriptor.FieldDescriptorProto_TYPE_FIXED64:
		return v, nil

	case descriptor.FieldDescriptorProto_TYPE_INT64,
		descriptor.FieldDescriptorProto_TYPE_SFIXED64:
		return int64(v), nil

	case descriptor.FieldDescriptorProto_TYPE_SINT64:
		return decodeZigZag64(v), nil

	case descriptor.FieldDescriptorProto_TYPE_FLOAT:
		if v > math.MaxUint32 {
			return nil, NumericOverflowError
		}
		return math.Float32frombits(uint32(v)), nil

	case descriptor.FieldDescriptorProto_TYPE_DOUBLE:
		return math.Float64frombits(v), nil

	default:
		// bytes, string, message, and group cannot be represented as a simple numeric value
		return nil, fmt.Errorf("bad input; field %s requires length-delimited wire type", fd.GetFullyQualifiedName())
	}
}

func unmarshalLengthDelimitedField(fd *desc.FieldDescriptor, bytes []byte, mf *MessageFactory) (interface{}, error) {
	switch {
	case fd.GetType() == descriptor.FieldDescriptorProto_TYPE_BYTES:
		return bytes, nil

	case fd.GetType() == descriptor.FieldDescriptorProto_TYPE_STRING:
		return string(bytes), nil

	case fd.GetType() == descriptor.FieldDescriptorProto_TYPE_MESSAGE ||
		fd.GetType() == descriptor.FieldDescriptorProto_TYPE_GROUP:
		msg := mf.NewMessage(fd.GetMessageType())
		err := proto.Unmarshal(bytes, msg)
		if err != nil {
			return nil, err
		} else {
			return msg, nil
		}

	default:
		// even if the field is not repeated or not packed, we still parse it as such for
		// backwards compatibility (e.g. message we are de-serializing could have been both
		// repeated and packed at the time of serialization)
		packedBuf := newCodedBuffer(bytes)
		var slice []interface{}
		var val interface{}
		for !packedBuf.eof() {
			var v uint64
			var err error
			if varintTypes[fd.GetType()] {
				v, err = packedBuf.decodeVarint()
			} else if fixed32Types[fd.GetType()] {
				v, err = packedBuf.decodeFixed32()
			} else if fixed64Types[fd.GetType()] {
				v, err = packedBuf.decodeFixed64()
			} else {
				return nil, fmt.Errorf("bad input; cannot parse length-delimited wire type for field %s", fd.GetFullyQualifiedName())
			}
			if err != nil {
				return nil, err
			}
			val, err = unmarshalSimpleField(fd, v)
			if err != nil {
				return nil, err
			}
			if fd.IsRepeated() {
				slice = append(slice, val)
			}
		}
		if fd.IsRepeated() {
			return slice, nil
		} else {
			// if not a repeated field, last value wins
			return val, nil
		}
	}
}

func (m *Message) unmarshalKnownField(fd *desc.FieldDescriptor, encoding int8, b *codedBuffer) error {
	var val interface{}
	var err error
	switch encoding {
	case proto.WireFixed32:
		var num uint64
		num, err = b.decodeFixed32()
		if err == nil {
			val, err = unmarshalSimpleField(fd, num)
		}
	case proto.WireFixed64:
		var num uint64
		num, err = b.decodeFixed64()
		if err == nil {
			val, err = unmarshalSimpleField(fd, num)
		}
	case proto.WireVarint:
		var num uint64
		num, err = b.decodeVarint()
		if err == nil {
			val, err = unmarshalSimpleField(fd, num)
		}

	case proto.WireBytes:
		if fd.GetType() == descriptor.FieldDescriptorProto_TYPE_BYTES {
			val, err = b.decodeRawBytes(true) // defensive copy
		} else if fd.GetType() == descriptor.FieldDescriptorProto_TYPE_STRING {
			var raw []byte
			raw, err = b.decodeRawBytes(true) // defensive copy
			if err == nil {
				val = string(raw)
			}
		} else {
			var raw []byte
			raw, err = b.decodeRawBytes(false)
			if err == nil {
				val, err = unmarshalLengthDelimitedField(fd, raw, m.mf)
			}
		}

	case proto.WireStartGroup:
		if fd.GetMessageType() == nil {
			return fmt.Errorf("cannot parse field %s from group-encoded wire type", fd.GetFullyQualifiedName())
		}
		msg := m.mf.NewMessage(fd.GetMessageType())
		if dm, ok := msg.(*Message); ok {
			err = dm.unmarshal(b, true)
			if err == nil {
				val = dm
			}
		} else {
			var groupEnd, dataEnd int
			groupEnd, dataEnd, err = skipGroup(b)
			if err == nil {
				err = proto.Unmarshal(b.buf[b.index:dataEnd], msg)
				if err == nil {
					val = msg
				}
				b.index = groupEnd
			}
		}

	default:
		return proto.ErrInternalBadWireType
	}
	if err != nil {
		return err
	}

	return mergeField(m, fd, val)
}

func (m *Message) unmarshalUnknownField(tagNumber int32, encoding int8, b *codedBuffer) error {
	u := UnknownField{Encoding: encoding}
	var err error
	switch encoding {
	case proto.WireFixed32:
		u.Value, err = b.decodeFixed32()
	case proto.WireFixed64:
		u.Value, err = b.decodeFixed64()
	case proto.WireVarint:
		u.Value, err = b.decodeVarint()
	case proto.WireBytes:
		u.Contents, err = b.decodeRawBytes(true)
	case proto.WireStartGroup:
		var groupEnd, dataEnd int
		groupEnd, dataEnd, err = skipGroup(b)
		if err == nil {
			u.Contents = make([]byte, dataEnd-b.index)
			copy(u.Contents, b.buf[b.index:])
			b.index = groupEnd
		}
	default:
		err = proto.ErrInternalBadWireType
	}
	if err != nil {
		return err
	}
	if m.unknownFields == nil {
		m.unknownFields = map[int32][]UnknownField{}
	}
	m.unknownFields[tagNumber] = append(m.unknownFields[tagNumber], u)
	return nil
}

func skipGroup(b *codedBuffer) (int, int, error) {
	bs := b.buf
	start := b.index
	defer func() {
		b.index = start
	}()
	for {
		fieldStart := b.index
		// read a field tag
		_, wireType, err := b.decodeTagAndWireType()
		if err != nil {
			return 0, 0, err
		}
		// skip past the field's data
		switch wireType {
		case proto.WireFixed32:
			if !b.skip(4) {
				return 0, 0, io.ErrUnexpectedEOF
			}
		case proto.WireFixed64:
			if !b.skip(8) {
				return 0, 0, io.ErrUnexpectedEOF
			}
		case proto.WireVarint:
			// skip varint by finding last byte (has high bit unset)
			i := b.index
			for {
				if i >= len(bs) {
					return 0, 0, io.ErrUnexpectedEOF
				}
				if bs[i]&0x80 == 0 {
					break
				}
				i++
			}
			b.index = i + 1
		case proto.WireBytes:
			l, err := b.decodeVarint()
			if err != nil {
				return 0, 0, err
			}
			if !b.skip(int(l)) {
				return 0, 0, io.ErrUnexpectedEOF
			}
		case proto.WireStartGroup:
			endIndex, _, err := skipGroup(b)
			if err != nil {
				return 0, 0, err
			}
			b.index = endIndex
		case proto.WireEndGroup:
			return b.index, fieldStart, nil
		default:
			return 0, 0, proto.ErrInternalBadWireType
		}
	}
}
