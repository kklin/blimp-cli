package tunnel

import (
	"context"
	"io"
	"net"
	"sync"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	protoAuth "github.com/kelda/blimp/pkg/proto/auth"
	"github.com/kelda/blimp/pkg/proto/node"
)

type tunnel interface {
	Send(*node.TunnelMsg) error
	Recv() (*node.TunnelMsg, error)
}

func ServerStream(nsrv node.Controller_TunnelServer, stream net.Conn) {
	streamBidirectional(stream, nsrv, func() {})
}

// TODO, How does this thing get cleaned up?  Do we leak a go routine here?
func Client(scc node.ControllerClient, ln net.Listener, auth *protoAuth.BlimpAuth,
	name string, port uint32) error {

	fields := log.Fields{
		"listen": ln.Addr().String(),
		"name":   name,
		"port":   port,
	}

	for {
		stream, err := ln.Accept()
		if err != nil {
			return err
		}

		log.WithFields(fields).Trace("new connection")
		go func() {
			connect(scc, stream, auth, name, port)
			log.WithFields(fields).Trace("finish connection")
		}()
	}
}

func connect(scc node.ControllerClient, stream net.Conn,
	auth *protoAuth.BlimpAuth, name string, port uint32) {
	defer stream.Close()

	ctx, cancel := context.WithCancel(context.Background())
	tnl, err := scc.Tunnel(ctx)
	if err != nil {
		log.WithError(err).Error("failed to establish tunnel")
		return
	}

	err = tnl.Send(&node.TunnelMsg{Msg: &node.TunnelMsg_Header{
		Header: &node.TunnelHeader{
			Auth: auth,
			Name: name,
			Port: port,
		}}})
	if err != nil {
		log.WithError(err).Error("failed to send tunnel connect")
		//nolint:errcheck // Nothing we could do to handle this anyway.
		tnl.CloseSend()
		return
	}

	streamBidirectional(stream, tnl, cancel)
}

func streamBidirectional(stream net.Conn, tnl tunnel, cancel func()) {
	var wg sync.WaitGroup
	wg.Add(2)

	streamDone := make(chan struct{})

	go func() {
		tunnelToStream(stream, tnl)
		close(streamDone)
		wg.Done()

	}()

	go func() {
		streamToTunnel(stream, tnl, streamDone)
		cancel()
		wg.Done()
	}()

	wg.Wait()
	stream.Close()
}

type readResult struct {
	err error
	buf []byte
}

// There's no good way to cancel a stream read when the other end of the
// connection closes, so we need to roll our own.
func asyncReadStream(ctx context.Context, stream io.Reader,
	bufChan <-chan []byte, resultChan chan<- readResult) {
	defer close(resultChan)

	for {
		buf := <-bufChan
		if buf == nil {
			return
		}

		n, err := stream.Read(buf)
		result := readResult{
			err: err,
			buf: buf[:n],
		}
		select {
		case <-ctx.Done():
			return
		case resultChan <- result:
		}
		if err != nil {
			return
		}
	}
}

func streamToTunnel(stream io.Reader, tnl tunnel, done <-chan struct{}) {
	var buf [1024 * 1024]byte

	bufChan := make(chan []byte)
	defer close(bufChan)

	asyncReadCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resultChan := make(chan readResult)
	go asyncReadStream(asyncReadCtx, stream, bufChan, resultChan)

loop:
	for {
		var result readResult

		bufChan <- buf[:]
		select {
		case <-done:
			break loop
		case result = <-resultChan:
		}

		if result.buf != nil {
			msg := node.TunnelMsg{
				Msg: &node.TunnelMsg_Buf{Buf: result.buf}}
			if err := tnl.Send(&msg); err != nil {
				log.WithError(err).Debug("tunnel send error")
				return
			}
		}

		err := result.err
		if err == io.EOF {
			break loop
		} else if err != nil {
			log.WithError(err).Debug("failed to read from local")
			break loop
		}
	}

	msg := node.TunnelMsg{
		Msg: &node.TunnelMsg_Eof{Eof: &node.EOF{}}}
	if err := tnl.Send(&msg); err != nil &&
		status.Code(err) != codes.Canceled {
		log.WithError(err).Debug("failed to send eof")
	}
}

func tunnelToStream(stream io.ReadWriter, tnl tunnel) {
	for {
		msg, err := tnl.Recv()
		switch {
		case err == io.EOF:
			return
		case status.Code(err) == codes.Canceled:
			return
		case err != nil:
			log.WithError(err).Debug("failed to receive on tunnel")
			return
		}

		if eof := msg.GetEof(); eof != nil {
			return
		}

		buf := msg.GetBuf()
		if buf == nil {
			// This shouldn't happen.  The other end of the
			// connection isn't following protocol and sent us the
			// wrong type of msg. Panicking seems too much though,
			// so just error and close the connection.
			log.Error("tunnel protocol error. expected buffer")
			return
		}

		if _, err := stream.Write(buf); err != nil {
			return
		}
	}
}
