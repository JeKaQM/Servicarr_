package resources

import (
	"encoding/json"
	"math"
)

func finiteFloatPtr(f float64) *float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	return &f
}

func asFloatPtr(v interface{}) *float64 {
	switch x := v.(type) {
	case nil:
		return nil
	case float64:
		return finiteFloatPtr(x)
	case string:
		n := json.Number(x)
		f, err := n.Float64()
		if err != nil {
			return nil
		}
		return finiteFloatPtr(f)
	case int:
		f := float64(x)
		return finiteFloatPtr(f)
	case int64:
		f := float64(x)
		return finiteFloatPtr(f)
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return nil
		}
		return finiteFloatPtr(f)
	default:
		return nil
	}
}

func asUint64Ptr(v interface{}) *uint64 {
	switch x := v.(type) {
	case nil:
		return nil
	case float64:
		if x < 0 || math.IsNaN(x) || math.IsInf(x, 0) || x >= math.Exp2(64) {
			return nil
		}
		u := uint64(x)
		return &u
	case int:
		if x < 0 {
			return nil
		}
		u := uint64(x)
		return &u
	case int64:
		if x < 0 {
			return nil
		}
		u := uint64(x)
		return &u
	case json.Number:
		f, err := x.Float64()
		if err != nil || f < 0 || math.IsNaN(f) || math.IsInf(f, 0) || f >= math.Exp2(64) {
			return nil
		}
		u := uint64(f)
		return &u
	default:
		return nil
	}
}
