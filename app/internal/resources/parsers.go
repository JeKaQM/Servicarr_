package resources

import "encoding/json"

func asFloatPtr(v interface{}) *float64 {
	switch x := v.(type) {
	case nil:
		return nil
	case float64:
		return &x
	case string:
		n := json.Number(x)
		f, err := n.Float64()
		if err != nil {
			return nil
		}
		return &f
	case int:
		f := float64(x)
		return &f
	case int64:
		f := float64(x)
		return &f
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return nil
		}
		return &f
	default:
		return nil
	}
}

func asUint64Ptr(v interface{}) *uint64 {
	switch x := v.(type) {
	case nil:
		return nil
	case float64:
		if x < 0 {
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
		if err != nil || f < 0 {
			return nil
		}
		u := uint64(f)
		return &u
	default:
		return nil
	}
}
