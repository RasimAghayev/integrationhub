// Package transform runs user-supplied Lua scripts over JSON payloads.
package transform

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

// Apply executes script with payload exposed as the global `payload` table and
// returns the global `out` table. The script must set `out` to a table.
func Apply(script string, payload map[string]any) (map[string]any, error) {
	if script == "" {
		return nil, fmt.Errorf("transform: script is required")
	}
	L := lua.NewState()
	defer L.Close()

	payloadTable, err := toTable(L, payload)
	if err != nil {
		return nil, err
	}
	L.SetGlobal("payload", payloadTable)

	if err := L.DoString(script); err != nil {
		return nil, fmt.Errorf("transform: script error: %w", err)
	}

	out := L.GetGlobal("out")
	tbl, ok := out.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("transform: script must set a global `out` table")
	}
	return fromTable(tbl)
}

func toTable(L *lua.LState, m map[string]any) (*lua.LTable, error) {
	tbl := L.NewTable()
	for k, v := range m {
		lv, err := toValue(L, v)
		if err != nil {
			return nil, err
		}
		tbl.RawSetString(k, lv)
	}
	return tbl, nil
}

func toValue(L *lua.LState, v any) (lua.LValue, error) {
	switch t := v.(type) {
	case nil:
		return lua.LNil, nil
	case bool:
		return lua.LBool(t), nil
	case string:
		return lua.LString(t), nil
	case float64:
		return lua.LNumber(t), nil
	case int:
		return lua.LNumber(t), nil
	case int64:
		return lua.LNumber(t), nil
	case []any:
		tbl := L.NewTable()
		for idx, item := range t {
			lv, err := toValue(L, item)
			if err != nil {
				return nil, err
			}
			tbl.RawSetInt(idx+1, lv)
		}
		return tbl, nil
	case map[string]any:
		return toTable(L, t)
	default:
		return nil, fmt.Errorf("transform: unsupported type %T", v)
	}
}

func fromTable(tbl *lua.LTable) (map[string]any, error) {
	out := map[string]any{}
	var err error
	tbl.ForEach(func(k, v lua.LValue) {
		if err != nil {
			return
		}
		key, ok := k.(lua.LString)
		if !ok {
			err = fmt.Errorf("transform: `out` keys must be strings")
			return
		}
		out[string(key)], err = fromValue(v)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func fromValue(v lua.LValue) (any, error) {
	switch t := v.(type) {
	case *lua.LNilType:
		return nil, nil
	case lua.LBool:
		return bool(t), nil
	case lua.LString:
		return string(t), nil
	case lua.LNumber:
		return float64(t), nil
	case *lua.LTable:
		if isSequence(t) {
			var arr []any
			for i := 1; ; i++ {
				item := t.RawGetInt(i)
				if item == lua.LNil {
					break
				}
				val, err := fromValue(item)
				if err != nil {
					return nil, err
				}
				arr = append(arr, val)
			}
			return arr, nil
		}
		return fromTable(t)
	default:
		return nil, fmt.Errorf("transform: unsupported lua type %T", v)
	}
}

func isSequence(tbl *lua.LTable) bool {
	n := 0
	tbl.ForEach(func(k, _ lua.LValue) {
		if _, ok := k.(lua.LNumber); ok {
			n++
		}
	})
	return n == tbl.Len() && tbl.Len() > 0
}
