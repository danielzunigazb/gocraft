package proto

// Trace, when set, is called for every packet read or framed for sending: the direction ("<" from
// the peer, ">" to it), the packet id and what follows the id. It is a debugging aid (-netlog).
var Trace func(dir string, id int32, payload []byte)

// Note, when set, is given a line of the client's own to put in the same log as the packets: what it saw when
// it decided something (how far a rival was when the player swung), which the packets alone do not say.
var Note func(text string)

func traceFrame(b []byte) {
	if Trace == nil {
		return
	}
	r := &Reader{b: b}
	if id := r.VarInt(); r.err == nil {
		Trace(">", id, r.b)
	}
}
