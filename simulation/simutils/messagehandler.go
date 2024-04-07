package simutils

import (
	"go.uber.org/zap"
	"time"
)

type MessageHandler struct {
	Log           *zap.Logger
	receiver      chan SimPacket
	simConnectors []*SimConnector
	modifyMsg     func(packet SimPacket) SimPacket
	waitRequests  *chan WaitRequest
}

func NewMessageHandler(log *zap.Logger, numInstances int, modifyMsg func(packet SimPacket) SimPacket, waitRequests *chan WaitRequest) (receiver chan SimPacket, handler *MessageHandler) {
	receiver = make(chan SimPacket)
	simConnectors := make([]*SimConnector, 0, numInstances)
	handler = &MessageHandler{
		Log:           log,
		receiver:      receiver,
		simConnectors: simConnectors,
		modifyMsg:     modifyMsg,
		waitRequests:  waitRequests,
	}
	return
}

func (h *MessageHandler) Start() {
	h.Log.Info("\u001B[34mMessage handler started\u001B[0m")
skip:
	for msg := range h.receiver {
		h.Log.Debug(
			"Message handler received message",
			zap.Binary("msg", msg.B),
			zap.String("target", msg.TargetAddr.String()),
			zap.String("source", msg.SourceAddr.String()),
		)
		// Ensure the simConn's local address and the messages target address have the same format
		msgTargetAddr := msg.TargetAddr.Addr().WithZone("").Unmap()
		for _, simConn := range h.simConnectors {
			laddrPort := simConn.LocalAddress.Host.AddrPort()
			simConnLocalAddress := laddrPort.Addr().WithZone("").Unmap()
			if simConnLocalAddress == msgTargetAddr {
				// necessary, otherwise the function will use the current simConn and msg during its execution instead of creation (what we want)
				tmp := *simConn
				tmpMsg := h.modifyMsg(msg)
				passOn := func(r, n time.Time) { // Create a function that will pass the message into the simConn's input channel
					tmp.Input <- tmpMsg
					h.Log.Debug(
						"Passed message on to instance",
						zap.String("receiving connector id", tmp.Id),
						zap.Stringer("source addr", tmpMsg.SourceAddr),
						zap.Duration("after", tmpMsg.Latency),
					)
				}
				*h.waitRequests <- WaitRequest{ // Request the passOn function to be executed after the duration specified in the message
					Id:           simConn.Id + "_msg",
					WaitDuration: msg.Latency,
					Action:       passOn,
				}
				continue skip // Break out of both for loops
			}
		}
		h.Log.Warn("Targeted address does not exist (yet), message dropped",
			zap.String("target", msgTargetAddr.String()))
	}
	h.Log.Info("\u001B[34mMessage handler terminating\u001B[0m")
}

func (h *MessageHandler) AddSimConnector(simConnector *SimConnector) {
	h.simConnectors = append(h.simConnectors, simConnector)
}
