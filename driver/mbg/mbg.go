package mbg

// References:
// https://kb.meinbergglobal.com/kb/driver_software/meinberg_sdks/meinberg_driver_and_api_concepts
// https://kb.meinbergglobal.com/mbglib-api/

import (
	"unsafe"

	"context"
	"encoding/binary"
	"time"

	"go.uber.org/zap"

	"golang.org/x/sys/unix"
)

const (
	// See https://man7.org/linux/man-pages/man2/ioctl.2.html#NOTES

	ioctlWrite = 1
	ioctlRead  = 2

	ioctlDirBits  = 2
	ioctlSizeBits = 14
	ioctlTypeBits = 8
	ioctlSNBits   = 8

	ioctlDirMask  = (1 << ioctlDirBits) - 1
	ioctlSizeMask = (1 << ioctlSizeBits) - 1
	ioctlTypeMask = (1 << ioctlTypeBits) - 1
	ioctlSNMask   = (1 << ioctlSNBits) - 1

	ioctlSNShift   = 0
	ioctlTypeShift = ioctlSNShift + ioctlSNBits
	ioctlSizeShift = ioctlTypeShift + ioctlTypeBits
	ioctlDirShift  = ioctlSizeShift + ioctlSizeBits
)

func ioctlRequest(d, s, t, n int) uint {
	// See https://man7.org/linux/man-pages/man2/ioctl.2.html#NOTES

	return (uint(d&ioctlDirMask) << ioctlDirShift) |
		(uint(s&ioctlSizeMask) << ioctlSizeShift) |
		(uint(t&ioctlTypeMask) << ioctlTypeShift) |
		(uint(n&ioctlSNMask) << ioctlSNShift)
}

func nanoseconds(frac uint32) int64 {
	// Binary fractions to nanoseconds:
	// nanoseconds(0x00000000) == 0
	// nanoseconds(0x80000000) == 500000000
	// nanoseconds(0xffffffff) == 999999999

	return int64((uint64(frac) * uint64(time.Second)) / (1 << 32))
}

func MeasureClockOffset(ctx context.Context, log *zap.Logger, dev string) (time.Duration, error) {
	fd, err := unix.Open(dev, unix.O_RDWR, 0)
	if err != nil {
		log.Error("unix.Open failed", zap.String("dev", dev), zap.Error(err))
		return 0, err
	}
	defer func(log *zap.Logger, dev string) {
		err = unix.Close(fd)
		if err != nil {
			log.Info("unix.Close failed", zap.String("dev", dev), zap.Error(err))
		}
	}(log, dev)

	featureType := uint32(2 /* PCPS */)
	featureNumber := uint32(6 /* HAS_HR_TIME */)

	featureData := make([]byte, 4+4)
	binary.LittleEndian.PutUint32(featureData[0:], featureType)
	binary.LittleEndian.PutUint32(featureData[4:], featureNumber)

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd),
		uintptr(ioctlRequest(ioctlWrite, len(featureData), 'M', 0xa4)),
		uintptr(unsafe.Pointer(&featureData[0])))
	if errno != 0 {
		log.Error("ioctl failed (features) or HR time not supported", zap.String("dev", dev), zap.Error(errno))
		return 0, errno
	}

	cycleFrequencyData := make([]byte, 8)
	_, _, errno = unix.Syscall(unix.SYS_IOCTL, uintptr(fd),
		uintptr(ioctlRequest(ioctlRead, len(cycleFrequencyData), 'M', 0x68)),
		uintptr(unsafe.Pointer(&cycleFrequencyData[0])))
	if errno != 0 {
		log.Error("ioctl failed (cycle frequency)", zap.String("dev", dev), zap.Error(errno))
		return 0, errno
	}

	cycleFrequency := binary.LittleEndian.Uint64(cycleFrequencyData[0:])

	timeData := make([]byte, 8+4+4+4+2+1+8+8+8+8)

	_, _, errno = unix.Syscall(unix.SYS_IOCTL, uintptr(fd),
		uintptr(ioctlRequest(ioctlRead, len(timeData), 'M', 0x80)),
		uintptr(unsafe.Pointer(&timeData[0])))
	if errno != 0 {
		log.Error("ioctl failed (time)", zap.String("dev", dev), zap.Error(errno))
		return 0, errno
	}

	refTimeCycles := int64(binary.LittleEndian.Uint64(timeData[0:]))
	refTimeSeconds := int64(binary.LittleEndian.Uint32(timeData[8:]))
	refTimeFractions := uint32(binary.LittleEndian.Uint32(timeData[12:]))
	refTimeUTCOffset := int32(binary.LittleEndian.Uint32(timeData[16:]))
	refTimeStatus := uint16(binary.LittleEndian.Uint16(timeData[20:]))
	refTimeSignal := uint8(timeData[22])
	sysTimeCyclesBefore := int64(binary.LittleEndian.Uint64(timeData[23:]))
	sysTimeCyclesAfter := int64(binary.LittleEndian.Uint64(timeData[31:]))
	sysTimeSeconds := int64(binary.LittleEndian.Uint64(timeData[39:]))
	sysTimeNanoseconds := int64(binary.LittleEndian.Uint64(timeData[47:]))

	refTime := time.Unix(refTimeSeconds, nanoseconds(refTimeFractions)).UTC()
	sysTime := time.Unix(sysTimeSeconds, sysTimeNanoseconds).UTC()

	log.Debug("mbg reference time",
		zap.Time("time", refTime),
		zap.Int32("UTC offset", refTimeUTCOffset),
		zap.Uint16("status", refTimeStatus),
		zap.Uint8("signal", refTimeSignal),
	)
	log.Debug("mbg system time",
		zap.Time("time", sysTime),
		zap.Int64("at", sysTimeCyclesBefore),
		zap.Int64("latency", refTimeCycles-sysTimeCyclesAfter),
		zap.Uint64("frequency", cycleFrequency),
	)
	log.Debug("mbg clock offset", zap.Duration("offset", refTime.Sub(sysTime)))

	return refTime.Sub(sysTime), nil
}
