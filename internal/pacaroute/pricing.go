package pacaroute

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// Amounts are decimal USD strings in configuration and integer micro-USD in the
// ledger. A price snapshot travels with every settlement; history is immutable.
type ModelPrice struct {
	Input      string `json:"input_usd"`
	Cached     string `json:"cached_usd"`
	Output     string `json:"output_usd"`
	CacheWrite string `json:"cache_write_usd,omitempty"`
}

var moneyPattern = regexp.MustCompile(`^[0-9]{1,9}(\.[0-9]{1,6})?$`)

func moneyMicros(value string) (int64, error) {
	if !moneyPattern.MatchString(value) {
		return 0, errors.New("USD amounts need at most six decimal places")
	}
	parts := strings.SplitN(value, ".", 2)
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	return strconv.ParseInt(parts[0]+fraction+strings.Repeat("0", 6-len(fraction)), 10, 64)
}
func moneyText(micros int64) string { return fmt.Sprintf("%d.%06d", micros/1000000, micros%1000000) }
func (p ModelPrice) validate() error {
	for _, v := range []string{p.Input, p.Cached, p.Output} {
		if _, e := moneyMicros(v); e != nil {
			return e
		}
	}
	if p.CacheWrite != "" {
		_, e := moneyMicros(p.CacheWrite)
		return e
	}
	return nil
}
func priceUsage(v Activity, p *ModelPrice, messages bool) (*int64, string) {
	if p == nil {
		return nil, "missing_price"
	}
	if v.Input == nil || v.Output == nil {
		return nil, "missing_usage"
	}
	inputRate, e := moneyMicros(p.Input)
	if e != nil {
		return nil, "invalid_price"
	}
	cachedRate, e := moneyMicros(p.Cached)
	if e != nil {
		return nil, "invalid_price"
	}
	outputRate, e := moneyMicros(p.Output)
	if e != nil {
		return nil, "invalid_price"
	}
	cached, write := int64(0), int64(0)
	if v.CacheRead != nil {
		cached = *v.CacheRead
	} else if inputRate != cachedRate {
		return nil, "missing_cache_usage"
	}
	writeRate := inputRate
	if messages {
		if v.CacheWrite == nil {
			return nil, "missing_cache_usage"
		}
		write = *v.CacheWrite
		if write > 0 {
			if p.CacheWrite == "" {
				return nil, "missing_cache_write_price"
			}
			writeRate, e = moneyMicros(p.CacheWrite)
			if e != nil {
				return nil, "invalid_price"
			}
		}
	}
	ordinary := *v.Input - cached - write
	if ordinary < 0 || cached < 0 || write < 0 || *v.Output < 0 {
		return nil, "invalid_usage"
	}
	sum := new(big.Int)
	for _, term := range [][2]int64{{ordinary, inputRate}, {cached, cachedRate}, {write, writeRate}, {*v.Output, outputRate}} {
		sum.Add(sum, new(big.Int).Mul(big.NewInt(term[0]), big.NewInt(term[1])))
	}
	// Round once per request, upwards to the micro-USD ledger unit.
	sum.Add(sum, big.NewInt(999999))
	sum.Div(sum, big.NewInt(1000000))
	if !sum.IsInt64() {
		return nil, "cost_overflow"
	}
	value := sum.Int64()
	return &value, ""
}
