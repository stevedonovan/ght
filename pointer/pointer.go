package pointer

import (
	"fmt"
	"strconv"
	"strings"
)

type Mapi = map[interface{}]interface{}

func MassageYaml(d interface{}) interface{} {
	obj,ok := d.(Mapi)
	if ok {
		res := make(Object,len(obj))
		for ikey, val := range obj {
			skey := fmt.Sprintf("%v", ikey)
			res[skey] = MassageYaml(val)
		}
		return res
	} else {
		arr,ok := d.(Array)
		if ok {
			res := make(Array,len(arr))
			for i,val := range arr {
				res[i] = MassageYaml(val)
			}
			return res
		} else {
			return d
		}
	}
}

type NotFoundWarningT struct{}

func (m *NotFoundWarningT) Error() string {
	return "key not found"
}

var NotFoundWarning = NotFoundWarningT{}

type Object = map[string]interface{}
type Array = []interface{}
type OrderedObject struct {
	data Object
	keys []string
}

func AsOrderedObject(d interface{}, msg string) (Object,[]string,error) {
	switch v := d.(type) {
	case OrderedObject:
		return v.data,v.keys,nil
	case Object:
		return v,[]string{},nil
	default:
		return Object{},[]string{},fmt.Errorf("%s: not an object")
	}
}


func Filter(d interface{}, s string, maybeKeys *[]string) (interface{}, error) {
	if s == "." {
		return d,nil
	}
	parts := strings.Split(s,".")
	return pointerParts(d, parts, nil, false, maybeKeys)
}

func pointerParts(d interface{}, parts []string, newKey *string, mustMatch bool, maybeKeys *[]string) (interface{}, error) {
	var array Array
	var object Object
	var ok bool
	if len(parts) == 0 {
		return d,nil
	}
	switch vi := d.(type) {
	case Object:
		object = vi
	case Array:
		array = vi
	default:
		return d, nil
	}
	var v interface{}
	last := len(parts) - 1
	for i, key := range parts {
		if object != nil {
			// collect over all elements of this object
			if key == "*" {
				res := make(Object,len(object))
				rest := parts[i+1:]
				for k,a := range object {
					va,e := pointerParts(a, rest, nil, true, maybeKeys)
					if e != nil {
						if e == &NotFoundWarning {
							continue
						}
						return nil, e
					}
					res[k] = va
				}
				return res, nil
			}
			raiseError := true
			negate := false
			// field list (maybe enclosed in {})
			if strings.Contains(key,",") {
				if strings.HasPrefix(key,"{") {
					key = key[1:len(key)-1]
				}
				keys := strings.Split(key,",")
				res := Object{}
				nk := ""
				actualKeys := []string{}
				for _,k := range keys {
					va,e := pointerParts(object, []string{k}, &nk, false, maybeKeys)
					if e != nil {
						if e == &NotFoundWarning {
							continue
						}
						return nil,e
					}
					if nk != "" {
						k = nk
					}
					res[k] = va
					actualKeys = append(actualKeys, k)
				}
				if maybeKeys != nil {
					*maybeKeys = actualKeys
				}
				v = res
				if mustMatch && len(actualKeys) < len(keys) {
					return nil, &NotFoundWarning
				}
				continue
			} else if strings.HasSuffix(key,"?") {
				negate = false
				raiseError = false
				key = key[:len(key)-1]
				if strings.HasSuffix(key,"!") {
					key = key[:len(key)-1]
					negate = true
				}
			}
			if newKey != nil {
				*newKey = key
			}
			v, ok = object[key]
			if ! raiseError {
				checked := true
				switch vi := v.(type) {
				case bool:
					ok = vi
				case string:
					ok = strings.TrimSpace(vi) != ""
				case Array:
					ok = len(vi) > 0
				default:
					if v == nil {
						ok = false
					} else {
						checked = false
					}
				}
				if checked && negate {
					ok = ! ok
				}
			}
			if !ok {
				if raiseError {
					return nil, fmt.Errorf("cannot find %q in object", key)
				} else {
					return nil, &NotFoundWarning
				}
			}
		} else { // MUST be an array
			// collect over all elements of this array
			if key == "*" {
				res := make(Array,0,len(array))
				rest := parts[i+1:]
				for _,a := range array {
					va,e := pointerParts(a, rest, nil, true, maybeKeys)
					if e != nil {
						if e == &NotFoundWarning {
							continue
						}
						return nil, e
					}
					res = append(res,va)
				}
				return res, nil
			}
			idx, e := strconv.Atoi(key)
			if e != nil {
				return nil, fmt.Errorf("not an array index %s", key)
			}
			if idx >= len(array) {
				return nil, fmt.Errorf("no index %d in array size %d", idx, len(array))
			}
			v = array[idx]
		}
		if i == last {
			break
		}
		object, ok = v.(Object)
		if !ok {
			array, ok = v.(Array)
			if !ok {
				return nil, fmt.Errorf("%q is not an object, is a %v", key, object)
			}
			object = nil
		} else {
			array = nil
		}
	}
	return v, nil
}
