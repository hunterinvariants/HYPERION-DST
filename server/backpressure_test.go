package server

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/hunterinvariants/promtact/protocol"
)

func TestFullRequestQueueReturnsBusy(t *testing.T) {
	instance := &Server{
		config:   Config{RequestTimeout: time.Second},
		requests: make(chan request, 1),
	}
	instance.requests <- request{}

	client, service := net.Pipe()
	done := make(chan struct{})
	go func() {
		instance.handleClient(context.Background(), service)
		close(done)
	}()

	request := protocol.ClientRequest{
		Operation: protocol.ClientPut,
		ClientID:  1,
		RequestID: 1,
		Key:       2,
		Value:     3,
	}
	if err := protocol.WriteFrame(client, protocol.KindClientRequest, protocol.EncodeClientRequest(request)); err != nil {
		t.Fatal(err)
	}
	kind, payload, err := protocol.ReadFrame(client)
	if err != nil {
		t.Fatal(err)
	}
	response, err := protocol.DecodeClientResponse(payload)
	if err != nil {
		t.Fatal(err)
	}
	if kind != protocol.KindClientResponse || response.Status != protocol.StatusBusy {
		t.Fatalf("backpressure response kind=%d value=%+v", kind, response)
	}
	client.Close()
	<-done
}
