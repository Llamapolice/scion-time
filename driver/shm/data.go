package shm

// Reference: https://www.ntp.org/documentation/drivers/driver28

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

type shmTime struct {
	mode int32
	// 0 - if valid is set:
	//       use values
	//       clear valid
	// 1 - if valid is set:
	//       if count before and after read of data is equal:
	//         use values
	//       clear valid
	count                int32
	clockTimeStampSec    int64
	clockTimeStampUSec   int32
	receiveTimeStampSec  int64
	receiveTimeStampUSec int32
	leap                 int32
	precision            int32
	nSamples             int32
	valid                int32
	clockTimeStampNSec   uint32 // Unsigned ns timestamps
	receiveTimeStampNSec uint32 // Unsigned ns timestamps
	dummy                [8]int32
}

const (
	sizeofSHMTime = 96 // sizeof(struct shmTime)

	offsetofSHMTimeMode                 = 0  // offsetof(struct shmTime, mode)
	offsetofSHMTimeCount                = 4  // offsetof(struct shmTime, count)
	offsetofSHMTimeClockTimeStampSec    = 8  // offsetof(struct shmTime, clockTimeStampSec)
	offsetofSHMTimeClockTimeStampUSec   = 16 // offsetof(struct shmTime, clockTimeStampUSec)
	offsetofSHMTimeReceiveTimeStampSec  = 24 // offsetof(struct shmTime, receiveTimeStampSec)
	offsetofSHMTimeReceiveTimeStampUSec = 32 // offsetof(struct shmTime, receiveTimeStampUSec)
	offsetofSHMTimeLeap                 = 36 // offsetof(struct shmTime, leap)
	offsetofSHMTimePrecision            = 40 // offsetof(struct shmTime, precision)
	offsetofSHMTimeNSamples             = 44 // offsetof(struct shmTime, nsamples)
	offsetofSHMTimeValid                = 48 // offsetof(struct shmTime, valid)
	offsetofSHMTimeClockTimeStampNSec   = 52 // offsetof(struct shmTime, clockTimeStampNSec)
	offsetofSHMTimeReceiveTimeStampNSec = 56 // offsetof(struct shmTime, receiveTimeStampNSec)
)

type segment struct {
	initialized bool
	time        *shmTime
}

func init() {
	var t shmTime
	if unsafe.Sizeof(t) != sizeofSHMTime ||
		unsafe.Offsetof(t.mode) != offsetofSHMTimeMode ||
		unsafe.Offsetof(t.count) != offsetofSHMTimeCount ||
		unsafe.Offsetof(t.clockTimeStampSec) != offsetofSHMTimeClockTimeStampSec ||
		unsafe.Offsetof(t.clockTimeStampUSec) != offsetofSHMTimeClockTimeStampUSec ||
		unsafe.Offsetof(t.receiveTimeStampSec) != offsetofSHMTimeReceiveTimeStampSec ||
		unsafe.Offsetof(t.receiveTimeStampUSec) != offsetofSHMTimeReceiveTimeStampUSec ||
		unsafe.Offsetof(t.leap) != offsetofSHMTimeLeap ||
		unsafe.Offsetof(t.precision) != offsetofSHMTimePrecision ||
		unsafe.Offsetof(t.nSamples) != offsetofSHMTimeNSamples ||
		unsafe.Offsetof(t.valid) != offsetofSHMTimeValid ||
		unsafe.Offsetof(t.clockTimeStampNSec) != offsetofSHMTimeClockTimeStampNSec ||
		unsafe.Offsetof(t.receiveTimeStampNSec) != offsetofSHMTimeReceiveTimeStampNSec {
		panic("unexpected memory layout")
	}
}

func initSegment(shm *segment, unit int) error {
	if shm.initialized {
		panic("SHM already initialized")
	}

	var key int = 0x4e545030 + unit
	var size int = 96 /* sizeof(struct shmTime) */
	var flags int = 01000 /* IPC_CREAT */ | 0600
	id, _, errno := unix.Syscall(unix.SYS_SHMGET, uintptr(key), uintptr(size), uintptr(flags))
	if int(id) < 0 {
		if int(id) != -1 {
			panic("shmget returned invalid value")
		}
		return errno
	}
	addr, _, errno := unix.Syscall(unix.SYS_SHMAT, id, uintptr(0), uintptr(0))
	if int(addr) == -1 {
		return errno
	}

	// go vet warns about a possible misuse of unsafe.Pointer in
	// `shm.time = (*shmTime)(unsafe.Pointer(addr))`
	// However, this seems to be in accordance with case (3) in
	// https://pkg.go.dev/unsafe#Pointer.
	// Maybe the follwing accepted proposal will help here:
	// https://github.com/golang/go/issues/58625
	shm.time = (*shmTime)(*(*unsafe.Pointer)(unsafe.Pointer(&addr)))

	shm.initialized = true

	return nil
}
