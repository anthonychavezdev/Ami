package main

import "reflect"

var userType = reflect.TypeOf(User{})

// markPresence fills User.Online on every user inside v, however deeply
// nested in structs, pointers, slices and maps, right before it is encoded.
// It returns the value to encode (a copy when v is not addressable).
func (s *server) markPresence(v any) any {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return v
	}
	if rv.Kind() != reflect.Ptr {
		tmp := reflect.New(rv.Type()).Elem()
		tmp.Set(rv)
		s.mark(tmp)
		return tmp.Interface()
	}
	s.mark(rv)
	return v
}

func (s *server) mark(rv reflect.Value) {
	switch rv.Kind() {
	case reflect.Ptr:
		if !rv.IsNil() {
			s.mark(rv.Elem())
		}
	case reflect.Interface:
		if rv.IsNil() {
			return
		}
		inner := rv.Elem()
		if inner.Kind() == reflect.Ptr {
			s.mark(inner)
			return
		}
		if !rv.CanSet() {
			return
		}
		tmp := reflect.New(inner.Type()).Elem()
		tmp.Set(inner)
		s.mark(tmp)
		rv.Set(tmp)
	case reflect.Struct:
		if rv.Type() == userType {
			if rv.CanSet() {
				u := rv.Addr().Interface().(*User)
				u.Online = u.ID != 0 && s.hub.present(u.ID)
			}
			return
		}
		for i := 0; i < rv.NumField(); i++ {
			if f := rv.Field(i); f.CanSet() {
				s.mark(f)
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			s.mark(rv.Index(i))
		}
	case reflect.Map:
		for _, k := range rv.MapKeys() {
			val := rv.MapIndex(k)
			tmp := reflect.New(val.Type()).Elem()
			tmp.Set(val)
			s.mark(tmp)
			rv.SetMapIndex(k, tmp)
		}
	}
}
