package pin

import (
	"sync/atomic"
	"unsafe"
)

const (
	detached uint32 = iota
	accessing
	attached
)

type Meta struct {
	_       noCopy
	flag    atomic.Uint32
	refcnt  uint32
	version uint16
	intable bool
	_pad    [1]byte
	Extra   any
}

func (m *Meta) Pin() (success bool) {
LOAD_FLAG:
	flag := m.flag.Load()
	switch flag {
	case accessing:
		goto LOAD_FLAG
	case detached:
		runtime_procPin()
		if !m.flag.CompareAndSwap(flag, accessing) {
			runtime_procUnpin()
			goto LOAD_FLAG
		}
		m.version = uint16(pinTable.nextVersion.Add(1))
		m.refcnt = 0
		m.intable = true
		pinTable.m.Store(uintptr(m.key()), m)
		m.flag.Store(attached)
		runtime_procUnpin()
		return true
	case attached:
	}
	return false
}

func (m *Meta) Unpin() (success bool) {
LOAD_FLAG:
	flag := m.flag.Load()
	switch flag {
	case accessing:
		goto LOAD_FLAG
	case detached:
	case attached:
		runtime_procPin()
		if !m.flag.CompareAndSwap(flag, accessing) {
			runtime_procUnpin()
			goto LOAD_FLAG
		}
		m.intable = false
		if m.refcnt == 0 {
			pinTable.m.Delete(m.key())
			m.flag.Store(detached)
		} else {
			m.flag.Store(attached)
		}
		runtime_procUnpin()
		return true
	}
	return false
}

func (m *Meta) key() uintptr { return uintptr(unsafe.Pointer(m)) }

func (m *Meta) decref(version uint16) {
LOAD_FLAG:
	flag := m.flag.Load()
	switch flag {
	case accessing:
		goto LOAD_FLAG
	case detached:
		return
	case attached:
		runtime_procPin()
		if !m.flag.CompareAndSwap(flag, accessing) {
			runtime_procUnpin()
			goto LOAD_FLAG
		}
		if m.version != version {
			m.flag.Store(flag)
			runtime_procUnpin()
			return
		}
		if m.refcnt == 0 {
			panic("dgo:go: decref() called on Meta with refcnt == 0")
		}
		m.refcnt--
		if m.refcnt == 0 && !m.intable {
			pinTable.m.Delete(uintptr(m.key()))
			m.flag.Store(detached)
		} else {
			m.flag.Store(attached)
		}
		runtime_procUnpin()
	}
}

//lint:ignore U1000 go:linkname
func metaNewToken(m *Meta) untypedToken {
LOAD_FLAG:
	flag := m.flag.Load()
	switch flag {
	case accessing:
		goto LOAD_FLAG
	case detached:
		panic("dgo:go: cannot call NewToken() on an unpinned object")
	case attached:
		runtime_procPin()
		if !m.flag.CompareAndSwap(flag, accessing) {
			runtime_procUnpin()
			goto LOAD_FLAG
		}
		m.refcnt++
		m.flag.Store(attached)
		runtime_procUnpin()
	}
	return newToken[struct{}](m)
}
