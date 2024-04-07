package phc

// Reference: https://github.com/torvalds/linux/blob/master/include/uapi/linux/ptp_clock.h

import (
	"unsafe"

	"context"
	"log/slog"
	"time"

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

type ptpClockTime struct {
	sec      int64  /* seconds */
	nsec     uint32 /* nanoseconds */
	reserved uint32
}

const (
	sizeofPTPClockTime = 16 // sizeof(struct ptp_clock_time)

	offsetofPTPClockTimeSec  = 0 // offsetof(struct ptp_clock_time, sec)
	offsetofPTPClockTimeNSec = 8 // offsetof(struct ptp_clock_time, nsec)
)

type ptpSysOffsetPrecise struct {
	device      ptpClockTime
	sysRealTime ptpClockTime
	sysMonoRaw  ptpClockTime
	reserved    [4]uint32 /* Reserved for future use. */
}

const (
	sizeofPTPSysOffsetPrecise = 64 // sizeof(struct ptp_sys_offset_precise)

	offsetofPTPSysOffsetPreciseDevice      = 0  // offsetof(struct ptp_sys_offset_precise, device)
	offsetofPTPSysOffsetPreciseSysRealTime = 16 // offsetof(struct ptp_sys_offset_precise, sys_realtime)
	offsetofPTPSysOffsetPreciseSysMonoRaw  = 32 // offsetof(struct ptp_sys_offset_precise, sys_monoraw)
)

type ReferenceClock struct {
	log *slog.Logger
	dev string
}

func init() {
	var t0 ptpClockTime
	if unsafe.Sizeof(t0) != sizeofPTPClockTime ||
		unsafe.Offsetof(t0.sec) != offsetofPTPClockTimeSec ||
		unsafe.Offsetof(t0.nsec) != offsetofPTPClockTimeNSec {
		panic("unexpected memory layout")
	}
	var t1 ptpSysOffsetPrecise
	if unsafe.Sizeof(t1) != sizeofPTPSysOffsetPrecise ||
		unsafe.Offsetof(t1.device) != offsetofPTPSysOffsetPreciseDevice ||
		unsafe.Offsetof(t1.sysRealTime) != offsetofPTPSysOffsetPreciseSysRealTime ||
		unsafe.Offsetof(t1.sysMonoRaw) != offsetofPTPSysOffsetPreciseSysMonoRaw {
		panic("unexpected memory layout")
	}
}

func ioctlRequest(d, s, t, n int) uint {
	// See https://man7.org/linux/man-pages/man2/ioctl.2.html#NOTES

	return (uint(d&ioctlDirMask) << ioctlDirShift) |
		(uint(s&ioctlSizeMask) << ioctlSizeShift) |
		(uint(t&ioctlTypeMask) << ioctlTypeShift) |
		(uint(n&ioctlSNMask) << ioctlSNShift)
}

func NewReferenceClock(log *slog.Logger, dev string) *ReferenceClock {
	return &ReferenceClock{log: log, dev: dev}
}

func (c *ReferenceClock) MeasureClockOffset(ctx context.Context) (
	time.Time, time.Duration, error) {
	fd, err := unix.Open(c.dev, unix.O_RDWR, 0)
	if err != nil {
		c.log.LogAttrs(ctx, slog.LevelError,
			"unix.Open failed",
			slog.String("dev", c.dev),
			slog.Any("error", err),
		)
		return time.Time{}, 0, err
	}
	defer func() {
		err = unix.Close(fd)
		if err != nil {
			c.log.LogAttrs(ctx, slog.LevelError,
				"unix.Close failed",
				slog.String("dev", c.dev),
				slog.Any("error", err),
			)
		}
	}()

	off := ptpSysOffsetPrecise{}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd),
		uintptr(ioctlRequest(ioctlRead|ioctlWrite, int(unsafe.Sizeof(off)), '=', 0x8)),
		uintptr(unsafe.Pointer(&off)),
	)
	if errno != 0 {
		c.log.LogAttrs(ctx, slog.LevelError,
			"ioctl failed",
			slog.String("dev", c.dev),
			slog.Uint64("errno", uint64(errno)),
		)
		return time.Time{}, 0, errno
	}

	sysRealTime := time.Unix(off.sysRealTime.sec, int64(off.sysRealTime.nsec)).UTC()
	deviceTime := time.Unix(off.device.sec, int64(off.device.nsec)).UTC()
	offset := deviceTime.Sub(sysRealTime)

	c.log.LogAttrs(ctx, slog.LevelDebug,
		"PTP hardware clock sample",
		slog.Time("sysRealTime", sysRealTime),
		slog.Time("deviceTime", deviceTime),
		slog.Duration("offset", offset),
	)

	return sysRealTime, offset, nil
}
