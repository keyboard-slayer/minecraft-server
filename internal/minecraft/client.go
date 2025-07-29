package minecraft

import (
	"bufio"
	"errors"
	"math"
	"net"
	"os"

	"log/slog"

	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"

	"encoding/binary"

	"github.com/beito123/nbt"
	"github.com/charmbracelet/log"
	"github.com/google/uuid"
	"github.com/keyboard-slayer/minecraft-server/internal/cfb8"
)

type chatMode byte
type particulStatus byte

const (
	enabled chatMode = iota
	commandsOnly
	hidden
)

func (self chatMode) string() string {
	return []string{"enabled", "commands only", "hidden"}[self]
}

const (
	all particulStatus = iota
	decreased
	minimal
)

func (self particulStatus) string() string {
	return []string{"all", "decreased", "minimal"}[self]
}

type userConfig struct {
	locale       string
	viewDistance int8
	chat         chatMode
	chatColors   bool
	skinPart     uint8
	isHandLeft   bool
	textFiltered bool
	allowListing bool
	particul     particulStatus
}

type userInfo struct {
	name string
	uuid uuid.UUID
	cfg  userConfig
}

type client struct {
	id       int
	teleport int
	logger   *slog.Logger
	socket   net.Conn
	reader   *bufio.Reader
	info     userInfo
	key      *rsa.PrivateKey
	rng      []byte
	enc      cipher.Stream
	dec      cipher.Stream
	state    State
}

func newClient(socket net.Conn, id int) (client, error) {
	handler := log.NewWithOptions(os.Stderr, log.Options{
		ReportCaller: true,
		Level:        log.DebugLevel,
		Prefix:       socket.RemoteAddr().String(),
	})

	logger := slog.New(handler)

	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return client{}, err
	}

	rng := make([]byte, 64)
	rand.Read(rng)

	return client{
		id:       id,
		teleport: 0,
		logger:   logger,
		reader:   bufio.NewReader(socket),
		info:     userInfo{},
		socket:   socket,
		key:      key,
		rng:      rng,
		state:    Handshaking,
	}, nil
}

func (self client) decode(ciphertext []byte) ([]byte, error) {
	plain, err := self.key.Decrypt(rand.Reader, ciphertext, nil)
	if err != nil {
		return []byte{}, err
	}

	return plain, nil
}

func (self *client) teleportId() int {
	self.teleport += 1
	return self.teleport
}

func (self client) send(protocol int, contents ...any) error {
	payload := make([]byte, 0)

	for i := range contents {
		switch v := contents[i].(type) {
		case string:
			bytes := []byte(v)
			length := writeVarInt(len(bytes))
			payload = append(payload, length...)
			payload = append(payload, bytes...)
		case []string:
			length := writeVarInt(len(v))
			payload = append(payload, length...)

			for _, s := range v {
				bytes := []byte(s)
				length := writeVarInt(len(bytes))
				payload = append(payload, length...)
				payload = append(payload, bytes...)
			}
		case []byte:
			length := writeVarInt(len(v))
			payload = append(payload, length...)
			payload = append(payload, v...)
		case float32:
			bytes := make([]byte, 4)
			binary.BigEndian.PutUint32(bytes, math.Float32bits(v))
			payload = append(payload, bytes...)
		case float64:
			bytes := make([]byte, 8)
			binary.BigEndian.PutUint64(bytes, math.Float64bits(v))
			payload = append(payload, bytes...)
		case int64:
			bytes := make([]byte, 8)
			binary.BigEndian.PutUint64(bytes, uint64(v))
			payload = append(payload, bytes...)
		case int32:
			bytes := make([]byte, 4)
			binary.BigEndian.PutUint32(bytes, uint32(v))
			payload = append(payload, bytes...)
		case int:
			payload = append(payload, writeVarInt(v)...)
		case uuid.UUID:
			bytes := v[:]
			payload = append(payload, bytes...)
		case bool:
			if v {
				payload = append(payload, byte(1))
			} else {
				payload = append(payload, byte(0))
			}
		case nbt.Tag:
			stream := nbt.NewStream(nbt.BigEndian)
			if err := stream.WriteTag(v); err != nil {
				return err
			}

			bytes := stream.Bytes()
			payload = append(payload, bytes...)

		default:
			return errors.New("Got an unknown type")
		}

	}

	payloadWithProt := append(writeVarInt(protocol), payload...)
	final := append(writeVarInt(len(payloadWithProt)), payloadWithProt...)

	if self.enc == nil {
		self.socket.Write(final)
	} else {
		cipher := make([]byte, len(final))
		self.enc.XORKeyStream(cipher, final)
		self.socket.Write(cipher)
	}

	return nil
}

func (self client) close() {
	self.socket.Close()
}

func (self client) read(length int) ([]byte, error) {
	if length == 0 {
		return []byte{}, nil
	}

	buff := make([]byte, length)
	_, err := self.reader.Read(buff)

	if err != nil {
		return []byte{}, err
	}

	if self.dec != nil {
		clean := make([]byte, length)
		self.dec.XORKeyStream(clean, buff)

		return clean, nil
	}

	return buff, nil
}

func (self client) readVarInt() (int, int, error) {
	var value int = 0
	var pos int = 0
	var length int = 0

	for {
		currentByte, err := self.read(1)
		length += 1

		if err != nil {
			return 0, 0, err
		}

		value |= (int(currentByte[0]) & SEGMENT_BITS) << pos

		if (currentByte[0] & CONTINUE_BIT) == 0 {
			break
		}

		pos += 7

		if pos >= 32 {
			return 0, 0, errors.New("VarInt is too big")
		}
	}

	return value, length, nil
}

func (self *client) register(name string, id uuid.UUID) {
	handler := log.NewWithOptions(os.Stderr, log.Options{
		ReportCaller: true,
		Level:        log.DebugLevel,
		Prefix:       name,
	})

	self.logger = slog.New(handler)
	self.logger.Info("Trying to connect...")

	self.info = userInfo{
		name: name,
		uuid: id,
		cfg:  userConfig{},
	}
}

func (self client) publicKey() ([]byte, error) {
	key, err := x509.MarshalPKIXPublicKey(&self.key.PublicKey)
	if err != nil {
		return []byte{}, err
	}

	return key, nil
}

func (self *client) registerSecret(key []byte) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}

	self.dec = cfb8.NewDecrypter(block, key)
	self.enc = cfb8.NewEncrypter(block, key)

	return nil
}
