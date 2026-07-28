package protocol

import (
	"net"
	"time"
)

func Call(address string, request ClientRequest, timeout time.Duration) (ClientResponse, error) {
	connection, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return ClientResponse{}, err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(timeout))
	if err := WriteFrame(connection, KindClientRequest, EncodeClientRequest(request)); err != nil {
		return ClientResponse{}, err
	}
	kind, payload, err := ReadFrame(connection)
	if err != nil {
		return ClientResponse{}, err
	}
	if kind != KindClientResponse {
		return ClientResponse{}, ErrFrame
	}
	return DecodeClientResponse(payload)
}
