package minecraft

import (
	"bytes"
	"errors"
	"fmt"

	"encoding/binary"
	"encoding/json"

	// "github.com/beito123/nbt"
	"github.com/google/uuid"
)

type State int

const (
	Handshaking State = iota
	Status
	Login
	Config
	Play
)

func (self State) string() string {
	return []string{"Handshaking", "Status", "Login", "Config", "Play"}[self]
}

func resName(c *client, id int) string {
	switch id {
	case 0x00:
		return []string{"intention", "status_request", "hello", "client_information", "??"}[c.state]
	case 0x01:
		return []string{"??", "ping_request", "key", "??", "??"}[c.state]
	case 0x02:
		return []string{"??", "??", "??", "custom_payload", "??"}[c.state]
	case 0x03:
		return []string{"??", "??", "login_acknowledged", "finish_configuration", "??"}[c.state]
	case 0x07:
		return []string{"??", "??", "??", "select_known_packs", "??"}[c.state]
	}

	return "??"
}

func router(c *client, id int, data []byte) error {
	c.logger.Debug("", "state", c.state.string(), "protocol", fmt.Sprintf("0x%02x", id), "resource", resName(c, id))

	switch id {
	case 0x00:
		return protocol0(c, data)
	case 0x01:
		return protocol1(c, data)
	case 0x02:
		return protocol2(c, data)
	case 0x03:
		return protocol3(c, data)
	case 0x07:
		return protocol7(c, data)
	default:
		return fmt.Errorf("Unknown protcol %d", id)
	}
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

		// status_response
		if err = c.send(0x00, jdata); err != nil {
			return err
		}

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

		if err = c.send(0x01, "", key, c.rng, true); err != nil {
			return err
		}

	case Config:
		// client_information
		m, err := readFromBuffer(data,
			factoryPair{"locale", bytesFactory},
			factoryPair{"view_distance", byteFactory},
			factoryPair{"chat_mode", intFactory},
			factoryPair{"chat_color", byteFactory},
			factoryPair{"skin_part", byteFactory},
			factoryPair{"main_hand", intFactory},
			factoryPair{"filter_text", byteFactory},
			factoryPair{"allow_listing", byteFactory},
			factoryPair{"particul_status", intFactory},
		)

		if err != nil {
			return err
		}

		c.info.cfg = userConfig{
			locale:       string(m["locale"].([]byte)),
			viewDistance: int8(m["view_distance"].(byte)),
			chat:         chatMode(m["chat_mode"].(int)),
			chatColors:   m["chat_color"].(byte) == 0x01,
			skinPart:     uint8(m["skin_part"].(byte)),
			isHandLeft:   m["main_hand"].(int) == 0x00,
			textFiltered: m["filter_text"].(byte) == 0x01,
			allowListing: m["allow_listing"].(byte) == 0x01,
			particul:     particulStatus(m["particul_status"].(int)),
		}

		c.logger.Debug("",
			"locale", c.info.cfg.locale,
			"viewDistance", c.info.cfg.viewDistance,
			"chatMode", c.info.cfg.chat.string(),
			"chatColors", c.info.cfg.chatColors,
			"skinPart", c.info.cfg.skinPart,
			"isHandLeft", c.info.cfg.isHandLeft,
			"textFiltered", c.info.cfg.textFiltered,
			"allowListing", c.info.cfg.allowListing,
			"particul", c.info.cfg.particul.string(),
		)

		// custom_payload
		if err = c.send(0x01, "minecraft:brand", "vanilla"); err != nil {
			return err
		}

		// select_known_packs
		if err = c.send(0x0e, 1, "minecraft", "core", "1.21.8"); err != nil {
			return err
		}

	default:
		return fmt.Errorf("State not handled %s", c.state.string())
	}

	return nil
}

func protocol1(c *client, data []byte) error {
	switch c.state {
	case Status:
		// ping_request
		timestamp := int64(binary.BigEndian.Uint64(data))
		c.logger.Debug("", "timestamp", timestamp)

		// pong_response
		if err := c.send(0x01, timestamp); err != nil {
			return err
		}

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

		// login_finished
		if err = c.send(0x02, c.info.uuid, c.info.name, []byte{}); err != nil {
			return err
		}

	default:
		return fmt.Errorf("State not handled %s", c.state.string())
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
	case Config:
		// finish_configuration
		c.state = Play

		// login
		c.send(
			0x2b, int32(c.id), false, []string{"minecraft", "overworld"}, 1, 10, 10, false, false, false, 0, "minecraft:overworld", 69, byte(1), byte(0), false, true, false,
			10, 0, false,
		)
	default:
		return fmt.Errorf("State not handled %s", c.state.string())
	}
	return nil
}

func protocol7(c *client, data []byte) error {
	switch c.state {
	case Config:
		// select_known_packs
		length, sz, err := readVarIntFromBuff(data)
		if err != nil {
			return err
		}

		data = data[sz:]

		for i := 0; i < length; i++ {
			m, err := readFromBuffer(data,
				factoryPair{"namespace", bytesFactory},
				factoryPair{"id", bytesFactory},
				factoryPair{"version", bytesFactory},
			)

			if err != nil {
				return err
			}

			c.logger.Debug("Available pack", "namespace", string(m["namespace"].([]byte)), "id", string(m["id"].([]byte)), "version", string(m["version"].([]byte)))
		}

		c.send(0x03)

	default:
		return fmt.Errorf("State not handled %s", c.state.string())
	}

	return nil
}
