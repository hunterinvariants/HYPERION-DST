//go:build linux && amd64

package uring

import (
	"errors"
	"fmt"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	sysIOUringEnter    = 426
	sysIOUringRegister = 427

	ioUringOffSQRing = 0
	ioUringOffCQRing = 0x08000000
	ioUringOffSQEs   = 0x10000000

	ioUringFeatSingleMMap = 1
	ioUringEnterGetEvents = 1

	ioUringRegisterBuffers   = 0
	ioUringUnregisterBuffers = 1
	ioUringRegisterFiles     = 2
	ioUringUnregisterFiles   = 3

	ioUringOpFsync      = 3
	ioUringOpWriteFixed = 5
	ioSQEFixedFile      = 1
)

type sqOffsets struct {
	Head        uint32
	Tail        uint32
	RingMask    uint32
	RingEntries uint32
	Flags       uint32
	Dropped     uint32
	Array       uint32
	Reserved1   uint32
	UserAddress uint64
}

type cqOffsets struct {
	Head        uint32
	Tail        uint32
	RingMask    uint32
	RingEntries uint32
	Overflow    uint32
	CQEs        uint32
	Flags       uint32
	Reserved1   uint32
	UserAddress uint64
}

type rawParams struct {
	SQEntries    uint32
	CQEntries    uint32
	Flags        uint32
	SQThreadCPU  uint32
	SQThreadIdle uint32
	Features     uint32
	WQFD         uint32
	Reserved     [3]uint32
	SQOff        sqOffsets
	CQOff        cqOffsets
}

type sqe struct {
	Opcode      uint8
	Flags       uint8
	IOPriority  uint16
	FD          int32
	Offset      uint64
	Address     uint64
	Length      uint32
	Operation   uint32
	UserData    uint64
	BufferIndex uint16
	Personality uint16
	SpliceFDIn  int32
	Address3    uint64
	Reserved    uint64
}

type cqe struct {
	UserData uint64
	Result   int32
	Flags    uint32
}

type Ring struct {
	fd       int
	sqMemory []byte
	cqMemory []byte
	sqes     []byte
	single   bool

	sqHead    *uint32
	sqTail    *uint32
	sqMask    *uint32
	sqEntries *uint32
	sqArray   *uint32
	cqHead    *uint32
	cqTail    *uint32
	cqMask    *uint32
	cqEntries *uint32
	cqes      *cqe
}

func OpenRing(entries uint32) (*Ring, error) {
	if entries == 0 {
		return nil, errors.New("uring: entries must be positive")
	}
	var params rawParams
	fd, _, errno := syscall.RawSyscall(sysIOUringSetup, uintptr(entries),
		uintptr(unsafe.Pointer(&params)), 0)
	if errno != 0 {
		return nil, fmt.Errorf("io_uring_setup: %w", errno)
	}
	ring := &Ring{fd: int(fd)}
	fail := func(err error) (*Ring, error) {
		_ = ring.Close()
		return nil, err
	}

	sqSize := int(params.SQOff.Array + params.SQEntries*4)
	cqSize := int(params.CQOff.CQEs + params.CQEntries*uint32(unsafe.Sizeof(cqe{})))
	ring.single = params.Features&ioUringFeatSingleMMap != 0
	if ring.single {
		size := max(sqSize, cqSize)
		memory, err := syscall.Mmap(ring.fd, ioUringOffSQRing, size,
			syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED|syscall.MAP_POPULATE)
		if err != nil {
			return fail(fmt.Errorf("uring: mmap shared ring: %w", err))
		}
		ring.sqMemory, ring.cqMemory = memory, memory
	} else {
		var err error
		ring.sqMemory, err = syscall.Mmap(ring.fd, ioUringOffSQRing, sqSize,
			syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED|syscall.MAP_POPULATE)
		if err != nil {
			return fail(fmt.Errorf("uring: mmap SQ ring: %w", err))
		}
		ring.cqMemory, err = syscall.Mmap(ring.fd, ioUringOffCQRing, cqSize,
			syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED|syscall.MAP_POPULATE)
		if err != nil {
			return fail(fmt.Errorf("uring: mmap CQ ring: %w", err))
		}
	}
	var err error
	ring.sqes, err = syscall.Mmap(ring.fd, ioUringOffSQEs,
		int(params.SQEntries*uint32(unsafe.Sizeof(sqe{}))),
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED|syscall.MAP_POPULATE)
	if err != nil {
		return fail(fmt.Errorf("uring: mmap SQEs: %w", err))
	}

	ring.sqHead = ptrAt[uint32](ring.sqMemory, params.SQOff.Head)
	ring.sqTail = ptrAt[uint32](ring.sqMemory, params.SQOff.Tail)
	ring.sqMask = ptrAt[uint32](ring.sqMemory, params.SQOff.RingMask)
	ring.sqEntries = ptrAt[uint32](ring.sqMemory, params.SQOff.RingEntries)
	ring.sqArray = ptrAt[uint32](ring.sqMemory, params.SQOff.Array)
	ring.cqHead = ptrAt[uint32](ring.cqMemory, params.CQOff.Head)
	ring.cqTail = ptrAt[uint32](ring.cqMemory, params.CQOff.Tail)
	ring.cqMask = ptrAt[uint32](ring.cqMemory, params.CQOff.RingMask)
	ring.cqEntries = ptrAt[uint32](ring.cqMemory, params.CQOff.RingEntries)
	ring.cqes = ptrAt[cqe](ring.cqMemory, params.CQOff.CQEs)
	return ring, nil
}

func (r *Ring) RegisterBuffers(buffers []syscall.Iovec) error {
	if len(buffers) == 0 {
		return errors.New("uring: no buffers")
	}
	return r.register(ioUringRegisterBuffers, unsafe.Pointer(&buffers[0]), uint32(len(buffers)))
}

func (r *Ring) RegisterFiles(files []int32) error {
	if len(files) == 0 {
		return errors.New("uring: no files")
	}
	return r.register(ioUringRegisterFiles, unsafe.Pointer(&files[0]), uint32(len(files)))
}

func (r *Ring) WriteFixed(fileIndex int32, offset uint64, buffer []byte, bufferIndex uint16, userData uint64) error {
	if len(buffer) == 0 {
		return errors.New("uring: empty write")
	}
	request := sqe{
		Opcode: ioUringOpWriteFixed, Flags: ioSQEFixedFile, FD: fileIndex,
		Offset: offset, Address: uint64(uintptr(unsafe.Pointer(&buffer[0]))),
		Length: uint32(len(buffer)), BufferIndex: bufferIndex, UserData: userData,
	}
	return r.submitAndWait(request)
}

func (r *Ring) Fsync(fileIndex int32, userData uint64) error {
	return r.submitAndWait(sqe{
		Opcode: ioUringOpFsync, Flags: ioSQEFixedFile, FD: fileIndex, UserData: userData,
	})
}

func (r *Ring) submitAndWait(request sqe) error {
	head := atomic.LoadUint32(r.sqHead)
	tail := atomic.LoadUint32(r.sqTail)
	if tail-head >= atomic.LoadUint32(r.sqEntries) {
		return errors.New("uring: submission queue full")
	}
	index := tail & atomic.LoadUint32(r.sqMask)
	target := (*sqe)(unsafe.Add(unsafe.Pointer(&r.sqes[0]), uintptr(index)*unsafe.Sizeof(sqe{})))
	*target = request
	array := unsafe.Slice(r.sqArray, atomic.LoadUint32(r.sqEntries))
	array[index] = index
	atomic.StoreUint32(r.sqTail, tail+1)

	for {
		submitted, _, errno := syscall.RawSyscall6(sysIOUringEnter, uintptr(r.fd),
			1, 0, 0, 0, 0)
		if errno == syscall.EINTR || errno == syscall.EAGAIN {
			// The kernel may consume the SQE before a signal interrupts
			// io_uring_enter. Never submit the published tail twice.
			if atomic.LoadUint32(r.sqHead) != head {
				break
			}
			continue
		}
		if errno != 0 {
			return fmt.Errorf("io_uring_enter submit: %w", errno)
		}
		if submitted == 1 || atomic.LoadUint32(r.sqHead) != head {
			break
		}
		return fmt.Errorf("io_uring_enter submitted %d SQEs, want 1", submitted)
	}

	cqHead := atomic.LoadUint32(r.cqHead)
	for cqHead == atomic.LoadUint32(r.cqTail) {
		_, _, errno := syscall.RawSyscall6(sysIOUringEnter, uintptr(r.fd),
			0, 1, ioUringEnterGetEvents, 0, 0)
		if errno == syscall.EINTR {
			continue
		}
		if errno != 0 {
			return fmt.Errorf("io_uring_enter wait: %w", errno)
		}
		cqHead = atomic.LoadUint32(r.cqHead)
	}
	completions := unsafe.Slice(r.cqes, atomic.LoadUint32(r.cqEntries))
	completion := completions[cqHead&atomic.LoadUint32(r.cqMask)]
	atomic.StoreUint32(r.cqHead, cqHead+1)
	if completion.UserData != request.UserData {
		return fmt.Errorf("uring: completion user_data=%d, want %d", completion.UserData, request.UserData)
	}
	if completion.Result < 0 {
		return fmt.Errorf("uring: CQE: %w", syscall.Errno(-completion.Result))
	}
	if request.Opcode == ioUringOpWriteFixed && completion.Result != int32(request.Length) {
		return fmt.Errorf("uring: short write %d/%d", completion.Result, request.Length)
	}
	return nil
}

func (r *Ring) register(opcode uint32, argument unsafe.Pointer, count uint32) error {
	_, _, errno := syscall.RawSyscall6(sysIOUringRegister, uintptr(r.fd),
		uintptr(opcode), uintptr(argument), uintptr(count), 0, 0)
	if errno != 0 {
		return fmt.Errorf("io_uring_register opcode %d: %w", opcode, errno)
	}
	return nil
}

func (r *Ring) Close() error {
	if r.fd < 0 {
		return nil
	}
	_, _, _ = syscall.RawSyscall6(sysIOUringRegister, uintptr(r.fd),
		ioUringUnregisterBuffers, 0, 0, 0, 0)
	_, _, _ = syscall.RawSyscall6(sysIOUringRegister, uintptr(r.fd),
		ioUringUnregisterFiles, 0, 0, 0, 0)
	if len(r.sqes) != 0 {
		_ = syscall.Munmap(r.sqes)
	}
	if len(r.sqMemory) != 0 {
		_ = syscall.Munmap(r.sqMemory)
	}
	if !r.single && len(r.cqMemory) != 0 {
		_ = syscall.Munmap(r.cqMemory)
	}
	err := syscall.Close(r.fd)
	r.fd = -1
	return err
}

func ptrAt[T any](memory []byte, offset uint32) *T {
	return (*T)(unsafe.Add(unsafe.Pointer(&memory[0]), uintptr(offset)))
}
