package golimiter

import (
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// Rate is the rate == 1-1-S-50
type Rate struct {
	Formatted string
	Period    time.Duration
	Limit     int64
	Delay     int64
}

// NewRateFromFormatted - returns the rate from the formatted version ()
func NewRateFromFormatted(formatted string) (Rate, error) {
	rate := Rate{}

	values := strings.Split(formatted, "-")
	if len(values) != 3 && len(values) != 4 {
		return rate, errors.Errorf("incorrect format '%s'", formatted)
	}
	periods := map[string]time.Duration{
		"S": time.Second, // Second
		"M": time.Minute, // Minute
		"H": time.Hour,   // Hour
	}

	limit, length, period := values[0], values[1], strings.ToUpper(values[2])

	duration, ok := periods[period]
	if !ok {
		return rate, errors.Errorf("incorrect period '%s'", period)
	}

	intLen, err := strconv.ParseInt(length, 10, 64)
	if err != nil {
		return rate, errors.Errorf("incorrect length '%s'", length)
	}
	p := time.Duration(intLen) * duration
	l, err := strconv.ParseInt(limit, 10, 64)
	if err != nil {
		return rate, errors.Errorf("incorrect limit '%s'", limit)
	}

	d := int64(0)
	if len(values) == 4 {
		delay := values[3]
		intDelayLen, err := strconv.ParseInt(delay, 10, 64)
		if err != nil {
			return rate, errors.Errorf("incorrect delay length '%s'", delay)
		}
		d = intDelayLen
	}
	rate = Rate{
		Formatted: formatted,
		Period:    p,
		Limit:     l,
		Delay:     d,
	}

	return rate, nil
}
