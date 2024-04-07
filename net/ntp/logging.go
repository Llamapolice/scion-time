package ntp

import (
	"log/slog"

	"go.uber.org/zap/zapcore"
)

type Time32LogValuer struct {
	T Time32
}

func (v Time32LogValuer) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Uint64("Seconds", uint64(v.T.Seconds)),
		slog.Uint64("Fraction", uint64(v.T.Fraction)),
	)
}

type Time64LogValuer struct {
	T Time64
}

func (v Time64LogValuer) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Uint64("Seconds", uint64(v.T.Seconds)),
		slog.Uint64("Fraction", uint64(v.T.Fraction)),
	)
}

type PacketLogValuer struct {
	Pkt *Packet
}

func (v PacketLogValuer) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Uint64("LVM", uint64(v.Pkt.LVM)),
		slog.Uint64("Stratum", uint64(v.Pkt.Stratum)),
		slog.Int64("Poll", int64(v.Pkt.Poll)),
		slog.Int64("Precision", int64(v.Pkt.Precision)),
		slog.Any("RootDelay", Time32LogValuer{T: v.Pkt.RootDelay}),
		slog.Any("RootDispersion", Time32LogValuer{T: v.Pkt.RootDispersion}),
		slog.Uint64("ReferenceID", uint64(v.Pkt.ReferenceID)),
		slog.Any("ReferenceTime", Time64LogValuer{T: v.Pkt.ReferenceTime}),
		slog.Any("OriginTime", Time64LogValuer{T: v.Pkt.OriginTime}),
		slog.Any("ReceiveTime", Time64LogValuer{T: v.Pkt.ReceiveTime}),
		slog.Any("TransmitTime", Time64LogValuer{T: v.Pkt.TransmitTime}),
	)
}

type Time32Marshaler struct {
	T Time32
}

func (m Time32Marshaler) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint16("Seconds", m.T.Seconds)
	enc.AddUint16("Fraction", m.T.Fraction)
	return nil
}

type Time64Marshaler struct {
	T Time64
}

func (m Time64Marshaler) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint32("Seconds", m.T.Seconds)
	enc.AddUint32("Fraction", m.T.Fraction)
	return nil
}

type PacketMarshaler struct {
	Pkt *Packet
}

func (m PacketMarshaler) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	var err error
	enc.AddUint8("LVM", m.Pkt.LVM)
	enc.AddUint8("Stratum", m.Pkt.Stratum)
	enc.AddInt8("Poll", m.Pkt.Poll)
	enc.AddInt8("Precision", m.Pkt.Precision)
	err = enc.AddObject("RootDelay", Time32Marshaler{T: m.Pkt.RootDelay})
	if err != nil {
		return err
	}
	err = enc.AddObject("RootDispersion", Time32Marshaler{T: m.Pkt.RootDispersion})
	if err != nil {
		return err
	}
	enc.AddUint32("ReferenceID", m.Pkt.ReferenceID)
	err = enc.AddObject("ReferenceTime", Time64Marshaler{T: m.Pkt.ReferenceTime})
	if err != nil {
		return err
	}
	err = enc.AddObject("OriginTime", Time64Marshaler{T: m.Pkt.OriginTime})
	if err != nil {
		return err
	}
	err = enc.AddObject("ReceiveTime", Time64Marshaler{T: m.Pkt.ReceiveTime})
	if err != nil {
		return err
	}
	err = enc.AddObject("TransmitTime", Time64Marshaler{T: m.Pkt.TransmitTime})
	if err != nil {
		return err
	}
	return nil
}
