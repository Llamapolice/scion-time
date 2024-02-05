package ntp_test

import (
	"testing"
	"time"

	"example.com/scion-time/net/ntp"
)

func TestTime64Conversion(t *testing.T) {
	t0 := time.Now()
	t64 := ntp.Time64FromTime(t0)
	t1 := ntp.TimeFromTime64(t64)

	if !t1.Equal(t0) {
		t.Errorf("t1 and t0 must be equal")
	}
}

func TestBeforeAfter(t *testing.T) {
	t0 := ntp.Time64{Seconds: 10, Fraction: 0}
	t1 := ntp.Time64{Seconds: 20, Fraction: 0}

	if !t0.Before(t1) {
		t.Errorf("t0 must be before t1")
	}
	if t1.Before(t0) {
		t.Errorf("t1 must not be before t0")
	}
	if !t1.After(t0) {
		t.Errorf("t1 must be after t0")
	}
	if t0.After(t1) {
		t.Errorf("t0 must not be after t1")
	}
}

func TestBeforeAfterWithFraction(t *testing.T) {
    t0 := ntp.Time64{Seconds: 10, Fraction: 0}
    t1 := ntp.Time64{Seconds: 20, Fraction: 0}
    t2 := ntp.Time64{Seconds: 10, Fraction: 100}
    t3 := ntp.Time64{Seconds: 10, Fraction: 200}

    // Testing with Fraction = 0
    if !t0.Before(t1) {
        t.Errorf("t0 must be before t1")
    }
    if t1.Before(t0) {
        t.Errorf("t1 must not be before t0")
    }
    if !t1.After(t0) {
        t.Errorf("t1 must be after t0")
    }
    if t0.After(t1) {
        t.Errorf("t0 must not be after t1")
    }

    // Testing with non-zero Fraction
    if !t2.Before(t3) {
        t.Errorf("t2 must be before t3")
    }
    if t3.Before(t2) {
        t.Errorf("t3 must not be before t2")
    }
    if !t3.After(t2) {
        t.Errorf("t3 must be after t2")
    }
    if t2.After(t3) {
        t.Errorf("t2 must not be after t3")
    }
}

func TestBeforeAfterVariousCases(t *testing.T) {
    // Case 1: Both Seconds and Fraction are zero
    t1 := ntp.Time64{Seconds: 0, Fraction: 0}
    t2 := ntp.Time64{Seconds: 0, Fraction: 0}

    if t1.Before(t2) {
        t.Errorf("t1 should not be before t2 when both are zero")
    }
    if t1.After(t2) {
        t.Errorf("t1 should not be after t2 when both are zero")
    }

    // Case 2: Non-zero Seconds, zero Fraction
    t3 := ntp.Time64{Seconds: 1, Fraction: 0}
    t4 := ntp.Time64{Seconds: 2, Fraction: 0}

    if !t3.Before(t4) {
        t.Errorf("t3 must be before t4 with non-zero seconds")
    }
    if !t4.After(t3) {
        t.Errorf("t4 must be after t3 with non-zero seconds")
    }

    // Case 3: Zero Seconds, non-zero Fraction
    t5 := ntp.Time64{Seconds: 0, Fraction: 100}
    t6 := ntp.Time64{Seconds: 0, Fraction: 200}

    if !t5.Before(t6) {
        t.Errorf("t5 must be before t6 with non-zero fraction")
    }
    if !t6.After(t5) {
        t.Errorf("t6 must be after t5 with non-zero fraction")
    }

    // Case 4: Both Seconds and Fraction are non-zero
    t7 := ntp.Time64{Seconds: 1, Fraction: 100}
    t8 := ntp.Time64{Seconds: 1, Fraction: 200}

    if !t7.Before(t8) {
        t.Errorf("t7 must be before t8 with both fields non-zero")
    }
    if !t8.After(t7) {
        t.Errorf("t8 must be after t7 with both fields non-zero")
    }
}

