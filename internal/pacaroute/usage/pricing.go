package usage

import (
	"math/big"

	"github.com/LandoWang/pacaroute/internal/pacaroute/domain"
)

func PriceUsage(v Activity, p *domain.ModelPrice, messages bool) (*int64, string) {
	if p == nil {
		return nil, "missing_price"
	}
	if v.Input == nil || v.Output == nil {
		return nil, "missing_usage"
	}
	inputRate, e := domain.MoneyMicros(p.Input)
	if e != nil {
		return nil, "invalid_price"
	}
	cachedRate, e := domain.MoneyMicros(p.Cached)
	if e != nil {
		return nil, "invalid_price"
	}
	outputRate, e := domain.MoneyMicros(p.Output)
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
			writeRate, e = domain.MoneyMicros(p.CacheWrite)
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
