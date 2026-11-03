//go:build windows

package client

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// The audio output on Windows: winmm's waveOut, called through syscall so it needs no cgo. The driver
// holds a short ring of buffers; Write copies a chunk into the next free one and queues it, and waits when
// they are all still playing. That wait is the pacing, so the sink is not "paced" (see output.run).

const (
	waveMapper = 0xFFFFFFFF // WAVE_MAPPER: the default output device
	whdrDone   = 0x1        // WHDR_DONE: the driver is finished with the buffer
)

var (
	winmm           = syscall.NewLazyDLL("winmm.dll")
	procWaveOpen    = winmm.NewProc("waveOutOpen")
	procWavePrepare = winmm.NewProc("waveOutPrepareHeader")
	procWaveUnprep  = winmm.NewProc("waveOutUnprepareHeader")
	procWaveWrite   = winmm.NewProc("waveOutWrite")
	procWaveReset   = winmm.NewProc("waveOutReset")
	procWaveClose   = winmm.NewProc("waveOutClose")
	errWaveClosed   = errors.New("audio output closed")
	errWaveNoDevice = errors.New("no audio output device")
	waveHdrSize     = unsafe.Sizeof(waveHdr{})
	waveFormatPCM16 = waveFormat{tag: 1, channels: 2, rate: mixRate, bytesPerSec: mixRate * 4, align: 4, bits: 16}
)

// waveFormat is WAVEFORMATEX.
type waveFormat struct {
	tag, channels uint16
	rate          uint32
	bytesPerSec   uint32
	align, bits   uint16
	cbSize        uint16
}

// waveHdr is WAVEHDR.
type waveHdr struct {
	data     uintptr
	length   uint32
	recorded uint32
	user     uintptr
	flags    uint32
	loops    uint32
	next     uintptr
	reserved uintptr
}

// waveQueue is how many buffers of mixChunk frames (20 ms each) the driver is given. More of them
// survive a frame that runs long, fewer make a sound heard sooner after it is played.
func waveQueue() int {
	// TODO(human): choose the number of buffers (3 to 8 is the sensible range; 4 means up to 80 ms queued).
	return 4
}

// audioAvailable is whether winmm can be loaded (a device is only known when it is opened).
func audioAvailable() bool { return winmm.Load() == nil }

type waveSink struct {
	mu     sync.Mutex
	h      uintptr
	hdrs   []*waveHdr
	bufs   [][]byte
	queued []bool
	next   int
	closed atomic.Bool
}

func openSink() (*sink, error) {
	if err := winmm.Load(); err != nil {
		return nil, err
	}
	var h uintptr
	wf := waveFormatPCM16
	if r, _, _ := procWaveOpen.Call(uintptr(unsafe.Pointer(&h)), waveMapper, uintptr(unsafe.Pointer(&wf)), 0, 0, 0); r != 0 {
		return nil, fmt.Errorf("%w (waveOutOpen: %d)", errWaveNoDevice, r)
	}
	n := waveQueue()
	s := &waveSink{h: h, hdrs: make([]*waveHdr, n), bufs: make([][]byte, n), queued: make([]bool, n)}
	for i := range s.hdrs {
		s.bufs[i] = make([]byte, 4*mixChunk)
		s.hdrs[i] = &waveHdr{data: uintptr(unsafe.Pointer(&s.bufs[i][0])), length: uint32(len(s.bufs[i]))}
		if r, _, _ := procWavePrepare.Call(h, uintptr(unsafe.Pointer(s.hdrs[i])), waveHdrSize); r != 0 {
			s.shutdown()
			return nil, fmt.Errorf("waveOutPrepareHeader: %d", r)
		}
	}
	return &sink{w: s, close: s.shutdown}, nil
}

// Write queues p, a chunk at a time, waiting for the driver to hand a buffer back when all are in use.
func (s *waveSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := len(p)
	for len(p) > 0 {
		i := s.next
		hdr := s.hdrs[i]
		for s.queued[i] && atomic.LoadUint32(&hdr.flags)&whdrDone == 0 {
			if s.closed.Load() || s.h == 0 {
				return total - len(p), errWaveClosed
			}
			time.Sleep(time.Millisecond)
		}
		if s.closed.Load() || s.h == 0 {
			return total - len(p), errWaveClosed
		}
		n := copy(s.bufs[i], p)
		hdr.length = uint32(n)
		if r, _, _ := procWaveWrite.Call(s.h, uintptr(unsafe.Pointer(hdr)), waveHdrSize); r != 0 {
			return total - len(p), fmt.Errorf("waveOutWrite: %d", r)
		}
		s.queued[i] = true
		s.next = (i + 1) % len(s.hdrs)
		p = p[n:]
	}
	return total, nil
}

// shutdown stops what is playing and gives the device back; a Write that is waiting gives up first.
func (s *waveSink) shutdown() {
	s.closed.Store(true)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.h == 0 {
		return
	}
	procWaveReset.Call(s.h) // returns every queued buffer, so they can be unprepared
	for _, hdr := range s.hdrs {
		procWaveUnprep.Call(s.h, uintptr(unsafe.Pointer(hdr)), waveHdrSize)
	}
	procWaveClose.Call(s.h)
	s.h = 0
}
