//+build appengine
// TODO: other build tags for environments where unsafe package is inappropriate

package desc

type jsonNameMap struct{}
type memoizedDefault struct{}

// FindFieldByJSONName finds the field with the given JSON field name. If no such
// field exists then nil is returned. Only regular fields are returned, not
// extensions.
func (md *MessageDescriptor) FindFieldByJSONName(jsonName string) *FieldDescriptor {
	// NB: With allowed use of unsafe, we use it to atomically define an index
	// via atomic.LoadPointer/atomic.StorePointer. Without it, we skip the index
	// and do an linear scan of fields each time.
	for _, f := range md.fields {
		jn := f.proto.GetJsonName()
		if jn == "" {
			jn = f.proto.GetName()
		}
		if jn == jsonName {
			return f
		}
	}
	return nil
}

func (fd *FieldDescriptor) getDefaultValue() interface{} {
	return fd.determineDefault()
}
