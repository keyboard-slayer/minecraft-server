package minecraft

import (
	"bytes"
	"errors"
	"fmt"

	"encoding/binary"
	"encoding/json"

	"github.com/google/uuid"
)

type State int

const (
	Handshaking State = iota
	Status
	Login
	Config
)

func (self State) string() string {
	return []string{"Handshaking", "Status", "Login", "Config"}[self]
}

func resName(c *client, id int) string {
	res := [][]string{
		{"intention", "status_request", "hello", "??"},
		{"??", "ping_request", "key", "??"},
		{"??", "??", "??", "custom_payload"},
		{"??", "??", "login_acknowledged", "??"},
	}

	if id > len(res) {
		return "??"
	}

	return res[id][c.state]
}

func router(c *client, id int, data []byte) error {
	c.logger.Debug(fmt.Sprintf("Current state: %s", c.state.string()), "resource", resName(c, id))

	switch id {
	case 0x0:
		return protocol0(c, data)
	case 0x1:
		return protocol1(c, data)
	case 0x2:
		return protocol2(c, data)
	case 0x3:
		return protocol3(c, data)
	default:
		return fmt.Errorf("Unknown protcol %d", id)
	}
}

func protocol1(c *client, data []byte) error {
	switch c.state {
	case Status:
		// ping_request

		timestamp := int64(binary.BigEndian.Uint64(data))
		c.logger.Debug("", "timestamp", timestamp)
		c.send(0x01, timestamp)

	case Login:
		// key

		m, err := readFromBuffer(data,
			factoryPair{"secret", bytesFactory},
			factoryPair{"token", bytesFactory},
		)

		if err != nil {
			return err
		}

		secret := m["secret"].([]byte)
		token := m["token"].([]byte)

		plain, err := c.decode(token)
		if err != nil {
			return err
		}

		if !bytes.Equal(plain, c.rng) {
			return errors.New("Token doesn't match")
		}

		secretKey, err := c.decode(secret)
		if err != nil {
			return err
		}

		err = c.registerSecret(secretKey)
		if err != nil {
			return err
		}

		c.logger.Debug("", "secret", secret)
		c.logger.Debug("", "token", plain)

		c.send(0x02, c.info.uuid, c.info.name, []byte{})

	default:
		return fmt.Errorf("State not handled %v", c.state)
	}

	return nil
}

func protocol0(c *client, data []byte) error {
	switch c.state {
	case Handshaking:
		// intention

		c.logger.Debug("", "buffer", data)

		m, err := readFromBuffer(data,
			factoryPair{"protocol", intFactory},
			factoryPair{"host", bytesFactory},
			factoryPair{"port", ushortFactory},
			factoryPair{"intent", intFactory},
		)

		if err != nil {
			return err
		}

		c.logger.Debug("", "protocol", m["protocol"].(int))
		c.logger.Debug("", "host", string(m["host"].([]byte)))
		c.logger.Debug("", "port", m["port"].(int))

		c.state = State(m["intent"].(int))

		return nil

	case Status:
		// status_request

		p := make([]player, 0)

		info := &status{
			Version: version{
				Name:     "1.21.8",
				Protocol: 772,
			},
			Players: players{
				Max:    1,
				Online: 0,
				Sample: p,
			},
			Description: description{
				Text: "§3✦ §cMinecraft Server in Go §3✦§7\n§eDon't forget to leave a star on Github",
			},
			Favicon:            AQUA,
			EnforcesSecureChat: false,
		}

		jdata, err := json.Marshal(info)
		if err != nil {
			return err
		}

		c.send(0x00, jdata)

	case Login:
		// hello
		m, err := readFromBuffer(data,
			factoryPair{"username", bytesFactory},
			factoryPair{"uuid", uuidFactory},
		)

		if err != nil {
			return err
		}

		username := string(m["username"].([]byte))
		id := m["uuid"].(uuid.UUID)

		c.register(username, id)

		key, err := c.publicKey()
		if err != nil {
			return err
		}

		c.logger.Debug("", "username", username)
		c.logger.Debug("", "uuid", id)

		c.send(0x01, "", key, c.rng, true)

	default:
		return fmt.Errorf("State not handled %v", c.state)
	}

	return nil
}

func protocol2(c *client, data []byte) error {
	switch c.state {
	case Config:
		// custom_payload
		m, err := readFromBuffer(data,
			factoryPair{"channel", bytesFactory},
			factoryPair{"data", bytesFactory},
		)

		if err != nil {
			return err
		}

		c.logger.Debug("", "channel", string(m["channel"].([]byte)))
		c.logger.Debug("", "data", m["data"].([]byte))

	default:
		return fmt.Errorf("State not handled %v", c.state)
	}

	return nil
}

func protocol3(c *client, data []byte) error {
	switch c.state {
	case Login:
		// login_acknowledged
		c.state = Config
	default:
		return fmt.Errorf("State not handled %v", c.state)
	}
	return nil
}
