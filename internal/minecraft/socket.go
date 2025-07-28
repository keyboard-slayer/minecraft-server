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
		length, _, err := c.readVarInt()
		if err != nil {
			// slog.Error("Couldn't read length", "error", err)
			return
		}

		id, sz, err := c.readVarInt()
		if err != nil {
			slog.Error("Couldn't get packet id", "error", err)
			return
		}

		data, err := c.read(length - sz)
		if err != nil {
			c.logger.Error("Couldn't read from socket", "error", err)
			return
		}

		err = router(&c, id, data)
		if err != nil {
			c.logger.Error(fmt.Sprintf("%s", err))
			return
		}
	}
}
