package protocol

import (
	"encoding/binary"
	"errors"
)

type ClientOp uint8

const (
	ClientPut ClientOp = iota + 1
	ClientDelete
	ClientGet
	ClientStatus
)

type Status uint16

const (
	StatusOK Status = iota
	StatusNotLeader
	StatusBusy
	StatusInvalid
	StatusNotFound
	StatusTimeout
	StatusInternal
)

type ClientRequest struct {
	Operation ClientOp
	ClientID  uint64
	RequestID uint64
	Key       uint64
	Value     uint64
}

type ClientResponse struct {
	Status    Status
	Leader    uint32
	RequestID uint64
	Value     uint64
	Commit    uint64
}

const (
	clientRequestSize  = 40
	clientResponseSize = 32
)

func EncodeClientRequest(request ClientRequest) []byte {
	out := make([]byte, clientRequestSize)
	out[0] = byte(request.Operation)
	binary.LittleEndian.PutUint64(out[8:16], request.ClientID)
	binary.LittleEndian.PutUint64(out[16:24], request.RequestID)
	binary.LittleEndian.PutUint64(out[24:32], request.Key)
	binary.LittleEndian.PutUint64(out[32:40], request.Value)
	return out
}

func DecodeClientRequest(data []byte) (ClientRequest, error) {
	if len(data) != clientRequestSize {
		return ClientRequest{}, ErrFrame
	}
	request := ClientRequest{
		Operation: ClientOp(data[0]),
		ClientID:  binary.LittleEndian.Uint64(data[8:16]),
		RequestID: binary.LittleEndian.Uint64(data[16:24]),
		Key:       binary.LittleEndian.Uint64(data[24:32]),
		Value:     binary.LittleEndian.Uint64(data[32:40]),
	}
	if request.Operation < ClientPut || request.Operation > ClientStatus {
		return ClientRequest{}, errors.New("protocol: invalid client operation")
	}
	return request, nil
}

func EncodeClientResponse(response ClientResponse) []byte {
	out := make([]byte, clientResponseSize)
	binary.LittleEndian.PutUint16(out[0:2], uint16(response.Status))
	binary.LittleEndian.PutUint32(out[4:8], response.Leader)
	binary.LittleEndian.PutUint64(out[8:16], response.RequestID)
	binary.LittleEndian.PutUint64(out[16:24], response.Value)
	binary.LittleEndian.PutUint64(out[24:32], response.Commit)
	return out
}

func DecodeClientResponse(data []byte) (ClientResponse, error) {
	if len(data) != clientResponseSize {
		return ClientResponse{}, ErrFrame
	}
	return ClientResponse{
		Status:    Status(binary.LittleEndian.Uint16(data[0:2])),
		Leader:    binary.LittleEndian.Uint32(data[4:8]),
		RequestID: binary.LittleEndian.Uint64(data[8:16]),
		Value:     binary.LittleEndian.Uint64(data[16:24]),
		Commit:    binary.LittleEndian.Uint64(data[24:32]),
	}, nil
}
