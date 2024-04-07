package simutils

import (
	"bufio"
	"go.uber.org/zap"
	"os"
	"time"
)

type TimeHandler struct {
	Log               *zap.Logger
	timeRequests      chan TimeRequest
	waitRequests      chan WaitRequest
	loopSpinThreshold int
	loopWaitDuration  time.Duration
	manualStep        bool

	now              time.Time
	loopsWaiting     int
	currentlyWaiting []WaitRequest
	scanner          *bufio.Scanner
}

func NewTimeHandler(log *zap.Logger, loopSpinThreshold int, loopWaitDuration time.Duration, manualStep bool) (timeRequests chan TimeRequest, waitRequests chan WaitRequest, handler *TimeHandler) {
	var scanner *bufio.Scanner
	if manualStep {
		scanner = bufio.NewScanner(os.Stdin)
	}
	timeRequests = make(chan TimeRequest)
	waitRequests = make(chan WaitRequest)
	handler = &TimeHandler{
		Log:               log,
		timeRequests:      timeRequests,
		waitRequests:      waitRequests,
		loopSpinThreshold: loopSpinThreshold,
		loopWaitDuration:  loopWaitDuration,
		manualStep:        manualStep,

		now:              time.Unix(10000, 0),
		loopsWaiting:     0,
		currentlyWaiting: make([]WaitRequest, 0),
		scanner:          scanner,
	}
	return timeRequests, waitRequests, handler
}

func (t TimeHandler) Start() {
	t.Log.Info("Time handler started")
	for {
		select {
		case req := <-t.timeRequests:
			t.Log.Debug("Time has been requested", zap.String("by", req.Id), zap.Time("time", t.now))
			t.loopsWaiting = 0
			req.ReturnChan <- t.now
		case req := <-t.waitRequests:
			if !req.WaitDeadline.IsZero() {
				req.WaitDuration = req.WaitDeadline.Sub(t.now)
			}
			t.Log.Debug("Wait has been requested", zap.String("by", req.Id), zap.Time("at", t.now), zap.Duration("duration", req.WaitDuration))
			t.loopsWaiting = 0
			req.ReceivedAt = t.now
			t.currentlyWaiting = append(t.currentlyWaiting, req)
		default:
			time.Sleep(t.loopWaitDuration)
			if len(t.currentlyWaiting) <= 0 {
				continue // No need to consider stepping ahead if nothing is waiting yet
			}
			t.loopsWaiting += 1
			if t.loopsWaiting <= t.loopSpinThreshold {
				continue // Haven't waited long enough, don't step yet
			}
			t.loopsWaiting = 0
			if t.manualStep { // If the manual flag is set in the config, wait for input before stepping
				t.Log.Info("Enter 'w' to wait one loop for other goroutines or enter 'p' to process the next request in the queue (q for os.Exit(0))")
				t.Log.Info("If other log messages appear when entering 'w', the time_handler_wait_duration and _spin_amount values are too small")
				t.scanner.Scan()
				if t.scanner.Text() == "q" {
					os.Exit(0)
				}
				if t.scanner.Text() != "p" {
					continue
				}
			}
			minDuration := time.Hour
			minIndex := 10000000
			for i, request := range t.currentlyWaiting {
				if request.WaitDuration < minDuration {
					minIndex = i
					minDuration = request.WaitDuration
				}
			}
			t.Log.Info("\u001B[41mHandling the next waiting request\u001B[0m", zap.Duration("after (simulated time)", minDuration))
			shortestRequest := t.currentlyWaiting[minIndex]
			t.currentlyWaiting = append(t.currentlyWaiting[:minIndex], t.currentlyWaiting[minIndex+1:]...)
			for i := range t.currentlyWaiting {
				if t.currentlyWaiting[i].WaitDuration > minDuration {
					t.currentlyWaiting[i].WaitDuration -= minDuration
				} else {
					t.currentlyWaiting[i].WaitDuration = 0
				}
			}
			t.now = t.now.Add(minDuration)
			t.Log.Debug(
				"Time handler unblocks a sleeper",
				zap.String("id", shortestRequest.Id),
				zap.Time("requested at", shortestRequest.ReceivedAt),
				zap.Time("updated time", t.now),
			)
			shortestRequest.Action(shortestRequest.ReceivedAt, t.now)
		}
	}
}
