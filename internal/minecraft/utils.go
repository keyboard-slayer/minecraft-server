package minecraft

import (
	"errors"

	"github.com/google/uuid"
)

const (
	SEGMENT_BITS = 0x7F
	CONTINUE_BIT = 0x80
)

func readVarIntFromBuff(buff []byte) (int, int, error) {
	var value int = 0
	var pos int = 0
	var idx int = 0

	for {
		if idx >= len(buff) {
			return 0, 0, errors.New("unexpected end of buffer while reading VarInt")
		}

		currentByte := buff[idx]
		idx += 1

		value |= (int(currentByte) & SEGMENT_BITS) << pos

		if (currentByte & CONTINUE_BIT) == 0 {
			break
		}

		pos += 7
		if pos >= 32 {
			return 0, 0, errors.New("VarInt is too big")
		}
	}

	return value, idx, nil

}

func writeVarInt(value int) []byte {
	bytes := make([]byte, 0)

	for {
		if (value & ^SEGMENT_BITS) == 0 {
			bytes = append(bytes, byte(value))
			return bytes
		}

		bytes = append(bytes, byte((value&SEGMENT_BITS)|CONTINUE_BIT))
		value >>= 7
	}
}

type factory func(buffer []byte) ([]byte, any, error)

type factoryPair struct {
	name    string
	factory factory
}

func uuidFactory(buffer []byte) ([]byte, any, error) {
	id, err := uuid.FromBytes(buffer[0:16])
	return buffer[16:], id, err
}

func intFactory(buffer []byte) ([]byte, any, error) {
	r, sz, err := readVarIntFromBuff(buffer)
	return buffer[sz:], r, err
}

func bytesFactory(buffer []byte) ([]byte, any, error) {
	length, sz, err := readVarIntFromBuff(buffer)
	if err != nil {
		return []byte{}, "", nil
	}

	if length > len(buffer[sz:]) {
		return []byte{}, "", errors.New("length is larger then the length of the buffer")
	}

	ret := buffer[sz : sz+length]
	return buffer[sz+length:], ret, nil
}

func ushortFactory(buffer []byte) ([]byte, any, error) {
	return buffer[2:], (int(buffer[0]) << 8) | int(buffer[1]), nil
}

func readFromBuffer(buffer []byte, pairs ...factoryPair) (map[string]any, error) {
	results := make(map[string]any)

	for _, p := range pairs {
		tmp, ret, err := p.factory(buffer)
		if err != nil {
			return map[string]any{}, err
		}

		results[p.name] = ret
		buffer = tmp
	}

	return results, nil
}
