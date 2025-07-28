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
		currentByte := buff[idx]

		value |= (int(currentByte) & SEGMENT_BITS) << pos

		if (currentByte & CONTINUE_BIT) == 0 {
			break
		}

		pos += 7
		idx += 1

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

	if len(buffer) == 16 {
		return []byte{}, id, err
	}

	return buffer[17:], id, err
}

func intFactory(buffer []byte) ([]byte, any, error) {
	r, sz, err := readVarIntFromBuff(buffer)
	return buffer[sz+1:], r, err
}

func bytesFactory(buffer []byte) ([]byte, any, error) {
	length, sz, err := readVarIntFromBuff(buffer)
	if err != nil {
		return []byte{}, "", nil
	}

	ret := buffer[sz+1 : sz+1+length]
	return buffer[sz+1+length:], ret, nil
}

func stringFactory(buffer []byte) ([]byte, any, error) {
	length, sz, err := readVarIntFromBuff(buffer)
	if err != nil {
		return []byte{}, "", nil
	}

	ret := string(buffer[sz+1 : sz+1+length])
	return buffer[sz+1+length:], ret, nil
}

func ushortFactory(buffer []byte) ([]byte, any, error) {
	return buffer[2:], (int(buffer[0]) << 8) | int(buffer[1]), nil
}

func readFromBuffer(buffer []byte, pairs map[string]factory) (map[string]any, error) {
	results := make(map[string]any)

	for key, factory := range pairs {
		tmp, ret, err := factory(buffer)
		if err != nil {
			return map[string]any{}, err
		}

		results[key] = ret
		buffer = tmp
	}

	return results, nil
}
