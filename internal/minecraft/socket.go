package minecraft

import (
	"fmt"
	"net"

	"log/slog"
)

type Server struct {
	socket net.Listener
}

func New(port uint16) (Server, error) {
	address := fmt.Sprintf("0.0.0.0:%d", port)

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return Server{}, err
	}

	return Server{socket: listener}, nil
}

func (self Server) Serve() {
	slog.Info(fmt.Sprintf("Serving server on %s", self.socket.Addr().String()))
	for {
		conn, err := self.socket.Accept()
		if err != nil {
			fmt.Println("Couldn't handle client: ", err)
			continue
		}

		slog.Info(fmt.Sprintf("New connection from %s", conn.RemoteAddr().String()))

		go self.handle(conn)
	}
}

func (self Server) handle(socket net.Conn) {
	c, err := newClient(socket)

	if err != nil {
		slog.Error("Couldn't create client object", "error", err)
		return
	}

	defer c.close()

	for {
		byte, err := c.reader.Peek(1)
		if err != nil {
			continue
		}

		if byte[0] == 0xFE {
			// Ignoring legacy ping request
			continue
		}

		length, _, err := c.readVarInt()
		if err != nil {
			continue
		}

		data, err := c.read(length)
		if err != nil {
			c.logger.Error("Couldn't read from socket", "error", err)
			return
		}

		id, sz, err := readVarIntFromBuff(data)
		if err != nil {
			slog.Error("Couldn't get packet id", "error", err)
			return
		}

		err = router(&c, id, data[sz:])
		if err != nil {
			c.logger.Error(fmt.Sprintf("%s", err))
			return
		}
	}
}
